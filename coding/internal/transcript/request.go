package transcript

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// RequestHeader is the complete provider-neutral definition of one model
// request. Transport credentials, endpoints, headers, callbacks, and retry
// policy are deliberately absent.
type RequestHeader struct {
	ProviderRequestID       string                 `json:"providerRequestId"`
	RunID                   string                 `json:"runId"`
	TurnID                  string                 `json:"turnId"`
	StepID                  string                 `json:"stepId"`
	Provider                string                 `json:"provider"`
	Model                   string                 `json:"model"`
	Protocol                llm.Protocol           `json:"protocol"`
	ThinkingLevel           llm.ModelThinkingLevel `json:"thinkingLevel,omitempty"`
	SystemPrompt            string                 `json:"systemPrompt,omitempty"`
	Tools                   []llm.ToolDefinition   `json:"tools,omitempty"`
	Options                 RequestOptions         `json:"options,omitempty"`
	InputSeq                int64                  `json:"inputSeq"`
	ActiveCompactionEntryID string                 `json:"activeCompactionEntryId,omitempty"`
	Attachments             []RequestAttachment    `json:"attachments,omitempty"`
}

// RequestAttachment places one previously committed context attachment in the
// final provider message list. MessageIndex is counted after all attachments
// have been inserted.
type RequestAttachment struct {
	AttachmentID string `json:"attachmentId"`
	MessageIndex int    `json:"messageIndex"`
}

// RequestOptions contains only settings that affect the provider-neutral
// logical request. Attempt policy and transport configuration are not durable
// conversation facts.
type RequestOptions struct {
	Temperature     *float64                `json:"temperature,omitempty"`
	MaxTokens       int64                   `json:"maxTokens,omitempty"`
	ProtocolOptions *RequestProtocolOptions `json:"protocolOptions,omitempty"`
}

// RequestProtocolOptions is the closed, secret-free durable form of the
// built-in protocol extensions.
type RequestProtocolOptions struct {
	ThinkingDisplay llm.ThinkingDisplay `json:"thinkingDisplay,omitempty"`
	ToolChoice      *RequestToolChoice  `json:"toolChoice,omitempty"`
}

// RequestToolChoice represents either a protocol-native mode or one named
// tool. Exactly one of Mode and Name may be set.
type RequestToolChoice struct {
	Mode string `json:"mode,omitempty"`
	Name string `json:"name,omitempty"`
}

// ProviderRequest is a request reconstructed only from committed transcript
// facts. HeaderEntrySeq is the checkpoint watermark; InputSeq is the prefix
// from which Input.Messages was derived.
type ProviderRequest struct {
	HeaderEntryID  string
	HeaderEntrySeq int64
	Header         RequestHeader
	Input          llm.Context
	Options        llm.StreamOptions
}

// NewRequestHeader creates an unsequenced durable request definition. The
// sequencer assigns InputSeq to the immediately preceding committed entry.
func NewRequestHeader(header RequestHeader) Entry {
	header = cloneRequestHeader(header)
	header.InputSeq = unassignedSequence
	return Entry{
		Seq: unassignedSequence, ID: NewID(), Timestamp: time.Now().UTC(),
		Type: RequestHeaderEntry, RequestHeader: &header,
	}
}

