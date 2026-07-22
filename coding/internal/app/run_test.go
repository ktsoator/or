package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ktsoator/or/coding/internal/config"
)

func TestRuntimeServesAPIAndClosesMoreThanOnce(t *testing.T) {
	runtime, err := New(context.Background(), config.Config{
		Cwd:     t.TempDir(),
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
