package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/observability"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

type memoryRecorder struct {
	mu     sync.Mutex
	events []observability.Event
}

func (r *memoryRecorder) Record(event observability.Event) {
	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()
}

func (*memoryRecorder) Close() error { return nil }

func (r *memoryRecorder) snapshot() []observability.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]observability.Event(nil), r.events...)
}

func eventsNamed(events []observability.Event, name string) []observability.Event {
	var matches []observability.Event
	for _, event := range events {
		if event.Name == name {
			matches = append(matches, event)
		}
	}
	return matches
}

func onlyEvent(t *testing.T, events []observability.Event, name string) observability.Event {
	t.Helper()
	matches := eventsNamed(events, name)
	if len(matches) != 1 {
		t.Fatalf("%s events = %#v, want exactly one", name, matches)
	}
	return matches[0]
}

func TestRunObservabilityUsesDurableRunIdentity(t *testing.T) {
	recorder := &memoryRecorder{}
	session, err := New(context.Background(), Options{
		SessionID: "session-1",
		Recorder:  recorder,
		Model:     llm.Model{Provider: "test", ID: "model"},
		Tools:     []tools.Tool{},
		Store:     &transcript.Memory{},
		StreamFn:  fixedResponse("answer"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "question"); err != nil {
		t.Fatal(err)
	}

	events := recorder.snapshot()
	started := onlyEvent(t, events, observability.RunStarted)
	completed := onlyEvent(t, events, observability.RunCompleted)
	if started.RunID == "" || started.RunID != completed.RunID ||
		started.SessionID != "session-1" || completed.Status != "completed" {
		t.Fatalf("run correlation = started %#v, completed %#v", started, completed)
	}
	entries := session.Entries()
	runEntry := entries[len(entries)-1]
	if runEntry.Type != transcript.RunEntry || runEntry.ID != started.RunID {
		t.Fatalf("run entry = %#v, events = %#v", runEntry, events)
	}
	if completed.Duration < 0 || completed.ErrorCode != "" {
		t.Fatalf("completed event = %#v", completed)
	}
}

func TestTurnCorrelationQueuesNextTurnBeforePriorTurnEnds(t *testing.T) {
	session := &Session{}
	startedAt := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	session.setRunState(context.Background(), "run-1", startedAt, 0)
	defer session.clearRunState()

	session.beginTurn("turn-1", startedAt.Add(time.Millisecond))
	firstRequest := session.attachRequest("request-1")
	session.beginTurn("turn-2", startedAt.Add(2*time.Millisecond))
	secondRequest := session.attachRequest("request-2")
	firstTurn, firstStartedAt := session.finishTurn()
	secondTurn, secondStartedAt := session.finishTurn()

	if firstRequest.turnID != "turn-1" || secondRequest.turnID != "turn-2" {
		t.Fatalf("request correlations = first %#v, second %#v", firstRequest, secondRequest)
	}
	if firstTurn.turnID != "turn-1" || firstTurn.requestID != "request-1" ||
		!firstStartedAt.Equal(startedAt.Add(time.Millisecond)) {
		t.Fatalf("first turn = %#v at %v", firstTurn, firstStartedAt)
	}
	if secondTurn.turnID != "turn-2" || secondTurn.requestID != "request-2" ||
		!secondStartedAt.Equal(startedAt.Add(2*time.Millisecond)) {
		t.Fatalf("second turn = %#v at %v", secondTurn, secondStartedAt)
	}
}

func TestProviderObservabilityCorrelatesPerformanceUsageAndAttempts(t *testing.T) {
	recorder := &memoryRecorder{}
	const (
		sensitiveURL     = "https://provider.example/private"
		sensitiveBody    = `{"prompt":"private prompt"}`
		sensitiveHeader  = "private-response-header"
		providerResponse = "response-17"
	)
	var requestCallbacks, responseCallbacks int
	var callbackURL, callbackBody, callbackHeader string
	model := llm.Model{Provider: "test-provider", ID: "test-model"}
	session, err := New(context.Background(), Options{
		SessionID: "session-provider",
		Recorder:  recorder,
		Model:     model,
		Tools:     []tools.Tool{},
		Store:     &transcript.Memory{},
		StreamOptions: llm.StreamOptions{
			OnRequest: func(_ string, url string, body []byte) {
				requestCallbacks++
				callbackURL = url
				callbackBody = string(body)
			},
			OnResponse: func(_ int, headers http.Header) {
				responseCallbacks++
				callbackHeader = headers.Get("X-Private")
			},
		},
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			_ llm.Context,
			options llm.StreamOptions,
		) (<-chan llm.Event, error) {
			options.OnRequest("POST", sensitiveURL, []byte(sensitiveBody))
			options.OnResponse(503, http.Header{"X-Private": []string{sensitiveHeader}})
			options.OnRequest("POST", sensitiveURL, []byte(sensitiveBody))
			options.OnResponse(200, http.Header{"X-Private": []string{sensitiveHeader}})
			time.Sleep(2 * time.Millisecond)

			partial := llm.NewAssistantMessage(model)
			message := llm.NewAssistantMessage(model)
			message.ResponseModel = "test-model-2026-08"
			message.ResponseID = providerResponse
			message.StopReason = llm.StopReasonStop
			message.Content = []llm.AssistantContent{&llm.TextContent{Text: "answer"}}
			message.Usage = llm.Usage{
				Input: 11, Output: 7, CacheRead: 3, CacheWrite: 2, TotalTokens: 23,
				Cost: llm.UsageCost{
					Input: 0.01, Output: 0.14, CacheRead: 0.01,
					CacheWrite: 0.04, Total: 0.20,
				},
			}
			events := make(chan llm.Event, 2)
			events <- llm.Event{Type: llm.EventTextDelta, Delta: "a", Partial: &partial}
			events <- llm.Event{Type: llm.EventDone, Message: &message}
			close(events)
			return events, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "question"); err != nil {
		t.Fatal(err)
	}
	if requestCallbacks != 2 || responseCallbacks != 2 || callbackURL != sensitiveURL ||
		callbackBody != sensitiveBody || callbackHeader != sensitiveHeader {
		t.Fatalf(
			"original callbacks = requests %d, responses %d, url %q, body %q, header %q",
			requestCallbacks, responseCallbacks, callbackURL, callbackBody, callbackHeader,
		)
	}

	events := recorder.snapshot()
	runStarted := onlyEvent(t, events, observability.RunStarted)
	turnStarted := onlyEvent(t, events, observability.TurnStarted)
	checkpoint := onlyEvent(t, events, observability.CheckpointCompleted)
	providerStarted := onlyEvent(t, events, observability.ProviderStarted)
	providerCompleted := onlyEvent(t, events, observability.ProviderCompleted)
	turnCompleted := onlyEvent(t, events, observability.TurnCompleted)

	if runStarted.RunID == "" || turnStarted.TurnID == "" || providerStarted.RequestID == "" {
		t.Fatalf("missing correlation IDs: run %#v, turn %#v, provider %#v", runStarted, turnStarted, providerStarted)
	}
	for _, event := range []observability.Event{checkpoint, providerStarted, providerCompleted, turnCompleted} {
		if event.SessionID != "session-provider" || event.RunID != runStarted.RunID ||
			event.TurnID != turnStarted.TurnID || event.RequestID != providerStarted.RequestID {
			t.Fatalf("correlation mismatch: %#v", event)
		}
	}
	if checkpoint.MessageCount != 1 || checkpoint.AttachmentCount == 0 || checkpoint.Duration < 0 {
		t.Fatalf("checkpoint event = %#v", checkpoint)
	}
	if providerCompleted.Status != "completed" || providerCompleted.Provider != model.Provider ||
		providerCompleted.Model != model.ID || providerCompleted.ResponseModel != "test-model-2026-08" ||
		providerCompleted.ProviderResponseID != providerResponse ||
		providerCompleted.StopReason != string(llm.StopReasonStop) ||
		providerCompleted.TimeToFirstOutput <= 0 ||
		providerCompleted.Duration < providerCompleted.TimeToFirstOutput {
		t.Fatalf("provider completion = %#v", providerCompleted)
	}
	if providerCompleted.InputTokens != 11 || providerCompleted.OutputTokens != 7 ||
		providerCompleted.CacheReadTokens != 3 || providerCompleted.CacheWriteTokens != 2 ||
		providerCompleted.TotalTokens != 23 || providerCompleted.CostTotal != 0.20 {
		t.Fatalf("provider usage = %#v", providerCompleted)
	}

	attemptStarts := eventsNamed(events, observability.HTTPAttemptStarted)
	attemptResponses := eventsNamed(events, observability.HTTPAttemptResponse)
	if len(attemptStarts) != 2 || len(attemptResponses) != 2 {
		t.Fatalf("attempt events = starts %#v, responses %#v", attemptStarts, attemptResponses)
	}
	for index := range attemptStarts {
		wantAttempt := index + 1
		if attemptStarts[index].Attempt != wantAttempt || attemptResponses[index].Attempt != wantAttempt ||
			attemptStarts[index].RequestID != providerStarted.RequestID ||
			attemptResponses[index].RequestID != providerStarted.RequestID {
			t.Fatalf("attempt %d correlation = start %#v, response %#v", wantAttempt, attemptStarts[index], attemptResponses[index])
		}
	}
	if attemptResponses[0].HTTPStatus != 503 || attemptResponses[1].HTTPStatus != 200 {
		t.Fatalf("attempt responses = %#v", attemptResponses)
	}
	serialized := fmt.Sprintf("%#v", events)
	for _, forbidden := range []string{sensitiveURL, sensitiveBody, sensitiveHeader, "private prompt"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("observability events contain sensitive value %q: %s", forbidden, serialized)
		}
	}
}

type observabilityToolArgs struct{}

func TestObservabilityUsesDistinctCorrelationForToolLoopTurns(t *testing.T) {
	recorder := &memoryRecorder{}
	call := 0
	tool := tools.Tool{
		AgentTool: agent.AgentTool{
			Definition: llm.MustTool[observabilityToolArgs]("observe", "observe"),
			Execute: func(
				context.Context,
				string,
				json.RawMessage,
				func(agent.ToolProgress),
			) (agent.ToolResult, error) {
				return agent.ToolResult{
					Content: []llm.ToolResultContent{&llm.TextContent{Text: "observed"}},
					Outcome: agent.ToolOutcome{Status: agent.ToolOutcomeSuccess},
				}, nil
			},
		},
		AccessFor: tools.InternalAccess,
	}
	session, err := New(context.Background(), Options{
		SessionID: "session-tool-loop",
		Recorder:  recorder,
		Model:     llm.Model{Provider: "test", ID: "model"},
		Tools:     []tools.Tool{tool},
		Store:     &transcript.Memory{},
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			_ llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			call++
			message := llm.NewAssistantMessage(model)
			if call == 1 {
				message.StopReason = llm.StopReasonToolUse
				message.Content = []llm.AssistantContent{
					&llm.ToolCall{ID: "call-1", Name: "observe", Arguments: map[string]any{}},
				}
			} else {
				message.StopReason = llm.StopReasonStop
				message.Content = []llm.AssistantContent{&llm.TextContent{Text: "done"}}
			}
			return finalEvents(llm.EventDone, &message), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "use the tool"); err != nil {
		t.Fatal(err)
	}

	events := recorder.snapshot()
	run := onlyEvent(t, events, observability.RunStarted)
	turnStarts := eventsNamed(events, observability.TurnStarted)
	turnEnds := eventsNamed(events, observability.TurnCompleted)
	providers := eventsNamed(events, observability.ProviderCompleted)
	checkpoints := eventsNamed(events, observability.CheckpointCompleted)
	if len(turnStarts) != 2 || len(turnEnds) != 2 || len(providers) != 2 || len(checkpoints) != 2 {
		t.Fatalf(
			"tool-loop events = turn starts %d, turn ends %d, providers %d, checkpoints %d",
			len(turnStarts), len(turnEnds), len(providers), len(checkpoints),
		)
	}
	if turnStarts[0].TurnID == turnStarts[1].TurnID || providers[0].RequestID == providers[1].RequestID {
		t.Fatalf("tool-loop IDs were reused: turns %#v, providers %#v", turnStarts, providers)
	}
	for index := range turnStarts {
		if turnStarts[index].RunID != run.RunID || turnEnds[index].RunID != run.RunID ||
			providers[index].RunID != run.RunID || checkpoints[index].RunID != run.RunID ||
			turnEnds[index].TurnID != turnStarts[index].TurnID ||
			providers[index].TurnID != turnStarts[index].TurnID ||
			checkpoints[index].TurnID != turnStarts[index].TurnID ||
			turnEnds[index].RequestID != providers[index].RequestID ||
			checkpoints[index].RequestID != providers[index].RequestID {
			t.Fatalf("tool-loop correlation at turn %d: starts %#v, ends %#v, provider %#v, checkpoint %#v", index, turnStarts[index], turnEnds[index], providers[index], checkpoints[index])
		}
	}
}

func TestObservabilityRecordsRetryReasonAndNewCorrelation(t *testing.T) {
	recorder := &memoryRecorder{}
	call := 0
	session, err := New(context.Background(), Options{
		SessionID: "session-retry",
		Recorder:  recorder,
		Model:     llm.Model{Provider: "test", ID: "model"},
		Tools:     []tools.Tool{},
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			_ llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			call++
			message := llm.NewAssistantMessage(model)
			if call == 1 {
				message.StopReason = llm.StopReasonError
				message.ErrorMessage = "temporarily unavailable"
				return finalEvents(llm.EventError, &message), nil
			}
			message.StopReason = llm.StopReasonStop
			message.Content = []llm.AssistantContent{&llm.TextContent{Text: "recovered"}}
			return finalEvents(llm.EventDone, &message), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "question"); err != nil {
		t.Fatal(err)
	}

	events := recorder.snapshot()
	turns := eventsNamed(events, observability.TurnCompleted)
	providers := append(
		eventsNamed(events, observability.ProviderFailed),
		eventsNamed(events, observability.ProviderCompleted)...,
	)
	discarded := onlyEvent(t, events, observability.TurnDiscarded)
	if len(turns) != 2 || len(providers) != 2 {
		t.Fatalf("retry lifecycle = turns %#v, providers %#v", turns, providers)
	}
	if turns[0].TurnID == turns[1].TurnID || turns[0].RequestID == turns[1].RequestID {
		t.Fatalf("retry reused correlation: %#v", turns)
	}
	if discarded.Reason != "retry" || discarded.TurnID != turns[0].TurnID ||
		discarded.RequestID != turns[0].RequestID {
		t.Fatalf("retry discard = %#v, first turn = %#v", discarded, turns[0])
	}
	if turns[0].Status != "failed" || turns[1].Status != "completed" {
		t.Fatalf("retry turn statuses = %#v", turns)
	}
}

func TestObservabilityRecordsContextOverflowCompactionReason(t *testing.T) {
	recorder := &memoryRecorder{}
	store := &transcript.Memory{}
	if err := store.Append(context.Background(), seededTurns(6)...); err != nil {
		t.Fatal(err)
	}
	compactor := &recordingCompactor{}
	call := 0
	session, err := New(context.Background(), Options{
		SessionID: "session-overflow",
		Recorder:  recorder,
		Model: llm.Model{
			Provider: "test", ID: "model", ContextWindow: 400, MaxTokens: 100,
		},
		Tools:     []tools.Tool{},
		Store:     store,
		Compactor: compactor,
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			_ llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			call++
			message := llm.NewAssistantMessage(model)
			if call == 1 {
				message.StopReason = llm.StopReasonError
				message.ErrorMessage = "prompt is too long"
				return finalEvents(llm.EventError, &message), nil
			}
			message.StopReason = llm.StopReasonStop
			message.Content = []llm.AssistantContent{&llm.TextContent{Text: "recovered"}}
			return finalEvents(llm.EventDone, &message), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "question"); err != nil {
		t.Fatal(err)
	}
	if len(compactor.requests) != 1 {
		t.Fatalf("compaction requests = %d, want 1", len(compactor.requests))
	}

	events := recorder.snapshot()
	turns := eventsNamed(events, observability.TurnCompleted)
	discarded := onlyEvent(t, events, observability.TurnDiscarded)
	if len(turns) != 2 {
		t.Fatalf("overflow turns = %#v", turns)
	}
	if discarded.Reason != "context_overflow" || discarded.TurnID != turns[0].TurnID ||
		discarded.RequestID != turns[0].RequestID {
		t.Fatalf("overflow discard = %#v, first turn = %#v", discarded, turns[0])
	}
	if turns[0].TurnID == turns[1].TurnID || turns[0].RequestID == turns[1].RequestID {
		t.Fatalf("overflow recovery reused correlation: %#v", turns)
	}
}

func TestRunObservabilityClassifiesFailureWithoutErrorText(t *testing.T) {
	recorder := &memoryRecorder{}
	sensitive := "provider failed with secret payload"
	zeroRetries := 0
	session, err := New(context.Background(), Options{
		SessionID:  "session-2",
		Recorder:   recorder,
		Model:      llm.Model{Provider: "test", ID: "model"},
		Tools:      []tools.Tool{},
		MaxRetries: &zeroRetries,
		StreamFn: func(
			_ context.Context,
			_ llm.Model,
			_ llm.Context,
			options llm.StreamOptions,
		) (<-chan llm.Event, error) {
			options.OnRequest("POST", "https://private.example", []byte(`{"prompt":"private"}`))
			return nil, errors.New(sensitive)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "question"); err == nil || err.Error() != sensitive {
		t.Fatalf("prompt error = %v", err)
	}

	events := recorder.snapshot()
	providerFailure := onlyEvent(t, events, observability.ProviderFailed)
	turnFailure := onlyEvent(t, events, observability.TurnCompleted)
	runFailure := onlyEvent(t, events, observability.RunFailed)
	if providerFailure.Status != "failed" || providerFailure.ErrorCode != "provider_setup_failed" ||
		turnFailure.Status != "failed" || turnFailure.ErrorCode != "provider_request_failed" ||
		runFailure.Status != "failed" || runFailure.ErrorCode != "run_failed" {
		t.Fatalf("failure events = provider %#v, turn %#v, run %#v", providerFailure, turnFailure, runFailure)
	}
	attempts := eventsNamed(events, observability.HTTPAttemptResponse)
	if len(attempts) != 1 || attempts[0].Status != "failed" ||
		attempts[0].ErrorCode != "no_response" || attempts[0].Attempt != 1 {
		t.Fatalf("failed HTTP attempt = %#v", attempts)
	}
	serialized := fmt.Sprintf("%#v", events)
	for _, forbidden := range []string{sensitive, "https://private.example", "private"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("failure events contain sensitive value %q: %s", forbidden, serialized)
		}
	}
}

func TestProviderObservabilityClassifiesStreamFailures(t *testing.T) {
	tests := []struct {
		name      string
		stream    agent.StreamFn
		errorCode string
		wantUsage bool
	}{
		{
			name: "terminal error",
			stream: func(
				_ context.Context,
				model llm.Model,
				_ llm.Context,
				_ llm.StreamOptions,
			) (<-chan llm.Event, error) {
				time.Sleep(2 * time.Millisecond)
				partial := llm.NewAssistantMessage(model)
				message := llm.NewAssistantMessage(model)
				message.StopReason = llm.StopReasonError
				message.ErrorMessage = "secret stream failure"
				message.Usage = llm.Usage{Input: 8, Output: 2, TotalTokens: 10}
				return streamEvents(
					llm.Event{Type: llm.EventThinkingDelta, Delta: "hidden", Partial: &partial},
					llm.Event{Type: llm.EventError, Message: &message, Err: errors.New(message.ErrorMessage)},
				), nil
			},
			errorCode: "provider_stream_failed",
			wantUsage: true,
		},
		{
			name: "closed without terminal",
			stream: func(
				_ context.Context,
				model llm.Model,
				_ llm.Context,
				_ llm.StreamOptions,
			) (<-chan llm.Event, error) {
				partial := llm.NewAssistantMessage(model)
				return streamEvents(
					llm.Event{Type: llm.EventTextDelta, Delta: "partial", Partial: &partial},
				), nil
			},
			errorCode: "stream_closed_without_terminal",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := &memoryRecorder{}
			zeroRetries := 0
			session, err := New(context.Background(), Options{
				SessionID:  "session-stream-failure",
				Recorder:   recorder,
				Model:      llm.Model{Provider: "test", ID: "model"},
				Tools:      []tools.Tool{},
				MaxRetries: &zeroRetries,
				StreamFn:   test.stream,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := session.Prompt(context.Background(), "question"); err == nil {
				t.Fatal("Prompt succeeded, want stream failure")
			}

			events := recorder.snapshot()
			failure := onlyEvent(t, events, observability.ProviderFailed)
			if failure.ErrorCode != test.errorCode || failure.Status != "failed" {
				t.Fatalf("provider failure = %#v", failure)
			}
			if test.wantUsage && (failure.InputTokens != 8 || failure.OutputTokens != 2 ||
				failure.TotalTokens != 10 || failure.TimeToFirstOutput <= 0) {
				t.Fatalf("failed provider usage = %#v", failure)
			}
			if completed := eventsNamed(events, observability.ProviderCompleted); len(completed) != 0 {
				t.Fatalf("provider completion recorded for failed stream: %#v", completed)
			}
			if strings.Contains(fmt.Sprintf("%#v", events), "secret stream failure") {
				t.Fatalf("provider error text leaked into events: %#v", events)
			}
		})
	}
}

func TestCheckpointFailureIsCorrelatedAndPreventsProviderRequest(t *testing.T) {
	recorder := &memoryRecorder{}
	storeErr := errors.New("secret checkpoint failure")
	store := &checkpointStore{failErr: storeErr}
	providerCalls := 0
	session, err := New(context.Background(), Options{
		SessionID: "session-checkpoint",
		Recorder:  recorder,
		Model:     llm.Model{Provider: "test", ID: "model"},
		Tools:     []tools.Tool{},
		Store:     store,
		StreamFn: func(
			context.Context,
			llm.Model,
			llm.Context,
			llm.StreamOptions,
		) (<-chan llm.Event, error) {
			providerCalls++
			return nil, errors.New("provider must not be called")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "question"); !errors.Is(err, storeErr) {
		t.Fatalf("Prompt error = %v, want checkpoint failure", err)
	}
	if providerCalls != 0 {
		t.Fatalf("provider calls = %d, want zero", providerCalls)
	}

	events := recorder.snapshot()
	turnStarted := onlyEvent(t, events, observability.TurnStarted)
	checkpoint := onlyEvent(t, events, observability.CheckpointFailed)
	turnCompleted := onlyEvent(t, events, observability.TurnCompleted)
	discarded := onlyEvent(t, events, observability.TurnDiscarded)
	runFailed := onlyEvent(t, events, observability.RunFailed)
	if checkpoint.ErrorCode != "checkpoint_persist_failed" || checkpoint.Status != "failed" ||
		checkpoint.RequestID == "" || checkpoint.TurnID != turnStarted.TurnID {
		t.Fatalf("checkpoint failure = %#v", checkpoint)
	}
	if turnCompleted.ErrorCode != "checkpoint_failed" || turnCompleted.RequestID != checkpoint.RequestID ||
		discarded.Reason != "persistence_failure" || discarded.RequestID != checkpoint.RequestID ||
		runFailed.ErrorCode != "checkpoint_failed" {
		t.Fatalf("checkpoint lifecycle = turn %#v, discarded %#v, run %#v", turnCompleted, discarded, runFailed)
	}
	if providers := eventsNamed(events, observability.ProviderStarted); len(providers) != 0 {
		t.Fatalf("provider request started after checkpoint failure: %#v", providers)
	}
	if strings.Contains(fmt.Sprintf("%#v", events), storeErr.Error()) {
		t.Fatalf("checkpoint error text leaked into events: %#v", events)
	}
}

func streamEvents(events ...llm.Event) <-chan llm.Event {
	stream := make(chan llm.Event, len(events))
	for _, event := range events {
		stream <- event
	}
	close(stream)
	return stream
}

var _ observability.Recorder = (*memoryRecorder)(nil)
