package responses

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ktsoator/or/llm"
	"github.com/ktsoator/or/llm/openai/internal/protocolutil"
	oai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

const responsesSignaturePrefix = "openai-responses:"

type responsesSignature struct {
	Kind             string `json:"kind"`
	ID               string `json:"id,omitempty"`
	Phase            string `json:"phase,omitempty"`
	EncryptedContent string `json:"encryptedContent,omitempty"`
}

func encodeResponsesSignature(signature responsesSignature) string {
	raw, _ := json.Marshal(signature)
	return responsesSignaturePrefix + base64.RawURLEncoding.EncodeToString(raw)
}

func decodeResponsesSignature(value, kind string) (responsesSignature, bool) {
	if !strings.HasPrefix(value, responsesSignaturePrefix) {
		return responsesSignature{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, responsesSignaturePrefix))
	if err != nil {
		return responsesSignature{}, false
	}
	var signature responsesSignature
	if err := json.Unmarshal(raw, &signature); err != nil || signature.Kind != kind {
		return responsesSignature{}, false
	}
	return signature, true
}

func convertResponsesInput(input llm.Context, model llm.Model) ([]responses.ResponseInputItemUnionParam, error) {
	transformed := llm.TransformMessages(input.Messages, model, responsesToolCallIDNormalizer(model))
	items := make([]responses.ResponseInputItemUnionParam, 0, len(transformed))
	for _, rawMessage := range transformed {
		switch message := rawMessage.(type) {
		case *llm.UserMessage:
			if message == nil {
				return nil, errors.New("user message is nil")
			}
			item, ok, err := convertResponsesUserMessage(message)
			if err != nil {
				return nil, err
			}
			if ok {
				items = append(items, item)
			}
		case *llm.AssistantMessage:
			if message == nil {
				return nil, errors.New("assistant message is nil")
			}
			converted, err := convertResponsesAssistantMessage(message)
			if err != nil {
				return nil, err
			}
			items = append(items, converted...)
		case *llm.ToolResultMessage:
			item, err := convertResponsesToolResult(message)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		default:
			return nil, fmt.Errorf("unsupported message type %T", rawMessage)
		}
	}
	return items, nil
}

// responsesToolCallIDNormalizer removes the Responses item ID when a tool call
// crosses a model boundary. Item IDs are paired with model-specific reasoning
// items, so only the portable call ID can be replayed safely.
func responsesToolCallIDNormalizer(model llm.Model) func(string) string {
	return func(id string) string {
		if index := strings.IndexByte(id, '|'); index >= 0 {
			return protocolutil.TruncateASCII(protocolutil.SanitizeToolCallID(id[:index]), 40)
		}
		if model.Provider == "openai" {
			return protocolutil.TruncateASCII(id, 40)
		}
		return id
	}
}

func convertResponsesUserMessage(message *llm.UserMessage) (responses.ResponseInputItemUnionParam, bool, error) {
	content := make(responses.ResponseInputMessageContentListParam, 0, len(message.Content))
	for _, rawContent := range message.Content {
		switch block := rawContent.(type) {
		case *llm.TextContent:
			if block == nil {
				return responses.ResponseInputItemUnionParam{}, false, errors.New("user text content is nil")
			}
			content = append(content, responses.ResponseInputContentParamOfInputText(block.Text))
		case *llm.ImageContent:
			image, err := convertResponsesImage(block)
			if err != nil {
				return responses.ResponseInputItemUnionParam{}, false, err
			}
			content = append(content, image)
		default:
			return responses.ResponseInputItemUnionParam{}, false, fmt.Errorf("unsupported user content type %T", rawContent)
		}
	}
	if len(content) == 0 {
		return responses.ResponseInputItemUnionParam{}, false, nil
	}
	return responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleUser), true, nil
}

func convertResponsesImage(content *llm.ImageContent) (responses.ResponseInputContentUnionParam, error) {
	if content == nil {
		return responses.ResponseInputContentUnionParam{}, errors.New("image content is nil")
	}
	if content.MIMEType == "" {
		return responses.ResponseInputContentUnionParam{}, errors.New("image content is missing MIME type")
	}
	if content.Data == "" {
		return responses.ResponseInputContentUnionParam{}, errors.New("image content is missing data")
	}
	image := responses.ResponseInputContentParamOfInputImage(responses.ResponseInputImageDetailAuto)
	image.OfInputImage.ImageURL = oai.String("data:" + content.MIMEType + ";base64," + content.Data)
	return image, nil
}