// CaptureRequestOptions extracts the semantic, serializable subset of stream
// options. RewriteRequest is rejected because it can change the logical body
// after the durable request has been reconstructed.
func CaptureRequestOptions(
	protocol llm.Protocol,
	tools []llm.ToolDefinition,
	options llm.StreamOptions,
) (RequestOptions, error) {
	if options.RewriteRequest != nil {
		return RequestOptions{}, fmt.Errorf(
			"transcript: request body rewrite cannot be represented durably",
		)
	}
	if err := options.Validate(protocol, tools); err != nil {
		return RequestOptions{}, fmt.Errorf("transcript: validate request options: %w", err)
	}
	captured := RequestOptions{MaxTokens: options.MaxTokens}
	if options.Temperature != nil {
		value := *options.Temperature
		captured.Temperature = &value
	}
	if options.ProtocolOptions == nil {
		return captured, nil
	}

	protocolOptions := &RequestProtocolOptions{}
	switch typed := options.ProtocolOptions.(type) {
	case *llm.AnthropicStreamOptions:
		protocolOptions.ThinkingDisplay = typed.ThinkingDisplay
		choice, err := captureAnthropicToolChoice(typed.ToolChoice)
		if err != nil {
			return RequestOptions{}, err
		}
		protocolOptions.ToolChoice = choice
	case *llm.OpenAICompletionsStreamOptions:
		choice, err := captureOpenAIToolChoice(typed.ToolChoice)
		if err != nil {
			return RequestOptions{}, err
		}
		protocolOptions.ToolChoice = choice
	case *llm.OpenAIResponsesStreamOptions:
		protocolOptions.ThinkingDisplay = typed.ThinkingDisplay
		choice, err := captureOpenAIToolChoice(typed.ToolChoice)
		if err != nil {
			return RequestOptions{}, err
		}
		protocolOptions.ToolChoice = choice
	default:
		return RequestOptions{}, fmt.Errorf(
			"transcript: protocol options %T cannot be represented durably",
			options.ProtocolOptions,
		)
	}
	captured.ProtocolOptions = protocolOptions
	return captured, nil
}

// StreamOptions restores the semantic options represented by this header. All
// transport and observer fields remain empty by construction.
func (header RequestHeader) StreamOptions() (llm.StreamOptions, error) {
	options := llm.StreamOptions{
		MaxTokens: header.Options.MaxTokens,
		Reasoning: header.ThinkingLevel,
	}
	if header.Options.Temperature != nil {
		value := *header.Options.Temperature
		options.Temperature = &value
	}
	protocolOptions := header.Options.ProtocolOptions
	if protocolOptions == nil {
		return options, nil
	}

	switch header.Protocol {
	case llm.ProtocolAnthropicMessages:
		choice, err := restoreAnthropicToolChoice(protocolOptions.ToolChoice)
		if err != nil {
			return llm.StreamOptions{}, err
		}
		options.ProtocolOptions = &llm.AnthropicStreamOptions{
			ThinkingDisplay: protocolOptions.ThinkingDisplay,
			ToolChoice:      choice,
		}
	case llm.ProtocolOpenAICompletions:
		if protocolOptions.ThinkingDisplay != "" {
			return llm.StreamOptions{}, fmt.Errorf(
				"transcript: protocol %q does not support thinking display",
				header.Protocol,
			)
		}
		choice, err := restoreOpenAIToolChoice(protocolOptions.ToolChoice)
		if err != nil {
			return llm.StreamOptions{}, err
		}
		options.ProtocolOptions = &llm.OpenAICompletionsStreamOptions{ToolChoice: choice}
	case llm.ProtocolOpenAIResponses:
		choice, err := restoreOpenAIToolChoice(protocolOptions.ToolChoice)
		if err != nil {
			return llm.StreamOptions{}, err
		}
		options.ProtocolOptions = &llm.OpenAIResponsesStreamOptions{
			ThinkingDisplay: protocolOptions.ThinkingDisplay,
			ToolChoice:      choice,
		}
	default:
		return llm.StreamOptions{}, fmt.Errorf(
			"transcript: protocol %q options cannot be reconstructed",
			header.Protocol,
		)
	}
	if err := options.Validate(header.Protocol, header.Tools); err != nil {
		return llm.StreamOptions{}, fmt.Errorf(
			"transcript: reconstruct request options: %w",
			err,
		)
	}
	return options, nil
}

