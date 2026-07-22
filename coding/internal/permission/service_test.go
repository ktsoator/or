package permission

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

type approverFunc func(context.Context, ApprovalRequest) (ApprovalResponse, error)

func (f approverFunc) Decide(ctx context.Context, req ApprovalRequest) (ApprovalResponse, error) {
	return f(ctx, req)
}

func TestServiceDefaultPolicy(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(filepath.Dir(workspace), "outside.txt")

	t.Run("workspace read is allowed without approval", func(t *testing.T) {
		service, err := NewService(workspace, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		decision, err := service.Authorize(context.Background(), Request{
			Tool: "read", Accesses: []Access{{Action: Read, Path: "file.txt"}},
		})
		if err != nil || decision.Behavior != Allow {
			t.Fatalf("Authorize() = %+v, %v, want Allow", decision, err)
		}
	})

	t.Run("external read asks and can be allowed once", func(t *testing.T) {
		var received ApprovalRequest
		service, err := NewService(workspace, nil, approverFunc(func(_ context.Context, req ApprovalRequest) (ApprovalResponse, error) {
			received = req
			return ApprovalResponse{Choice: AllowOnce}, nil
		}))
		if err != nil {
			t.Fatal(err)
		}
		decision, err := service.Authorize(context.Background(), Request{
			Tool: "read", Accesses: []Access{{Action: Read, Path: outside}},
		})
		if err != nil || decision.Behavior != Allow {
			t.Fatalf("Authorize() = %+v, %v, want Allow", decision, err)
		}
		if len(received.Request.Accesses) != 1 || received.Request.Accesses[0].Location != OutsideWorkspace {
			t.Fatalf("approval request access = %+v, want external", received.Request.Accesses)
		}
	})

	t.Run("writes and commands require approval", func(t *testing.T) {
		calls := 0
		service, err := NewService(workspace, nil, approverFunc(func(_ context.Context, _ ApprovalRequest) (ApprovalResponse, error) {
			calls++
			return ApprovalResponse{Choice: Reject}, nil
		}))
		if err != nil {
			t.Fatal(err)
		}
		for _, request := range []Request{
			{Tool: "write", Accesses: []Access{{Action: Write, Path: "file.txt"}}},
			{Tool: "bash", Accesses: []Access{{Action: Execute, Command: "pwd"}}},
		} {
			decision, err := service.Authorize(context.Background(), request)
			if err != nil || decision.Behavior != Deny {
				t.Fatalf("Authorize(%s) = %+v, %v, want Deny", request.Tool, decision, err)
			}
		}
		if calls != 2 {
			t.Fatalf("approval calls = %d, want 2", calls)
		}
	})
}

func TestServiceCancelsApproval(t *testing.T) {
	service, err := NewService(t.TempDir(), nil, approverFunc(func(ctx context.Context, _ ApprovalRequest) (ApprovalResponse, error) {
		<-ctx.Done()
		return ApprovalResponse{}, ctx.Err()
	}))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	decision, err := service.Authorize(ctx, Request{
		Tool: "bash", Accesses: []Access{{Action: Execute, Command: "pwd"}},
	})
	if !errors.Is(err, context.Canceled) || decision.Behavior != Deny {
		t.Fatalf("Authorize() = %+v, %v, want cancelled Deny", decision, err)
	}
}

func TestPolicyModes(t *testing.T) {
	tests := []struct {
		name   string
		mode   Mode
		access Access
		want   Behavior
	}{
		{name: "ask allows workspace reads", mode: ModeAsk, access: Access{Action: Read, Location: Workspace}, want: Allow},
		{name: "ask prompts for workspace writes", mode: ModeAsk, access: Access{Action: Write, Location: Workspace}, want: Ask},
		{name: "auto edit allows workspace writes", mode: ModeAutoEdit, access: Access{Action: Write, Location: Workspace}, want: Allow},
		{name: "auto edit prompts for external writes", mode: ModeAutoEdit, access: Access{Action: Write, Location: OutsideWorkspace}, want: Ask},
		{name: "auto edit prompts for shell commands", mode: ModeAutoEdit, access: Access{Action: Execute}, want: Ask},
		{name: "read only blocks writes", mode: ModeReadOnly, access: Access{Action: Write, Location: Workspace}, want: Deny},
		{name: "read only blocks shell commands", mode: ModeReadOnly, access: Access{Action: Execute}, want: Deny},
		{name: "read only still prompts for external reads", mode: ModeReadOnly, access: Access{Action: Read, Location: OutsideWorkspace}, want: Ask},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := PolicyForMode(test.mode)(Request{Accesses: []Access{test.access}})
			if got.Behavior != test.want {
				t.Fatalf("PolicyForMode(%q) = %+v, want %q", test.mode, got, test.want)
			}
		})
	}
}

func TestServiceCanChangePolicy(t *testing.T) {
	service, err := NewService(t.TempDir(), PolicyForMode(ModeAsk), nil)
	if err != nil {
		t.Fatal(err)
	}
	request := Request{Tool: "write", Accesses: []Access{{Action: Write, Path: "file.txt"}}}
	before, err := service.Authorize(context.Background(), request)
	if err != nil || before.Behavior != Deny {
		t.Fatalf("Authorize() before mode change = %+v, %v, want denied without approver", before, err)
	}
	service.SetPolicy(PolicyForMode(ModeAutoEdit))
	after, err := service.Authorize(context.Background(), request)
	if err != nil || after.Behavior != Allow {
		t.Fatalf("Authorize() after mode change = %+v, %v, want Allow", after, err)
	}
}

func TestReadOnlyDenyTakesPriorityOverApproval(t *testing.T) {
	decision := PolicyForMode(ModeReadOnly)(Request{Accesses: []Access{
		{Action: Read, Location: OutsideWorkspace},
		{Action: Write, Location: Workspace},
	}})
	if decision.Behavior != Deny {
		t.Fatalf("read-only mixed access decision = %+v, want Deny", decision)
	}
}
