package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ktsoator/or/coding/internal/permission"
)

type approvalResult struct {
	response permission.ApprovalResponse
	err      error
}

func TestApprovalBrokerResolvesRequest(t *testing.T) {
	hub := NewHub()
	events, syncRequired := hub.add(0)
	if syncRequired {
		t.Fatal("unexpected sync requirement")
	}
	defer hub.remove(events)
	broker := NewApprovalBroker(hub)
	result := make(chan approvalResult, 1)

	go func() {
		response, err := broker.Decide(context.Background(), approvalRequest())
		result <- approvalResult{response: response, err: err}
	}()

	requested := readApprovalEvent(t, events)
	if requested.Type != "approval_request" || requested.ID == "" {
		t.Fatalf("request event = %+v", requested)
	}
	if !broker.Resolve(requested.ID, permission.AllowOnce) {
		t.Fatal("Resolve returned false")
	}
	resolved := readApprovalEvent(t, events)
	if resolved.Type != "approval_resolved" || resolved.ID != requested.ID {
		t.Fatalf("resolved event = %+v", resolved)
	}

	select {
	case got := <-result:
		if got.err != nil || got.response.Choice != permission.AllowOnce {
			t.Fatalf("Decide() = %+v, %v", got.response, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("Decide did not return")
	}
}

func TestApprovalBrokerCancelsRequest(t *testing.T) {
	hub := NewHub()
	events, syncRequired := hub.add(0)
	if syncRequired {
		t.Fatal("unexpected sync requirement")
	}
	defer hub.remove(events)
	broker := NewApprovalBroker(hub)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan approvalResult, 1)

	go func() {
		response, err := broker.Decide(ctx, approvalRequest())
		result <- approvalResult{response: response, err: err}
	}()

	requested := readApprovalEvent(t, events)
	cancel()
	cancelled := readApprovalEvent(t, events)
	if cancelled.Type != "approval_cancelled" || cancelled.ID != requested.ID {
		t.Fatalf("cancelled event = %+v", cancelled)
	}
	select {
	case got := <-result:
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("Decide error = %v, want context.Canceled", got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled Decide did not return")
	}
	if broker.HasPending() {
		t.Fatal("broker still has a pending approval")
	}
}

func approvalRequest() permission.ApprovalRequest {
	return permission.ApprovalRequest{
		Request: permission.Request{
			Tool:     "bash",
			Args:     map[string]any{"command": "go test ./..."},
			Accesses: []permission.Access{{Action: permission.Execute, Command: "go test ./..."}},
		},
		Reason: "shell commands require approval",
	}
}

func readApprovalEvent(t *testing.T, events <-chan hubFrame) wireEvent {
	t.Helper()
	select {
	case frame := <-events:
		var event wireEvent
		if err := json.Unmarshal(frame.data, &event); err != nil {
			t.Fatal(err)
		}
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for approval event")
		return wireEvent{}
	}
}
