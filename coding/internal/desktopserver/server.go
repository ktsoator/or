// Package desktopserver serves the Electron renderer and product API from one
// authenticated loopback origin.
package desktopserver

import (
	"crypto/subtle"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// CookieName is the HttpOnly cookie Electron installs before loading the app.
const CookieName = "coding_desktop_session"

// New returns a handler that protects both the renderer assets and API with a
// per-launch token. Keeping them on one origin preserves relative fetch and
// EventSource URLs without exposing a fixed unauthenticated localhost API.
func New(api http.Handler, assets fs.FS, token string) http.Handler {
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !authorized(request, token) {
			http.Error(response, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		if request.URL.Path == "/api" || strings.HasPrefix(request.URL.Path, "/api/") {
			api.ServeHTTP(response, request)
			return
		}
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			response.Header().Set("Allow", "GET, HEAD")
			http.Error(response, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		name := strings.TrimPrefix(path.Clean("/"+request.URL.Path), "/")
		if name == "." || name == "" {
			name = "index.html"
		}
		if info, err := fs.Stat(assets, name); err != nil {
			if path.Ext(name) != "" {
				http.NotFound(response, request)
				return
			}
			name = "index.html"
		} else if info.IsDir() {
			name = "index.html"
		}

		clone := request.Clone(request.Context())
		clone.URL = cloneURL(request.URL)
		clone.URL.Path = "/" + name
		if name == "index.html" {
			response.Header().Set("Cache-Control", "no-store")
			clone.URL.Path = "/"
		}
		files.ServeHTTP(response, clone)
	})
}

func authorized(request *http.Request, token string) bool {
	if token == "" {
		return false
	}
	cookie, err := request.Cookie(CookieName)
	if err != nil || len(cookie.Value) != len(token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(token)) == 1
}

func cloneURL(source *url.URL) *url.URL {
	clone := *source
	return &clone
}
