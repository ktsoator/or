package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/ktsoator/or/agent"
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
				_ func(agent.ToolResult),
			) (agent.ToolResult, error) {
				var parsed checkpointToolArgs
				if err := json.Unmarshal(args, &parsed); err != nil {
					return agent.ToolResult{}, err
				}
				return agent.ToolResult{
					Content: []llm.ToolResultContent{&llm.TextContent{Text: parsed.Text}},
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
	if len(entries) != 7 {
		t.Fatalf(
			"durable entries = %d, want context, user, tool call, two results, final assistant, run",
			len(entries),
		)
	}
	if len(batches) != 3 || len(batches[0]) != 2 || len(batches[1]) != 3 || len(batches[2]) != 2 {
		t.Fatalf("append batch sizes = %v, want [2 3 2]", batchSizes(batches))
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
	if len(entries) != 5 {
		return fmt.Errorf("entries before second model request = %d, want 5", len(entries))
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
	for index := 3; index < 5; index++ {
		if _, ok := llmEntry(entries[index]).(*llm.ToolResultMessage); !ok {
			return fmt.Errorf("checkpoint[%d] = %T, want tool result", index, llmEntry(entries[index]))
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
