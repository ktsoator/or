package tools

import (
	"reflect"
	"testing"
)

func TestCoreAndBrowserToolGroupsRemainSeparate(t *testing.T) {
	core, tasks := CoreTools(t.TempDir())
	defer tasks.Shutdown()
	browser := BrowserTools(t.TempDir())

	if got := toolNames(core); !reflect.DeepEqual(got, []string{
		"read", "grep", "glob", "edit", "write", "bash", "task_stop", "todo_write",
	}) {
		t.Fatalf("core tools = %v", got)
	}
	if got := toolNames(browser); !reflect.DeepEqual(got, []string{
		"open_preview", "tabs_context", "inspect_browser",
	}) {
		t.Fatalf("browser tools = %v", got)
	}
}

func toolNames(toolSet []Tool) []string {
	names := make([]string, len(toolSet))
	for index, tool := range toolSet {
		names[index] = tool.Name()
	}
	return names
}
