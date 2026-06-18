package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/ktsoator/or/internal/llm"
)

// Adapter translates the OpenAI-compatible Chat Completions protocol.
type Adapter struct {
	httpClient *http.Client
}

// NewAdapter creates an adapter that uses httpClient for requests.
// A nil client uses http.DefaultClient.
func NewAdapter(httpClient *http.Client) *Adapter {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Adapter{httpClient: httpClient}
}

// Protocol returns the registry key for the Chat Completions protocol.
func (a *Adapter) Protocol() llm.Protocol {
	return llm.ProtocolOpenAICompletions
}

// Stream validates and translates a request, then delegates the asynchronous
// response lifecycle to consumeStream.
func (a *Adapter) Stream(
	ctx context.Context,
	model llm.Model,
	input llm.Context,
	options llm.StreamOptions,
) (<-chan llm.Event, error) {
	if model.Protocol != a.Protocol() {
		return nil, fmt.Errorf("model protocol %q does not match adapter protocol %q", model.Protocol, a.Protocol())
	}
	if model.ID == "" {
		return nil, errors.New("model ID is empty")
	}
	if model.Compatibility != nil {
		compatibility, ok := model.Compatibility.(*llm.OpenAICompletionsCompatibility)
		if !ok || compatibility == nil {
			return nil, fmt.Errorf(
				"model compatibility type %T is not valid for protocol %q",
				model.Compatibility,
				model.Protocol,
			)
		}
	}
	if options.APIKey == "" {
		return nil, errors.New("OpenAI API key is empty")
	}

	compat := resolveCompat(model)
	messages, err := convertMessages(input, model)
	if err != nil {
		return nil, err
	}
	tools, err := convertTools(input.Tools)
	if err != nil {
		return nil, err
	}

	client := buildClient(a.httpClient, model, options)
	params := buildParams(model, messages, tools, options, compat)
	events := make(chan llm.Event)
	go consumeStream(ctx, client, params, model, events)
	return events, nil
}