func convertResponsesAssistantMessage(message *llm.AssistantMessage) ([]responses.ResponseInputItemUnionParam, error) {
	items := make([]responses.ResponseInputItemUnionParam, 0, len(message.Content))
	for _, rawContent := range message.Content {
		switch block := rawContent.(type) {
		case *llm.TextContent:
			if block == nil {
				return nil, errors.New("assistant text content is nil")
			}
			if block.Text == "" {
				continue
			}
			outputText := &responses.ResponseOutputTextParam{
				Text:        block.Text,
				Annotations: make([]responses.ResponseOutputTextAnnotationUnionParam, 0),
			}
			text := responses.ResponseOutputMessageContentUnionParam{OfOutputText: outputText}
			if signature, ok := decodeResponsesSignature(block.TextSignature, "text"); ok && signature.ID != "" {
				item := responses.ResponseInputItemParamOfOutputMessage(
					[]responses.ResponseOutputMessageContentUnionParam{text},
					signature.ID,
					responses.ResponseOutputMessageStatusCompleted,
				)
				item.OfOutputMessage.Phase = responses.ResponseOutputMessagePhase(signature.Phase)
				items = append(items, item)
			} else {
				items = append(items, responses.ResponseInputItemParamOfMessage(block.Text, responses.EasyInputMessageRoleAssistant))
			}
		case *llm.ThinkingContent:
			if block == nil {
				return nil, errors.New("assistant thinking content is nil")
			}
			signature, ok := decodeResponsesSignature(block.ThinkingSignature, "reasoning")
			if !ok || signature.ID == "" {
				continue
			}
			summary := make([]responses.ResponseReasoningItemSummaryParam, 0, 1)
			if block.Thinking != "" && !block.Redacted {
				summary = append(summary, responses.ResponseReasoningItemSummaryParam{Text: block.Thinking})
			}
			item := responses.ResponseInputItemParamOfReasoning(signature.ID, summary)
			if signature.EncryptedContent != "" {
				item.OfReasoning.EncryptedContent = oai.String(signature.EncryptedContent)
			}
			items = append(items, item)
		case *llm.ToolCall:
			if block == nil {
				return nil, errors.New("assistant tool call content is missing tool call data")
			}
			arguments, err := protocolutil.EncodeToolArguments(block.Arguments)
			if err != nil {
				return nil, fmt.Errorf("encode arguments for tool call %q: %w", block.Name, err)
			}
			callID, itemID := splitResponsesToolCallID(block.ID)
			item := responses.ResponseInputItemParamOfFunctionCall(arguments, callID, block.Name)
			if itemID != "" {
				item.OfFunctionCall.ID = oai.String(itemID)
			}
			items = append(items, item)
		default:
			return nil, fmt.Errorf("unsupported assistant content type %T", rawContent)
		}
	}
	return items, nil
}

func convertResponsesToolResult(message *llm.ToolResultMessage) (responses.ResponseInputItemUnionParam, error) {
	if message == nil {
		return responses.ResponseInputItemUnionParam{}, errors.New("tool result message is nil")
	}
	if message.ToolCallID == "" {
		return responses.ResponseInputItemUnionParam{}, errors.New("tool result message is missing tool call ID")
	}
	parts := make(responses.ResponseFunctionCallOutputItemListParam, 0, len(message.Content))
	for _, rawContent := range message.Content {
		switch block := rawContent.(type) {
		case *llm.TextContent:
			if block == nil {
				return responses.ResponseInputItemUnionParam{}, errors.New("tool result text content is nil")
			}
			parts = append(parts, responses.ResponseFunctionCallOutputItemUnionParam{
				OfInputText: &responses.ResponseInputTextContentParam{Text: block.Text},
			})
		case *llm.ImageContent:
			if block == nil {
				return responses.ResponseInputItemUnionParam{}, errors.New("image content is nil")
			}
			if block.MIMEType == "" {
				return responses.ResponseInputItemUnionParam{}, errors.New("image content is missing MIME type")
			}
			if block.Data == "" {
				return responses.ResponseInputItemUnionParam{}, errors.New("image content is missing data")
			}
			imageURL := "data:" + block.MIMEType + ";base64," + block.Data
			parts = append(parts, responses.ResponseFunctionCallOutputItemUnionParam{
				OfInputImage: &responses.ResponseInputImageContentParam{
					Detail:   responses.ResponseInputImageContentDetailAuto,
					ImageURL: oai.String(imageURL),
				},
			})
		default:
			return responses.ResponseInputItemUnionParam{}, fmt.Errorf("unsupported tool result content type %T", rawContent)
		}
	}
	callID, _ := splitResponsesToolCallID(message.ToolCallID)
	if len(parts) == 0 {
		return responses.ResponseInputItemParamOfFunctionCallOutput(callID, ""), nil
	}
	return responses.ResponseInputItemParamOfFunctionCallOutput(callID, parts), nil
}

