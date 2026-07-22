package tools

import (
	"testing"

	"github.com/ktsoator/or/coding/internal/permission"
)

func TestBuiltInToolsDescribeAccess(t *testing.T) {
	toolSet, shells := CodingToolsWithShells(t.TempDir(), LocalOps{})
	defer shells.Shutdown()
	byName := make(map[string]Tool, len(toolSet))
	for _, tool := range toolSet {
		byName[tool.Name()] = tool
	}

	tests := []struct {
		tool    string
		args    map[string]any
		action  permission.Action
		path    string
		command string
	}{
		{tool: "read", args: map[string]any{"path": "README.md"}, action: permission.Read, path: "README.md"},
		{tool: "grep", args: map[string]any{}, action: permission.Read},
		{tool: "glob", args: map[string]any{"path": "src"}, action: permission.Read, path: "src"},
		{tool: "ls", args: map[string]any{"path": "src"}, action: permission.Read, path: "src"},
		{tool: "edit", args: map[string]any{"path": "main.go"}, action: permission.Write, path: "main.go"},
		{tool: "write", args: map[string]any{"path": "main.go"}, action: permission.Write, path: "main.go"},
		{tool: "bash", args: map[string]any{"command": "pwd"}, action: permission.Execute, command: "pwd"},
		{tool: "bash_output", args: map[string]any{}, action: permission.Internal},
		{tool: "kill_bash", args: map[string]any{}, action: permission.Internal},
	}
	for _, test := range tests {
		t.Run(test.tool, func(t *testing.T) {
			tool, ok := byName[test.tool]
			if !ok {
				t.Fatalf("tool %q not registered", test.tool)
			}
			accesses := tool.Accesses(test.args)
			if len(accesses) != 1 {
				t.Fatalf("Accesses() = %+v, want one access", accesses)
			}
			got := accesses[0]
			if got.Action != test.action || got.Path != test.path || got.Command != test.command {
				t.Fatalf("Accesses()[0] = %+v, want action=%q path=%q command=%q", got, test.action, test.path, test.command)
			}
		})
	}
}
