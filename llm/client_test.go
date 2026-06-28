package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeAdapter is a controllable ProtocolAdapter used to drive Client.Stream /
// Client.Complete without touching the network.
type fakeAdapter struct {
	protocol Protocol
	// onStream lets each test shape the event stream and observe the options the
	// client passes through.
	onStream func(ctx context.Context, model Model, input Context, options StreamOptions) (<-chan Event, error)
}

func (a *fakeAdapter) Protocol() Protocol { return a.protocol }
func (a *fakeAdapter) Stream(ctx context.Context, model Model, input Context, options StreamOptions) (<-chan Event, error) {
	return a.onStream(ctx, model, input, options)
}

func newFakeAdapter(protocol Protocol, fn func(StreamOptions) (<-chan Event, error)) *fakeAdapter {
	return &fakeAdapter{
		protocol: protocol,
		onStream: func(_ context.Context, _ Model, _ Context, options StreamOptions) (<-chan Event, error) {
			return fn(options)
		},
	}
}

func registryWith(t *testing.T, adapters ...ProtocolAdapter) *Registry {
	t.Helper()
	reg := NewRegistry()
	for _, a := range adapters {
		if err := reg.Register(a); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}
	return reg
}

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	adapter := newFakeAdapter(ProtocolOpenAICompletions, nil)
	if err := reg.Register(adapter); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, ok := reg.Get(ProtocolOpenAICompletions)
	if !ok || got != adapter {
		t.Fatalf("Get = (%v, %v), want adapter ok", got, ok)
	}
}

func TestRegistryRegisterRejectsNilAndEmptyProtocol(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(nil); err == nil || !strings.Contains(err.Error(), "nil") {
		t.Fatalf("Register(nil) = %v, want nil error", err)
	}
	empty := newFakeAdapter(Protocol(""), nil)
	if err := reg.Register(empty); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("Register(empty protocol) = %v, want empty error", err)
	}
}

func TestRegistryRegisterReplacesExisting(t *testing.T) {
	reg := NewRegistry()
	first := newFakeAdapter(ProtocolOpenAICompletions, nil)
	second := newFakeAdapter(ProtocolOpenAICompletions, nil)
	_ = reg.Register(first)
	_ = reg.Register(second)
	got, _ := reg.Get(ProtocolOpenAICompletions)
	if got != second {
		t.Fatalf("Get returned first adapter, want second")
	}
}

func TestRegistryGetUnknownProtocol(t *testing.T) {
	reg := NewRegistry()
	if got, ok := reg.Get(Protocol("nope")); ok || got != nil {
		t.Fatalf("Get unknown = (%v, %v), want (nil, false)", got, ok)
	}
}

func TestClientStreamReturnsErrorWhenRegistryIsNil(t *testing.T) {
	client := NewClient(nil)
	_, err := client.Stream(context.Background(), Model{Protocol: ProtocolOpenAICompletions}, Context{}, StreamOptions{})
	if err == nil || !strings.Contains(err.Error(), "registry is nil") {
		t.Fatalf("Stream error = %v, want nil-registry error", err)
	}
}

func TestClientStreamReturnsErrorForUnknownProtocol(t *testing.T) {
	client := NewClient(NewRegistry())
	_, err := client.Stream(context.Background(), Model{Protocol: Protocol("nope")}, Context{}, StreamOptions{})
	if err == nil || !strings.Contains(err.Error(), "no adapter") {
		t.Fatalf("Stream error = %v, want no-adapter error", err)
	}
}

func TestClientStreamSurfacesOptionsValidationError(t *testing.T) {
	adapter := newFakeAdapter(ProtocolOpenAICompletions, func(StreamOptions) (<-chan Event, error) {
		t.Fatalf("adapter must not be called when validation fails")
		return nil, nil
	})
	client := NewClient(registryWith(t, adapter))
	// Send Anthropic-shaped options into an OpenAI model — Validate must reject.
	options := StreamOptions{ProtocolOptions: &AnthropicStreamOptions{}}
	_, err := client.Stream(
		context.Background(),
		Model{Protocol: ProtocolOpenAICompletions},
		Context{},
		options,
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("Stream error = %v, want protocol mismatch", err)
	}
}

