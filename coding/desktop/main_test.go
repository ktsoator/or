package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestFirstExistingDirectory(t *testing.T) {
	existing := t.TempDir()
	missing := filepath.Join(t.TempDir(), "missing")

	if got := firstExistingDirectory("", missing, existing); got != existing {
		t.Fatalf("firstExistingDirectory() = %q, want %q", got, existing)
	}
	if got := firstExistingDirectory("", missing); got != "" {
		t.Fatalf("firstExistingDirectory() = %q, want empty", got)
	}
}

func TestRouteAPI(t *testing.T) {
	api := http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusAccepted)
		_, _ = response.Write([]byte("api"))
	})
	assets := http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte("assets"))
	})
	handler := routeAPI(api)(assets)

	tests := []struct {
		path   string
		status int
		body   string
	}{
		{path: "/api", status: http.StatusAccepted, body: "api"},
		{path: "/api/models", status: http.StatusAccepted, body: "api"},
		{path: "/assets/index.js", status: http.StatusOK, body: "assets"},
		{path: "/apiary", status: http.StatusOK, body: "assets"},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
			if response.Body.String() != test.body {
				t.Fatalf("body = %q, want %q", response.Body.String(), test.body)
			}
		})
	}
}
