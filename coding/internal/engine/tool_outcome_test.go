package engine

import (
	"reflect"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/tools"
)

func TestToolOutcomeRoundTrip(t *testing.T) {
	exitCode := 17
	want := agent.ToolOutcome{
		Status:    agent.ToolOutcomeFailed,
		ErrorCode: "command_exit_nonzero",
		ExitCode:  &exitCode,
		Data: tools.PreviewRequest{
			Path:         "/workspace/web/index.html",
			RelativePath: "web/index.html",
			Title:        "Static page",
			GrantID:      "preview-grant",
			PreviewPath:  "index.html",
		},
	}
	persisted := encodeOutcome("call-17", want)
	if persisted.ToolCallID != "call-17" || persisted.DataKind != kindPreview {
		t.Fatalf("persisted outcome = %#v", persisted)
	}
	got, ok := decodeOutcome(persisted)
	if !ok {
		t.Fatalf("tool outcome was not decoded: %#v", persisted)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded outcome = %#v, want %#v", got, want)
	}
}