func TestClientStreamDispatchesToAdapterMatchingModelProtocol(t *testing.T) {
	called := ""
	oai := newFakeAdapter(ProtocolOpenAICompletions, func(StreamOptions) (<-chan Event, error) {
		called = "openai"
		return doneChannel(), nil
	})
	ant := newFakeAdapter(ProtocolAnthropicMessages, func(StreamOptions) (<-chan Event, error) {
		called = "anthropic"
		return doneChannel(), nil
	})
	client := NewClient(registryWith(t, oai, ant))
	_, err := client.Stream(
		context.Background(),
		Model{Protocol: ProtocolAnthropicMessages, Provider: "anthropic"},
		Context{},
		StreamOptions{APIKey: "x"},
	)
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}
	if called != "anthropic" {
		t.Fatalf("called = %q, want anthropic", called)
	}
}

func TestClientStreamFillsAPIKeyFromProviderEnvWhenEmpty(t *testing.T) {
	// When APIKey is empty, the client must look it up from ProviderEnv before
	// dispatching, so adapters never see a credential-less request.
	var seen StreamOptions
	adapter := newFakeAdapter(ProtocolOpenAICompletions, func(opts StreamOptions) (<-chan Event, error) {
		seen = opts
		return doneChannel(), nil
	})
	client := NewClient(registryWith(t, adapter))
	_, err := client.Stream(
		context.Background(),
		Model{Protocol: ProtocolOpenAICompletions, Provider: "deepseek"},
		Context{},
		StreamOptions{Env: ProviderEnv{"DEEPSEEK_API_KEY": "env-key"}},
	)
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}
	if seen.APIKey != "env-key" {
		t.Fatalf("APIKey = %q, want env-key", seen.APIKey)
	}
}

func TestClientStreamLeavesExplicitAPIKeyUnchanged(t *testing.T) {
	var seen StreamOptions
	adapter := newFakeAdapter(ProtocolOpenAICompletions, func(opts StreamOptions) (<-chan Event, error) {
		seen = opts
		return doneChannel(), nil
	})
	client := NewClient(registryWith(t, adapter))
	_, err := client.Stream(
		context.Background(),
		Model{Protocol: ProtocolOpenAICompletions, Provider: "deepseek"},
		Context{},
		StreamOptions{
			APIKey: "explicit",
			Env:    ProviderEnv{"DEEPSEEK_API_KEY": "env-key"},
		},
	)
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}
	if seen.APIKey != "explicit" {
		t.Fatalf("APIKey = %q, want explicit", seen.APIKey)
	}
}

func TestClientCompleteReturnsAssistantMessageFromDoneEvent(t *testing.T) {
	final := &AssistantMessage{
		Content:    []AssistantContent{&TextContent{Text: "hi"}},
		StopReason: StopReasonStop,
		Provider:   "demo",
	}
	adapter := newFakeAdapter(ProtocolOpenAICompletions, func(StreamOptions) (<-chan Event, error) {
		events := make(chan Event, 2)
		events <- Event{Type: EventTextDelta, Delta: "hi"}
		events <- Event{Type: EventDone, Message: final}
		close(events)
		return events, nil
	})
	client := NewClient(registryWith(t, adapter))
	got, err := client.Complete(context.Background(), Model{Protocol: ProtocolOpenAICompletions}, Context{}, StreamOptions{APIKey: "x"})
	if err != nil {
		t.Fatalf("Complete error = %v", err)
	}
	if got.StopReason != StopReasonStop || len(got.Content) != 1 {
		t.Fatalf("Complete result = %#v, want non-empty stop", got)
	}
}

