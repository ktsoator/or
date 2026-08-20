package engine

import (
	"context"
	"sync"
)

// runExecutionState contains run-scoped mechanics that are not lifecycle
// decisions. Run, Turn, and Step ownership lives in lifecycleCoordinator.
type runExecutionState struct {
	mu sync.RWMutex

	ctx                context.Context
	autoCompactClaimed bool
	persistenceErr     error
}

func (state *runExecutionState) begin(ctx context.Context) {
	state.mu.Lock()
	state.ctx = ctx
	state.autoCompactClaimed = false
	state.persistenceErr = nil
	state.mu.Unlock()
}

func (state *runExecutionState) end() {
	state.mu.Lock()
	state.ctx = nil
	state.autoCompactClaimed = false
	state.persistenceErr = nil
	state.mu.Unlock()
}

func (state *runExecutionState) recordPersistenceError(err error) {
	if err == nil {
		return
	}
	state.mu.Lock()
	if state.persistenceErr == nil {
		state.persistenceErr = err
	}
	state.mu.Unlock()
}

func (state *runExecutionState) persistenceError() error {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.persistenceErr
}

func (state *runExecutionState) runContext() context.Context {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.ctx
}

func (state *runExecutionState) claimAutoCompaction() bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.autoCompactClaimed {
		return false
	}
	state.autoCompactClaimed = true
	return true
}

func (state *runExecutionState) releaseAutoCompactionClaim() {
	state.mu.Lock()
	state.autoCompactClaimed = false
	state.mu.Unlock()
}
