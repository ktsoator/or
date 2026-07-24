package httpapi

import "testing"

func TestSessionTransportsRemovesClosedTransport(t *testing.T) {
	transports := NewSessionTransports()
	created := transports.New("session-1")
	transport, ok := transports.get("session-1")
	if !ok {
		t.Fatal("created transport was not registered")
	}
	client, syncRequired := transport.hub.add(0)
	if syncRequired {
		t.Fatal("unexpected sync requirement")
	}

	created.Close()
	created.Close()

	if _, ok := transports.get("session-1"); ok {
		t.Fatal("closed transport remains registered")
	}
	if _, ok := <-client; ok {
		t.Fatal("closing transport did not disconnect its client")
	}
}

func TestSessionTransportsReplacementSurvivesOldClose(t *testing.T) {
	transports := NewSessionTransports()
	first := transports.New("session-1")
	second := transports.New("session-1")

	first.Close()

	got, ok := transports.get("session-1")
	if !ok || got != second {
		t.Fatal("closing replaced transport removed the current transport")
	}
	second.Close()
}
