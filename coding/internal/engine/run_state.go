package engine

import (
	"context"
	"sync"
	"time"
)

type sessionRunState struct {
	mu sync.RWMutex

	ctx                  context.Context
	runID                string
	startedAt            time.Time
	entryStart           int
	pendingTurns         []turnCorrelationState
	toolCalls            map[string]toolCorrelationState
	lastTurnID           string
	lastRequestID        string
	autoCompactAttempted bool
	persistenceErr       error
}

func (s *Session) setRunState(ctx context.Context, runID string, startedAt time.Time, entryStart int) {
	s.runState.mu.Lock()
	s.runState.ctx = ctx
	s.runState.runID = runID
	s.runState.startedAt = startedAt
	s.runState.entryStart = entryStart
	s.runState.pendingTurns = nil
	s.runState.toolCalls = nil
	s.runState.lastTurnID = ""
	s.runState.lastRequestID = ""
	s.runState.autoCompactAttempted = false
	s.runState.persistenceErr = nil
	s.runState.mu.Unlock()
}

func (s *Session) clearRunState() {
	s.runState.mu.Lock()
	s.runState.ctx = nil
	s.runState.runID = ""
	s.runState.startedAt = time.Time{}
	s.runState.entryStart = 0
	s.runState.pendingTurns = nil
	s.runState.toolCalls = nil
	s.runState.lastTurnID = ""
	s.runState.lastRequestID = ""
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

type requestCorrelation struct {
	runID     string
	turnID    string
	requestID string
}

type turnCorrelationState struct {
	turnID    string
	requestID string
	startedAt time.Time
}

type toolCorrelationState struct {
	correlation requestCorrelation
	toolCallID  string
	toolName    string
	startedAt   time.Time
}

func (s *Session) beginTurn(turnID string, startedAt time.Time) (runID string) {
	s.runState.mu.Lock()
	defer s.runState.mu.Unlock()
	s.runState.pendingTurns = append(s.runState.pendingTurns, turnCorrelationState{
		turnID: turnID, startedAt: startedAt,
	})
	return s.runState.runID
}

func (s *Session) attachRequest(requestID string) requestCorrelation {
	s.runState.mu.Lock()
	defer s.runState.mu.Unlock()
	turnID := ""
	if count := len(s.runState.pendingTurns); count > 0 {
		current := &s.runState.pendingTurns[count-1]
		current.requestID = requestID
		turnID = current.turnID
	}
	return requestCorrelation{
		runID:     s.runState.runID,
		turnID:    turnID,
		requestID: requestID,
	}
}

func (s *Session) finishTurn() (requestCorrelation, time.Time) {
	s.runState.mu.Lock()
	defer s.runState.mu.Unlock()
	if len(s.runState.pendingTurns) == 0 {
		return requestCorrelation{runID: s.runState.runID}, time.Time{}
	}
	turn := s.runState.pendingTurns[0]
	s.runState.pendingTurns = s.runState.pendingTurns[1:]
	correlation := requestCorrelation{
		runID:     s.runState.runID,
		turnID:    turn.turnID,
		requestID: turn.requestID,
	}
	s.runState.lastTurnID = turn.turnID
	s.runState.lastRequestID = turn.requestID
	return correlation, turn.startedAt
}

func (s *Session) lastTurnCorrelation() requestCorrelation {
	s.runState.mu.RLock()
	defer s.runState.mu.RUnlock()
	return requestCorrelation{
		runID:     s.runState.runID,
		turnID:    s.runState.lastTurnID,
		requestID: s.runState.lastRequestID,
	}
}

func (s *Session) beginTool(
	toolCallID, toolName string,
	startedAt time.Time,
) (toolCorrelationState, bool) {
	s.runState.mu.Lock()
	defer s.runState.mu.Unlock()
	if existing, ok := s.runState.toolCalls[toolCallID]; ok {
		return existing, false
	}
	correlation := requestCorrelation{runID: s.runState.runID}
	if len(s.runState.pendingTurns) > 0 {
		turn := s.runState.pendingTurns[0]
		correlation.turnID = turn.turnID
		correlation.requestID = turn.requestID
	}
	state := toolCorrelationState{
		correlation: correlation,
		toolCallID:  toolCallID,
		toolName:    toolName,
		startedAt:   startedAt,
	}
	if s.runState.toolCalls == nil {
		s.runState.toolCalls = make(map[string]toolCorrelationState)
	}
	s.runState.toolCalls[toolCallID] = state
	return state, true
}

func (s *Session) finishTool(toolCallID string) (toolCorrelationState, bool) {
	s.runState.mu.Lock()
	defer s.runState.mu.Unlock()
	state, ok := s.runState.toolCalls[toolCallID]
	if ok {
		delete(s.runState.toolCalls, toolCallID)
	}
	return state, ok
}

func (s *Session) toolState(toolCallID string) (toolCorrelationState, bool) {
	s.runState.mu.RLock()
	defer s.runState.mu.RUnlock()
	state, ok := s.runState.toolCalls[toolCallID]
	return state, ok
}