func splitResponsesToolCallID(id string) (callID, itemID string) {
	callID, itemID, _ = strings.Cut(id, "|")
	return callID, itemID
}

func convertResponsesTools(tools []llm.ToolDefinition) ([]responses.ToolUnionParam, error) {
	converted := make([]responses.ToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		if tool.Name == "" {
			return nil, errors.New("tool definition is missing a name")
		}
		parameters := make(map[string]any)
		if len(tool.Parameters) > 0 {
			if err := json.Unmarshal(tool.Parameters, &parameters); err != nil {
				return nil, fmt.Errorf("decode parameters for tool %q: %w", tool.Name, err)
			}
		}
		function := &responses.FunctionToolParam{
			Name:       tool.Name,
			Parameters: parameters,
		}
		if tool.Description != "" {
			function.Description = oai.String(tool.Description)
		}
		if tool.Strict != nil {
			function.Strict = oai.Bool(*tool.Strict)
		}
		converted = append(converted, responses.ToolUnionParam{OfFunction: function})
	}
	return converted, nil
}

func buildResponsesParams(
	model llm.Model,
	instructions string,
	input []responses.ResponseInputItemUnionParam,
	tools []responses.ToolUnionParam,
	options llm.StreamOptions,
) responses.ResponseNewParams {
	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(model.ID),
		Input: responses.ResponseNewParamsInputUnion{OfInputItemList: input},
		Store: oai.Bool(false),
		Tools: tools,
	}
	if instructions != "" {
		params.Instructions = oai.String(instructions)
	}
	if options.MaxTokens > 0 {
		params.MaxOutputTokens = oai.Int(options.MaxTokens)
	}
	if options.Temperature != nil {
		params.Temperature = oai.Float(*options.Temperature)
	}
	if responsesOptions, ok := options.ProtocolOptions.(*llm.OpenAIResponsesStreamOptions); ok && responsesOptions != nil {
		applyResponsesToolChoice(&params, responsesOptions.ToolChoice)
	}
	applyResponsesThinking(&params, model, options)
	return params
}

func applyResponsesToolChoice(params *responses.ResponseNewParams, choice llm.OpenAIToolChoice) {
	switch typed := choice.(type) {
	case llm.OpenAIToolChoiceMode:
		switch typed {
		case llm.OpenAIToolChoiceAuto, llm.OpenAIToolChoiceNone, llm.OpenAIToolChoiceRequired:
			params.ToolChoice.OfToolChoiceMode = param.NewOpt(responses.ToolChoiceOptions(typed))
		}
	case llm.OpenAIToolChoiceFunction:
		params.ToolChoice.OfFunctionTool = &responses.ToolChoiceFunctionParam{Name: typed.Name}
	case *llm.OpenAIToolChoiceFunction:
		if typed != nil {
			params.ToolChoice.OfFunctionTool = &responses.ToolChoiceFunctionParam{Name: typed.Name}
		}
	}
}

func applyResponsesThinking(params *responses.ResponseNewParams, model llm.Model, options llm.StreamOptions) {
	if !model.Reasoning {
		return
	}
	thinking := protocolutil.ResolveThinking(model, options.Reasoning)
	if thinking.Enabled() {
		effort, mapped := protocolutil.ExplicitMappedEffort(model, thinking.Level)
		if mapped || len(model.ThinkingLevelMap) == 0 {
			if !mapped {
				effort = string(thinking.Level)
			}
			params.Reasoning.Effort = shared.ReasoningEffort(effort)
		}
	} else if options.Reasoning == llm.ModelThinkingOff {
		params.Reasoning.Effort = shared.ReasoningEffort(protocolutil.OffEffort(model))
	}
	display := llm.ThinkingDisplaySummarized
	if responsesOptions, ok := options.ProtocolOptions.(*llm.OpenAIResponsesStreamOptions); ok && responsesOptions != nil && responsesOptions.ThinkingDisplay != "" {
		display = responsesOptions.ThinkingDisplay
	}
	if display == llm.ThinkingDisplaySummarized {
		params.Reasoning.Summary = shared.ReasoningSummaryAuto
	}
	params.Include = []responses.ResponseIncludable{responses.ResponseIncludableReasoningEncryptedContent}
}