func TestClientCompleteReturnsErrorFromErrorEvent(t *testing.T) {
	want := errors.New("boom")
	final := &AssistantMessage{StopReason: StopReasonError, ErrorMessage: "boom"}
	adapter := newFakeAdapter(ProtocolOpenAICompletions, func(StreamOptions) (<-chan Event, error) {
		events := make(chan Event, 1)
		events <- Event{Type: EventError, Err: want, Message: final}
		close(events)
		return events, nil
	})
	client := NewClient(registryWith(t, adapter))
	got, err := client.Complete(context.Background(), Model{Protocol: ProtocolOpenAICompletions}, Context{}, StreamOptions{APIKey: "x"})
	if !errors.Is(err, want) {
		t.Fatalf("Complete err = %v, want %v", err, want)
	}
	// Even on failure the partial message should be returned so callers can
	// inspect what was assembled.
	if got.StopReason != StopReasonError {
		t.Fatalf("Complete result = %#v, want error result", got)
	}
}

func TestClientCompleteHandlesErrorEventWithoutErr(t *testing.T) {
	adapter := newFakeAdapter(ProtocolOpenAICompletions, func(StreamOptions) (<-chan Event, error) {
		events := make(chan Event, 1)
		events <- Event{Type: EventError}
		close(events)
		return events, nil
	})
	client := NewClient(registryWith(t, adapter))
	_, err := client.Complete(context.Background(), Model{Protocol: ProtocolOpenAICompletions}, Context{}, StreamOptions{APIKey: "x"})
	if err == nil || !strings.Contains(err.Error(), "stream failed") {
		t.Fatalf("Complete err = %v, want stream-failed default", err)
	}
}

func TestClientCompleteDoneWithoutMessageIsError(t *testing.T) {
	adapter := newFakeAdapter(ProtocolOpenAICompletions, func(StreamOptions) (<-chan Event, error) {
		events := make(chan Event, 1)
		events <- Event{Type: EventDone}
		close(events)
		return events, nil
	})
	client := NewClient(registryWith(t, adapter))
	_, err := client.Complete(context.Background(), Model{Protocol: ProtocolOpenAICompletions}, Context{}, StreamOptions{APIKey: "x"})
	if err == nil || !strings.Contains(err.Error(), "does not contain a message") {
		t.Fatalf("Complete err = %v, want missing-message error", err)
	}
}

func TestClientCompleteStreamClosedWithoutTerminalIsError(t *testing.T) {
	adapter := newFakeAdapter(ProtocolOpenAICompletions, func(StreamOptions) (<-chan Event, error) {
		events := make(chan Event, 1)
		events <- Event{Type: EventTextDelta, Delta: "x"}
		close(events)
		return events, nil
	})
	client := NewClient(registryWith(t, adapter))
	_, err := client.Complete(context.Background(), Model{Protocol: ProtocolOpenAICompletions}, Context{}, StreamOptions{APIKey: "x"})
	if err == nil || !strings.Contains(err.Error(), "closed without a final event") {
		t.Fatalf("Complete err = %v, want closed-without-terminal error", err)
	}
}

func TestClientCompletePropagatesStreamSetupError(t *testing.T) {
	adapter := newFakeAdapter(ProtocolOpenAICompletions, func(StreamOptions) (<-chan Event, error) {
		return nil, errors.New("setup failed")
	})
	client := NewClient(registryWith(t, adapter))
	_, err := client.Complete(context.Background(), Model{Protocol: ProtocolOpenAICompletions}, Context{}, StreamOptions{APIKey: "x"})
	if err == nil || !strings.Contains(err.Error(), "setup failed") {
		t.Fatalf("Complete err = %v, want setup error", err)
	}
}

func doneChannel() <-chan Event {
	events := make(chan Event, 1)
	events <- Event{Type: EventDone, Message: &AssistantMessage{StopReason: StopReasonStop}}
	close(events)
	return events
}
