package client

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (function roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestHeaderTransportKeepsCredentialsOnConfiguredOrigin(t *testing.T) {
	seen := make(map[string]string)
	base := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		seen[request.URL.Host] = request.Header.Get("Authorization")
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})
	transport := &headerTransport{
		base:    base,
		origin:  "https://mcp.example.com",
		headers: http.Header{"Authorization": []string{"Bearer secret"}},
	}
	for _, target := range []string{"https://mcp.example.com/tools", "https://other.example.com/tools"} {
		request, err := http.NewRequest(http.MethodPost, target, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := transport.RoundTrip(request); err != nil {
			t.Fatal(err)
		}
	}
	if seen["mcp.example.com"] != "Bearer secret" {
		t.Fatalf("configured origin header = %q", seen["mcp.example.com"])
	}
	if seen["other.example.com"] != "" {
		t.Fatalf("cross-origin header leaked: %q", seen["other.example.com"])
	}
}
