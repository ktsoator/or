package llm

import (
	"context"
	"fmt"
)

// FakeProvider is an in-memory provider useful for tests and local development.
type FakeProvider struct {
	response string
}

// NewFakeProvider creates a fake provider that streams the given response.
func NewFakeProvider(response string) *FakeProvider {
	return &FakeProvider{
		response: response,
	}
}

// API returns the registry key for the fake provider.
func (p *FakeProvider) API() string {
	return "fake"
}

// Stream emits a deterministic response without calling an external service.
func (p *FakeProvider) Stream(
	ctx context.Context,
	model Model,
	input Context,
	options StreamOptions,
) (<-chan Event, error) {
	if model.API != p.API() {
		return nil, fmt.Errorf(
			"model API %q does not match provider API %q",
			model.API,
			p.API(),
		)
	}

	events := make(chan Event, 4)

	go func() {
		defer close(events)

		partial := AssistantMessage{
			Model: model.ID,
		}

		events <- Event{
			Type:    EventStart,
			Partial: &partial,
		}

		select {
		case <-ctx.Done():
			failed := AssistantMessage{
				Model:      model.ID,
				StopReason: "aborted",
			}

			events <- Event{
				Type:    EventError,
				Message: &failed,
				Err:     ctx.Err(),
			}
			return

		default:
		}

		partial = AssistantMessage{
			Model: model.ID,
			Content: []Content{
				{
					Type: ContentText,
					Text: p.response,
				},
			},
		}

		events <- Event{
			Type:    EventTextDelta,
			Delta:   p.response,
			Partial: &partial,
		}

		finalMessage := partial
		finalMessage.StopReason = "stop"

		events <- Event{
			Type:    EventDone,
			Message: &finalMessage,
		}
	}()

	_ = input
	_ = options

	return events, nil
}
