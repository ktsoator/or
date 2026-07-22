package httpapi

import (
	"net/http"
	"slices"
	"strings"
)

// allowClientOrigin enables browser access when the React client is
// deployed on a different origin. An empty origin keeps the API same-origin
// only, which is the default when using the Vite or production reverse proxy.
//
// This is plain net/http rather than gin middleware on purpose: preflight has
// to be answered before routing, for paths gin has no OPTIONS route for, and
// the policy applies to the whole server rather than to any group of routes.
//
// methods is the set the router actually serves. Deriving it from the route
// table rather than hard-coding it keeps preflight from silently rejecting a
// verb that a new endpoint starts using.
func allowClientOrigin(next http.Handler, allowedOrigin string, methods []string) http.Handler {
	if allowedOrigin == "" {
		return next
	}
	allowMethods := joinMethods(methods)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		w.Header().Add("Vary", "Origin")
		if origin == allowedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Methods", allowMethods)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}

		if r.Method == http.MethodOptions {
			if origin != allowedOrigin {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// joinMethods returns the deduplicated, sorted verb list for the preflight
// response. OPTIONS is always included: it is the preflight itself and is never
// a registered route.
func joinMethods(methods []string) string {
	out := make([]string, 0, len(methods)+1)
	for _, method := range methods {
		if method != "" && !slices.Contains(out, method) {
			out = append(out, method)
		}
	}
	if !slices.Contains(out, http.MethodOptions) {
		out = append(out, http.MethodOptions)
	}
	slices.Sort(out)
	return strings.Join(out, ", ")
}
