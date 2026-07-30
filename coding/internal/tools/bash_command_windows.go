//go:build windows

package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const gitBashPathEnv = "CODING_GIT_BASH_PATH"

func findBash() (string, error) {
	if configured := strings.Trim(strings.TrimSpace(os.Getenv(gitBashPathEnv)), `"`); configured != "" {
		if isExecutableFile(configured) {
			return configured, nil
		}
		return "", fmt.Errorf("%s does not point to bash.exe: %s", gitBashPathEnv, configured)
	}

	var candidates []string
	addRoot := func(root string) {
		if root != "" {
			candidates = append(candidates, filepath.Join(root, "Git", "bin", "bash.exe"))
		}
	}
	addRoot(os.Getenv("ProgramW6432"))
	addRoot(os.Getenv("ProgramFiles"))
	addRoot(os.Getenv("ProgramFiles(x86)"))
	if localAppData := os.Getenv("LocalAppData"); localAppData != "" {
		candidates = append(candidates, filepath.Join(localAppData, "Programs", "Git", "bin", "bash.exe"))
	}
	if userProfile := os.Getenv("UserProfile"); userProfile != "" {
		candidates = append(candidates, filepath.Join(userProfile, "scoop", "apps", "git", "current", "bin", "bash.exe"))
	}

	if git, err := exec.LookPath("git.exe"); err == nil {
		gitDir := filepath.Dir(git)
		candidates = append(
			candidates,
			filepath.Join(filepath.Dir(gitDir), "bin", "bash.exe"),
			filepath.Join(filepath.Dir(filepath.Dir(gitDir)), "bin", "bash.exe"),
		)
	}

	seen := make(map[string]bool)
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		key := strings.ToLower(candidate)
		if seen[key] {
			continue
		}
		seen[key] = true
		if isExecutableFile(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("Git for Windows Bash was not found; install Git for Windows or set %s to bash.exe", gitBashPathEnv)
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func configureBashEnvironment(cmd *exec.Cmd, dir string) {
	cmd.Env = append(
		os.Environ(),
		"CHERE_INVOKING=1",
		"CODING_WORKSPACE="+windowsPathToBash(dir),
	)
}

func windowsPathToBash(path string) string {
	path = strings.ReplaceAll(path, `\`, "/")
	if len(path) >= 2 &&
		((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) &&
		path[1] == ':' {
		drive := strings.ToLower(path[:1])
		rest := path[2:]
		if rest == "" {
			rest = "/"
		} else if !strings.HasPrefix(rest, "/") {
			return path
		}
		return "/" + drive + rest
	}
	return path
}
