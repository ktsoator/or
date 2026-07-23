package engine

import (
	"testing"

	"github.com/ktsoator/or/coding/internal/tools"
)

func TestPreviewDetailsRoundTrip(t *testing.T) {
	want := tools.PreviewRequest{
		Path:         "/workspace/web/index.html",
		RelativePath: "web/index.html",
		Title:        "Static page",
	}
	raw, ok := encodeDetails(want)
	if !ok {
		t.Fatal("preview details were not encoded")
	}
	got, ok := decodeDetails(raw).(tools.PreviewRequest)
	if !ok {
		t.Fatalf("decoded details = %#v, want tools.PreviewRequest", decodeDetails(raw))
	}
	if got != want {
		t.Fatalf("decoded preview = %#v, want %#v", got, want)
	}
}
