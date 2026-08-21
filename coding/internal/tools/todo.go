package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

const (
	ToolNameTodoWrite = "todo_write"
	maxTodoItems      = 20
	maxTodoContent    = 200
)

// TodoStatus is the model-visible lifecycle state of one checklist item.
type TodoStatus string

const (
	TodoPending    TodoStatus = "pending"
	TodoInProgress TodoStatus = "in_progress"
	TodoCompleted  TodoStatus = "completed"
)

// TodoItem is one concrete step in the current turn's execution checklist.
type TodoItem struct {
	Content string     `json:"content" jsonschema:"description=One short imperative step,minLength=1,maxLength=200"`
	Status  TodoStatus `json:"status" jsonschema:"description=The step's current execution state,enum=pending,enum=in_progress,enum=completed"`
}

// TodoSnapshot is the complete, canonical checklist returned to product
// projections. Every successful todo_write replaces the previous snapshot.
type TodoSnapshot struct {
	Todos []TodoItem `json:"todos"`
}

type todoWriteArgs struct {
	Todos []TodoItem `json:"todos" jsonschema:"description=The complete checklist replacing the previous list,maxItems=20"`
}

func todoWriteTool() Tool {
	def := llm.MustTool[todoWriteArgs](ToolNameTodoWrite, todoWriteText.description)
	return Tool{
		AgentTool: agent.AgentTool{
			Definition: def,
			Label:      "Update todos",
			Execute: func(
				_ context.Context,
				_ string,
				raw json.RawMessage,
				_ func(agent.ToolProgress),
			) (agent.ToolResult, error) {
				var in todoWriteArgs
				if err := json.Unmarshal(raw, &in); err != nil {
					return agent.ToolResult{}, err
				}
				todos, err := normalizeTodos(in.Todos)
				if err != nil {
					return textResult(err.Error()), err
				}
				return resultWith(
					renderTodoUpdate(todos),
					TodoSnapshot{Todos: todos},
				), nil
			},
		},
		AccessFor: InternalAccess,
	}
}

func normalizeTodos(items []TodoItem) ([]TodoItem, error) {
	if items == nil {
		return nil, fmt.Errorf("todo_write: todos must be an array")
	}
	if len(items) > maxTodoItems {
		return nil, fmt.Errorf(
			"todo_write: todos must contain at most %d items (got %d)",
			maxTodoItems,
			len(items),
		)
	}

	todos := make([]TodoItem, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			return nil, fmt.Errorf("todo_write: every item needs non-empty content")
		}
		if utf8.RuneCountInString(content) > maxTodoContent {
			return nil, fmt.Errorf(
				"todo_write: item %q is longer than %d characters",
				content,
				maxTodoContent,
			)
		}
		if seen[content] {
			return nil, fmt.Errorf("todo_write: duplicate item %q", content)
		}
		seen[content] = true
		if !validTodoStatus(item.Status) {
			return nil, fmt.Errorf("todo_write: item %q has invalid status %q", content, item.Status)
		}
		todos = append(todos, TodoItem{Content: content, Status: item.Status})
	}
	return todos, nil
}

func validTodoStatus(status TodoStatus) bool {
	switch status {
	case TodoPending, TodoInProgress, TodoCompleted:
		return true
	default:
		return false
	}
}

func renderTodoUpdate(todos []TodoItem) string {
	counts := map[TodoStatus]int{}
	for _, todo := range todos {
		counts[todo.Status]++
	}
	return fmt.Sprintf(
		"Updated todo list: %d pending, %d in progress, %d completed.",
		counts[TodoPending],
		counts[TodoInProgress],
		counts[TodoCompleted],
	)
}
