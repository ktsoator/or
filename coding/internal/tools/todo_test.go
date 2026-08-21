package tools

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/permission"
)

func todoArgs(t *testing.T, todos []TodoItem) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(todoWriteArgs{Todos: todos})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestTodoWriteReturnsCanonicalWholeList(t *testing.T) {
	tool := todoWriteTool()
	result, err := execute(t, tool, todoArgs(t, []TodoItem{
		{Content: "  Inspect authentication  ", Status: TodoCompleted},
		{Content: "Fix token refresh", Status: TodoInProgress},
		{Content: "Run tests", Status: TodoPending},
	}))
	if err != nil {
		t.Fatal(err)
	}

	want := []TodoItem{
		{Content: "Inspect authentication", Status: TodoCompleted},
		{Content: "Fix token refresh", Status: TodoInProgress},
		{Content: "Run tests", Status: TodoPending},
	}
	snapshot, ok := result.Outcome.Data.(TodoSnapshot)
	if !ok {
		t.Fatalf("outcome data = %T, want TodoSnapshot", result.Outcome.Data)
	}
	if result.Outcome.Status != agent.ToolOutcomeSuccess || !reflect.DeepEqual(snapshot.Todos, want) {
		t.Fatalf("result = %#v, want successful snapshot %#v", result, want)
	}
	if text := resultText(t, result); text != "Updated todo list: 1 pending, 1 in progress, 1 completed." {
		t.Fatalf("result text = %q", text)
	}
}

func TestTodoWriteAcceptsAnExplicitEmptyList(t *testing.T) {
	result, err := execute(t, todoWriteTool(), json.RawMessage(`{"todos":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := result.Outcome.Data.(TodoSnapshot)
	if snapshot.Todos == nil || len(snapshot.Todos) != 0 {
		t.Fatalf("empty snapshot = %#v, want non-nil empty list", snapshot.Todos)
	}
	if text := resultText(t, result); text != "Updated todo list: 0 pending, 0 in progress, 0 completed." {
		t.Fatalf("result text = %q", text)
	}
}

func TestTodoWriteRejectsInvalidLists(t *testing.T) {
	tooMany := make([]TodoItem, maxTodoItems+1)
	for index := range tooMany {
		tooMany[index] = TodoItem{Content: string(rune('A' + index)), Status: TodoPending}
	}
	tests := map[string]struct {
		raw  json.RawMessage
		want string
	}{
		"missing list": {
			raw:  json.RawMessage(`{}`),
			want: "must be an array",
		},
		"blank content": {
			raw:  todoArgs(t, []TodoItem{{Content: "  ", Status: TodoPending}}),
			want: "non-empty content",
		},
		"duplicate after trimming": {
			raw: todoArgs(t, []TodoItem{
				{Content: "Run tests", Status: TodoInProgress},
				{Content: " Run tests ", Status: TodoPending},
			}),
			want: "duplicate item",
		},
		"oversized content": {
			raw: todoArgs(t, []TodoItem{{
				Content: strings.Repeat("x", maxTodoContent+1), Status: TodoPending,
			}}),
			want: "longer than 200 characters",
		},
		"invalid status": {
			raw:  todoArgs(t, []TodoItem{{Content: "Run tests", Status: "started"}}),
			want: "invalid status",
		},
		"too many items": {
			raw:  todoArgs(t, tooMany),
			want: "at most 20 items",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			result, err := execute(t, todoWriteTool(), test.raw)
			if err == nil {
				t.Fatal("invalid todo list was accepted")
			}
			if !strings.Contains(resultText(t, result), test.want) {
				t.Fatalf("result = %q, want it to mention %q", resultText(t, result), test.want)
			}
		})
	}
}

func TestTodoWriteSchemaAndDescriptionCarryModelPolicy(t *testing.T) {
	tool := todoWriteTool()
	schema := string(tool.Definition.Parameters)
	for _, want := range []string{
		`"maxItems":20`,
		`"maxLength":200`,
		`"pending"`,
		`"in_progress"`,
		`"completed"`,
	} {
		if !strings.Contains(schema, want) {
			t.Errorf("schema missing %q:\n%s", want, schema)
		}
	}
	for _, want := range []string{"ENTIRE list", "trivial single-step", "genuinely running in parallel"} {
		if !strings.Contains(tool.Definition.Description, want) {
			t.Errorf("description missing %q:\n%s", want, tool.Definition.Description)
		}
	}
	accesses := tool.Accesses(nil)
	if len(accesses) != 1 || accesses[0].Action != permission.Internal {
		t.Fatalf("accesses = %#v, want one internal access", accesses)
	}
}
