// Package plugin defines the transport-neutral contract between Coding and one
// external capability process. A concrete stdio or RPC transport can implement
// Supervisor without leaking process management into the agent SDK.
package plugin

import (
	"context"
	"encoding/json"
)

// ProtocolVersion is the only plugin protocol version understood by this host.
const ProtocolVersion = 1

// HostInfo identifies the Coding host during the initialization handshake.
type HostInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeRequest starts a protocol-version handshake with one plugin.
type InitializeRequest struct {
	ProtocolVersion int      `json:"protocolVersion"`
	Host            HostInfo `json:"host"`
}

// Manifest identifies one plugin instance.
type Manifest struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Version string `json:"version"`
}

// InitializeResponse confirms the selected protocol and plugin identity.
type InitializeResponse struct {
	ProtocolVersion int      `json:"protocolVersion"`
	Plugin          Manifest `json:"plugin"`
}

// ListToolsRequest asks the initialized plugin for its current tool set.
type ListToolsRequest struct{}

// ExecutionMode controls whether the agent may execute a plugin tool alongside
// other calls from the same model turn.
type ExecutionMode string

const (
	ExecutionParallel   ExecutionMode = "parallel"
	ExecutionSequential ExecutionMode = "sequential"
)

// ToolDescriptor is the serializable portion of a Coding tool. Access metadata
// is deliberately absent in protocol v1, so adapted plugin tools take the
// conservative unknown-access path and require product authorization.
type ToolDescriptor struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	InputSchema   json.RawMessage `json:"inputSchema"`
	Label         string          `json:"label,omitempty"`
	Guidelines    []string        `json:"guidelines,omitempty"`
	ExecutionMode ExecutionMode   `json:"executionMode,omitempty"`
}

// ListToolsResponse contains the tools contributed by one plugin.
type ListToolsResponse struct {
	Tools []ToolDescriptor `json:"tools"`
}

// ExecuteRequest starts one validated tool call. Arguments are the validated
// JSON object produced by the agent engine, not the provider's original input.
type ExecuteRequest struct {
	CallID    string          `json:"callId"`
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
}

// ContentType identifies one model-facing content block.
type ContentType string

const (
	ContentText  ContentType = "text"
	ContentImage ContentType = "image"
)

// Content is the wire representation of text and image tool-result content.
type Content struct {
	Type     ContentType `json:"type"`
	Text     string      `json:"text,omitempty"`
	Data     string      `json:"data,omitempty"`
	MIMEType string      `json:"mimeType,omitempty"`
}

// ProgressNotification is a non-terminal update for one active call.
type ProgressNotification struct {
	CallID  string          `json:"callId"`
	Content []Content       `json:"content,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// OutcomeStatus is the terminal state reported by a plugin tool.
type OutcomeStatus string

const (
	OutcomeSuccess   OutcomeStatus = "success"
	OutcomeFailed    OutcomeStatus = "failed"
	OutcomeCancelled OutcomeStatus = "cancelled"
	OutcomeTimeout   OutcomeStatus = "timeout"
)

// Outcome is the wire representation of a terminal tool outcome.
type Outcome struct {
	Status    OutcomeStatus   `json:"status"`
	ErrorCode string          `json:"errorCode,omitempty"`
	ExitCode  *int            `json:"exitCode,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}

// Result is the unique terminal response for one ExecuteRequest.
type Result struct {
	CallID    string    `json:"callId"`
	Content   []Content `json:"content,omitempty"`
	Outcome   Outcome   `json:"outcome"`
	Terminate bool      `json:"terminate,omitempty"`
}

// CancelRequest asks the plugin to stop an active call.
type CancelRequest struct {
	CallID string `json:"callId"`
}

// CancelResponse reports whether the plugin found an active call to cancel.
type CancelResponse struct {
	Accepted bool `json:"accepted"`
}

// Supervisor owns one initialized plugin connection. Implementations are
// responsible for serializing concurrent calls and translating Execute context
// cancellation into the transport's cancel operation.
type Supervisor interface {
	Initialize(context.Context, InitializeRequest) (InitializeResponse, error)
	ListTools(context.Context, ListToolsRequest) (ListToolsResponse, error)
	Execute(context.Context, ExecuteRequest, func(ProgressNotification)) (Result, error)
	Cancel(context.Context, CancelRequest) (CancelResponse, error)
}
