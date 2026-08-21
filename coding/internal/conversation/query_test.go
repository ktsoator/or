package conversation

import (
	"testing"

	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/transcript"
)

func TestManagerReconstructsRequestSnapshotFromTranscript(t *testing.T) {
	manager := newTestManager(t, t.TempDir())
	manager.streamFn = forkResponses("answer")
	model, thinking := testCatalogModel(t)
	created, err := manager.Create(
		"Diagnostics", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.StartPromptWithFiles(created.ID, "question", nil); err != nil {
		t.Fatal(err)
	}
	waitForSessionIdle(t, manager, created.ID)

	var requestID string
	for _, entry := range mustRuntime(t, manager, created.ID).session.Entries() {
		if entry.Type == transcript.RequestHeaderEntry {
			requestID = entry.RequestHeader.ProviderRequestID
			break
		}
	}
	if requestID == "" {
		t.Fatal("transcript has no provider request")
	}
	record, err := manager.LoadForSession(created.ID, requestID)
	if err != nil {
		t.Fatal(err)
	}
	if record.SessionID != created.ID || record.ProviderRequestID != requestID ||
		len(record.Input.Messages) == 0 || record.Output == nil ||
		record.Output.Message.Content[0].Text != "answer" {
		t.Fatalf("request snapshot = %#v", record)
	}
}
