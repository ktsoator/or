package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ktsoator/or/coding/internal/provider"
	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/all"
)

func TestProviderConnectionTestEndpointProjectsRequestAndResponse(t *testing.T) {
	registry := llm.NewProviderRegistry()
	if err := registry.Register(llm.NewSpecProvider(llm.ProviderSpec{
		ID:   "test-provider",
		Name: "Test Provider",
		Models: []llm.Model{{
			ID:        "test-model",
			Name:      "Test Model",
			Provider:  "test-provider",
			Protocol:  llm.ProtocolOpenAICompletions,
			BaseURL:   "https://catalog.example.com/v1",
			Reasoning: true,
			Input:     []llm.ModelInput{llm.Text},
		}},
	})); err != nil {
		t.Fatal(err)
	}
	store, err := provider.NewStore(t.TempDir(), registry)
	if err != nil {
		t.Fatal(err)
	}
	var gotModel llm.Model
	var gotOptions llm.StreamOptions
	tester := provider.NewConnectionTester(store, func(
		_ context.Context,
		model llm.Model,
		_ llm.Context,
		options llm.StreamOptions,
	) (llm.AssistantMessage, error) {
		gotModel = model
		gotOptions = options
		options.OnResponse(http.StatusOK, nil)
		return llm.AssistantMessage{
			Content: []llm.AssistantContent{
				&llm.ThinkingContent{Thinking: "considering"},
				&llm.TextContent{Text: "Hello!"},
			},
			StopReason: llm.StopReasonStop,
			Usage:      llm.Usage{Input: 1, Output: 8},
		}, nil
	})

	server := &Server{providerTests: tester}
	router := gin.New()
	server.mountProviders(router.Group("/api"))
	body := []byte(`{
		"connectionId":"draft-connection",
		"keyId":"draft-key",
		"baseURL":"https://draft.example.com/v1",
		"apiKey":"draft-secret",
		"model":"test-model",
		"thinkingLevel":"low"
	}`)
	request := httptest.NewRequest(http.MethodPost, "/api/providers/test-provider/test-connection", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload providerConnectionTestResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != provider.ConnectionTestSuccess ||
		payload.Model != "test-model" ||
		payload.ModelName != "Test Model" ||
		payload.RequestText != "hi" ||
		payload.ThinkingLevel != llm.ModelThinkingLow ||
		payload.ThinkingText != "considering" ||
		payload.ResponseText != "Hello!" ||
		payload.StopReason != llm.StopReasonStop ||
		payload.InputTokens != 1 ||
		payload.OutputTokens != 8 ||
		payload.ProviderStatus != http.StatusOK {
		t.Fatalf("response = %+v", payload)
	}
	if gotModel.ID != "test-model" ||
		gotOptions.APIKey != "draft-secret" ||
		gotOptions.BaseURL != "https://draft.example.com/v1" ||
		gotOptions.Reasoning != llm.ModelThinkingLow {
		t.Fatalf("model = %+v, options route = key %q, URL %q", gotModel, gotOptions.APIKey, gotOptions.BaseURL)
	}
}

func TestProviderConnectionTestEndpointReportsUnavailableService(t *testing.T) {
	server := &Server{}
	router := gin.New()
	server.mountProviders(router.Group("/api"))
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/providers/test-provider/test-connection",
		bytes.NewBufferString(`{"model":"test-model"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}
