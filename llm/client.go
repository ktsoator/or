package llm

import (
	"context"
	"errors"
	"fmt"
)

// Client routes LLM requests to the adapter registered for a model protocol,
// resolving provider configuration (API keys, overrides, headers) through an
// optional provider registry first.
type Client struct {
	adapters  *AdapterRegistry
	providers *ProviderRegistry
}

// NewClient creates a client backed by the given registries. providers may be
// nil, in which case requests skip provider resolution and only fall back to
// the environment API key lookup.
func NewClient(adapters *AdapterRegistry, providers *ProviderRegistry) *Client {
	return &Client{
		adapters:  adapters,
		providers: providers,
	}
}

// Stream starts a streaming completion request for the given model and input.
// The request first passes through the provider registry, which fills the API
// key and applies any provider override, then goes to the protocol adapter.
func (c *Client) Stream(ctx context.Context, model Model, input Context, options StreamOptions) (<-chan Event, error) {
	if c.adapters == nil {
		return nil, errors.New("adapter registry is nil")
	}
	if err := options.Validate(model.Protocol, input.Tools); err != nil {
		return nil, err
	}

	// A nil provider registry still resolves the legacy environment API key.
	model, options = c.providers.ResolveRequest(model, options)

	adapter, ok := c.adapters.Get(model.Protocol)
	if !ok {
		return nil, fmt.Errorf(
			"no adapter registered for protocol %q",
			model.Protocol,
		)
	}

	return adapter.Stream(ctx, model, input, options)
}

// Complete consumes a provider stream and returns the final assistant message.
func (c *Client) Complete(
	ctx context.Context,
	model Model,
	input Context,
	options StreamOptions,
) (AssistantMessage, error) {
	events, err := c.Stream(ctx, model, input, options)
	if err != nil {
		return AssistantMessage{}, err
	}

	for event := range events {
		switch event.Type {
		case EventDone:
			if event.Message == nil {
				return AssistantMessage{}, errors.New(
					"done event does not contain a message",
				)
			}

			return *event.Message, nil

		case EventError:
			if event.Message != nil {
				if event.Err != nil {
					return *event.Message, event.Err
				}

				return *event.Message, errors.New(
					"provider stream failed",
				)
			}

			if event.Err != nil {
				return AssistantMessage{}, event.Err
			}

			return AssistantMessage{}, errors.New(
				"provider stream failed",
			)
		}
	}

	return AssistantMessage{}, errors.New(
		"provider stream closed without a final event",
	)
}