// ReconstructProviderRequest rebuilds one provider-neutral request using only
// the committed transcript. Diagnostic snapshots and live agent state are not
// consulted.
func ReconstructProviderRequest(
	entries []Entry,
	providerRequestID string,
) (ProviderRequest, error) {
	projection, err := ProjectSession(entries)
	if err != nil {
		return ProviderRequest{}, err
	}
	var projected *ProjectedProviderRequest
	for index := range projection.ProviderRequests {
		candidate := &projection.ProviderRequests[index]
		if candidate.Header.ProviderRequestID == providerRequestID {
			projected = candidate
			break
		}
	}
	if projected == nil {
		return ProviderRequest{}, fmt.Errorf(
			"transcript: provider request %q was not found",
			providerRequestID,
		)
	}
	header := cloneRequestHeader(projected.Header)
	if header.InputSeq < 0 || header.InputSeq >= int64(len(entries)) {
		return ProviderRequest{}, fmt.Errorf(
			"transcript: provider request %q input sequence %d is outside the transcript",
			providerRequestID,
			header.InputSeq,
		)
	}
	prefix := entries[:header.InputSeq+1]
	modelContext, err := ProjectModelContext(prefix)
	if err != nil {
		return ProviderRequest{}, err
	}
	contexts := make([]ProjectedContext, 0, len(projection.Contexts))
	for _, context := range projection.Contexts {
		if context.EntryIndex <= int(header.InputSeq) {
			contexts = append(contexts, context)
		}
	}
	return reconstructProjectedProviderRequest(*projected, modelContext, contexts)
}

// ReconstructCommittedProviderRequest rebuilds the request at the current
// shared projection watermark. It is the online counterpart to the complete
// replay API and does not scan transcript entries.
func ReconstructCommittedProviderRequest(
	session *SessionProjection,
	modelContext *ModelContextProjection,
	providerRequestID string,
) (ProviderRequest, error) {
	if session == nil || modelContext == nil {
		return ProviderRequest{}, fmt.Errorf("transcript: request projections are nil")
	}
	if session.AsOfSeq != modelContext.AsOfSeq {
		return ProviderRequest{}, fmt.Errorf(
			"transcript: request projections have different watermarks %d and %d",
			session.AsOfSeq,
			modelContext.AsOfSeq,
		)
	}
	var projected *ProjectedProviderRequest
	for index := range session.ProviderRequests {
		candidate := &session.ProviderRequests[index]
		if candidate.Header.ProviderRequestID == providerRequestID {
			projected = candidate
			break
		}
	}
	if projected == nil {
		return ProviderRequest{}, fmt.Errorf(
			"transcript: provider request %q was not found",
			providerRequestID,
		)
	}
	if projected.EntrySeq != session.AsOfSeq {
		return ProviderRequest{}, fmt.Errorf(
			"transcript: provider request %q checkpoint is at %d, projection is at %d",
			providerRequestID,
			projected.EntrySeq,
			session.AsOfSeq,
		)
	}
	return reconstructProjectedProviderRequest(
		*projected,
		modelContext,
		session.Contexts,
	)
}

func reconstructProjectedProviderRequest(
	projected ProjectedProviderRequest,
	modelContext *ModelContextProjection,
	contexts []ProjectedContext,
) (ProviderRequest, error) {
	header := cloneRequestHeader(projected.Header)
	providerRequestID := header.ProviderRequestID
	if modelContext.ActiveCompactionEntryID != header.ActiveCompactionEntryID {
		return ProviderRequest{}, fmt.Errorf(
			"transcript: provider request %q compaction boundary changed",
			providerRequestID,
		)
	}

	canonical := make([]llm.Message, 0, len(modelContext.Messages))
	for index, wrapped := range modelContext.Messages {
		message, ok := agent.ToLLM(wrapped)
		if !ok {
			return ProviderRequest{}, fmt.Errorf(
				"transcript: provider request %q canonical message %d is not model-facing",
				providerRequestID,
				index,
			)
		}
		canonical = append(canonical, message)
	}
	attachments := make(map[string]ContextAttachment)
	for _, context := range contexts {
		attachments[context.Attachment.AttachmentID] = context.Attachment
	}
	messages, err := insertRequestAttachments(canonical, header.Attachments, attachments)
	if err != nil {
		return ProviderRequest{}, fmt.Errorf(
			"transcript: reconstruct provider request %q: %w",
			providerRequestID,
			err,
		)
	}
	options, err := header.StreamOptions()
	if err != nil {
		return ProviderRequest{}, err
	}
	return ProviderRequest{
		HeaderEntryID: projected.EntryID, HeaderEntrySeq: projected.EntrySeq,
		Header: header,
		Input: llm.Context{
			SystemPrompt: header.SystemPrompt,
			Messages:     messages,
			Tools:        cloneToolDefinitions(header.Tools),
		},
		Options: options,
	}, nil
}

