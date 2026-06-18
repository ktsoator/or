package fake

import (
	"context"
	"fmt"

	"github.com/ktsoator/or/internal/llm"
)

const Protocol llm.Protocol = "fake"

// Adapter is an in-memory protocol adapter useful for tests and local development.
type Adapter struct {
	response string
}

// NewAdapter creates a fake adapter that streams the given response.
func NewAdapter(response string) *Adapter {
	return &Adapter{
		response: response,
	}
}

// Protocol returns the registry key for the fake provider.
func (a *Adapter) Protocol() llm.Protocol {
	return Protocol
}

// Stream emits a deterministic response without calling an external service.
func (a *Adapter) Stream(
	ctx context.Context,
	model llm.Model,
	input llm.Context,
	options llm.StreamOptions,
) (<-chan llm.Event, error) {
	if model.Protocol != a.Protocol() {
		return nil, fmt.Errorf(
			"model protocol %q does not match adapter protocol %q",
			model.Protocol,
			a.Protocol(),
		)
	}

	events := make(chan llm.Event, 6)

	go func() {
		defer close(events)

		partial := llm.NewAssistantMessage(model)

		events <- llm.Event{
			Type:    llm.EventStart,
			Partial: &partial,
		}

		select {
		case <-ctx.Done():
			failed := llm.NewAssistantMessage(model)
			failed.StopReason = "aborted"
			failed.ErrorMessage = ctx.Err().Error()

			events <- llm.Event{
				Type:    llm.EventError,
				Message: &failed,
				Err:     ctx.Err(),
			}
			return

		default:
		}

		textStart := partial
		textStart.Content = []llm.AssistantContent{&llm.TextContent{}}

		events <- llm.Event{
			Type:         llm.EventTextStart,
			ContentIndex: 0,
			Partial:      &textStart,
		}

		partial.Content = []llm.AssistantContent{&llm.TextContent{Text: a.response}}

		events <- llm.Event{
			Type:         llm.EventTextDelta,
			ContentIndex: 0,
			Delta:        a.response,
			Partial:      &partial,
		}

		finalMessage := partial
		finalMessage.StopReason = "stop"
		events <- llm.Event{
			Type:         llm.EventTextEnd,
			ContentIndex: 0,
			Content:      a.response,
			Partial:      &finalMessage,
		}

		events <- llm.Event{
			Type:    llm.EventDone,
			Message: &finalMessage,
		}
	}()

	_ = input
	_ = options

	return events, nil
}
