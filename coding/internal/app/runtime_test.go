package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ktsoator/or/coding/internal/config"
	"github.com/ktsoator/or/coding/internal/conversation"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/llm"
)

func TestRuntimeServesAPIAndClosesMoreThanOnce(t *testing.T) {
	runtime, err := New(context.Background(), config.Config{
		DataDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	response := httptest.NewRecorder()
	runtime.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /api/sessions status = %d, want %d", response.Code, http.StatusOK)
	}

	runtime.Close()
	runtime.Close()
}

func TestRuntimeOpensRestoredSessionHistoryLazily(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dataDir := t.TempDir()
	projectDir := t.TempDir()
	cfg := config.Config{DataDir: dataDir}
	model, thinking := runtimeTestModel(t)

	first, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	created, err := first.conversations.Create(
		"Restored",
		projectDir,
		conversation.ScopeProject,
		model,
		thinking,
		permission.ModeAsk,
	)
	if err != nil {
		first.Close()
		t.Fatal(err)
	}
	first.Close()

	restored, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/sessions/"+created.ID+"/history",
		nil,
	)
	response := httptest.NewRecorder()
	restored.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf(
			"GET restored history status = %d, want %d: %s",
			response.Code,
			http.StatusOK,
			response.Body.String(),
		)
	}
}

func runtimeTestModel(t *testing.T) (llm.Model, llm.ModelThinkingLevel) {
	t.Helper()
	for _, provider := range llm.GetProviders() {
		for _, model := range llm.GetModels(provider) {
			levels := llm.SupportedThinkingLevels(model)
			if len(levels) > 0 {
				return model, levels[0]
			}
		}
	}
	t.Fatal("built-in model catalog is empty")
	return llm.Model{}, ""
}
