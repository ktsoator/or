package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBindCreateSessionRequestWithUnknownContentLength(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(`{
		"scope":"chat",
		"provider":"deepseek",
		"model":"deepseek-v4-flash",
		"thinkingLevel":"high",
		"permissionMode":"ask"
	}`))
	request.ContentLength = -1
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = request

	body, ok := bindCreateSessionRequest(ctx)
	if !ok {
		t.Fatalf("bindCreateSessionRequest rejected body with unknown content length: status %d", recorder.Code)
	}
	if body.Scope != "chat" || body.Provider != "deepseek" || body.Model != "deepseek-v4-flash" {
		t.Fatalf("body = %#v", body)
	}
	if body.ThinkingLevel != "high" || body.PermissionMode != "ask" {
		t.Fatalf("settings = thinking %q, permission %q", body.ThinkingLevel, body.PermissionMode)
	}
}
