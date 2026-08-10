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

func TestServiceAskMode(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(filepath.Dir(workspace), "outside.txt")

	t.Run("workspace read is allowed without approval", func(t *testing.T) {
		service, err := NewService(workspace, ModeAsk, nil)
		if err != nil {
			t.Fatal(err)
		}
		result, err := service.Authorize(context.Background(), Request{
			Tool: "read", Accesses: []Access{{Action: Read, Path: "file.txt"}},
		})
		if err != nil || !result.Allowed {
			t.Fatalf("Authorize() = %+v, %v, want allowed", result, err)
		}
	})

	t.Run("external read asks and can be allowed once", func(t *testing.T) {
		var received ApprovalRequest
		service, err := NewService(workspace, ModeAsk, approverFunc(func(_ context.Context, req ApprovalRequest) (ApprovalResponse, error) {
			received = req
			return ApprovalResponse{Choice: AllowOnce}, nil
		}))
		if err != nil {
			t.Fatal(err)
		}
		result, err := service.Authorize(context.Background(), Request{
			Tool: "read", Accesses: []Access{{Action: Read, Path: outside}},
		})
		if err != nil || !result.Allowed {
			t.Fatalf("Authorize() = %+v, %v, want allowed", result, err)
		}
		if len(received.Request.Accesses) != 1 || received.Request.Accesses[0].Location != OutsideWorkspace {
			t.Fatalf("approval request access = %+v, want external", received.Request.Accesses)
		}
	})

	t.Run("writes and commands require approval", func(t *testing.T) {
		calls := 0
		service, err := NewService(workspace, ModeAsk, approverFunc(func(_ context.Context, _ ApprovalRequest) (ApprovalResponse, error) {
			calls++
			return ApprovalResponse{Choice: Reject}, nil
		}))
		if err != nil {
			t.Fatal(err)
		}
		for _, request := range []Request{
			{Tool: "write", Accesses: []Access{{Action: Write, Path: "file.txt"}}},
			{Tool: "bash", Accesses: []Access{{Action: Execute}}},
		} {
			result, err := service.Authorize(context.Background(), request)
			if err != nil || result.Allowed {
				t.Fatalf("Authorize(%s) = %+v, %v, want denied", request.Tool, result, err)
			}
		}
		if calls != 2 {
			t.Fatalf("approval calls = %d, want 2", calls)
		}
	})

	t.Run("network access asks and preserves its target", func(t *testing.T) {
		var received ApprovalRequest
		service, err := NewService(workspace, ModeAsk, approverFunc(func(_ context.Context, req ApprovalRequest) (ApprovalResponse, error) {
			received = req
			return ApprovalResponse{Choice: AllowOnce}, nil
		}))
		if err != nil {
			t.Fatal(err)
		}
		result, err := service.Authorize(context.Background(), Request{
			Tool:     "open_preview",
			Args:     map[string]any{"url": "https://example.com/docs"},
			Accesses: []Access{{Action: Network}},
		})
		if err != nil || !result.Allowed {
			t.Fatalf("Authorize() = %+v, %v, want allowed", result, err)
		}
		if got := received.Request.Args["url"]; got != "https://example.com/docs" {
			t.Fatalf("approval request URL = %v, want network target", got)
		}
	})
}

func TestServiceCancelsApproval(t *testing.T) {
	service, err := NewService(t.TempDir(), ModeAsk, approverFunc(func(ctx context.Context, _ ApprovalRequest) (ApprovalResponse, error) {
		<-ctx.Done()
		return ApprovalResponse{}, ctx.Err()
	}))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := service.Authorize(ctx, Request{
		Tool: "bash", Accesses: []Access{{Action: Execute}},
	})
	if !errors.Is(err, context.Canceled) || result.Allowed {
		t.Fatalf("Authorize() = %+v, %v, want cancelled denial", result, err)
	}
}

