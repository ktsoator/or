package engine

import (
	"encoding/json"
	"fmt"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/transcript"
)

const todoProjectionKey = "todos"

// TodoItem is one product-neutral execution-checklist row.
type TodoItem struct {
	Content string `json:"content"`
	Status  string `json:"status"`
}

// TodoSnapshot is the current turn's complete execution checklist. A nil
// snapshot means the current turn has not written a checklist; a non-nil empty
// Todos slice means todo_write explicitly cleared it.
type TodoSnapshot struct {
	Todos []TodoItem `json:"todos"`
}

type todoProjectionUnit struct {
	current *TodoSnapshot
	err     error
}

func newTodoProjectionUnit() *todoProjectionUnit { return &todoProjectionUnit{} }

func (*todoProjectionUnit) ProjectionKey() string { return todoProjectionKey }

func (p *todoProjectionUnit) ApplyProjection(event transcript.ProjectionEvent) {
	switch event.Entry.Type {
	case transcript.TurnStartEntry:
		p.current = nil
		p.err = nil

	case transcript.ToolOutcomeEntry:
		outcome := event.Entry.ToolOutcome
		if outcome == nil || outcome.Status != agent.ToolOutcomeSuccess ||
			outcome.DataKind != kindTodoList {
			return
		}
		var stored tools.TodoSnapshot
		if err := json.Unmarshal(outcome.Data, &stored); err != nil {
			p.err = fmt.Errorf("decode todo outcome %s: %w", event.Entry.ID, err)
			return
		}
		projected := projectTodoSnapshot(stored)
		p.current = &projected
		p.err = nil
	}
}

func (p *todoProjectionUnit) SnapshotProjection() (any, error) {
	if p.err != nil {
		return nil, p.err
	}
	return cloneTodoSnapshot(p.current), nil
}

func projectTodoSnapshot(source tools.TodoSnapshot) TodoSnapshot {
	items := make([]TodoItem, len(source.Todos))
	for index, item := range source.Todos {
		items[index] = TodoItem{Content: item.Content, Status: string(item.Status)}
	}
	return TodoSnapshot{Todos: items}
}

func cloneTodoSnapshot(source *TodoSnapshot) *TodoSnapshot {
	if source == nil {
		return nil
	}
	items := make([]TodoItem, len(source.Todos))
	copy(items, source.Todos)
	return &TodoSnapshot{Todos: items}
}

// Todos returns the latest committed checklist for the current turn. The
// returned value is detached from the live projection.
func (s *Session) Todos() *TodoSnapshot {
	todos, err := s.journal.todoSnapshot()
	if err != nil {
		// Engine-produced logs are validated before Session is exposed. Avoid
		// presenting a partial or malformed checklist as committed state.
		return nil
	}
	return todos
}
