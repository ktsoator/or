package tools

import (
	"reflect"
	"testing"

	"github.com/ktsoator/or/llm"
)

func TestCoreAndBrowserToolGroupsRemainSeparate(t *testing.T) {
	core, tasks := CoreTools(t.TempDir())
	defer tasks.Shutdown()
	browser := BrowserTools(t.TempDir())

	if got := toolNames(core); !reflect.DeepEqual(got, []string{
		"read", "view_image", "grep", "glob", "edit", "write", "bash", "task_stop", "todo_write",
	}) {
		t.Fatalf("core tools = %v", got)
	}
	if got := toolNames(browser); !reflect.DeepEqual(got, []string{
		"open_preview", "tabs_context", "inspect_browser",
	}) {
		t.Fatalf("browser tools = %v", got)
	}
}

func TestToolSupportsRequiredInputs(t *testing.T) {
	tool := Tool{RequiredInputs: []llm.ModelInput{llm.ModelInputImage}}
	if tool.Supports(llm.Model{Input: []llm.ModelInput{llm.ModelInputText}}) {
		t.Fatal("image tool supports a text-only model")
	}
	if tool.Supports(llm.Model{}) {
		t.Fatal("image tool supports a model with unknown modalities")
	}
	if !tool.Supports(llm.Model{Input: []llm.ModelInput{llm.ModelInputText, llm.ModelInputImage}}) {
		t.Fatal("image tool does not support a vision model")
	}
	if !(Tool{}).Supports(llm.Model{}) {
		t.Fatal("tool without requirements was rejected")
	}
}

func toolNames(toolSet []Tool) []string {
	names := make([]string, len(toolSet))
	for index, tool := range toolSet {
		names[index] = tool.Name()
	}
	return names
}
