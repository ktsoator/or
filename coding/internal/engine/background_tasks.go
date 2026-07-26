package engine

import (
	"fmt"
	"time"

	"github.com/ktsoator/or/coding/internal/tools"
)

// BackgroundTask is the product-neutral state of one session-owned process.
type BackgroundTask struct {
	ID          string
	Command     string
	Description string
	Status      string
	OutputPath  string
	ExitCode    *int
	StartedAt   time.Time
	CompletedAt time.Time
}

// TaskOutput is a bounded tail of one background task's combined output.
type TaskOutput struct {
	Content   string
	Truncated bool
}

func projectBackgroundTask(state tools.TaskState) BackgroundTask {
	return BackgroundTask{
		ID:          state.ID,
		Command:     state.Command,
		Description: state.Description,
		Status:      string(state.Status),
		OutputPath:  state.OutputPath,
		ExitCode:    state.ExitCode,
		StartedAt:   state.StartedAt,
		CompletedAt: state.CompletedAt,
	}
}

// Tasks returns every managed task's latest state in creation order.
func (s *Session) Tasks() []BackgroundTask {
	if s.tasks == nil {
		return nil
	}
	states := s.tasks.Snapshot()
	tasks := make([]BackgroundTask, 0, len(states))
	for _, state := range states {
		tasks = append(tasks, projectBackgroundTask(state))
	}
	return tasks
}

// StopTask terminates one session-owned background task.
func (s *Session) StopTask(id string) error {
	if s.tasks == nil {
		return fmt.Errorf("%w: %s", tools.ErrTaskNotFound, id)
	}
	return s.tasks.Stop(id)
}

// TaskOutput returns a bounded tail of one session-owned task's logs.
func (s *Session) TaskOutput(id string) (TaskOutput, error) {
	if s.tasks == nil {
		return TaskOutput{}, fmt.Errorf("%w: %s", tools.ErrTaskNotFound, id)
	}
	output, err := s.tasks.ReadOutput(id, 0)
	if err != nil {
		return TaskOutput{}, err
	}
	return TaskOutput{Content: output.Content, Truncated: output.Truncated}, nil
}