func validateRequestHeader(header RequestHeader, entrySeq int64) error {
	for name, value := range map[string]string{
		"provider request id": header.ProviderRequestID,
		"run id":              header.RunID,
		"turn id":             header.TurnID,
		"step id":             header.StepID,
		"provider":            header.Provider,
		"model":               header.Model,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is empty", name)
		}
	}
	if entrySeq == unassignedSequence {
		if header.InputSeq != unassignedSequence {
			return fmt.Errorf("unsequenced request has input sequence %d", header.InputSeq)
		}
	} else if header.InputSeq != entrySeq-1 {
		return fmt.Errorf(
			"input sequence %d does not immediately precede header sequence %d",
			header.InputSeq,
			entrySeq,
		)
	}
	if !validThinkingLevel(header.ThinkingLevel) {
		return fmt.Errorf("invalid thinking level %q", header.ThinkingLevel)
	}
	if header.Options.MaxTokens < 0 {
		return fmt.Errorf("max tokens %d is negative", header.Options.MaxTokens)
	}
	if temperature := header.Options.Temperature; temperature != nil &&
		(math.IsNaN(*temperature) || math.IsInf(*temperature, 0)) {
		return fmt.Errorf("temperature is not finite")
	}
	toolNames := make(map[string]struct{}, len(header.Tools))
	for index, tool := range header.Tools {
		if strings.TrimSpace(tool.Name) == "" {
			return fmt.Errorf("tool %d name is empty", index)
		}
		if _, exists := toolNames[tool.Name]; exists {
			return fmt.Errorf("tool name %q is duplicated", tool.Name)
		}
		toolNames[tool.Name] = struct{}{}
		if len(tool.Parameters) > 0 && !json.Valid(tool.Parameters) {
			return fmt.Errorf("tool %q parameters are not valid JSON", tool.Name)
		}
	}
	if _, err := header.StreamOptions(); err != nil {
		return err
	}
	seenAttachments := make(map[string]struct{}, len(header.Attachments))
	previousIndex := -1
	for _, attachment := range header.Attachments {
		if attachment.AttachmentID == "" {
			return fmt.Errorf("attachment id is empty")
		}
		if _, exists := seenAttachments[attachment.AttachmentID]; exists {
			return fmt.Errorf("attachment %q is duplicated", attachment.AttachmentID)
		}
		if attachment.MessageIndex <= previousIndex {
			return fmt.Errorf("attachment message indexes are not strictly increasing")
		}
		seenAttachments[attachment.AttachmentID] = struct{}{}
		previousIndex = attachment.MessageIndex
	}
	return nil
}

func validThinkingLevel(level llm.ModelThinkingLevel) bool {
	switch level {
	case "", llm.ModelThinkingOff, llm.ModelThinkingMinimal, llm.ModelThinkingLow,
		llm.ModelThinkingMedium, llm.ModelThinkingHigh, llm.ModelThinkingXHigh,
		llm.ModelThinkingMax:
		return true
	default:
		return false
	}
}

func insertRequestAttachments(
	canonical []llm.Message,
	references []RequestAttachment,
	attachments map[string]ContextAttachment,
) ([]llm.Message, error) {
	result := make([]llm.Message, 0, len(canonical)+len(references))
	canonicalIndex := 0
	referenceIndex := 0
	for finalIndex := 0; finalIndex < cap(result); finalIndex++ {
		if referenceIndex < len(references) &&
			references[referenceIndex].MessageIndex == finalIndex {
			reference := references[referenceIndex]
			attachment, ok := attachments[reference.AttachmentID]
			if !ok {
				return nil, fmt.Errorf("attachment %q was not found", reference.AttachmentID)
			}
			result = append(result, llm.UserText(attachment.Rendered))
			referenceIndex++
			continue
		}
		if canonicalIndex >= len(canonical) {
			return nil, fmt.Errorf("attachment message index is outside the request input")
		}
		result = append(result, canonical[canonicalIndex])
		canonicalIndex++
	}
	if canonicalIndex != len(canonical) || referenceIndex != len(references) {
		return nil, fmt.Errorf("attachment positions do not cover the request input")
	}
	return result, nil
}

