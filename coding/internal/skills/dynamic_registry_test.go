package skills

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

func TestDynamicRegistryToolUsesCurrentSnapshotWithStableDefinition(t *testing.T) {
	initial := NewRegistry([]Skill{{
		Name: "review", Description: "review changes", Content: "old body", Dir: "/skills/review",
	}})
	dynamic := NewDynamicRegistry(initial)
	tool := dynamic.Tool()
	definition := tool.Definition

	first, err := tool.Execute(
		context.Background(),
		"call-1",
		json.RawMessage(`{"name":"review"}`),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := dynamicResultText(first); got == "" || !containsAll(got, "old body", `name="review"`) {
		t.Fatalf("initial tool result = %q", got)
	}

	dynamic.Replace(NewRegistry([]Skill{{
		Name: "test", Description: "run tests", Content: "new body", Dir: "/skills/test",
	}}))
	if !reflect.DeepEqual(tool.Definition, definition) {
		t.Fatalf("tool definition changed:\nbefore: %#v\nafter:  %#v", definition, tool.Definition)
	}
	if _, err := tool.Execute(
		context.Background(),
		"call-2",
		json.RawMessage(`{"name":"review"}`),
		nil,
	); err == nil {
		t.Fatal("removed skill should no longer resolve")
	}
	second, err := tool.Execute(
		context.Background(),
		"call-3",
		json.RawMessage(`{"name":"test"}`),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := dynamicResultText(second); !containsAll(got, "new body", `name="test"`) {
		t.Fatalf("replacement tool result = %q", got)
	}
}

func TestRegistryRevisionAndDiffIncludeBodyOnlyUpdates(t *testing.T) {
	before := NewRegistry([]Skill{
		{Name: "remove", Description: "remove", Content: "body"},
		{Name: "update", Description: "same", Content: "old"},
	})
	after := NewRegistry([]Skill{
		{Name: "add", Description: "add", Content: "body"},
		{Name: "update", Description: "same", Content: "new"},
	})

	if before.Revision() == after.Revision() {
		t.Fatal("different registry snapshots have the same revision")
	}
	delta := Diff(before, after)
	if got := skillNames(delta.Added); !reflect.DeepEqual(got, []string{"add"}) {
		t.Fatalf("added = %v", got)
	}
	if got := skillNames(delta.Updated); !reflect.DeepEqual(got, []string{"update"}) {
		t.Fatalf("updated = %v", got)
	}
	if !reflect.DeepEqual(delta.Removed, []string{"remove"}) {
		t.Fatalf("removed = %v", delta.Removed)
	}
}

func skillNames(items []Skill) []string {
	names := make([]string, len(items))
	for index, item := range items {
		names[index] = item.Name
	}
	return names
}

func dynamicResultText(result agent.ToolResult) string {
	var text string
	for _, content := range result.Content {
		if block, ok := content.(*llm.TextContent); ok {
			text += block.Text
		}
	}
	return text
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
