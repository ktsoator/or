package engine

import (
	"context"
	"sync"
	"time"
)

type sessionRunState struct {
	mu sync.RWMutex

	ctx                  context.Context
	startedAt            time.Time
	entryStart           int
	autoCompactAttempted bool
	persistenceErr       error
}

func (s *Session) setRunState(ctx context.Context, startedAt time.Time, entryStart int) {
	s.runState.mu.Lock()
	s.runState.ctx = ctx
	s.runState.startedAt = startedAt
	s.runState.entryStart = entryStart
	s.runState.autoCompactAttempted = false
	s.runState.persistenceErr = nil
	s.runState.mu.Unlock()
}

func (s *Session) clearRunState() {
	s.runState.mu.Lock()
	s.runState.ctx = nil
	s.runState.startedAt = time.Time{}
	s.runState.entryStart = 0
	s.runState.autoCompactAttempted = false
	s.runState.persistenceErr = nil
	s.runState.mu.Unlock()
}

func (s *Session) recordRunPersistenceError(err error) {
	if err == nil {
		return
	}
	s.runState.mu.Lock()
	if s.runState.persistenceErr == nil {
		s.runState.persistenceErr = err
	}
	s.runState.mu.Unlock()
}

func (s *Session) runPersistenceError() error {
	s.runState.mu.RLock()
	defer s.runState.mu.RUnlock()
	return s.runState.persistenceErr
}

func (s *Session) activeRunState() (context.Context, time.Time, int) {
	s.runState.mu.RLock()
	defer s.runState.mu.RUnlock()
	return s.runState.ctx, s.runState.startedAt, s.runState.entryStart
}
