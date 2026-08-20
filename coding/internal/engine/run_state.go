package engine

import (
	"context"
	"sync"
	"time"
)

// runExecutionState contains run-scoped mechanics that are not lifecycle
// decisions. Run, Turn, and Step ownership lives in lifecycleCoordinator.
type runExecutionState struct {
	mu sync.RWMutex

	ctx                  context.Context
	autoCompactAttempted bool
	persistenceErr       error
}

func (s *Session) setRunExecutionState(ctx context.Context) {
	s.execution.mu.Lock()
	s.execution.ctx = ctx
	s.execution.autoCompactAttempted = false
	s.execution.persistenceErr = nil
	s.execution.mu.Unlock()
}

func (s *Session) clearRunExecutionState() {
	s.execution.mu.Lock()
	s.execution.ctx = nil
	s.execution.autoCompactAttempted = false
	s.execution.persistenceErr = nil
	s.execution.mu.Unlock()
}

func (s *Session) recordRunPersistenceError(err error) {
	if err == nil {
		return
	}
	s.execution.mu.Lock()
	if s.execution.persistenceErr == nil {
		s.execution.persistenceErr = err
	}
	s.execution.mu.Unlock()
}

func (s *Session) runPersistenceError() error {
	s.execution.mu.RLock()
	defer s.execution.mu.RUnlock()
	return s.execution.persistenceErr
}

func (s *Session) activeRunState() (context.Context, string, time.Time) {
	s.execution.mu.RLock()
	ctx := s.execution.ctx
	s.execution.mu.RUnlock()
	runID, startedAt := s.lifecycle.activeRun()
	return ctx, runID, startedAt
}

func (s *Session) autoCompactionWasAttempted() bool {
	s.execution.mu.RLock()
	defer s.execution.mu.RUnlock()
	return s.execution.autoCompactAttempted
}

func (s *Session) markAutoCompactionAttempted() {
	s.execution.mu.Lock()
	s.execution.autoCompactAttempted = true
	s.execution.mu.Unlock()
}
