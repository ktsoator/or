package tools

import (
	"path/filepath"
	"testing"

	"github.com/ktsoator/or/coding/internal/permission"
)

func TestBuiltInToolsDescribeAccess(t *testing.T) {
	root := t.TempDir()
	toolSet, tasks := CoreTools(root)
	defer tasks.Shutdown()
	background, err := tasks.Start("true", "Run test task", root)
	if err != nil {
		t.Fatal(err)
	}
	toolSet = append(toolSet, BrowserTools(root)...)
	byName := make(map[string]Tool, len(toolSet))
	for _, tool := range toolSet {
		byName[tool.Name()] = tool
	}

	tests := []struct {
		name   string
		tool   string
		args   map[string]any
		action permission.Action
		path   string
	}{
		{name: "workspace read", tool: "read", args: map[string]any{"path": "README.md"}, action: permission.Read, path: "README.md"},
		{name: "workspace grep", tool: "grep", args: map[string]any{}, action: permission.Read},
		{name: "workspace glob", tool: "glob", args: map[string]any{"path": "src"}, action: permission.Read, path: "src"},
		{name: "workspace edit", tool: "edit", args: map[string]any{"path": "main.go"}, action: permission.Write, path: "main.go"},
		{name: "workspace write", tool: "write", args: map[string]any{"path": "main.go"}, action: permission.Write, path: "main.go"},
		{name: "shell command", tool: "bash", args: map[string]any{"command": "pwd"}, action: permission.Execute},
		{name: "owned background output", tool: "read", args: map[string]any{"path": background.OutputPath}, action: permission.Internal},
		{name: "unowned background output", tool: "read", args: map[string]any{"path": filepath.Join(filepath.Dir(background.OutputPath), "other.log")}, action: permission.Read, path: filepath.Join(filepath.Dir(background.OutputPath), "other.log")},
		{name: "public browser navigation", tool: "open_preview", args: map[string]any{"url": "https://example.com"}, action: permission.Network},
		{name: "localhost browser navigation", tool: "open_preview", args: map[string]any{"url": "http://localhost:5173"}, action: permission.Network},
		{name: "workspace HTML preview", tool: "open_preview", args: map[string]any{"url": "web/index.html"}, action: permission.Read, path: "web/index.html"},
		{name: "browser tab metadata", tool: "tabs_context", args: map[string]any{}, action: permission.Internal},
		{name: "browser inspection", tool: "inspect_browser", args: map[string]any{}, action: permission.Network},
		{name: "owned task control", tool: "task_stop", args: map[string]any{}, action: permission.Internal},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tool, ok := byName[test.tool]
			if !ok {
				t.Fatalf("tool %q not registered", test.tool)
			}
			accesses := tool.Accesses(test.args)
			if len(accesses) != 1 {
				t.Fatalf("Accesses() = %+v, want one access", accesses)
			}
			got := accesses[0]
			if got.Action != test.action || got.Path != test.path {
				t.Fatalf("Accesses()[0] = %+v, want action=%q path=%q", got, test.action, test.path)
			}
		})
	}
}