func TestModeApprovalReasons(t *testing.T) {
	tests := []struct {
		name         string
		mode         Mode
		access       Access
		wantApproval bool
	}{
		{name: "ask allows workspace reads", mode: ModeAsk, access: Access{Action: Read, Location: Workspace}},
		{name: "ask allows internal access", mode: ModeAsk, access: Access{Action: Internal}},
		{name: "ask prompts for workspace writes", mode: ModeAsk, access: Access{Action: Write, Location: Workspace}, wantApproval: true},
		{name: "ask prompts for sensitive workspace reads", mode: ModeAsk, access: Access{Action: Read, Location: Workspace, Sensitive: SecretFile}, wantApproval: true},
		{name: "auto edit allows workspace reads", mode: ModeAutoEdit, access: Access{Action: Read, Location: Workspace}},
		{name: "auto edit allows internal access", mode: ModeAutoEdit, access: Access{Action: Internal}},
		{name: "auto edit allows workspace writes", mode: ModeAutoEdit, access: Access{Action: Write, Location: Workspace}},
		{name: "auto edit prompts for sensitive workspace writes", mode: ModeAutoEdit, access: Access{Action: Write, Location: Workspace, Sensitive: SecretFile}, wantApproval: true},
		{name: "auto edit prompts for external writes", mode: ModeAutoEdit, access: Access{Action: Write, Location: OutsideWorkspace}, wantApproval: true},
		{name: "auto edit prompts for shell commands", mode: ModeAutoEdit, access: Access{Action: Execute}, wantApproval: true},
		{name: "ask prompts for network access", mode: ModeAsk, access: Access{Action: Network}, wantApproval: true},
		{name: "auto edit prompts for network access", mode: ModeAutoEdit, access: Access{Action: Network}, wantApproval: true},
		{name: "full access allows external reads", mode: ModeFullAccess, access: Access{Action: Read, Location: OutsideWorkspace}},
		{name: "full access allows external writes", mode: ModeFullAccess, access: Access{Action: Write, Location: OutsideWorkspace}},
		{name: "full access allows sensitive writes", mode: ModeFullAccess, access: Access{Action: Write, Location: Workspace, Sensitive: SecretFile}},
		{name: "full access allows shell commands", mode: ModeFullAccess, access: Access{Action: Execute}},
		{name: "full access allows network access", mode: ModeFullAccess, access: Access{Action: Network}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reason := approvalReason(test.mode, Request{Accesses: []Access{test.access}})
			if got := reason != ""; got != test.wantApproval {
				t.Fatalf("approvalReason(%q) = %q, approval=%t, want %t", test.mode, reason, got, test.wantApproval)
			}
		})
	}
}

func TestFullAccessAllowsToolsWithoutDeclaredAccess(t *testing.T) {
	if reason := approvalReason(ModeFullAccess, Request{}); reason != "" {
		t.Fatalf("approvalReason(%q) without accesses = %q, want no approval", ModeFullAccess, reason)
	}
}

func TestServiceCanChangeMode(t *testing.T) {
	service, err := NewService(t.TempDir(), ModeAsk, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := Request{Tool: "write", Accesses: []Access{{Action: Write, Path: "file.txt"}}}
	before, err := service.Authorize(context.Background(), request)
	if err != nil || before.Allowed {
		t.Fatalf("Authorize() before mode change = %+v, %v, want denied without approver", before, err)
	}
	service.SetMode(ModeAutoEdit)
	after, err := service.Authorize(context.Background(), request)
	if err != nil || !after.Allowed {
		t.Fatalf("Authorize() after mode change = %+v, %v, want allowed", after, err)
	}
}

func TestRemovedReadOnlyModeFallsBackToAsk(t *testing.T) {
	legacy := Mode("read_only")
	if legacy.Valid() {
		t.Fatal("removed read-only mode is still valid")
	}
	if got := NormalizeMode(legacy); got != ModeAsk {
		t.Fatalf("NormalizeMode(%q) = %q, want %q", legacy, got, ModeAsk)
	}
	if reason := approvalReason(legacy, Request{Accesses: []Access{{Action: Write, Location: Workspace}}}); reason == "" {
		t.Fatal("legacy read-only mode allowed a workspace write without approval")
	}
}
