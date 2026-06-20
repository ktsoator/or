package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Client routes LLM requests to the adapter registered for a model protocol.
type Client struct {
	registry *Registry
}

// NewClient creates a client backed by the given provider registry.
func NewClient(registry *Registry) *Client {
	return &Client{
		registry: registry,
	}
}

// Stream starts a streaming completion request for the given model and input.
func (c *Client) Stream(ctx context.Context, model Model, input Context, options StreamOptions) (<-chan Event, error) {
	if c.registry == nil {
		return nil, errors.New("provider registry is nil")
	}
	if err := options.Validate(model.Protocol, input.Tools); err != nil {
		return nil, err
	}

	adapter, ok := c.registry.Get(model.Protocol)
	if !ok {
		return nil, fmt.Errorf(
			"no adapter registered for protocol %q",
			model.Protocol,
		)
	}
	if strings.TrimSpace(options.APIKey) == "" {
		options.APIKey = GetEnvAPIKeyWithEnv(model.Provider, options.Env)
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