func captureAnthropicToolChoice(choice llm.AnthropicToolChoice) (*RequestToolChoice, error) {
	switch typed := choice.(type) {
	case nil:
		return nil, nil
	case llm.AnthropicToolChoiceMode:
		return &RequestToolChoice{Mode: string(typed)}, nil
	case llm.AnthropicToolChoiceTool:
		return &RequestToolChoice{Name: typed.Name}, nil
	case *llm.AnthropicToolChoiceTool:
		if typed == nil {
			return nil, fmt.Errorf("transcript: Anthropic named tool choice is nil")
		}
		return &RequestToolChoice{Name: typed.Name}, nil
	default:
		return nil, fmt.Errorf("transcript: unsupported Anthropic tool choice %T", choice)
	}
}

func captureOpenAIToolChoice(choice llm.OpenAIToolChoice) (*RequestToolChoice, error) {
	switch typed := choice.(type) {
	case nil:
		return nil, nil
	case llm.OpenAIToolChoiceMode:
		return &RequestToolChoice{Mode: string(typed)}, nil
	case llm.OpenAIToolChoiceFunction:
		return &RequestToolChoice{Name: typed.Name}, nil
	case *llm.OpenAIToolChoiceFunction:
		if typed == nil {
			return nil, fmt.Errorf("transcript: OpenAI named tool choice is nil")
		}
		return &RequestToolChoice{Name: typed.Name}, nil
	default:
		return nil, fmt.Errorf("transcript: unsupported OpenAI tool choice %T", choice)
	}
}

func restoreAnthropicToolChoice(choice *RequestToolChoice) (llm.AnthropicToolChoice, error) {
	if choice == nil {
		return nil, nil
	}
	if err := validateRequestToolChoice(*choice); err != nil {
		return nil, err
	}
	if choice.Name != "" {
		return llm.AnthropicToolChoiceTool{Name: choice.Name}, nil
	}
	return llm.AnthropicToolChoiceMode(choice.Mode), nil
}

func restoreOpenAIToolChoice(choice *RequestToolChoice) (llm.OpenAIToolChoice, error) {
	if choice == nil {
		return nil, nil
	}
	if err := validateRequestToolChoice(*choice); err != nil {
		return nil, err
	}
	if choice.Name != "" {
		return llm.OpenAIToolChoiceFunction{Name: choice.Name}, nil
	}
	return llm.OpenAIToolChoiceMode(choice.Mode), nil
}

func validateRequestToolChoice(choice RequestToolChoice) error {
	if (choice.Mode == "") == (choice.Name == "") {
		return fmt.Errorf("transcript: request tool choice must set exactly one of mode or name")
	}
	return nil
}

func cloneRequestHeader(source RequestHeader) RequestHeader {
	clone := source
	clone.Tools = cloneToolDefinitions(source.Tools)
	if source.Options.Temperature != nil {
		value := *source.Options.Temperature
		clone.Options.Temperature = &value
	}
	if source.Options.ProtocolOptions != nil {
		protocolOptions := *source.Options.ProtocolOptions
		if source.Options.ProtocolOptions.ToolChoice != nil {
			choice := *source.Options.ProtocolOptions.ToolChoice
			protocolOptions.ToolChoice = &choice
		}
		clone.Options.ProtocolOptions = &protocolOptions
	}
	clone.Attachments = append([]RequestAttachment(nil), source.Attachments...)
	return clone
}

func cloneToolDefinitions(source []llm.ToolDefinition) []llm.ToolDefinition {
	if source == nil {
		return nil
	}
	clone := make([]llm.ToolDefinition, len(source))
	for index, tool := range source {
		clone[index] = tool
		clone[index].Parameters = append(json.RawMessage(nil), tool.Parameters...)
		if tool.Strict != nil {
			strict := *tool.Strict
			clone[index].Strict = &strict
		}
	}
	return clone
}
