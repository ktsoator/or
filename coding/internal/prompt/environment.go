package prompt

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Environment is the ambient machine state a coding agent has to know before it
// can write a correct shell command or reason about dates. It is captured from
// the host rather than guessed by the model, and it is refreshed with the rest
// of the project context, so a session that crosses midnight or switches branch
// does not keep reporting stale values.
type Environment struct {
	// OS and Arch are the Go runtime's platform identifiers, e.g. "darwin" and
	// "arm64". Shell commands differ between platforms, so the model needs them.
	OS   string
	Arch string
	// Shell is the user's login shell, e.g. "/bin/zsh". Empty when unknown.
	Shell string
	// Date is the local calendar date in YYYY-MM-DD form. A model's training
	// cutoff makes it a poor source for "today".
	Date string
	// GitRepo reports whether the workspace is inside a Git working tree.
	GitRepo bool
	// GitBranch is the checked-out branch, or a short commit for a detached
	// HEAD. Empty when the workspace is not a repository or HEAD is unreadable.
	GitBranch string
}

// DetectEnvironment captures the environment for a workspace root. Git state is
// read directly from the repository's HEAD file rather than by running git: it
// avoids a subprocess on a hot path, and it cannot report a working tree as
// clean when it is not, because it never claims to know.
func DetectEnvironment(root string) Environment {
	env := Environment{
		OS:    runtime.GOOS,
		Arch:  runtime.GOARCH,
		Shell: strings.TrimSpace(os.Getenv("SHELL")),
		Date:  time.Now().Format(time.DateOnly),
	}
	if gitDir, ok := findGitDir(root); ok {
		env.GitRepo = true
		env.GitBranch = readHeadBranch(gitDir)
	}
	return env
}

// findGitDir walks from dir to the filesystem root looking for the repository's
// git directory. A .git file (a worktree or submodule link) is resolved to the
// directory it points at.
func findGitDir(dir string) (string, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	for {
		candidate := filepath.Join(abs, ".git")
		if info, err := os.Stat(candidate); err == nil {
			if info.IsDir() {
				return candidate, true
			}
			if linked, ok := readGitDirLink(candidate); ok {
				return linked, true
			}
			// A .git file that cannot be resolved still marks a repository.
			return "", true
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", false
		}
		abs = parent
	}
}

// readGitDirLink resolves the "gitdir: <path>" pointer written by worktrees and
// submodules. A relative target resolves against the link file's directory.
func readGitDirLink(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	target, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
	if !ok {
		return "", false
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return "", false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	return filepath.Clean(target), true
}

// readHeadBranch returns the branch named by HEAD, or a short commit id when
// HEAD is detached. An unreadable HEAD yields an empty string rather than a
// guess.
func readHeadBranch(gitDir string) string {
	if gitDir == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	head := strings.TrimSpace(string(data))
	if ref, ok := strings.CutPrefix(head, "ref:"); ok {
		ref = strings.TrimSpace(ref)
		if branch, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
			return branch
		}
		return ref
	}
	if len(head) > 12 {
		return head[:12]
	}
	return head
}
