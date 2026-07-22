package httpapi

import (
	"testing"
	"time"
)

func TestHubReplaysFramesAfterSnapshot(t *testing.T) {
	hub := NewHub()
	hub.Broadcast([]byte(`{"type":"before"}`))
	baseline := hub.snapshot(func() {})
	hub.Broadcast([]byte(`{"type":"first"}`))
	hub.Broadcast([]byte(`{"type":"second"}`))

	ch, syncRequired := hub.add(baseline)
	if syncRequired {
		t.Fatal("unexpected sync requirement")
	}
	defer hub.remove(ch)
	for index, want := range []string{`{"type":"first"}`, `{"type":"second"}`} {
		select {
		case frame := <-ch:
			if frame.sequence != baseline+uint64(index)+1 || string(frame.data) != want {
				t.Fatalf("frame %d = %#v", index, frame)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for frame %d", index)
		}
	}
}

func TestHubTreatsFutureSequenceAsFreshConnection(t *testing.T) {
	hub := NewHub()
	hub.Broadcast([]byte(`{"type":"current"}`))

	ch, syncRequired := hub.add(100)
	if syncRequired {
		t.Fatal("unexpected sync requirement")
	}
	defer hub.remove(ch)
	select {
	case frame := <-ch:
		if frame.sequence != 1 || string(frame.data) != `{"type":"current"}` {
			t.Fatalf("frame = %#v", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for replay after sequence reset")
	}
}

func TestHubRequiresSyncWhenReplayWindowHasExpired(t *testing.T) {
	hub := NewHub()
	for index := 0; index < hubReplayLimit+1; index++ {
		hub.Broadcast([]byte(`{"type":"event"}`))
	}

	ch, syncRequired := hub.add(0)
	if !syncRequired {
		if ch != nil {
			hub.remove(ch)
		}
		t.Fatal("expected an expired replay window to require a history sync")
	}
	if ch != nil {
		t.Fatal("sync-required connection unexpectedly registered a client")
	}
}
