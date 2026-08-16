package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/observability"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

type checkpointStore struct {
	mu          sync.Mutex
	entries     []transcript.Entry
	batches     [][]transcript.Entry
	appendCalls int
	failErr     error
	failOnce    bool
	failed      bool
}

func (s *checkpointStore) Load(context.Context) ([]transcript.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]transcript.Entry(nil), s.entries...), nil
}

func (s *checkpointStore) Append(_ context.Context, entries ...transcript.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendCalls++
	if s.failErr != nil && (!s.failOnce || !s.failed) {
		s.failed = true
		return s.failErr
	}
	batch := append([]transcript.Entry(nil), entries...)
	s.batches = append(s.batches, batch)
	s.entries = append(s.entries, batch...)
	return nil
}

func (s *checkpointStore) snapshot() ([]transcript.Entry, [][]transcript.Entry, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := append([]transcript.Entry(nil), s.entries...)
	batches := make([][]transcript.Entry, len(s.batches))
	for index := range s.batches {
		batches[index] = append([]transcript.Entry(nil), s.batches[index]...)
	}
	return entries, batches, s.appendCalls
}

func (s *checkpointStore) failNext(err error) {
	s.mu.Lock()
	s.failErr = err
	s.failOnce = true
	s.failed = false
	s.mu.Unlock()
}

