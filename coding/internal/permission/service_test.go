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
