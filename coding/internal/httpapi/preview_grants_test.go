package httpapi

import (
	"path/filepath"
	"testing"

	"github.com/ktsoator/or/coding/internal/tools"
)

func TestPreviewGrantIsScopedToSessionAndEntryDirectory(t *testing.T) {
	workspace := t.TempDir()
	entry := writePreviewFile(t, workspace, "web/index.html", "safe")
	store := newPreviewGrantStore()

	first, err := store.issue("session-1", workspace, previewRequest(entry))
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.issue("session-1", workspace, previewRequest(entry))
	if err != nil {
		t.Fatal(err)
	}
	otherSession, err := store.issue("session-2", workspace, previewRequest(entry))
	if err != nil {
		t.Fatal(err)
	}
	if first.GrantID == "" || first.GrantID != again.GrantID {
		t.Fatalf("same entry grants = %q and %q", first.GrantID, again.GrantID)
	}
	if first.GrantID == otherSession.GrantID {
		t.Fatal("grant was reused across sessions")
	}
	if first.PreviewPath != "index.html" || first.RelativePath != "web/index.html" {
		t.Fatalf("preview = %#v", first)
	}
	grant, ok := store.resolve("session-1", first.GrantID)
	wantRoot, err := filepath.EvalSymlinks(filepath.Join(workspace, "web"))
	if err != nil {
		t.Fatal(err)
	}
	if !ok || grant.Root != wantRoot {
		t.Fatalf("grant = %#v, ok = %v", grant, ok)
	}
	if _, ok := store.resolve("session-2", first.GrantID); ok {
		t.Fatal("grant resolved for the wrong session")
	}
}

func TestReissuePreviewGrantsReplacesPersistedGrant(t *testing.T) {
	workspace := t.TempDir()
	entry := writePreviewFile(t, workspace, "web/index.html", "safe")
	events := []wireEvent{{Preview: &wirePreview{
		Path:         entry,
		RelativePath: "web/index.html",
		Title:        "Static page",
		GrantID:      "stale-grant",
		PreviewPath:  "old.html",
	}}}

	store := newPreviewGrantStore()
	reissuePreviewGrants(store, "session-1", workspace, events)
	first := events[0].Preview
	if first == nil || first.GrantID == "" || first.GrantID == "stale-grant" || first.PreviewPath != "index.html" {
		t.Fatalf("reissued preview = %#v", first)
	}

	restartedStore := newPreviewGrantStore()
	reissuePreviewGrants(restartedStore, "session-1", workspace, events)
	if events[0].Preview == nil || events[0].Preview.GrantID == first.GrantID {
		t.Fatalf("restart did not replace grant: before %#v, after %#v", first, events[0].Preview)
	}
}

func TestReissuePreviewGrantsDropsEntryOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := writePreviewFile(t, t.TempDir(), "index.html", "outside")
	events := []wireEvent{{Preview: &wirePreview{Path: outside}}}
	reissuePreviewGrants(newPreviewGrantStore(), "session-1", workspace, events)
	if events[0].Preview != nil {
		t.Fatalf("outside preview survived reissue: %#v", events[0].Preview)
	}
}

func TestPreviewGrantRejectsHiddenEntryDirectory(t *testing.T) {
	workspace := t.TempDir()
	entry := writePreviewFile(t, workspace, ".private/index.html", "hidden")
	_, err := newPreviewGrantStore().issue("session-1", workspace, previewRequest(entry))
	if err == nil {
		t.Fatal("preview grant accepted an entry in a hidden directory")
	}
}

func previewRequest(path string) tools.PreviewRequest {
	return tools.PreviewRequest{Path: path, Title: "Static page"}
}
