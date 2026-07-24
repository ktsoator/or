package desktopserver

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

const testToken = "0123456789abcdef0123456789abcdef"

func TestHandlerRequiresDesktopSession(t *testing.T) {
	handler := testHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestHandlerRoutesAPIAndAssets(t *testing.T) {
	handler := testHandler(t)
	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "api", path: "/api/models", wantStatus: http.StatusAccepted, wantBody: "api:/api/models"},
		{name: "index", path: "/", wantStatus: http.StatusOK, wantBody: "index"},
		{name: "asset", path: "/assets/app.js", wantStatus: http.StatusOK, wantBody: "asset"},
		{name: "spa fallback", path: "/sessions/abc", wantStatus: http.StatusOK, wantBody: "index"},
		{name: "missing asset", path: "/assets/missing.js", wantStatus: http.StatusNotFound, wantBody: "404 page not found\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := authenticatedRequest(http.MethodGet, test.path)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if response.Body.String() != test.wantBody {
				t.Fatalf("body = %q, want %q", response.Body.String(), test.wantBody)
			}
		})
	}
}

func TestHandlerRejectsAssetMutation(t *testing.T) {
	handler := testHandler(t)
	request := authenticatedRequest(http.MethodPost, "/assets/app.js")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	assets := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("index"), Mode: fs.FileMode(0o644)},
		"assets/app.js": &fstest.MapFile{Data: []byte("asset"), Mode: fs.FileMode(0o644)},
	}
	api := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusAccepted)
		_, _ = response.Write([]byte("api:" + request.URL.Path))
	})
	return New(api, assets, testToken)
}

func authenticatedRequest(method, target string) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	request.AddCookie(&http.Cookie{Name: CookieName, Value: testToken})
	return request
}
