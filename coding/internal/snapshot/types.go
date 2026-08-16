// Package snapshot stores inspectable, provider-neutral model exchanges.
// Snapshots are separate from the privacy-safe performance event log because
// they contain conversation content and are loaded only on explicit request.
package snapshot

import (
	"encoding/json"
	"time"

	"github.com/ktsoator/or/llm"
)

const CurrentVersion = 4

// Attachment identifies one product-generated message inside Input.Messages.
type Attachment struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Placement    string `json:"placement"`
	Path         string `json:"path,omitempty"`
	Revision     string `json:"revision,omitempty"`
	MessageIndex int    `json:"messageIndex"`
}

// Image contains inspectable metadata without retaining base64 image bytes.
type Image struct {
	MIMEType     string `json:"mimeType"`
	EncodedBytes int    `json:"encodedBytes,omitempty"`
}

// Content is a signature-free, provider-neutral input content block.
type Content struct {
	Type       string         `json:"type"`
	Text       string         `json:"text,omitempty"`
	Thinking   string         `json:"thinking,omitempty"`
	Redacted   bool           `json:"redacted,omitempty"`
	Image      *Image         `json:"image,omitempty"`
	ToolCallID string         `json:"toolCallId,omitempty"`
	ToolName   string         `json:"toolName,omitempty"`
	Arguments  map[string]any `json:"arguments,omitempty"`
}

// Message is one model-input message with only inspectable replay content.
type Message struct {
	Role              string    `json:"role"`
	Content           []Content `json:"content"`
	ProviderRequestID string    `json:"providerRequestId,omitempty"`
	ToolCallID        string    `json:"toolCallId,omitempty"`
	ToolName          string    `json:"toolName,omitempty"`
	IsError           bool      `json:"isError,omitempty"`
}

// Tool is one tool definition advertised to the model.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Strict      *bool           `json:"strict,omitempty"`
}

// Input is the complete inspectable request after context projection.
type Input struct {
	SystemPrompt string    `json:"systemPrompt,omitempty"`
	Messages     []Message `json:"messages"`
	Tools        []Tool    `json:"tools,omitempty"`
}

// Output is the terminal provider message captured after streaming completes.
type Output struct {
	CapturedAt   time.Time `json:"capturedAt"`
	Message      Message   `json:"message"`
	StopReason   string    `json:"stopReason,omitempty"`
	ErrorMessage string    `json:"errorMessage,omitempty"`
}

// Snapshot correlates a provider-neutral exchange with the performance timeline.
type Snapshot struct {
	Version           int          `json:"version"`
	CapturedAt        time.Time    `json:"capturedAt"`
	SessionID         string       `json:"sessionId"`
	RunID             string       `json:"runId"`
	TurnID            string       `json:"turnId"`
	StepID            string       `json:"stepId"`
	ProviderRequestID string       `json:"providerRequestId"`
	Provider          string       `json:"provider"`
	Model             string       `json:"model"`
	Input             Input        `json:"input"`
	Output            *Output      `json:"output,omitempty"`
	Attachments       []Attachment `json:"attachments,omitempty"`
}

// Writer records request snapshots without coupling the engine to file I/O.
type Writer interface {
	Save(Snapshot) error
	SaveOutput(providerRequestID string, message *llm.AssistantMessage) error
}

// Reader loads one snapshot on demand for the diagnostics API.
type Reader interface {
	Load(providerRequestID string) (Snapshot, error)
}

// SessionCleaner removes every private snapshot owned by one session.
type SessionCleaner interface {
	DeleteSession(sessionID string) error
}

// DiscardWriter keeps snapshot capture optional and fail-open.
type DiscardWriter struct{}

func (DiscardWriter) Save(Snapshot) error                            { return nil }
func (DiscardWriter) SaveOutput(string, *llm.AssistantMessage) error { return nil }
func (DiscardWriter) DeleteSession(string) error                     { return nil }

// OrDiscard replaces a nil writer with a no-op implementation.
func OrDiscard(writer Writer) Writer {
	if writer == nil {
		return DiscardWriter{}
	}
	return writer
}

// NewSnapshot removes replay-only signatures and image payloads while keeping
// every piece of content a person can meaningfully inspect.
func NewSnapshot(
	sessionID, runID, turnID, stepID, requestID, provider, model string,
	input llm.Context,
	attachments []Attachment,
) Snapshot {
	return Snapshot{
		Version: CurrentVersion, CapturedAt: time.Now().UTC(),
		SessionID: sessionID, RunID: runID, TurnID: turnID, StepID: stepID,
		ProviderRequestID: requestID, Provider: provider, Model: model,
		Input:       sanitizeInput(input),
		Attachments: append([]Attachment(nil), attachments...),
	}
}
