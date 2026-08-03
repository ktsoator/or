package openai

import (
	"context"
	"net/http"

	"github.com/ktsoator/or/llm"
	"github.com/ktsoator/or/llm/openai/internal/chatcompletions"
	"github.com/ktsoator/or/llm/openai/internal/responses"
)

// Adapter is the OpenAI-compatible Chat Completions protocol adapter.
type Adapter struct {
	delegate *chatcompletions.Adapter
}

// NewAdapter creates a Chat Completions adapter that uses httpClient for
// requests. A nil client uses http.DefaultClient.
func NewAdapter(httpClient *http.Client) *Adapter {
	return &Adapter{delegate: chatcompletions.NewAdapter(httpClient)}
}

// Protocol returns the registry key for the Chat Completions protocol.
func (a *Adapter) Protocol() llm.Protocol {
	return a.delegate.Protocol()
}

// Stream delegates the request to the Chat Completions implementation.
func (a *Adapter) Stream(
	ctx context.Context,
	model llm.Model,
	input llm.Context,
	options llm.StreamOptions,
) (<-chan llm.Event, error) {
	return a.delegate.Stream(ctx, model, input, options)
}

// ResponsesAdapter is the OpenAI Responses protocol adapter.
type ResponsesAdapter struct {
	delegate *responses.Adapter
}

// NewResponsesAdapter creates a Responses adapter that uses httpClient for
// requests. A nil client uses http.DefaultClient.
func NewResponsesAdapter(httpClient *http.Client) *ResponsesAdapter {
	return &ResponsesAdapter{delegate: responses.NewAdapter(httpClient)}
}

// Protocol returns the registry key for the Responses protocol.
func (a *ResponsesAdapter) Protocol() llm.Protocol {
	return a.delegate.Protocol()
}

// Stream delegates the request to the Responses implementation.
func (a *ResponsesAdapter) Stream(
	ctx context.Context,
	model llm.Model,
	input llm.Context,
	options llm.StreamOptions,
) (<-chan llm.Event, error) {
	return a.delegate.Stream(ctx, model, input, options)
}

var (
	_ llm.ProtocolAdapter = (*Adapter)(nil)
	_ llm.ProtocolAdapter = (*ResponsesAdapter)(nil)
)
