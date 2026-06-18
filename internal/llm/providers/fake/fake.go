package fake

import (
	"context"
	"fmt"

	"github.com/ktsoator/or/internal/llm"
)

const API = "fake"

// Provider is an in-memory provider useful for tests and local development.
type Provider struct {
	response string
}

// NewProvider creates a fake provider that streams the given response.
func NewProvider(response string) *Provider {
	return &Provider{
		response: response,
	}
}

// API returns the registry key for the fake provider.
func (p *Provider) API() string {
	return API
}

// Stream emits a deterministic response without calling an external service.
func (p *Provider) Stream(
	ctx context.Context,
	model llm.Model,
	input llm.Context,
	options llm.StreamOptions,
) (<-chan llm.Event, error) {
	if model.API != p.API() {
		return nil, fmt.Errorf(
			"model API %q does not match provider API %q",
			model.API,
			p.API(),
		)
	}

	events := make(chan llm.Event, 4)

	go func() {
		defer close(events)

		partial := llm.AssistantMessage{
			Model: model.ID,
		}

		events <- llm.Event{
			Type:    llm.EventStart,
			Partial: &partial,
		}

		select {
		case <-ctx.Done():
			failed := llm.AssistantMessage{
				Model:      model.ID,
				StopReason: "aborted",
			}

			events <- llm.Event{
				Type:    llm.EventError,
				Message: &failed,
				Err:     ctx.Err(),
			}
			return

		default:
		}

		partial = llm.AssistantMessage{
			Model: model.ID,
			Content: []llm.Content{
				{
					Type: llm.ContentText,
					Text: p.response,
				},
			},
		}

		events <- llm.Event{
			Type:    llm.EventTextDelta,
			Delta:   p.response,
			Partial: &partial,
		}

		finalMessage := partial
		finalMessage.StopReason = "stop"

		events <- llm.Event{
			Type:    llm.EventDone,
			Message: &finalMessage,
		}
	}()

	_ = input
	_ = options

	return events, nil
}
