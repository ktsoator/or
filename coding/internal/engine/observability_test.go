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
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/snapshot"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

type memoryRecorder struct {
	mu     sync.Mutex
	events []observability.Event
}

type memorySnapshotWriter struct {
	mu        sync.Mutex
	snapshots []snapshot.Snapshot
}

func (writer *memorySnapshotWriter) Save(record snapshot.Snapshot) error {
	writer.mu.Lock()
	writer.snapshots = append(writer.snapshots, record)
	writer.mu.Unlock()
	return nil
}

func (writer *memorySnapshotWriter) SaveOutput(requestID string, message *llm.AssistantMessage) error {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	for index := len(writer.snapshots) - 1; index >= 0; index-- {
		if writer.snapshots[index].ProviderRequestID != requestID {
			continue
		}
		output := snapshot.Output{
			CapturedAt:   time.Now().UTC(),
			Message:      snapshot.Message{Role: "assistant", ProviderRequestID: requestID},
			StopReason:   string(message.StopReason),
			ErrorMessage: message.ErrorMessage,
		}
		for _, content := range message.Content {
			switch typed := content.(type) {
			case *llm.TextContent:
				output.Message.Content = append(output.Message.Content, snapshot.Content{Type: "text", Text: typed.Text})
			case *llm.ThinkingContent:
				output.Message.Content = append(output.Message.Content, snapshot.Content{Type: "thinking", Thinking: typed.Thinking})
			case *llm.ToolCall:
				output.Message.Content = append(output.Message.Content, snapshot.Content{Type: "toolCall", ToolCallID: typed.ID, ToolName: typed.Name, Arguments: typed.Arguments})
			}
		}
		writer.snapshots[index].Output = &output
		break
	}
	return nil
}

func (writer *memorySnapshotWriter) snapshot() snapshot.Snapshot {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if len(writer.snapshots) == 0 {
		return snapshot.Snapshot{}
	}
	return writer.snapshots[len(writer.snapshots)-1]
}

func (writer *memorySnapshotWriter) allSnapshots() []snapshot.Snapshot {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return append([]snapshot.Snapshot(nil), writer.snapshots...)
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
	projection, err := transcript.ProjectSession(entries)
	if err != nil || len(projection.Runs) != 1 || projection.Runs[0].ID != started.RunID {
		t.Fatalf("run projection = %#v, %v; events = %#v", projection, err, events)
	}
	if completed.Duration < 0 || completed.ErrorCode != "" {
		t.Fatalf("completed event = %#v", completed)
	}
}

