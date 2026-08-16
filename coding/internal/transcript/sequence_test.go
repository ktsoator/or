package transcript

import (
	"context"
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
)

func sequencedForTest(entries ...Entry) []Entry {
	sequenced, err := SequenceEntries(entries, 0)
	if err != nil {
		panic(err)
	}
	return sequenced
}

func TestMemoryAppendRejectsEntryIDFromEarlierBatch(t *testing.T) {
	store := &Memory{}
	first := sequencedForTest(NewMessage(agent.UserMessage("first")))[0]
	if err := store.Append(context.Background(), first); err != nil {
		t.Fatal(err)
	}

	duplicate := NewMessage(agent.UserMessage("duplicate"))
	duplicate.ID = first.ID
	duplicate.Seq = 1
	if err := store.Append(context.Background(), duplicate); err == nil ||
		!strings.Contains(err.Error(), "repeats existing entry id") {
		t.Fatalf("Append() error = %v, want existing entry id rejection", err)
	}
}