func TestSessionCheckpointsPromptBeforeModelRequest(t *testing.T) {
	ctx := context.Background()
	store := &checkpointStore{}
	var checkpointErr error

	session, err := New(ctx, Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		Store: store,
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			_ llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			entries, _, _ := store.snapshot()
			if len(entries) != 2 {
				checkpointErr = fmt.Errorf(
					"entries before model request = %d, want base context and user",
					len(entries),
				)
			} else if entries[0].Type != transcript.ContextEntry {
				checkpointErr = fmt.Errorf("first durable entry = %q, want context", entries[0].Type)
			} else if _, ok := llmEntry(entries[1]).(*llm.UserMessage); !ok {
				checkpointErr = fmt.Errorf("first durable message = %T, want user", llmEntry(entries[1]))
			}
			return assistantEvents(model, "answer"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := session.Prompt(ctx, "question"); err != nil {
		t.Fatal(err)
	}
	if checkpointErr != nil {
		t.Fatal(checkpointErr)
	}

	entries, batches, _ := store.snapshot()
	if len(entries) != 4 {
		t.Fatalf("durable entries = %d, want context, user, assistant, run", len(entries))
	}
	if len(batches) != 2 || len(batches[0]) != 2 || len(batches[1]) != 2 {
		t.Fatalf("append batch sizes = %v, want [2 2]", batchSizes(batches))
	}
	if entries[3].Type != transcript.RunEntry {
		t.Fatalf("last entry type = %q, want run", entries[3].Type)
	}
}

type checkpointToolArgs struct {
	Text string `json:"text"`
}

func TestSessionCheckpointsToolIntentBeforeExecution(t *testing.T) {
	ctx := context.Background()
	store := &checkpointStore{}
	modelCalls := 0
	toolCalls := 0
	var bodyErr error

	tool := tools.Tool{
		AgentTool: agent.AgentTool{
			Definition: llm.MustTool[checkpointToolArgs]("echo", "echo text"),
			Execute: func(
				_ context.Context,
				_ string,
				_ json.RawMessage,
				_ func(agent.ToolProgress),
			) (agent.ToolResult, error) {
				toolCalls++
				entries, _, _ := store.snapshot()
				if len(entries) != 4 {
					bodyErr = fmt.Errorf("entries before tool body = %d, want context, user, assistant, intent", len(entries))
				} else if _, ok := llmEntry(entries[2]).(*llm.AssistantMessage); !ok {
					bodyErr = fmt.Errorf("checkpoint[2] = %T, want assistant", llmEntry(entries[2]))
				} else if intent := entries[3]; intent.Type != transcript.ToolCallEntry ||
					intent.ToolCall == nil || intent.ToolCall.ToolCallID != "call-1" ||
					intent.ToolCall.ToolName != "echo" ||
					string(intent.ToolCall.Arguments) != `{"text":"one"}` {
					bodyErr = fmt.Errorf("checkpoint[3] = %#v, want durable tool intent", intent)
				}
				return agent.ToolResult{
					Content: []llm.ToolResultContent{&llm.TextContent{Text: "one"}},
				}, nil
			},
		},
		AccessFor: tools.InternalAccess,
	}
	session, err := New(ctx, Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{tool},
		Store: store,
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			_ llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			modelCalls++
			if modelCalls == 1 {
				message := llm.NewAssistantMessage(model)
				message.StopReason = llm.StopReasonToolUse
				message.Content = []llm.AssistantContent{
					&llm.ToolCall{ID: "call-1", Name: "echo", Arguments: map[string]any{"text": "one"}},
				}
				return finalEvents(llm.EventDone, &message), nil
			}
			return assistantEvents(model, "done"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := session.Prompt(ctx, "use the tool"); err != nil {
		t.Fatal(err)
	}
	if bodyErr != nil {
		t.Fatal(bodyErr)
	}
	if toolCalls != 1 || modelCalls != 2 {
		t.Fatalf("tool executions = %d, model requests = %d, want 1 and 2", toolCalls, modelCalls)
	}
}

func TestSessionCheckpointsCompleteToolBatchBeforeNextModelRequest(t *testing.T) {
	ctx := context.Background()
	store := &checkpointStore{}
	call := 0
	var checkpointErr error

	tool := tools.Tool{
		AgentTool: agent.AgentTool{
			Definition: llm.MustTool[checkpointToolArgs]("echo", "echo text"),
			Execute: func(
				_ context.Context,
				_ string,
				args json.RawMessage,
				_ func(agent.ToolProgress),
			) (agent.ToolResult, error) {
				var parsed checkpointToolArgs
				if err := json.Unmarshal(args, &parsed); err != nil {
					return agent.ToolResult{}, err
				}
				if parsed.Text == "one" {
					exitCode := 17
					return agent.ToolResult{
						Content: []llm.ToolResultContent{&llm.TextContent{Text: "first failed"}},
						Outcome: agent.ToolOutcome{
							Status:    agent.ToolOutcomeFailed,
							ErrorCode: "command_exit_nonzero",
							ExitCode:  &exitCode,
							Data: tools.PreviewRequest{
								Path:         "/workspace/one.html",
								RelativePath: "one.html",
								Title:        "First preview",
							},
						},
					}, nil
				}
				return agent.ToolResult{
					Content: []llm.ToolResultContent{&llm.TextContent{Text: "second timed out"}},
					Outcome: agent.ToolOutcome{
						Status:    agent.ToolOutcomeTimeout,
						ErrorCode: "tool_timeout",
						Data: tools.FileChange{
							Path: "two.txt", Kind: tools.ChangeUpdate, Additions: 2, Deletions: 1,
						},
					},
				}, nil
			},
		},
		AccessFor: tools.InternalAccess,
	}
	session, err := New(ctx, Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{tool},
		Store: store,
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			_ llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			defer func() { call++ }()
			switch call {
			case 0:
				message := llm.NewAssistantMessage(model)
				message.StopReason = llm.StopReasonToolUse
				message.Content = []llm.AssistantContent{
					&llm.ToolCall{ID: "call-1", Name: "echo", Arguments: map[string]any{"text": "one"}},
					&llm.ToolCall{ID: "call-2", Name: "echo", Arguments: map[string]any{"text": "two"}},
				}
				return finalEvents(llm.EventDone, &message), nil
			case 1:
				entries, _, _ := store.snapshot()
				if err := validateToolCheckpoint(entries); err != nil {
					checkpointErr = err
				}
				return assistantEvents(model, "done"), nil
			default:
				return nil, fmt.Errorf("unexpected model request %d", call+1)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := session.Prompt(ctx, "use both tools"); err != nil {
		t.Fatal(err)
	}
	if checkpointErr != nil {
		t.Fatal(checkpointErr)
	}
	if call != 2 {
		t.Fatalf("model requests = %d, want 2", call)
	}

	entries, batches, _ := store.snapshot()
	if len(entries) != 11 {
		t.Fatalf(
			"durable entries = %d, want context, user, assistant, two tool intents, two result/outcome pairs, final assistant, run",
			len(entries),
		)
	}
	if got := batchSizes(batches); !slices.Equal(got, []int{2, 2, 1, 4, 2}) {
		t.Fatalf("append batch sizes = %v, want [2 2 1 4 2]", got)
	}

	restored, err := New(ctx, Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		Store: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	toolResults := make(map[string]agent.ToolOutcome)
	for _, item := range restored.History() {
		if item.Type == HistoryToolResult {
			toolResults[item.ToolCallID] = item.ToolOutcome
		}
	}
	first := toolResults["call-1"]
	if first.Status != agent.ToolOutcomeFailed || first.ErrorCode != "command_exit_nonzero" ||
		first.ExitCode == nil || *first.ExitCode != 17 {
		t.Fatalf("restored call-1 outcome = %#v", first)
	}
	if preview, ok := first.Data.(tools.PreviewRequest); !ok || preview.RelativePath != "one.html" {
		t.Fatalf("restored call-1 data = %#v", first.Data)
	}
	second := toolResults["call-2"]
	if second.Status != agent.ToolOutcomeTimeout || second.ErrorCode != "tool_timeout" {
		t.Fatalf("restored call-2 outcome = %#v", second)
	}
	if change, ok := second.Data.(tools.FileChange); !ok || change.Path != "two.txt" || change.Additions != 2 {
		t.Fatalf("restored call-2 data = %#v", second.Data)
	}
}

func TestSessionToolCheckpointFailureDoesNotExecuteToolOrContinueModel(t *testing.T) {
	ctx := context.Background()
	storeErr := errors.New("tool checkpoint unavailable")
	store := &checkpointStore{}
	recorder := &memoryRecorder{}
	modelCalls := 0
	toolCalls := 0

	tool := tools.Tool{
		AgentTool: agent.AgentTool{
			Definition: llm.MustTool[checkpointToolArgs]("echo", "echo text"),
			Execute: func(
				context.Context,
				string,
				json.RawMessage,
				func(agent.ToolProgress),
			) (agent.ToolResult, error) {
				toolCalls++
				return agent.ToolResult{
					Content: []llm.ToolResultContent{&llm.TextContent{Text: "must not execute"}},
				}, nil
			},
		},
		AccessFor: tools.InternalAccess,
	}
	session, err := New(ctx, Options{
		SessionID: "session-tool-checkpoint-failure",
		Recorder:  recorder,
		Model:     llm.Model{Provider: "test", ID: "model"},
		Tools:     []tools.Tool{tool},
		Store:     store,
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			_ llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			modelCalls++
			if modelCalls > 1 {
				return nil, errors.New("model must not continue after a tool checkpoint failure")
			}
			store.failNext(storeErr)
			message := llm.NewAssistantMessage(model)
			message.StopReason = llm.StopReasonToolUse
			message.Content = []llm.AssistantContent{
				&llm.ToolCall{ID: "call-1", Name: "echo", Arguments: map[string]any{"text": "one"}},
				&llm.ToolCall{ID: "call-2", Name: "echo", Arguments: map[string]any{"text": "two"}},
			}
			return finalEvents(llm.EventDone, &message), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = session.Prompt(ctx, "use the tool")
	if !errors.Is(err, storeErr) {
		t.Fatalf("Prompt error = %v, want tool checkpoint error", err)
	}
	if toolCalls != 0 {
		t.Fatalf("tool executions = %d, want 0", toolCalls)
	}
	if modelCalls != 1 {
		t.Fatalf("model requests = %d, want 1", modelCalls)
	}
	entries, batches, appendCalls := store.snapshot()
	if appendCalls != 3 {
		t.Fatalf("store append attempts = %d, want provider checkpoint, failed tool checkpoint, and final flush", appendCalls)
	}
	if got := batchSizes(batches); !slices.Equal(got, []int{2, 6}) {
		t.Fatalf("successful append batch sizes = %v, want [2 6]", got)
	}
	for _, entry := range entries {
		if entry.Type == transcript.ToolCallEntry {
			t.Fatalf("failed checkpoint persisted tool intent %#v", entry)
		}
	}
	var checkpointFailure *observability.Event
	for _, event := range recorder.snapshot() {
		if event.Name == observability.CheckpointFailed && event.Reason == "tool_dispatch" {
			copy := event
			checkpointFailure = &copy
			break
		}
	}
	if checkpointFailure == nil || checkpointFailure.ToolCallID != "call-1" ||
		checkpointFailure.ToolName != "echo" || checkpointFailure.ErrorCode != "checkpoint_persist_failed" {
		t.Fatalf("tool checkpoint failure event = %#v", checkpointFailure)
	}
	for _, event := range recorder.snapshot() {
		if event.Name == observability.CheckpointCompleted && event.ToolCallID == "call-2" {
			t.Fatalf("later tool checkpoint continued after failure: %#v", event)
		}
	}
}

func TestSessionCheckpointsFollowUpBeforeNextModelRequest(t *testing.T) {
	ctx := context.Background()
	store := &checkpointStore{}
	call := 0
	var checkpointErr error

	session, err := New(ctx, Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		Store: store,
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			_ llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			defer func() { call++ }()
			if call == 1 {
				entries, _, _ := store.snapshot()
				if len(entries) != 4 {
					checkpointErr = fmt.Errorf(
						"entries before follow-up request = %d, want context, user, assistant, follow-up",
						len(entries),
					)
				} else {
					_, firstUser := llmEntry(entries[1]).(*llm.UserMessage)
					_, assistant := llmEntry(entries[2]).(*llm.AssistantMessage)
					_, followUp := llmEntry(entries[3]).(*llm.UserMessage)
					if entries[0].Type != transcript.ContextEntry || !firstUser || !assistant || !followUp {
						checkpointErr = fmt.Errorf(
							"follow-up checkpoint types = %q, %T, %T, %T",
							entries[0].Type,
							llmEntry(entries[1]),
							llmEntry(entries[2]),
							llmEntry(entries[3]),
						)
					}
				}
			}
			return assistantEvents(model, fmt.Sprintf("answer %d", call+1)), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	session.FollowUp("one more thing")

	if err := session.Prompt(ctx, "question"); err != nil {
		t.Fatal(err)
	}
	if checkpointErr != nil {
		t.Fatal(checkpointErr)
	}
	if call != 2 {
		t.Fatalf("model requests = %d, want 2", call)
	}
}

func TestSessionPersistenceFailureDoesNotReachOrRetryModel(t *testing.T) {
	ctx := context.Background()
	storeErr := errors.New("disk temporarily unavailable")
	store := &checkpointStore{failErr: storeErr}
	modelCalls := 0

	session, err := New(ctx, Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		Store: store,
		StreamFn: func(
			context.Context,
			llm.Model,
			llm.Context,
			llm.StreamOptions,
		) (<-chan llm.Event, error) {
			modelCalls++
			return nil, errors.New("model must not be called")
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = session.Prompt(ctx, "question")
	if !errors.Is(err, storeErr) {
		t.Fatalf("Prompt error = %v, want persistence error", err)
	}
	if modelCalls != 0 {
		t.Fatalf("model requests = %d, want 0", modelCalls)
	}
	_, _, appendCalls := store.snapshot()
	if appendCalls != 2 {
		t.Fatalf("store append attempts = %d, want checkpoint and final flush only", appendCalls)
	}
	messages := session.Snapshot().Messages
	if len(messages) != 1 {
		t.Fatalf("active messages = %d, want accepted user only", len(messages))
	}
	if _, ok := agent.ToLLM(messages[0]); !ok {
		t.Fatalf("active message = %T, want standard user message", messages[0])
	}
}

func TestSessionRetriesFinalFlushAfterTransientCheckpointFailure(t *testing.T) {
	ctx := context.Background()
	storeErr := errors.New("disk temporarily unavailable")
	store := &checkpointStore{failErr: storeErr, failOnce: true}
	modelCalls := 0

	session, err := New(ctx, Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		Store: store,
		StreamFn: func(
			context.Context,
			llm.Model,
			llm.Context,
			llm.StreamOptions,
		) (<-chan llm.Event, error) {
			modelCalls++
			return nil, errors.New("model must not be called")
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = session.Prompt(ctx, "question")
	if !errors.Is(err, storeErr) {
		t.Fatalf("Prompt error = %v, want original checkpoint error", err)
	}
	if modelCalls != 0 {
		t.Fatalf("model requests = %d, want 0", modelCalls)
	}
	entries, batches, appendCalls := store.snapshot()
	if appendCalls != 2 {
		t.Fatalf("store append attempts = %d, want checkpoint and final flush", appendCalls)
	}
	if len(batches) != 1 || len(batches[0]) != 2 {
		t.Fatalf("successful append batch sizes = %v, want [2]", batchSizes(batches))
	}
	if len(entries) != 2 {
		t.Fatalf("durable entries = %d, want user and run metadata", len(entries))
	}
	if _, ok := llmEntry(entries[0]).(*llm.UserMessage); !ok {
		t.Fatalf("durable message = %T, want user", llmEntry(entries[0]))
	}
	if entries[1].Type != transcript.RunEntry {
		t.Fatalf("last entry type = %q, want run", entries[1].Type)
	}
}

func TestSessionRetryDoesNotPersistFailedAssistantOrDuplicatePrompt(t *testing.T) {
	ctx := context.Background()
	store := &checkpointStore{}
	call := 0

	session, err := New(ctx, Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		Store: store,
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			_ llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			call++
			if call == 1 {
				message := llm.NewAssistantMessage(model)
				message.StopReason = llm.StopReasonError
				message.ErrorMessage = "temporarily unavailable"
				return finalEvents(llm.EventError, &message), nil
			}
			return assistantEvents(model, "recovered"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := session.Prompt(ctx, "question"); err != nil {
		t.Fatal(err)
	}
	if call != 2 {
		t.Fatalf("model requests = %d, want 2", call)
	}
	entries, batches, _ := store.snapshot()
	if len(entries) != 4 {
		t.Fatalf("durable entries = %d, want context, user, successful assistant, run", len(entries))
	}
	if len(batches) != 2 || len(batches[0]) != 2 || len(batches[1]) != 2 {
		t.Fatalf("append batch sizes = %v, want [2 2]", batchSizes(batches))
	}
	assistant, ok := llmEntry(entries[2]).(*llm.AssistantMessage)
	if !ok || assistant.StopReason != llm.StopReasonStop || assistant.Text() != "recovered" {
		t.Fatalf("persisted assistant = %#v, want successful retry only", assistant)
	}
}

func validateToolCheckpoint(entries []transcript.Entry) error {
	// The base context is checkpointed once, ahead of the first user message.
	if len(entries) != 9 {
		return fmt.Errorf("entries before second model request = %d, want 9", len(entries))
	}
	if entries[0].Type != transcript.ContextEntry {
		return fmt.Errorf("checkpoint[0] = %q, want context", entries[0].Type)
	}
	if _, ok := llmEntry(entries[1]).(*llm.UserMessage); !ok {
		return fmt.Errorf("checkpoint[1] = %T, want user", llmEntry(entries[1]))
	}
	assistant, ok := llmEntry(entries[2]).(*llm.AssistantMessage)
	if !ok || len(assistant.ToolCalls()) != 2 {
		return fmt.Errorf("checkpoint[2] = %#v, want assistant with two tool calls", assistant)
	}
	for index, callID := range []string{"call-1", "call-2"} {
		intentIndex := 3 + index
		intent := entries[intentIndex]
		if intent.Type != transcript.ToolCallEntry || intent.ToolCall == nil ||
			intent.ToolCall.ToolCallID != callID {
			return fmt.Errorf("checkpoint[%d] = %#v, want tool intent %s", intentIndex, intent, callID)
		}
		messageIndex := 5 + index*2
		outcomeIndex := messageIndex + 1
		result, ok := llmEntry(entries[messageIndex]).(*llm.ToolResultMessage)
		if !ok || result.ToolCallID != callID {
			return fmt.Errorf("checkpoint[%d] = %#v, want tool result %s", messageIndex, result, callID)
		}
		outcome := entries[outcomeIndex]
		if outcome.Type != transcript.ToolOutcomeEntry || outcome.ToolOutcome == nil ||
			outcome.ToolOutcome.ToolCallID != callID {
			return fmt.Errorf("checkpoint[%d] = %#v, want tool outcome %s", outcomeIndex, outcome, callID)
		}
	}
	return nil
}

func llmEntry(entry transcript.Entry) llm.Message {
	if entry.Type != transcript.MessageEntry {
		return nil
	}
	message, _ := agent.ToLLM(entry.Message)
	return message
}

func assistantEvents(model llm.Model, text string) <-chan llm.Event {
	message := llm.NewAssistantMessage(model)
	message.Content = []llm.AssistantContent{&llm.TextContent{Text: text}}
	message.StopReason = llm.StopReasonStop
	return finalEvents(llm.EventDone, &message)
}

func finalEvents(eventType llm.EventType, message *llm.AssistantMessage) <-chan llm.Event {
	events := make(chan llm.Event, 1)
	events <- llm.Event{Type: eventType, Message: message}
	close(events)
	return events
}

func batchSizes(batches [][]transcript.Entry) []int {
	sizes := make([]int, len(batches))
	for index := range batches {
		sizes[index] = len(batches[index])
	}
	return sizes
}
