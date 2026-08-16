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
	"github.com/ktsoator/or/coding/internal/requestsnapshot"
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
	snapshots []requestsnapshot.Snapshot
}

func (writer *memorySnapshotWriter) Save(snapshot requestsnapshot.Snapshot) error {
	writer.mu.Lock()
	writer.snapshots = append(writer.snapshots, snapshot)
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
		output := requestsnapshot.Output{
			CapturedAt:   time.Now().UTC(),
			Message:      requestsnapshot.Message{Role: "assistant", ProviderRequestID: requestID},
			StopReason:   string(message.StopReason),
			ErrorMessage: message.ErrorMessage,
		}
		for _, content := range message.Content {
			switch typed := content.(type) {
			case *llm.TextContent:
				output.Message.Content = append(output.Message.Content, requestsnapshot.Content{Type: "text", Text: typed.Text})
			case *llm.ThinkingContent:
				output.Message.Content = append(output.Message.Content, requestsnapshot.Content{Type: "thinking", Thinking: typed.Thinking})
			case *llm.ToolCall:
				output.Message.Content = append(output.Message.Content, requestsnapshot.Content{Type: "toolCall", ToolCallID: typed.ID, ToolName: typed.Name, Arguments: typed.Arguments})
			}
		}
		writer.snapshots[index].Output = &output
		break
	}
	return nil
}

func (writer *memorySnapshotWriter) snapshot() requestsnapshot.Snapshot {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if len(writer.snapshots) == 0 {
		return requestsnapshot.Snapshot{}
	}
	return writer.snapshots[len(writer.snapshots)-1]
}

func (writer *memorySnapshotWriter) allSnapshots() []requestsnapshot.Snapshot {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return append([]requestsnapshot.Snapshot(nil), writer.snapshots...)
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
	checkpoint := onlyEvent(t, events, observability.CheckpointCompleted)
	providerStarted := onlyEvent(t, events, observability.ProviderStarted)
	providerCompleted := onlyEvent(t, events, observability.ProviderCompleted)
	turnCompleted := onlyEvent(t, events, observability.TurnCompleted)
	captured := snapshots.snapshot()

	if runStarted.RunID == "" || turnStarted.TurnID == "" || providerStarted.RequestID == "" {
		t.Fatalf("missing correlation IDs: run %#v, turn %#v, provider %#v", runStarted, turnStarted, providerStarted)
	}
	if captured.SessionID != "session-provider" || captured.RunID != runStarted.RunID ||
		captured.TurnID != turnStarted.TurnID || captured.ProviderRequestID != providerStarted.RequestID ||
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
	provider := eventsNamed(events, observability.ProviderCompleted)[0]
	toolStarted := onlyEvent(t, events, observability.ToolStarted)
	toolCompleted := onlyEvent(t, events, observability.ToolCompleted)
	approvalStarted := onlyEvent(t, events, observability.ApprovalStarted)
	approvalCompleted := onlyEvent(t, events, observability.ApprovalCompleted)
	for _, event := range []observability.Event{
		toolStarted, toolCompleted, approvalStarted, approvalCompleted,
	} {
		if event.SessionID != "session-tool" || event.RunID != turn.RunID ||
			event.TurnID != turn.TurnID || event.RequestID != provider.RequestID ||
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

func TestObservabilityUsesDistinctCorrelationForToolLoopTurns(t *testing.T) {
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
	if len(turnStarts) != 2 || len(turnEnds) != 2 || len(providers) != 2 ||
		len(providerCheckpoints) != 2 || len(toolCheckpoints) != 1 {
		t.Fatalf(
			"tool-loop events = turn starts %d, turn ends %d, providers %d, provider checkpoints %d, tool checkpoints %d",
			len(turnStarts), len(turnEnds), len(providers), len(providerCheckpoints), len(toolCheckpoints),
		)
	}
	if turnStarts[0].TurnID == turnStarts[1].TurnID || providers[0].RequestID == providers[1].RequestID {
		t.Fatalf("tool-loop IDs were reused: turns %#v, providers %#v", turnStarts, providers)
	}
	for index := range turnStarts {
		if turnStarts[index].RunID != run.RunID || turnEnds[index].RunID != run.RunID ||
			providers[index].RunID != run.RunID || providerCheckpoints[index].RunID != run.RunID ||
			turnEnds[index].TurnID != turnStarts[index].TurnID ||
			providers[index].TurnID != turnStarts[index].TurnID ||
			providerCheckpoints[index].TurnID != turnStarts[index].TurnID ||
			turnEnds[index].RequestID != providers[index].RequestID ||
			providerCheckpoints[index].RequestID != providers[index].RequestID {
			t.Fatalf("tool-loop correlation at turn %d: starts %#v, ends %#v, provider %#v, checkpoint %#v", index, turnStarts[index], turnEnds[index], providers[index], providerCheckpoints[index])
		}
	}
	toolCheckpoint := toolCheckpoints[0]
	if toolCheckpoint.RunID != run.RunID || toolCheckpoint.TurnID != turnStarts[0].TurnID ||
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
	var historicalAssistant *requestsnapshot.Message
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
