package provider

import (
	"context"
	"errors"
	"net"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/ktsoator/or/llm"
)

func TestConnectionTesterUsesDraftValuesWithoutPersisting(t *testing.T) {
	store := newTestStore(t, nil)
	before := store.Snapshot()
	var gotModel llm.Model
	var gotContext llm.Context
	var gotOptions llm.StreamOptions
	tester := NewConnectionTester(store, func(
		_ context.Context,
		model llm.Model,
		input llm.Context,
		options llm.StreamOptions,
	) (llm.AssistantMessage, error) {
		gotModel = model
		gotContext = input
		gotOptions = options
		return *llm.AssistantText("Hello!"), nil
	})

	result, err := tester.Test(context.Background(), ConnectionTestRequest{
		ProviderID:   "test-provider",
		ConnectionID: "draft-connection",
		KeyID:        "draft-key",
		BaseURL:      "https://draft.example.com/v1",
		APIKey:       "draft-secret",
		ModelID:      "test-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ConnectionTestSuccess || result.ModelID != "test-model" || result.ModelName != "Test Model" {
		t.Fatalf("result = %+v", result)
	}
	if result.RequestText != connectionTestPrompt || result.ResponseText != "Hello!" {
		t.Fatalf("exchange = request %q, response %q", result.RequestText, result.ResponseText)
	}
	if len(gotContext.Messages) != 1 {
		t.Fatalf("prompt context = %+v", gotContext)
	}
	user, ok := gotContext.Messages[0].(*llm.UserMessage)
	if !ok || len(user.Content) != 1 || user.Content[0].(*llm.TextContent).Text != result.RequestText {
		t.Fatalf("actual prompt does not match projected request: %+v", gotContext.Messages[0])
	}
	if gotModel.ID != "test-model" {
		t.Fatalf("model = %+v", gotModel)
	}
	if gotOptions.APIKey != "draft-secret" || gotOptions.BaseURL != "https://draft.example.com/v1" {
		t.Fatalf("options route = key %q, URL %q", gotOptions.APIKey, gotOptions.BaseURL)
	}
	if gotOptions.MaxRetries == nil || *gotOptions.MaxRetries != 0 {
		t.Fatalf("max retries = %v", gotOptions.MaxRetries)
	}
	if gotOptions.MaxTokens != 0 || gotOptions.Timeout != connectionTestStandardTimeout {
		t.Fatalf("probe limits = tokens %d, timeout %s", gotOptions.MaxTokens, gotOptions.Timeout)
	}
	if gotOptions.Reasoning != llm.ModelThinkingOff {
		t.Fatalf("reasoning = %q", gotOptions.Reasoning)
	}
	if after := store.Snapshot(); !reflect.DeepEqual(after, before) {
		t.Fatalf("test changed provider settings\nbefore: %#v\nafter:  %#v", before, after)
	}
}

func TestConnectionTesterResolvesSavedCredential(t *testing.T) {
	store := newTestStore(t, nil)
	_, err := store.Replace("test-provider", Update{
		ActiveConnectionID: OfficialConnectionID,
		Connections: []ConnectionUpdate{
			{ID: OfficialConnectionID},
			{
				ID:      "work",
				Name:    "Work",
				BaseURL: "https://saved.example.com/v1",
				Keys:    []KeyUpdate{{ID: "saved-key", Name: "Saved", APIKey: "saved-secret"}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var gotOptions llm.StreamOptions
	tester := NewConnectionTester(store, func(
		_ context.Context,
		_ llm.Model,
		_ llm.Context,
		options llm.StreamOptions,
	) (llm.AssistantMessage, error) {
		gotOptions = options
		return llm.AssistantMessage{}, nil
	})

	_, err = tester.Test(context.Background(), ConnectionTestRequest{
		ProviderID:   "test-provider",
		ConnectionID: "work",
		KeyID:        "saved-key",
		BaseURL:      "https://current-draft.example.com/v1",
		ModelID:      "test-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotOptions.APIKey != "saved-secret" {
		t.Fatalf("API key = %q", gotOptions.APIKey)
	}
	if gotOptions.BaseURL != "https://current-draft.example.com/v1" {
		t.Fatalf("base URL = %q", gotOptions.BaseURL)
	}
}

func TestConnectionTesterUsesCatalogURLForOfficialConnection(t *testing.T) {
	store := newTestStore(t, nil)
	var gotOptions llm.StreamOptions
	tester := NewConnectionTester(store, func(
		_ context.Context,
		_ llm.Model,
		_ llm.Context,
		options llm.StreamOptions,
	) (llm.AssistantMessage, error) {
		gotOptions = options
		return llm.AssistantMessage{}, nil
	})

	_, err := tester.Test(context.Background(), ConnectionTestRequest{
		ProviderID:   "test-provider",
		ConnectionID: OfficialConnectionID,
		KeyID:        "draft-key",
		BaseURL:      "https://must-not-be-used.example.com/v1",
		APIKey:       "draft-secret",
		ModelID:      "test-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotOptions.BaseURL != "https://catalog.example.com/v1" {
		t.Fatalf("base URL = %q", gotOptions.BaseURL)
	}
}

func TestConnectionTesterRejectsInvalidTargets(t *testing.T) {
	store := newTestStore(t, nil)
	tester := NewConnectionTester(store, func(
		context.Context,
		llm.Model,
		llm.Context,
		llm.StreamOptions,
	) (llm.AssistantMessage, error) {
		t.Fatal("invalid target reached the model client")
		return llm.AssistantMessage{}, nil
	})

	tests := []struct {
		name    string
		request ConnectionTestRequest
		want    string
	}{
		{
			name: "missing fields",
			request: ConnectionTestRequest{
				ProviderID: "test-provider",
			},
			want: "required",
		},
		{
			name: "unknown provider",
			request: ConnectionTestRequest{
				ProviderID:   "missing",
				ConnectionID: "draft",
				KeyID:        "key",
				BaseURL:      "https://example.com/v1",
				APIKey:       "secret",
				ModelID:      "test-model",
			},
			want: "unknown provider",
		},
		{
			name: "unknown model",
			request: ConnectionTestRequest{
				ProviderID:   "test-provider",
				ConnectionID: "draft",
				KeyID:        "key",
				BaseURL:      "https://example.com/v1",
				APIKey:       "secret",
				ModelID:      "missing",
			},
			want: "not available",
		},
		{
			name: "invalid custom URL",
			request: ConnectionTestRequest{
				ProviderID:   "test-provider",
				ConnectionID: "draft",
				KeyID:        "key",
				BaseURL:      "file:///tmp/provider",
				APIKey:       "secret",
				ModelID:      "test-model",
			},
			want: "valid HTTP(S)",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := tester.Test(context.Background(), test.request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestConnectionTesterClassifiesProviderFailures(t *testing.T) {
	store := newTestStore(t, nil)
	tests := []struct {
		name   string
		status int
		err    error
		want   ConnectionTestStatus
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, err: errors.New("unauthorized"), want: ConnectionTestAuthenticationFailed},
		{name: "forbidden", status: http.StatusForbidden, err: errors.New("forbidden"), want: ConnectionTestAuthenticationFailed},
		{name: "rate limited", status: http.StatusTooManyRequests, err: errors.New("rate limited"), want: ConnectionTestRateLimited},
		{name: "request timeout", status: http.StatusRequestTimeout, err: errors.New("timeout"), want: ConnectionTestTimeout},
		{name: "gateway timeout", status: http.StatusGatewayTimeout, err: errors.New("timeout"), want: ConnectionTestTimeout},
		{name: "not found", status: http.StatusNotFound, err: errors.New("not found"), want: ConnectionTestNotFound},
		{name: "deadline", err: context.DeadlineExceeded, want: ConnectionTestTimeout},
		{name: "network", err: &net.DNSError{Err: "no such host", Name: "provider.invalid"}, want: ConnectionTestUnreachable},
		{name: "provider error", status: http.StatusInternalServerError, err: errors.New("provider failed"), want: ConnectionTestProviderError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tester := NewConnectionTester(store, func(
				_ context.Context,
				_ llm.Model,
				_ llm.Context,
				options llm.StreamOptions,
			) (llm.AssistantMessage, error) {
				if test.status != 0 {
					options.OnResponse(test.status, nil)
				}
				return llm.AssistantMessage{}, test.err
			})
			result, err := tester.Test(context.Background(), ConnectionTestRequest{
				ProviderID:   "test-provider",
				ConnectionID: "draft",
				KeyID:        "key",
				BaseURL:      "https://example.com/v1",
				APIKey:       "secret",
				ModelID:      "test-model",
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != test.want || result.ProviderStatus != test.status {
				t.Fatalf("result = %+v, want status %q and HTTP %d", result, test.want, test.status)
			}
		})
	}
}

func TestConnectionTesterUsesLowestSupportedReasoningLevel(t *testing.T) {
	registry := llm.NewProviderRegistry()
	model := testModel()
	model.ID = "reasoning-model"
	model.Reasoning = true
	model.ThinkingLevelMap = map[llm.ModelThinkingLevel]*string{
		llm.ModelThinkingOff: nil,
	}
	if err := registry.Register(llm.NewSpecProvider(llm.ProviderSpec{
		ID:     "test-provider",
		Name:   "Test Provider",
		Models: []llm.Model{model},
	})); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(t.TempDir(), registry)
	if err != nil {
		t.Fatal(err)
	}
	var reasoning llm.ModelThinkingLevel
	tester := NewConnectionTester(store, func(
		_ context.Context,
		_ llm.Model,
		_ llm.Context,
		options llm.StreamOptions,
	) (llm.AssistantMessage, error) {
		reasoning = options.Reasoning
		return llm.AssistantMessage{}, nil
	})
	_, err = tester.Test(context.Background(), ConnectionTestRequest{
		ProviderID:   "test-provider",
		ConnectionID: "draft",
		KeyID:        "key",
		BaseURL:      "https://example.com/v1",
		APIKey:       "secret",
		ModelID:      model.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reasoning != llm.ModelThinkingMinimal {
		t.Fatalf("reasoning = %q, want %q", reasoning, llm.ModelThinkingMinimal)
	}
}

func TestConnectionTesterProjectsSelectedThinkingAndText(t *testing.T) {
	registry := llm.NewProviderRegistry()
	model := testModel()
	model.ID = "reasoning-model"
	model.Reasoning = true
	if err := registry.Register(llm.NewSpecProvider(llm.ProviderSpec{
		ID:     "test-provider",
		Name:   "Test Provider",
		Models: []llm.Model{model},
	})); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(t.TempDir(), registry)
	if err != nil {
		t.Fatal(err)
	}
	var gotOptions llm.StreamOptions
	tester := NewConnectionTester(store, func(
		_ context.Context,
		_ llm.Model,
		_ llm.Context,
		options llm.StreamOptions,
	) (llm.AssistantMessage, error) {
		gotOptions = options
		return llm.AssistantMessage{
			Content: []llm.AssistantContent{
				&llm.ThinkingContent{Thinking: "first thought"},
				&llm.ThinkingContent{Thinking: "second thought"},
				&llm.TextContent{Text: "Hello!"},
			},
			StopReason: llm.StopReasonStop,
			Usage:      llm.Usage{Input: 1, Output: 18},
		}, nil
	})

	result, err := tester.Test(context.Background(), ConnectionTestRequest{
		ProviderID:    "test-provider",
		ConnectionID:  "draft",
		KeyID:         "key",
		BaseURL:       "https://example.com/v1",
		APIKey:        "secret",
		ModelID:       model.ID,
		ThinkingLevel: llm.ModelThinkingLow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotOptions.Reasoning != llm.ModelThinkingLow ||
		gotOptions.MaxTokens != 0 ||
		gotOptions.Timeout != connectionTestReasoningTimeout {
		t.Fatalf("probe options = %+v", gotOptions)
	}
	if result.ThinkingLevel != llm.ModelThinkingLow ||
		result.ThinkingText != "first thought\n\nsecond thought" ||
		result.ResponseText != "Hello!" ||
		result.StopReason != llm.StopReasonStop ||
		result.InputTokens != 1 ||
		result.OutputTokens != 18 {
		t.Fatalf("result = %+v", result)
	}
}

func TestConnectionTesterRequestsSummarizedAnthropicThinking(t *testing.T) {
	registry := llm.NewProviderRegistry()
	model := testModel()
	model.ID = "anthropic-reasoning-model"
	model.Protocol = llm.ProtocolAnthropicMessages
	model.Reasoning = true
	if err := registry.Register(llm.NewSpecProvider(llm.ProviderSpec{
		ID:     "test-provider",
		Name:   "Test Provider",
		Models: []llm.Model{model},
	})); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(t.TempDir(), registry)
	if err != nil {
		t.Fatal(err)
	}
	var gotOptions llm.StreamOptions
	tester := NewConnectionTester(store, func(
		_ context.Context,
		_ llm.Model,
		_ llm.Context,
		options llm.StreamOptions,
	) (llm.AssistantMessage, error) {
		gotOptions = options
		return llm.AssistantMessage{}, nil
	})

	_, err = tester.Test(context.Background(), ConnectionTestRequest{
		ProviderID:    "test-provider",
		ConnectionID:  "draft",
		KeyID:         "key",
		BaseURL:       "https://example.com/v1",
		APIKey:        "secret",
		ModelID:       model.ID,
		ThinkingLevel: llm.ModelThinkingMedium,
	})
	if err != nil {
		t.Fatal(err)
	}
	protocolOptions, ok := gotOptions.ProtocolOptions.(*llm.AnthropicStreamOptions)
	if !ok || protocolOptions.ThinkingDisplay != llm.ThinkingDisplaySummarized {
		t.Fatalf("protocol options = %#v", gotOptions.ProtocolOptions)
	}
	if gotOptions.MaxTokens != 0 {
		t.Fatalf("max tokens = %d", gotOptions.MaxTokens)
	}
}

func TestConnectionTesterRejectsUnsupportedExplicitThinkingLevel(t *testing.T) {
	store := newTestStore(t, nil)
	tester := NewConnectionTester(store, func(
		context.Context,
		llm.Model,
		llm.Context,
		llm.StreamOptions,
	) (llm.AssistantMessage, error) {
		t.Fatal("unsupported thinking level reached the model client")
		return llm.AssistantMessage{}, nil
	})

	_, err := tester.Test(context.Background(), ConnectionTestRequest{
		ProviderID:    "test-provider",
		ConnectionID:  "draft",
		KeyID:         "key",
		BaseURL:       "https://example.com/v1",
		APIKey:        "secret",
		ModelID:       "test-model",
		ThinkingLevel: llm.ModelThinkingHigh,
	})
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("error = %v", err)
	}
}

func TestConnectionTesterLeavesOutputLimitUnset(t *testing.T) {
	registry := llm.NewProviderRegistry()
	model := testModel()
	model.MaxTokens = 8
	if err := registry.Register(llm.NewSpecProvider(llm.ProviderSpec{
		ID:     "test-provider",
		Name:   "Test Provider",
		Models: []llm.Model{model},
	})); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(t.TempDir(), registry)
	if err != nil {
		t.Fatal(err)
	}
	var maxTokens int64
	tester := NewConnectionTester(store, func(
		_ context.Context,
		_ llm.Model,
		_ llm.Context,
		options llm.StreamOptions,
	) (llm.AssistantMessage, error) {
		maxTokens = options.MaxTokens
		return llm.AssistantMessage{}, nil
	})
	_, err = tester.Test(context.Background(), ConnectionTestRequest{
		ProviderID:   "test-provider",
		ConnectionID: "draft",
		KeyID:        "key",
		BaseURL:      "https://example.com/v1",
		APIKey:       "secret",
		ModelID:      model.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if maxTokens != 0 {
		t.Fatalf("max tokens = %d, want unset", maxTokens)
	}
}
