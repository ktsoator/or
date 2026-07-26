package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

type fakeSupervisor struct {
	initialize func(context.Context, InitializeRequest) (InitializeResponse, error)
	listTools  func(context.Context, ListToolsRequest) (ListToolsResponse, error)
	execute    func(context.Context, ExecuteRequest, func(ProgressNotification)) (Result, error)
	cancel     func(context.Context, CancelRequest) (CancelResponse, error)
}

func (f *fakeSupervisor) Initialize(ctx context.Context, request InitializeRequest) (InitializeResponse, error) {
	return f.initialize(ctx, request)
}

func (f *fakeSupervisor) ListTools(ctx context.Context, request ListToolsRequest) (ListToolsResponse, error) {
	return f.listTools(ctx, request)
}

func (f *fakeSupervisor) Execute(
	ctx context.Context,
	request ExecuteRequest,
	onProgress func(ProgressNotification),
) (Result, error) {
	return f.execute(ctx, request, onProgress)
}

func (f *fakeSupervisor) Cancel(ctx context.Context, request CancelRequest) (CancelResponse, error) {
	if f.cancel == nil {
		return CancelResponse{}, nil
	}
	return f.cancel(ctx, request)
}

func validSupervisor() *fakeSupervisor {
	return &fakeSupervisor{
		initialize: func(_ context.Context, request InitializeRequest) (InitializeResponse, error) {
			return InitializeResponse{
				ProtocolVersion: request.ProtocolVersion,
				Plugin:          Manifest{ID: "example.tools", Name: "Example", Version: "1.2.3"},
			}, nil
		},
		listTools: func(context.Context, ListToolsRequest) (ListToolsResponse, error) {
			return ListToolsResponse{Tools: []ToolDescriptor{{
				Name:          "query_database",
				Description:   "Query the development database",
				InputSchema:   json.RawMessage(`{"type":"object","properties":{"sql":{"type":"string"}},"required":["sql"]}`),
				Label:         "Database",
				Guidelines:    []string{" Use read-only queries. ", ""},
				ExecutionMode: ExecutionSequential,
			}}}, nil
		},
		execute: func(_ context.Context, request ExecuteRequest, onProgress func(ProgressNotification)) (Result, error) {
			onProgress(ProgressNotification{
				CallID:  request.CallID,
				Content: []Content{{Type: ContentText, Text: "connecting"}},
				Data:    json.RawMessage(`{"phase":"connect","attempt":1}`),
			})
			exitCode := 0
			return Result{
				CallID:  request.CallID,
				Content: []Content{{Type: ContentText, Text: "2 rows"}},
				Outcome: Outcome{
					Status:   OutcomeSuccess,
					ExitCode: &exitCode,
					Data:     json.RawMessage(`{"rows":2}`),
				},
			}, nil
		},
	}
}