func TestRunObservabilityReportsFinalPersistenceFailure(t *testing.T) {
	recorder := &memoryRecorder{}
	store := &checkpointStore{}
	storeErr := errors.New("final persistence unavailable")
	stream := fixedResponse("answer")
	session, err := New(context.Background(), Options{
		SessionID: "session-final-persistence",
		Recorder:  recorder,
		Model:     llm.Model{Provider: "test", ID: "model"},
		Tools:     []tools.Tool{},
		Store:     store,
		StreamFn: func(
			ctx context.Context,
			model llm.Model,
			input llm.Context,
			options llm.StreamOptions,
		) (<-chan llm.Event, error) {
			store.failNext(storeErr)
			return stream(ctx, model, input, options)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "question"); !errors.Is(err, storeErr) {
		t.Fatalf("Prompt error = %v, want final persistence failure", err)
	}

	events := recorder.snapshot()
	step := onlyEvent(t, events, observability.StepCompleted)
	turn := onlyEvent(t, events, observability.TurnCompleted)
	run := onlyEvent(t, events, observability.RunFailed)
	if step.Status != "completed" || step.ErrorCode != "" ||
		turn.Status != "failed" || turn.ErrorCode != "persistence_failed" ||
		run.Status != "failed" || run.ErrorCode != "persistence_failed" {
		t.Fatalf("terminal events = step %#v, turn %#v, run %#v", step, turn, run)
	}
	if failed := eventsNamed(events, observability.CheckpointFailed); len(failed) != 0 {
		t.Fatalf("final persistence failure recorded as checkpoint failure: %#v", failed)
	}
}

func TestStepCorrelationQueuesNextStepBeforePriorStepEnds(t *testing.T) {
	coordinator := &lifecycleCoordinator{}
	startedAt := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	coordinator.startRunWithIDs(
		0,
		"run-1",
		"lifecycle-turn-1",
		startedAt,
	)
	coordinator.beginStepCheckpointWithID(
		0,
		"step-1",
		startedAt.Add(time.Millisecond),
	)
	firstRequest := coordinator.attachProviderRequestWithID("request-1")
	coordinator.beginStepCheckpointWithID(
		0,
		"step-2",
		startedAt.Add(2*time.Millisecond),
	)
	secondRequest := coordinator.attachProviderRequestWithID("request-2")
	firstStep := coordinator.completeStep(0, startedAt.Add(3*time.Millisecond), "completed", "").step
	secondStep := coordinator.completeStep(0, startedAt.Add(4*time.Millisecond), "completed", "").step

	if firstRequest.turnID != "lifecycle-turn-1" || secondRequest.turnID != "lifecycle-turn-1" ||
		firstRequest.stepID != "step-1" || secondRequest.stepID != "step-2" {
		t.Fatalf("request correlations = first %#v, second %#v", firstRequest, secondRequest)
	}
	if firstStep.stepID != "step-1" || firstStep.requestID != "request-1" ||
		!firstStep.startedAt.Equal(startedAt.Add(time.Millisecond)) {
		t.Fatalf("first step = %#v at %v", firstStep, firstStep.startedAt)
	}
	if secondStep.stepID != "step-2" || secondStep.requestID != "request-2" ||
		!secondStep.startedAt.Equal(startedAt.Add(2*time.Millisecond)) {
		t.Fatalf("second step = %#v at %v", secondStep, secondStep.startedAt)
	}
}

func TestProviderObservabilityCorrelatesPerformanceUsageAndAttempts(t *testing.T) {
	recorder := &memoryRecorder{}
	snapshots := &memorySnapshotWriter{}
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
		SessionID:        "session-provider",
		Recorder:         recorder,
		RequestSnapshots: snapshots,
		Model:            model,
		Tools:            []tools.Tool{},
		Store:            &transcript.Memory{},
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
	stepStarted := onlyEvent(t, events, observability.StepStarted)
	checkpoint := onlyEvent(t, events, observability.CheckpointCompleted)
	providerStarted := onlyEvent(t, events, observability.ProviderStarted)
	providerCompleted := onlyEvent(t, events, observability.ProviderCompleted)
	turnCompleted := onlyEvent(t, events, observability.TurnCompleted)
	stepCompleted := onlyEvent(t, events, observability.StepCompleted)
	captured := snapshots.snapshot()

	if runStarted.RunID == "" || turnStarted.TurnID == "" || stepStarted.StepID == "" ||
		providerStarted.RequestID == "" {
		t.Fatalf("missing correlation IDs: run %#v, turn %#v, provider %#v", runStarted, turnStarted, providerStarted)
	}
	if captured.SessionID != "session-provider" || captured.RunID != runStarted.RunID ||
		captured.TurnID != turnStarted.TurnID || captured.StepID != stepStarted.StepID ||
		captured.ProviderRequestID != providerStarted.RequestID ||
		captured.Provider != model.Provider || captured.Model != model.ID {
		t.Fatalf("request snapshot correlation = %#v", captured)
	}
	if len(captured.Input.Messages) < 2 || captured.Input.Messages[len(captured.Input.Messages)-1].Content[0].Text != "question" ||
		len(captured.Attachments) == 0 || captured.Attachments[0].MessageIndex != 0 {
		t.Fatalf("request snapshot input = %#v", captured)
	}
	if captured.Output == nil || len(captured.Output.Message.Content) != 1 ||
		captured.Output.Message.Content[0].Text != "answer" || captured.Output.StopReason != "stop" ||
		captured.Output.Message.ProviderRequestID != providerStarted.RequestID {
		t.Fatalf("request snapshot output = %#v", captured.Output)
	}
	for _, event := range []observability.Event{checkpoint, providerStarted, providerCompleted, stepCompleted} {
		if event.SessionID != "session-provider" || event.RunID != runStarted.RunID ||
			event.TurnID != turnStarted.TurnID || event.StepID != stepStarted.StepID ||
			event.RequestID != providerStarted.RequestID {
			t.Fatalf("correlation mismatch: %#v", event)
		}
	}
	if turnCompleted.RunID != runStarted.RunID || turnCompleted.TurnID != turnStarted.TurnID ||
		turnCompleted.StepID != "" || turnCompleted.RequestID != "" {
		t.Fatalf("turn correlation = start %#v, completed %#v", turnStarted, turnCompleted)
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
			attemptStarts[index].AttemptID == "" ||
			attemptStarts[index].AttemptID != attemptResponses[index].AttemptID ||
			attemptStarts[index].RequestID != providerStarted.RequestID ||
			attemptResponses[index].RequestID != providerStarted.RequestID ||
			attemptStarts[index].StepID != stepStarted.StepID ||
			attemptResponses[index].StepID != stepStarted.StepID {
			t.Fatalf("attempt %d correlation = start %#v, response %#v", wantAttempt, attemptStarts[index], attemptResponses[index])
		}
	}
	if attemptStarts[0].AttemptID == attemptStarts[1].AttemptID {
		t.Fatalf("attempt ID reused: %#v", attemptStarts)
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

type observabilityApprovalArgs struct {
	Command string `json:"command"`
}

type engineApproverFunc func(
	context.Context,
	permission.ApprovalRequest,
) (permission.ApprovalResponse, error)

func (f engineApproverFunc) Decide(
	ctx context.Context,
	request permission.ApprovalRequest,
) (permission.ApprovalResponse, error) {
	return f(ctx, request)
}

func TestToolObservabilityRecordsApprovalAndExecutionLatency(t *testing.T) {
	recorder := &memoryRecorder{}
	const (
		toolCallID      = "call-sensitive"
		sensitiveArg    = "secret command payload"
		sensitiveResult = "secret tool result"
	)
	var approvalRequest permission.ApprovalRequest
	executed := false
	tool := tools.Tool{
		AgentTool: agent.AgentTool{
			Definition: llm.MustTool[observabilityApprovalArgs]("shell", "run command"),
			Execute: func(
				_ context.Context,
				_ string,
				_ json.RawMessage,
				_ func(agent.ToolProgress),
			) (agent.ToolResult, error) {
				executed = true
				time.Sleep(2 * time.Millisecond)
				return agent.ToolResult{
					Content: []llm.ToolResultContent{&llm.TextContent{Text: sensitiveResult}},
					Outcome: agent.ToolOutcome{Status: agent.ToolOutcomeSuccess},
				}, nil
			},
		},
		AccessFor: func(map[string]any) []permission.Access {
			return []permission.Access{{Action: permission.Execute}}
		},
	}
	call := 0
	session, err := New(context.Background(), Options{
		SessionID:      "session-tool",
		Recorder:       recorder,
		Model:          llm.Model{Provider: "test", ID: "model"},
		Tools:          []tools.Tool{tool},
		PermissionMode: permission.ModeAsk,
		Approver: engineApproverFunc(func(
			_ context.Context,
			request permission.ApprovalRequest,
		) (permission.ApprovalResponse, error) {
			approvalRequest = request
			time.Sleep(2 * time.Millisecond)
			return permission.ApprovalResponse{Choice: permission.AllowOnce}, nil
		}),
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
				message.Content = []llm.AssistantContent{&llm.ToolCall{
					ID: toolCallID, Name: "shell",
					Arguments: map[string]any{"command": sensitiveArg},
				}}
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
	if err := session.Prompt(context.Background(), "use shell"); err != nil {
		t.Fatal(err)
	}
	if !executed || approvalRequest.Request.ToolCallID != toolCallID ||
		approvalRequest.Request.Args["command"] != sensitiveArg {
		t.Fatalf("approval transparency = executed %t, request %#v", executed, approvalRequest)
	}

	events := recorder.snapshot()
	turn := eventsNamed(events, observability.TurnStarted)[0]
	step := eventsNamed(events, observability.StepStarted)[0]
	provider := eventsNamed(events, observability.ProviderCompleted)[0]
	toolStarted := onlyEvent(t, events, observability.ToolStarted)
	toolCompleted := onlyEvent(t, events, observability.ToolCompleted)
	approvalStarted := onlyEvent(t, events, observability.ApprovalStarted)
	approvalCompleted := onlyEvent(t, events, observability.ApprovalCompleted)
	for _, event := range []observability.Event{
		toolStarted, toolCompleted, approvalStarted, approvalCompleted,
	} {
		if event.SessionID != "session-tool" || event.RunID != turn.RunID ||
			event.TurnID != turn.TurnID || event.StepID != step.StepID ||
			event.RequestID != provider.RequestID ||
			event.ToolCallID != toolCallID || event.ToolName != "shell" {
			t.Fatalf("tool correlation = %#v", event)
		}
	}
	if toolStarted.Status != "running" || toolCompleted.Status != "success" ||
		toolCompleted.Duration <= 0 || approvalStarted.Status != "waiting" ||
		approvalCompleted.Status != "allowed" || approvalCompleted.Duration <= 0 ||
		toolCompleted.Duration < approvalCompleted.Duration {
		t.Fatalf(
			"tool timings = start %#v, completed %#v, approval start %#v, approval completed %#v",
			toolStarted, toolCompleted, approvalStarted, approvalCompleted,
		)
	}
	serialized := fmt.Sprintf("%#v", events)
	for _, forbidden := range []string{sensitiveArg, sensitiveResult, approvalRequest.Reason} {
		if forbidden != "" && strings.Contains(serialized, forbidden) {
			t.Fatalf("tool events contain sensitive value %q: %s", forbidden, serialized)
		}
	}
}

func TestToolObservabilityRecordsDeniedApprovalWithoutExecution(t *testing.T) {
	recorder := &memoryRecorder{}
	executed := false
	tool := tools.Tool{
		AgentTool: agent.AgentTool{
			Definition: llm.MustTool[observabilityToolArgs]("blocked", "blocked tool"),
			Execute: func(
				context.Context,
				string,
				json.RawMessage,
				func(agent.ToolProgress),
			) (agent.ToolResult, error) {
				executed = true
				return agent.ToolResult{}, nil
			},
		},
		AccessFor: func(map[string]any) []permission.Access {
			return []permission.Access{{Action: permission.Write, Path: "private/path"}}
		},
	}
	call := 0
	session, err := New(context.Background(), Options{
		SessionID: "session-denied", Recorder: recorder,
		Model: llm.Model{Provider: "test", ID: "model"}, Tools: []tools.Tool{tool},
		PermissionMode: permission.ModeAsk,
		Approver: engineApproverFunc(func(
			context.Context,
			permission.ApprovalRequest,
		) (permission.ApprovalResponse, error) {
			return permission.ApprovalResponse{Choice: permission.Reject}, nil
		}),
		StreamFn: toolThenStopStream(&call, llm.ToolCall{
			ID: "call-denied", Name: "blocked", Arguments: map[string]any{},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "try blocked tool"); err != nil {
		t.Fatal(err)
	}
	if executed {
		t.Fatal("denied tool executed")
	}
	events := recorder.snapshot()
	approval := onlyEvent(t, events, observability.ApprovalCompleted)
	toolFailure := onlyEvent(t, events, observability.ToolFailed)
	if approval.Status != "denied" || approval.ErrorCode != "" {
		t.Fatalf("denied approval = %#v", approval)
	}
	if toolFailure.Status != "failed" || toolFailure.ErrorCode != "tool_blocked" ||
		toolFailure.ToolCallID != approval.ToolCallID {
		t.Fatalf("blocked tool = %#v, approval = %#v", toolFailure, approval)
	}
	for _, event := range events {
		if event.Reason == "tool_dispatch" {
			t.Fatalf("denied tool recorded a dispatch checkpoint: %#v", event)
		}
	}
	if strings.Contains(fmt.Sprintf("%#v", events), "private/path") {
		t.Fatalf("tool events leaked access path: %#v", events)
	}
}

func TestToolObservabilitySanitizesExternalFailureCode(t *testing.T) {
	recorder := &memoryRecorder{}
	const sensitiveCode = "secret-provider-error-detail"
	tool := tools.Tool{
		AgentTool: agent.AgentTool{
			Definition: llm.MustTool[observabilityToolArgs]("external", "external tool"),
			Execute: func(
				context.Context,
				string,
				json.RawMessage,
				func(agent.ToolProgress),
			) (agent.ToolResult, error) {
				return agent.ToolResult{Outcome: agent.ToolOutcome{
					Status: agent.ToolOutcomeFailed, ErrorCode: sensitiveCode,
				}}, nil
			},
		},
		AccessFor: tools.InternalAccess,
	}
	call := 0
	session, err := New(context.Background(), Options{
		SessionID: "session-external", Recorder: recorder,
		Model: llm.Model{Provider: "test", ID: "model"}, Tools: []tools.Tool{tool},
		StreamFn: toolThenStopStream(&call, llm.ToolCall{
			ID: "call-external", Name: "external", Arguments: map[string]any{},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "use external tool"); err != nil {
		t.Fatal(err)
	}
	failure := onlyEvent(t, recorder.snapshot(), observability.ToolFailed)
	if failure.ErrorCode != "tool_failed" || strings.Contains(fmt.Sprintf("%#v", failure), sensitiveCode) {
		t.Fatalf("sanitized tool failure = %#v", failure)
	}
}

func toolThenStopStream(call *int, toolCall llm.ToolCall) agent.StreamFn {
	return func(
		_ context.Context,
		model llm.Model,
		_ llm.Context,
		_ llm.StreamOptions,
	) (<-chan llm.Event, error) {
		*call = *call + 1
		message := llm.NewAssistantMessage(model)
		if *call == 1 {
			message.StopReason = llm.StopReasonToolUse
			message.Content = []llm.AssistantContent{&toolCall}
		} else {
			message.StopReason = llm.StopReasonStop
			message.Content = []llm.AssistantContent{&llm.TextContent{Text: "done"}}
		}
		return finalEvents(llm.EventDone, &message), nil
	}
}

func TestObservabilityUsesDistinctStepCorrelationForToolLoop(t *testing.T) {
	recorder := &memoryRecorder{}
	snapshots := &memorySnapshotWriter{}
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
		SessionID:        "session-tool-loop",
		Recorder:         recorder,
		RequestSnapshots: snapshots,
		Model:            llm.Model{Provider: "test", ID: "model"},
		Tools:            []tools.Tool{tool},
		Store:            &transcript.Memory{},
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
	stepStarts := eventsNamed(events, observability.StepStarted)
	stepEnds := eventsNamed(events, observability.StepCompleted)
	providers := eventsNamed(events, observability.ProviderCompleted)
	checkpoints := eventsNamed(events, observability.CheckpointCompleted)
	var providerCheckpoints, toolCheckpoints []observability.Event
	for _, checkpoint := range checkpoints {
		switch checkpoint.Reason {
		case "provider_request":
			providerCheckpoints = append(providerCheckpoints, checkpoint)
		case "tool_dispatch":
			toolCheckpoints = append(toolCheckpoints, checkpoint)
		}
	}
	if len(turnStarts) != 1 || len(turnEnds) != 1 ||
		len(stepStarts) != 2 || len(stepEnds) != 2 || len(providers) != 2 ||
		len(providerCheckpoints) != 2 || len(toolCheckpoints) != 1 {
		t.Fatalf(
			"tool-loop events = turns %d/%d, steps %d/%d, providers %d, provider checkpoints %d, tool checkpoints %d",
			len(turnStarts), len(turnEnds), len(stepStarts), len(stepEnds),
			len(providers), len(providerCheckpoints), len(toolCheckpoints),
		)
	}
	if stepStarts[0].StepID == stepStarts[1].StepID || providers[0].RequestID == providers[1].RequestID {
		t.Fatalf("tool-loop IDs were reused: steps %#v, providers %#v", stepStarts, providers)
	}
	for index := range stepStarts {
		if stepStarts[index].RunID != run.RunID || stepEnds[index].RunID != run.RunID ||
			providers[index].RunID != run.RunID || providerCheckpoints[index].RunID != run.RunID ||
			stepStarts[index].TurnID != turnStarts[0].TurnID ||
			stepEnds[index].TurnID != turnStarts[0].TurnID ||
			providers[index].TurnID != turnStarts[0].TurnID ||
			providerCheckpoints[index].TurnID != turnStarts[0].TurnID ||
			stepEnds[index].StepID != stepStarts[index].StepID ||
			providers[index].StepID != stepStarts[index].StepID ||
			providerCheckpoints[index].StepID != stepStarts[index].StepID ||
			stepEnds[index].RequestID != providers[index].RequestID ||
			providerCheckpoints[index].RequestID != providers[index].RequestID {
			t.Fatalf("tool-loop correlation at step %d: starts %#v, ends %#v, provider %#v, checkpoint %#v", index, stepStarts[index], stepEnds[index], providers[index], providerCheckpoints[index])
		}
	}
	toolCheckpoint := toolCheckpoints[0]
	if toolCheckpoint.RunID != run.RunID || toolCheckpoint.TurnID != turnStarts[0].TurnID ||
		toolCheckpoint.StepID != stepStarts[0].StepID ||
		toolCheckpoint.RequestID != providers[0].RequestID || toolCheckpoint.ToolCallID != "call-1" ||
		toolCheckpoint.ToolName != "observe" {
		t.Fatalf("tool checkpoint correlation = %#v", toolCheckpoint)
	}

	captured := snapshots.allSnapshots()
	if len(captured) != 2 {
		t.Fatalf("tool-loop snapshots = %d, want 2: %#v", len(captured), captured)
	}
	if captured[0].Output == nil ||
		captured[0].Output.Message.ProviderRequestID != providers[0].RequestID {
		t.Fatalf("first request output provenance = %#v, want %q", captured[0].Output, providers[0].RequestID)
	}
	var historicalAssistant *snapshot.Message
	for index := range captured[1].Input.Messages {
		message := &captured[1].Input.Messages[index]
		if message.Role == "assistant" {
			historicalAssistant = message
			break
		}
	}
	if historicalAssistant == nil || historicalAssistant.ProviderRequestID != providers[0].RequestID {
		t.Fatalf("second request historical assistant provenance = %#v, want %q", historicalAssistant, providers[0].RequestID)
	}
	if captured[1].Output == nil ||
		captured[1].Output.Message.ProviderRequestID != providers[1].RequestID {
		t.Fatalf("second request output provenance = %#v, want %q", captured[1].Output, providers[1].RequestID)
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
	turnStarted := onlyEvent(t, events, observability.TurnStarted)
	turnCompleted := onlyEvent(t, events, observability.TurnCompleted)
	steps := eventsNamed(events, observability.StepCompleted)
	providers := append(
		eventsNamed(events, observability.ProviderFailed),
		eventsNamed(events, observability.ProviderCompleted)...,
	)
	discarded := onlyEvent(t, events, observability.StepDiscarded)
	if len(steps) != 2 || len(providers) != 2 {
		t.Fatalf("retry lifecycle = steps %#v, providers %#v", steps, providers)
	}
	if turnCompleted.TurnID != turnStarted.TurnID ||
		steps[0].TurnID != turnStarted.TurnID || steps[1].TurnID != turnStarted.TurnID ||
		steps[0].StepID == steps[1].StepID || steps[0].RequestID == steps[1].RequestID {
		t.Fatalf("retry correlation = turn %#v/%#v, steps %#v", turnStarted, turnCompleted, steps)
	}
	if discarded.Reason != "retry" || discarded.TurnID != steps[0].TurnID ||
		discarded.StepID != steps[0].StepID || discarded.RequestID != steps[0].RequestID {
		t.Fatalf("retry discard = %#v, first step = %#v", discarded, steps[0])
	}
	if steps[0].Status != "failed" || steps[1].Status != "completed" {
		t.Fatalf("retry step statuses = %#v", steps)
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
	turn := onlyEvent(t, events, observability.TurnStarted)
	steps := eventsNamed(events, observability.StepCompleted)
	discarded := onlyEvent(t, events, observability.StepDiscarded)
	if len(steps) != 2 {
		t.Fatalf("overflow steps = %#v", steps)
	}
	if discarded.Reason != "context_overflow" || discarded.TurnID != turn.TurnID ||
		discarded.StepID != steps[0].StepID || discarded.RequestID != steps[0].RequestID {
		t.Fatalf("overflow discard = %#v, first step = %#v", discarded, steps[0])
	}
	if steps[0].TurnID != steps[1].TurnID || steps[0].StepID == steps[1].StepID ||
		steps[0].RequestID == steps[1].RequestID {
		t.Fatalf("overflow recovery correlation = %#v", steps)
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
	stepFailure := onlyEvent(t, events, observability.StepCompleted)
	turnFailure := onlyEvent(t, events, observability.TurnCompleted)
	runFailure := onlyEvent(t, events, observability.RunFailed)
	if providerFailure.Status != "failed" || providerFailure.ErrorCode != "provider_setup_failed" ||
		stepFailure.Status != "failed" || stepFailure.ErrorCode != "provider_request_failed" ||
		turnFailure.Status != "failed" || turnFailure.ErrorCode != "run_failed" ||
		runFailure.Status != "failed" || runFailure.ErrorCode != "run_failed" {
		t.Fatalf("failure events = provider %#v, step %#v, turn %#v, run %#v", providerFailure, stepFailure, turnFailure, runFailure)
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
	stepStarted := onlyEvent(t, events, observability.StepStarted)
	checkpoint := onlyEvent(t, events, observability.CheckpointFailed)
	turnCompleted := onlyEvent(t, events, observability.TurnCompleted)
	stepCompleted := onlyEvent(t, events, observability.StepCompleted)
	discarded := onlyEvent(t, events, observability.StepDiscarded)
	runFailed := onlyEvent(t, events, observability.RunFailed)
	if checkpoint.ErrorCode != "checkpoint_persist_failed" || checkpoint.Status != "failed" ||
		checkpoint.RequestID == "" || checkpoint.TurnID != turnStarted.TurnID ||
		checkpoint.StepID != stepStarted.StepID {
		t.Fatalf("checkpoint failure = %#v", checkpoint)
	}
	if stepCompleted.ErrorCode != "checkpoint_failed" || stepCompleted.RequestID != checkpoint.RequestID ||
		turnCompleted.ErrorCode != "checkpoint_failed" || turnCompleted.RequestID != "" ||
		discarded.Reason != "persistence_failure" || discarded.StepID != checkpoint.StepID ||
		discarded.RequestID != checkpoint.RequestID ||
		runFailed.ErrorCode != "checkpoint_failed" {
		t.Fatalf("checkpoint lifecycle = step %#v, turn %#v, discarded %#v, run %#v", stepCompleted, turnCompleted, discarded, runFailed)
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
