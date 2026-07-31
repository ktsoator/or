package permission

import (
	"context"
	"path/filepath"
	"testing"
)

func TestClassifySensitive(t *testing.T) {
	tests := []struct {
		name string
		path string
		want SensitiveKind
	}{
		{"empty", "", NotSensitive},
		{"source file", "/ws/internal/tools/grep.go", NotSensitive},
		{"readme", "/ws/README.md", NotSensitive},

		{"dotenv", "/ws/.env", SecretFile},
		{"dotenv local", "/ws/.env.local", SecretFile},
		{"dotenv production", "/ws/.env.production", SecretFile},
		{"dotenv example stays ordinary", "/ws/.env.example", NotSensitive},
		{"dotenv sample stays ordinary", "/ws/.env.sample", NotSensitive},
		{"dotenv template stays ordinary", "/ws/.env.template", NotSensitive},
		{"environment prefix is not dotenv", "/ws/.environment", NotSensitive},

		{"private key", "/ws/certs/server.key", SecretFile},
		{"pem", "/ws/certs/chain.pem", SecretFile},
		{"uppercase extension", "/ws/certs/CHAIN.PEM", SecretFile},
		{"ssh key by name", "/ws/deploy/id_ed25519", SecretFile},
		{"npmrc", "/ws/.npmrc", SecretFile},
		{"netrc", "/home/u/.netrc", SecretFile},

		{"ssh directory", "/home/u/.ssh/known_hosts", SecretFile},
		{"aws directory", "/home/u/.aws/config", SecretFile},
		{"kube directory", "/home/u/.kube/config", SecretFile},

		{"git config may embed a token", "/ws/.git/config", SecretFile},
		{"git credentials", "/ws/.git/.git-credentials", SecretFile},
		{"git hook", "/ws/.git/hooks/pre-commit", RepositoryInternals},
		{"git head", "/ws/.git/HEAD", RepositoryInternals},
		{"nested worktree git dir", "/ws/sub/.git/hooks/pre-push", RepositoryInternals},
		{"gitignore is ordinary", "/ws/.gitignore", NotSensitive},
		{"github workflow is ordinary", "/ws/.github/workflows/ci.yml", NotSensitive},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ClassifySensitive(filepath.FromSlash(test.path)); got != test.want {
				t.Fatalf("ClassifySensitive(%q) = %q, want %q", test.path, got, test.want)
			}
		})
	}
}

func TestSensitiveWorkspaceFileStillNeedsApproval(t *testing.T) {
	workspace := t.TempDir()

	// Auto-edit exists so ordinary workspace edits stop prompting. It must not
	// also hand over the project's credentials or its Git state.
	cases := []struct {
		name      string
		action    Action
		path      string
		wantAsked bool
	}{
		{"ordinary read", Read, "internal/tools/grep.go", false},
		{"ordinary write", Write, "internal/tools/grep.go", false},
		{"secret read", Read, ".env", true},
		{"secret write", Write, ".env", true},
		{"key read", Read, "certs/server.key", true},
		{"git hook write", Write, ".git/hooks/pre-commit", true},
		{"git config read", Read, ".git/config", true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			asked := false
			service, err := NewService(workspace, PolicyForMode(ModeAutoEdit),
				approverFunc(func(_ context.Context, _ ApprovalRequest) (ApprovalResponse, error) {
					asked = true
					return ApprovalResponse{Choice: AllowOnce}, nil
				}))
			if err != nil {
				t.Fatal(err)
			}
			decision, err := service.Authorize(context.Background(), Request{
				Tool:     "read",
				Accesses: []Access{{Action: test.action, Path: test.path}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if decision.Behavior != Allow {
				t.Fatalf("Authorize(%s %s) = %q (%s), want it allowed after approval",
					test.action, test.path, decision.Behavior, decision.Reason)
			}
			if asked != test.wantAsked {
				t.Fatalf("Authorize(%s %s) asked the user = %t, want %t",
					test.action, test.path, asked, test.wantAsked)
			}
		})
	}
}

func TestReadOnlyModeStillDeniesSensitiveWrites(t *testing.T) {
	workspace := t.TempDir()
	service, err := NewService(workspace, PolicyForMode(ModeReadOnly), nil)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := service.Authorize(t.Context(), Request{
		Tool:     "write",
		Accesses: []Access{{Action: Write, Path: ".env"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Read-only denies every write; sensitivity must not soften that to Ask.
	if decision.Behavior != Deny {
		t.Fatalf("Authorize() = %q (%s), want deny", decision.Behavior, decision.Reason)
	}
}
