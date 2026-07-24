package prompt

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDetectEnvironmentReadsTheCheckedOutBranch(t *testing.T) {
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, ".git", "HEAD"), "ref: refs/heads/feature/one\n")

	env := DetectEnvironment(workspace)
	if !env.GitRepo || env.GitBranch != "feature/one" {
		t.Fatalf("git state = %+v, want the feature/one branch", env)
	}
	if env.OS != runtime.GOOS || env.Arch != runtime.GOARCH {
		t.Errorf("platform = %s/%s, want %s/%s", env.OS, env.Arch, runtime.GOOS, runtime.GOARCH)
	}
	if len(env.Date) != len("2006-01-02") {
		t.Errorf("date = %q, want a calendar date", env.Date)
	}
}

func TestDetectEnvironmentShortensADetachedHead(t *testing.T) {
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, ".git", "HEAD"), strings.Repeat("a", 40))

	env := DetectEnvironment(workspace)
	if !env.GitRepo || env.GitBranch != strings.Repeat("a", 12) {
		t.Fatalf("detached head = %+v, want a short commit", env)
	}
}

func TestDetectEnvironmentFindsTheRepositoryFromASubdirectory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main")
	nested := filepath.Join(root, "internal", "pkg")
	writeFile(t, filepath.Join(nested, "keep"), "")

	if env := DetectEnvironment(nested); !env.GitRepo || env.GitBranch != "main" {
		t.Fatalf("nested detection = %+v, want the enclosing repository", env)
	}
}

func TestDetectEnvironmentResolvesAWorktreeLink(t *testing.T) {
	repo := t.TempDir()
	gitDir := filepath.Join(repo, "actual-git-dir")
	writeFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/worktree-branch")

	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, ".git"), "gitdir: "+gitDir+"\n")

	if env := DetectEnvironment(workspace); !env.GitRepo || env.GitBranch != "worktree-branch" {
		t.Fatalf("worktree detection = %+v, want the linked git dir", env)
	}
}

func TestDetectEnvironmentReportsNoRepositoryOutsideOne(t *testing.T) {
	env := DetectEnvironment(t.TempDir())
	if env.GitRepo || env.GitBranch != "" {
		t.Fatalf("environment = %+v, want no repository", env)
	}
}
