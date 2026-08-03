package responses

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/ktsoator/or/llm"
	"github.com/ktsoator/or/llm/openai/internal/transport"
)

// Adapter translates the OpenAI Responses protocol.
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

// Protocol returns the registry key for the OpenAI Responses protocol.
func (a *Adapter) Protocol() llm.Protocol {
	return llm.ProtocolOpenAIResponses
}

// Stream validates and translates a request, then consumes the Responses SSE
// stream asynchronously.
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
		return nil, fmt.Errorf("model compatibility type %T is not valid for protocol %q", model.Compatibility, model.Protocol)
	}
	if strings.TrimSpace(options.APIKey) == "" {
		return nil, llm.MissingAPIKeyError(model.Provider)
	}

	items, err := convertResponsesInput(input, model)
	if err != nil {
		return nil, err
	}
	tools, err := convertResponsesTools(input.Tools)
	if err != nil {
		return nil, err
	}

	client := transport.BuildClient(a.httpClient, model, options)
	params := buildResponsesParams(model, input.SystemPrompt, items, tools, options)
	events := make(chan llm.Event)
	go consumeResponsesStream(ctx, client, params, model, events)
	return events, nil
}
