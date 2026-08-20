package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
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
			projection, err := transcript.ProjectSession(entries)
			if err != nil {
				checkpointErr = err
			} else if len(projection.Contexts) != 1 || len(projection.Messages) != 1 {
				checkpointErr = fmt.Errorf("checkpoint projection = %#v", projection)
			} else if projection.Contexts[0].StepID == "" {
				checkpointErr = errors.New("base context has no owning step")
			} else if _, ok := agent.ToLLM(projection.Messages[0].Message); !ok {
				checkpointErr = errors.New("durable user message is not model-facing")
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
	entries = withoutLifecycle(entries)
	if len(entries) != 3 {
		t.Fatalf("durable entries = %d, want user, context, assistant", len(entries))
	}
	if got := payloadBatchSizes(batches); !slices.Equal(got, []int{2, 1}) {
		t.Fatalf("append batch sizes = %v, want [2 1]", got)
	}
}

func TestSessionProviderDispatchUsesCheckpointedRequestBoundary(t *testing.T) {
	ctx := context.Background()
	store := &checkpointStore{}
	temperature := 0.25
	maxRetries := 2
	model := llm.Model{
		Provider: "test-provider",
		ID:       "test-model",
		Protocol: llm.ProtocolOpenAIResponses,
	}
	tool := tools.Tool{
		AgentTool: agent.AgentTool{
			Definition: llm.MustTool[checkpointToolArgs]("echo", "echo text"),
			Execute: func(
				context.Context,
				string,
				json.RawMessage,
				func(agent.ToolProgress),
			) (agent.ToolResult, error) {
				return agent.ToolResult{}, nil
			},
		},
		AccessFor: tools.InternalAccess,
	}
	baseOptions := llm.StreamOptions{
		APIKey:      "stale-key",
		BaseURL:     "https://provider.example/v1",
		Temperature: &temperature,
		MaxTokens:   321,
		Headers:     map[string]string{"X-Feature": "request-baseline"},
		Reasoning:   llm.ModelThinkingLow,
		ProtocolOptions: &llm.OpenAIResponsesStreamOptions{
			ThinkingDisplay: llm.ThinkingDisplayOmitted,
		},
		MaxRetries: &maxRetries,
		Timeout:    3 * time.Second,
	}

	providerCalls := 0
	keyProvider := ""
	var gotModel llm.Model
	var gotInput llm.Context
	var gotOptions llm.StreamOptions
	var dispatchEntries []transcript.Entry
	session, err := New(ctx, Options{
		Model:         model,
		ThinkingLevel: llm.ModelThinkingHigh,
		Cwd:           t.TempDir(),
		Tools:         []tools.Tool{tool},
		Store:         store,
		Instructions:  "REQUEST HEADER BASELINE",
		StreamOptions: baseOptions,
		GetAPIKey: func(provider string) string {
			keyProvider = provider
			return "current-key"
		},
		StreamFn: func(
			_ context.Context,
			streamModel llm.Model,
			input llm.Context,
			options llm.StreamOptions,
		) (<-chan llm.Event, error) {
			providerCalls++
			gotModel = streamModel
			gotInput = input
			gotOptions = options
			dispatchEntries, _, _ = store.snapshot()
			return assistantEvents(streamModel, "answer"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := session.Prompt(ctx, "question"); err != nil {
		t.Fatal(err)
	}
	if providerCalls != 1 {
		t.Fatalf("provider calls = %d, want 1", providerCalls)
	}
	if keyProvider != model.Provider {
		t.Fatalf("API key provider = %q, want %q", keyProvider, model.Provider)
	}
	if len(dispatchEntries) == 0 {
		t.Fatal("provider reached before transcript checkpoint")
	}
	dispatchSeq := dispatchEntries[len(dispatchEntries)-1].Seq
	wantTypes := []transcript.EntryType{
		transcript.RunStartEntry,
		transcript.TurnStartEntry,
		transcript.MessageEntry,
		transcript.StepStartEntry,
		transcript.ContextEntry,
	}
	if len(dispatchEntries) != len(wantTypes) {
		t.Fatalf("dispatch checkpoint entries = %d, want %d", len(dispatchEntries), len(wantTypes))
	}
	for index, entry := range dispatchEntries {
		if entry.Seq != int64(index) || entry.Type != wantTypes[index] {
			t.Fatalf(
				"dispatch checkpoint[%d] = seq %d type %q, want seq %d type %q",
				index, entry.Seq, entry.Type, index, wantTypes[index],
			)
		}
	}

	projection, err := transcript.ProjectSession(dispatchEntries)
	if err != nil {
		t.Fatal(err)
	}
	if projection.AsOfSeq != dispatchSeq || projection.Open.RunID == "" ||
		projection.Open.TurnID == "" || projection.Open.StepID == "" {
		t.Fatalf(
			"dispatch projection = seq %d open %#v, want seq %d and open step",
			projection.AsOfSeq, projection.Open, dispatchSeq,
		)
	}

	canonical, err := transcript.BuildContext(dispatchEntries)
	if err != nil {
		t.Fatal(err)
	}
	wantMessages := make([]llm.Message, 0, len(projection.Contexts)+len(canonical))
	for _, projected := range projection.Contexts {
		if projected.Attachment.Placement == "prefix" {
			wantMessages = append(wantMessages, llm.UserText(projected.Attachment.Rendered))
		}
	}
	for _, message := range canonical {
		projected, ok := agent.ToLLM(message)
		if !ok {
			t.Fatalf("canonical message %T is not model-facing", message)
		}
		wantMessages = append(wantMessages, projected)
	}
	for _, projected := range projection.Contexts {
		if projected.Attachment.Placement == "after-current" {
			wantMessages = append(wantMessages, llm.UserText(projected.Attachment.Rendered))
		}
	}
	if !reflect.DeepEqual(gotInput.Messages, wantMessages) {
		t.Fatalf(
			"provider messages differ from checkpoint projection\ngot:  %#v\nwant: %#v",
			gotInput.Messages, wantMessages,
		)
	}

	if gotModel.Provider != model.Provider || gotModel.ID != model.ID ||
		gotModel.Protocol != model.Protocol {
		t.Fatalf("provider model = %#v, want %#v", gotModel, model)
	}
	if !strings.Contains(gotInput.SystemPrompt, "REQUEST HEADER BASELINE") {
		t.Fatal("provider system prompt omitted configured instructions")
	}
	if len(gotInput.Tools) != 1 || !reflect.DeepEqual(gotInput.Tools[0], tool.Definition) {
		t.Fatalf("provider tools = %#v, want echo definition", gotInput.Tools)
	}
	if gotOptions.APIKey != "current-key" || gotOptions.BaseURL != baseOptions.BaseURL ||
		gotOptions.Temperature == nil || *gotOptions.Temperature != temperature ||
		gotOptions.MaxTokens != baseOptions.MaxTokens ||
		gotOptions.Headers["X-Feature"] != "request-baseline" ||
		gotOptions.Reasoning != llm.ModelThinkingHigh ||
		gotOptions.ProtocolOptions != baseOptions.ProtocolOptions ||
		gotOptions.MaxRetries != baseOptions.MaxRetries || gotOptions.Timeout != baseOptions.Timeout {
		t.Fatalf("effective stream options = %#v", gotOptions)
	}
	entries, _, _ := store.snapshot()
	if finalSeq := entries[len(entries)-1].Seq; finalSeq <= dispatchSeq {
		t.Fatalf("terminal transcript sequence = %d, want after dispatch boundary %d", finalSeq, dispatchSeq)
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
				entries = withoutLifecycle(entries)
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
	entries = withoutLifecycle(entries)
	if len(entries) != 10 {
		t.Fatalf(
			"durable entries = %d, want user, context, assistant, two tool intents, two result/outcome pairs, final assistant",
			len(entries),
		)
	}
	if got := payloadBatchSizes(batches); !slices.Equal(got, []int{2, 2, 1, 4, 1}) {
		t.Fatalf("append batch sizes = %v, want [2 2 1 4 1]", got)
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
	if got := payloadBatchSizes(batches); !slices.Equal(got, []int{2, 5}) {
		t.Fatalf("successful append batch sizes = %v, want [2 5]", got)
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
				entries = withoutLifecycle(entries)
				if len(entries) != 4 {
					checkpointErr = fmt.Errorf(
						"entries before follow-up request = %d, want context, user, assistant, follow-up",
						len(entries),
					)
				} else {
					_, firstUser := llmEntry(entries[0]).(*llm.UserMessage)
					_, assistant := llmEntry(entries[2]).(*llm.AssistantMessage)
					_, followUp := llmEntry(entries[3]).(*llm.UserMessage)
					if entries[1].Type != transcript.ContextEntry || !firstUser || !assistant || !followUp {
						checkpointErr = fmt.Errorf(
							"follow-up checkpoint types = %q, %T, %T, %T",
							entries[1].Type,
							llmEntry(entries[0]),
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
	for index, entry := range entries {
		if entry.Seq != int64(index) {
			t.Fatalf("durable entry %d sequence = %d", index, entry.Seq)
		}
	}
	entries = withoutLifecycle(entries)
	if appendCalls != 2 {
		t.Fatalf("store append attempts = %d, want checkpoint and final flush", appendCalls)
	}
	if got := payloadBatchSizes(batches); !slices.Equal(got, []int{1}) {
		t.Fatalf("successful append batch sizes = %v, want [1]", got)
	}
	if len(entries) != 1 {
		t.Fatalf("durable entries = %d, want user", len(entries))
	}
	if _, ok := llmEntry(entries[0]).(*llm.UserMessage); !ok {
		t.Fatalf("durable message = %T, want user", llmEntry(entries[0]))
	}
}

func TestSessionJournalRejectsInvalidBatchBeforeStoreAppend(t *testing.T) {
	ctx := context.Background()
	store := &checkpointStore{}
	journal, _, _, err := newSessionJournal(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	messages := []agent.AgentMessage{agent.UserMessage("question")}
	if err := journal.persistMessages(ctx, messages, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "has no open turn") {
		t.Fatalf("persistMessages() error = %v", err)
	}
	entries, _, appendCalls := store.snapshot()
	if appendCalls != 0 || len(entries) != 0 {
		t.Fatalf("invalid batch reached store: calls=%d entries=%#v", appendCalls, entries)
	}

	positioned := positionedLifecycle(
		0,
		transcript.NewRunStart("run-1"),
		transcript.NewTurnStart("run-1", "turn-1"),
	)
	if err := journal.persistMessages(ctx, messages, nil, positioned); err != nil {
		t.Fatalf("valid append after rejection: %v", err)
	}
	entries, _, appendCalls = store.snapshot()
	if appendCalls != 1 || len(entries) != 3 {
		t.Fatalf("valid batch = calls=%d entries=%#v", appendCalls, entries)
	}
	for index, entry := range entries {
		if entry.Seq != int64(index) {
			t.Fatalf("entry %d sequence = %d", index, entry.Seq)
		}
	}
}

func TestSessionJournalAdvancesProjectionOnlyAfterStoreSuccess(t *testing.T) {
	ctx := context.Background()
	storeErr := errors.New("projection checkpoint unavailable")
	store := &checkpointStore{}
	journal, _, _, err := newSessionJournal(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	messages := []agent.AgentMessage{agent.UserMessage("question")}
	positioned := positionedLifecycle(
		0,
		transcript.NewRunStart("run-1"),
		transcript.NewTurnStart("run-1", "turn-1"),
	)

	store.failNext(storeErr)
	if err := journal.persistMessages(ctx, messages, nil, positioned); !errors.Is(err, storeErr) {
		t.Fatalf("persistMessages() error = %v, want Store failure", err)
	}
	projection, persistedLen, err := journal.projectionSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	modelContext, err := journal.modelContextSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if projection.AsOfSeq != -1 || projection.AppliedEntries != 0 ||
		modelContext.AsOfSeq != -1 || modelContext.AppliedEntries != 0 ||
		len(modelContext.Messages) != 0 ||
		journal.validator.NextSeq() != 0 || persistedLen != 0 {
		t.Fatalf(
			"state after Store failure = projection %#v model context %#v validator %d persisted %d",
			projection,
			modelContext,
			journal.validator.NextSeq(),
			persistedLen,
		)
	}

	if err := journal.persistMessages(ctx, messages, nil, positioned); err != nil {
		t.Fatalf("retry persistMessages(): %v", err)
	}
	projection, persistedLen, err = journal.projectionSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	modelContext, err = journal.modelContextSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if projection.AsOfSeq != 2 || projection.AppliedEntries != 3 ||
		modelContext.AsOfSeq != 2 || modelContext.AppliedEntries != 3 ||
		len(modelContext.Messages) != 1 ||
		journal.validator.NextSeq() != 3 || persistedLen != 1 || len(projection.Messages) != 1 {
		t.Fatalf(
			"state after retry = projection %#v model context %#v validator %d persisted %d",
			projection,
			modelContext,
			journal.validator.NextSeq(),
			persistedLen,
		)
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
	wantLifecycle := []transcript.EntryType{
		transcript.RunStartEntry,
		transcript.TurnStartEntry,
		transcript.StepStartEntry,
		transcript.StepEndEntry,
		transcript.StepStartEntry,
		transcript.StepEndEntry,
		transcript.TurnEndEntry,
		transcript.RunEndEntry,
	}
	if got := lifecycleTypes(entries); !equalEntryTypes(got, wantLifecycle) {
		t.Fatalf("retry lifecycle = %v, want %v", got, wantLifecycle)
	}
	assertLifecycleIDs(t, entries, 1, 1, 2)
	entries = withoutLifecycle(entries)
	if len(entries) != 3 {
		t.Fatalf("durable entries = %d, want user, context, successful assistant", len(entries))
	}
	if got := payloadBatchSizes(batches); !slices.Equal(got, []int{2, 1}) {
		t.Fatalf("append batch sizes = %v, want [2 1]", got)
	}
	assistant, ok := llmEntry(entries[2]).(*llm.AssistantMessage)
	if !ok || assistant.StopReason != llm.StopReasonStop || assistant.Text() != "recovered" {
		t.Fatalf("persisted assistant = %#v, want successful retry only", assistant)
	}
}

func validateToolCheckpoint(entries []transcript.Entry) error {
	projection, err := transcript.ProjectSession(entries)
	if err != nil {
		return err
	}
	if len(projection.Runs) != 1 || len(projection.ToolCalls) != 2 || len(projection.Contexts) != 1 {
		return fmt.Errorf("tool checkpoint projection = %#v", projection)
	}
	for _, call := range projection.ToolCalls {
		if call.RunID == "" || call.TurnID == "" || call.StepID == "" ||
			call.DispatchEntryID == "" || call.ResultMessageEntryID == "" {
			return fmt.Errorf("incomplete projected tool call = %#v", call)
		}
	}
	entries = withoutLifecycle(entries)
	// The base context is checkpointed once inside the first model step.
	if len(entries) != 9 {
		return fmt.Errorf("entries before second model request = %d, want 9", len(entries))
	}
	if entries[1].Type != transcript.ContextEntry {
		return fmt.Errorf("checkpoint[1] = %q, want context", entries[1].Type)
	}
	if _, ok := llmEntry(entries[0]).(*llm.UserMessage); !ok {
		return fmt.Errorf("checkpoint[0] = %T, want user", llmEntry(entries[0]))
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

func withoutLifecycle(entries []transcript.Entry) []transcript.Entry {
	result := make([]transcript.Entry, 0, len(entries))
	for _, entry := range entries {
		switch entry.Type {
		case transcript.RunStartEntry, transcript.RunEndEntry,
			transcript.TurnStartEntry, transcript.TurnEndEntry,
			transcript.StepStartEntry, transcript.StepEndEntry:
			continue
		default:
			result = append(result, entry)
		}
	}
	return result
}

func payloadBatchSizes(batches [][]transcript.Entry) []int {
	sizes := make([]int, 0, len(batches))
	for _, batch := range batches {
		if size := len(withoutLifecycle(batch)); size > 0 {
			sizes = append(sizes, size)
		}
	}
	return sizes
}
