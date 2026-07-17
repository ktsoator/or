package tools

import "testing"

func TestCommandIsReadOnly(t *testing.T) {
	readOnly := []string{
		"ls -la",
		"cat foo.go",
		"grep -rn TODO .",
		"rg pattern",
		"git status",
		"git log --oneline -5",
		"git diff HEAD",
		"go version",
		"go doc net/http",
		"ls | grep foo",
		"cat a.go && wc -l b.go",
		"FOO=bar echo hi",
		"head -n 20 file; tail -n 5 file",
		"/bin/ls",
	}
	for _, cmd := range readOnly {
		if !commandIsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}

	mutating := []string{
		"",
		"rm -rf build",
		"echo hi > file",  // redirection
		"cat a >> b",      // append redirection
		"git push",        // mutating subcommand
		"git commit -m x", //
		"go build ./...",  // not in go read-only set
		"go test ./...",   //
		"sed -i s/a/b/ f", // in-place edit, excluded
		"ls && rm x",      // one mutating segment
		"cat $(rm -rf x)", // command substitution
		"echo `rm x`",     // backtick substitution
		"mv a b",          // not listed
		"npm install",     //
		"git",             // bare git, no subcommand
		"env rm -rf x",    // env runs an arbitrary program
		"xargs rm",        // command runner
		"sudo ls",         // privilege wrapper
	}
	for _, cmd := range mutating {
		if commandIsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

func TestToolIsReadOnlyPerCall(t *testing.T) {
	bash := Bash("/tmp", LocalOps{})
	if !bash.IsReadOnly(map[string]any{"command": "ls -la"}) {
		t.Error("ls should be read-only")
	}
	if bash.IsReadOnly(map[string]any{"command": "rm -rf x"}) {
		t.Error("rm should not be read-only")
	}

	// A tool with no per-call classifier falls back to its static flag.
	read := Read("/tmp", LocalOps{}, NewFileStateStore())
	if !read.IsReadOnly(nil) {
		t.Error("read should be read-only via static flag")
	}
}