func TestLoadAdaptsPluginToolsIntoCodingCapabilities(t *testing.T) {
	supervisor := validSupervisor()
	var initialized InitializeRequest
	var executed ExecuteRequest
	originalInitialize := supervisor.initialize
	originalExecute := supervisor.execute
	supervisor.initialize = func(ctx context.Context, request InitializeRequest) (InitializeResponse, error) {
		initialized = request
		return originalInitialize(ctx, request)
	}
	supervisor.execute = func(ctx context.Context, request ExecuteRequest, onProgress func(ProgressNotification)) (Result, error) {
		executed = request
		return originalExecute(ctx, request, onProgress)
	}

	definition, err := Load(context.Background(), supervisor, HostInfo{Name: "Coding", Version: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if initialized.ProtocolVersion != ProtocolVersion || initialized.Host.Name != "Coding" {
		t.Fatalf("initialize request = %#v", initialized)
	}
	if definition.Manifest.ID != "example.tools" || definition.Manifest.Version != "1.2.3" {
		t.Fatalf("capability manifest = %#v", definition.Manifest)
	}
	if len(definition.Tools) != 1 {
		t.Fatalf("tool contributions = %d, want 1", len(definition.Tools))
	}
	tool := definition.Tools[0].Tool
	if tool.Name() != "query_database" || tool.Label != "Database" ||
		tool.ExecutionMode != agent.ExecutionSequential {
		t.Fatalf("adapted tool = %#v", tool)
	}
	if !reflect.DeepEqual(tool.Guidelines, []string{"Use read-only queries."}) {
		t.Fatalf("guidelines = %#v", tool.Guidelines)
	}
	if accesses := tool.Accesses(map[string]any{"sql": "select 1"}); accesses != nil {
		t.Fatalf("plugin tool accesses = %#v, want conservative unknown access", accesses)
	}

	var progress []agent.ToolProgress
	result, err := tool.Execute(
		context.Background(),
		"call-1",
		json.RawMessage(`{"sql":"select 1"}`),
		func(update agent.ToolProgress) { progress = append(progress, update) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if executed.CallID != "call-1" || executed.Tool != "query_database" || string(executed.Arguments) != `{"sql":"select 1"}` {
		t.Fatalf("execute request = %#v", executed)
	}
	if len(progress) != 1 {
		t.Fatalf("progress count = %d, want 1", len(progress))
	}
	progressText, ok := progress[0].Content[0].(*llm.TextContent)
	if !ok || progressText.Text != "connecting" {
		t.Fatalf("progress content = %#v", progress[0].Content)
	}
	progressData := progress[0].Data.(map[string]any)
	if progressData["phase"] != "connect" || progressData["attempt"] != json.Number("1") {
		t.Fatalf("progress data = %#v", progressData)
	}
	resultText, ok := result.Content[0].(*llm.TextContent)
	if !ok || resultText.Text != "2 rows" {
		t.Fatalf("result content = %#v", result.Content)
	}
	if result.Outcome.Status != agent.ToolOutcomeSuccess || result.Outcome.ExitCode == nil || *result.Outcome.ExitCode != 0 {
		t.Fatalf("result outcome = %#v", result.Outcome)
	}
	if rows := result.Outcome.Data.(map[string]any)["rows"]; rows != json.Number("2") {
		t.Fatalf("result rows = %#v", rows)
	}
}

func TestAdaptedToolPassesCancellationToSupervisor(t *testing.T) {
	supervisor := validSupervisor()
	started := make(chan struct{})
	supervisor.execute = func(ctx context.Context, _ ExecuteRequest, _ func(ProgressNotification)) (Result, error) {
		close(started)
		<-ctx.Done()
		return Result{}, ctx.Err()
	}
	definition, err := Load(context.Background(), supervisor, HostInfo{Name: "Coding"})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, executeErr := definition.Tools[0].Tool.Execute(ctx, "call-cancel", json.RawMessage(`{"sql":"select 1"}`), nil)
		done <- executeErr
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("execute error = %v, want context cancellation", err)
	}
}

func TestAdaptedToolRejectsProtocolViolations(t *testing.T) {
	tests := []struct {
		name     string
		execute  func(ExecuteRequest, func(ProgressNotification)) Result
		wantCode string
	}{
		{
			name: "mismatched progress call id",
			execute: func(request ExecuteRequest, onProgress func(ProgressNotification)) Result {
				onProgress(ProgressNotification{CallID: "other"})
				return Result{CallID: request.CallID, Outcome: Outcome{Status: OutcomeSuccess}}
			},
			wantCode: "plugin_progress_invalid",
		},
		{
			name: "mismatched result call id",
			execute: func(ExecuteRequest, func(ProgressNotification)) Result {
				return Result{CallID: "other", Outcome: Outcome{Status: OutcomeSuccess}}
			},
			wantCode: "plugin_result_invalid",
		},
		{
			name: "failure without error code",
			execute: func(request ExecuteRequest, _ func(ProgressNotification)) Result {
				return Result{CallID: request.CallID, Outcome: Outcome{Status: OutcomeFailed}}
			},
			wantCode: "plugin_result_invalid",
		},
		{
			name: "multiple result data values",
			execute: func(request ExecuteRequest, _ func(ProgressNotification)) Result {
				return Result{
					CallID: request.CallID,
					Outcome: Outcome{
						Status: OutcomeSuccess,
						Data:   json.RawMessage(`{"first":1} {"second":2}`),
					},
				}
			},
			wantCode: "plugin_result_invalid",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			supervisor := validSupervisor()
			supervisor.execute = func(_ context.Context, request ExecuteRequest, onProgress func(ProgressNotification)) (Result, error) {
				return test.execute(request, onProgress), nil
			}
			definition, err := Load(context.Background(), supervisor, HostInfo{Name: "Coding"})
			if err != nil {
				t.Fatal(err)
			}
			result, err := definition.Tools[0].Tool.Execute(
				context.Background(), "call-1", json.RawMessage(`{"sql":"select 1"}`), nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			if result.Outcome.Status != agent.ToolOutcomeFailed || result.Outcome.ErrorCode != test.wantCode {
				t.Fatalf("outcome = %#v", result.Outcome)
			}
		})
	}
}

func TestAdaptedToolIgnoresProgressAfterExecuteReturns(t *testing.T) {
	supervisor := validSupervisor()
	var late func(ProgressNotification)
	supervisor.execute = func(_ context.Context, request ExecuteRequest, onProgress func(ProgressNotification)) (Result, error) {
		late = onProgress
		return Result{CallID: request.CallID, Outcome: Outcome{Status: OutcomeSuccess}}, nil
	}
	definition, err := Load(context.Background(), supervisor, HostInfo{Name: "Coding"})
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	updates := 0
	_, err = definition.Tools[0].Tool.Execute(
		context.Background(), "call-1", json.RawMessage(`{"sql":"select 1"}`),
		func(agent.ToolProgress) {
			mu.Lock()
			updates++
			mu.Unlock()
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	late(ProgressNotification{CallID: "call-1"})
	mu.Lock()
	defer mu.Unlock()
	if updates != 0 {
		t.Fatalf("late progress updates = %d, want 0", updates)
	}
}

func TestLoadRejectsInvalidPluginDescriptions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fakeSupervisor)
	}{
		{
			name: "protocol version",
			mutate: func(supervisor *fakeSupervisor) {
				supervisor.initialize = func(context.Context, InitializeRequest) (InitializeResponse, error) {
					return InitializeResponse{ProtocolVersion: ProtocolVersion + 1, Plugin: Manifest{ID: "example", Version: "1"}}, nil
				}
			},
		},
		{
			name: "missing plugin version",
			mutate: func(supervisor *fakeSupervisor) {
				supervisor.initialize = func(context.Context, InitializeRequest) (InitializeResponse, error) {
					return InitializeResponse{ProtocolVersion: ProtocolVersion, Plugin: Manifest{ID: "example"}}, nil
				}
			},
		},
		{
			name: "duplicate tool",
			mutate: func(supervisor *fakeSupervisor) {
				original := supervisor.listTools
				supervisor.listTools = func(ctx context.Context, request ListToolsRequest) (ListToolsResponse, error) {
					listed, err := original(ctx, request)
					listed.Tools = append(listed.Tools, listed.Tools[0])
					return listed, err
				}
			},
		},
		{
			name: "non-object schema",
			mutate: func(supervisor *fakeSupervisor) {
				supervisor.listTools = func(context.Context, ListToolsRequest) (ListToolsResponse, error) {
					return ListToolsResponse{Tools: []ToolDescriptor{{
						Name: "bad", InputSchema: json.RawMessage(`{"type":"string"}`),
					}}}, nil
				}
			},
		},
		{
			name: "execution mode",
			mutate: func(supervisor *fakeSupervisor) {
				supervisor.listTools = func(context.Context, ListToolsRequest) (ListToolsResponse, error) {
					return ListToolsResponse{Tools: []ToolDescriptor{{
						Name: "bad", InputSchema: json.RawMessage(`{"type":"object"}`), ExecutionMode: "sometimes",
					}}}, nil
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			supervisor := validSupervisor()
			test.mutate(supervisor)
			if _, err := Load(context.Background(), supervisor, HostInfo{Name: "Coding"}); err == nil {
				t.Fatal("Load succeeded for invalid plugin description")
			}
		})
	}
}
