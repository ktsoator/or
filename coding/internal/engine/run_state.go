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
	lifecycleTurnID      string
	lifecycleTurnStarted time.Time
	startedAt            time.Time
	pendingSteps         []stepCorrelationState
	pendingLifecycle     []positionedJournalEntry
	toolCalls            map[string]toolCorrelationState
	lastStepID           string
	lastRequestID        string
	autoCompactAttempted bool
	persistenceErr       error
}

func (s *Session) setRunState(
	ctx context.Context,
	runID, lifecycleTurnID string,
	startedAt time.Time,
) {
	s.runState.mu.Lock()
	s.runState.ctx = ctx
	s.runState.runID = runID
	s.runState.lifecycleTurnID = lifecycleTurnID
	s.runState.lifecycleTurnStarted = startedAt
	s.runState.startedAt = startedAt
	s.runState.pendingSteps = nil
	s.runState.pendingLifecycle = nil
	s.runState.toolCalls = nil
	s.runState.lastStepID = ""
	s.runState.lastRequestID = ""
	s.runState.autoCompactAttempted = false
	s.runState.persistenceErr = nil
	s.runState.mu.Unlock()
}

func (s *Session) clearRunState() {
	s.runState.mu.Lock()
	s.runState.ctx = nil
	s.runState.runID = ""
	s.runState.lifecycleTurnID = ""
	s.runState.lifecycleTurnStarted = time.Time{}
	s.runState.startedAt = time.Time{}
	s.runState.pendingSteps = nil
	s.runState.pendingLifecycle = nil
	s.runState.toolCalls = nil
	s.runState.lastStepID = ""
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

func (s *Session) activeRunState() (context.Context, string, time.Time) {
	s.runState.mu.RLock()
	defer s.runState.mu.RUnlock()
	return s.runState.ctx, s.runState.runID, s.runState.startedAt
}

type requestCorrelation struct {
	runID     string
	turnID    string
	stepID    string
	requestID string
}

type stepCorrelationState struct {
	runID            string
	stepID           string
	requestID        string
	lifecycleTurnID  string
	lifecycleDurable bool
	startedAt        time.Time
}

type toolCorrelationState struct {
	correlation requestCorrelation
	toolCallID  string
	toolName    string
	startedAt   time.Time
}

func (s *Session) beginStep(stepID string, startedAt time.Time) stepCorrelationState {
	s.runState.mu.Lock()
	defer s.runState.mu.Unlock()
	state := stepCorrelationState{
		runID: s.runState.runID, stepID: stepID,
		lifecycleTurnID: s.runState.lifecycleTurnID, startedAt: startedAt,
	}
	s.runState.pendingSteps = append(s.runState.pendingSteps, state)
	return state
}

func (s *Session) attachRequest(requestID string) requestCorrelation {
	s.runState.mu.Lock()
	defer s.runState.mu.Unlock()
	turnID := ""
	stepID := ""
	if count := len(s.runState.pendingSteps); count > 0 {
		current := &s.runState.pendingSteps[count-1]
		current.requestID = requestID
		turnID = current.lifecycleTurnID
		stepID = current.stepID
	}
	return requestCorrelation{
		runID:     s.runState.runID,
		turnID:    turnID,
		stepID:    stepID,
		requestID: requestID,
	}
}

func (s *Session) finishStep() stepCorrelationState {
	s.runState.mu.Lock()
	defer s.runState.mu.Unlock()
	if len(s.runState.pendingSteps) == 0 {
		return stepCorrelationState{}
	}
	step := s.runState.pendingSteps[0]
	s.runState.pendingSteps = s.runState.pendingSteps[1:]
	s.runState.lastStepID = step.stepID
	s.runState.lastRequestID = step.requestID
	return step
}

func (s *Session) finishOpenSteps() []stepCorrelationState {
	s.runState.mu.Lock()
	defer s.runState.mu.Unlock()
	open := append([]stepCorrelationState(nil), s.runState.pendingSteps...)
	s.runState.pendingSteps = nil
	if len(open) > 0 {
		last := open[len(open)-1]
		s.runState.lastStepID = last.stepID
		s.runState.lastRequestID = last.requestID
	}
	return open
}

func (s *Session) markStepDurable(stepID string) {
	s.runState.mu.Lock()
	defer s.runState.mu.Unlock()
	for index := range s.runState.pendingSteps {
		if s.runState.pendingSteps[index].stepID == stepID {
			s.runState.pendingSteps[index].lifecycleDurable = true
			return
		}
	}
}

func (s *Session) lifecycleIDs() (runID, turnID string) {
	s.runState.mu.RLock()
	defer s.runState.mu.RUnlock()
	return s.runState.runID, s.runState.lifecycleTurnID
}

func (s *Session) transitionLifecycleTurn(
	turnID string,
	startedAt time.Time,
) (runID, previousTurnID string, previousStartedAt time.Time) {
	s.runState.mu.Lock()
	defer s.runState.mu.Unlock()
	previousTurnID = s.runState.lifecycleTurnID
	previousStartedAt = s.runState.lifecycleTurnStarted
	s.runState.lifecycleTurnID = turnID
	s.runState.lifecycleTurnStarted = startedAt
	return s.runState.runID, previousTurnID, previousStartedAt
}

func (s *Session) activeLifecycleTurn() (runID, turnID string, startedAt time.Time) {
	s.runState.mu.RLock()
	defer s.runState.mu.RUnlock()
	return s.runState.runID, s.runState.lifecycleTurnID, s.runState.lifecycleTurnStarted
}

func (s *Session) queueLifecycle(entries ...positionedJournalEntry) {
	if len(entries) == 0 {
		return
	}
	s.runState.mu.Lock()
	s.runState.pendingLifecycle = append(s.runState.pendingLifecycle, entries...)
	s.runState.mu.Unlock()
}

func (s *Session) pendingLifecycle() []positionedJournalEntry {
	s.runState.mu.RLock()
	defer s.runState.mu.RUnlock()
	return append([]positionedJournalEntry(nil), s.runState.pendingLifecycle...)
}

func (s *Session) clearPendingLifecycle(count int) {
	if count <= 0 {
		return
	}
	s.runState.mu.Lock()
	if count >= len(s.runState.pendingLifecycle) {
		s.runState.pendingLifecycle = nil
	} else {
		s.runState.pendingLifecycle = append(
			[]positionedJournalEntry(nil),
			s.runState.pendingLifecycle[count:]...,
		)
	}
	s.runState.mu.Unlock()
}

func (s *Session) rewindPendingLifecycle(messageCount int) {
	s.runState.mu.Lock()
	for index := range s.runState.pendingLifecycle {
		if s.runState.pendingLifecycle[index].messageIndex > messageCount {
			s.runState.pendingLifecycle[index].messageIndex = messageCount
		}
	}
	s.runState.mu.Unlock()
}

func (s *Session) lastStepCorrelation() requestCorrelation {
	s.runState.mu.RLock()
	defer s.runState.mu.RUnlock()
	return requestCorrelation{
		runID:     s.runState.runID,
		turnID:    s.runState.lifecycleTurnID,
		stepID:    s.runState.lastStepID,
		requestID: s.runState.lastRequestID,
	}
}

func (s *Session) activeRequestCorrelation() requestCorrelation {
	s.runState.mu.RLock()
	defer s.runState.mu.RUnlock()
	for index := len(s.runState.pendingSteps) - 1; index >= 0; index-- {
		step := s.runState.pendingSteps[index]
		if step.requestID != "" {
			return requestCorrelation{
				runID: s.runState.runID, turnID: step.lifecycleTurnID,
				stepID: step.stepID, requestID: step.requestID,
			}
		}
	}
	return requestCorrelation{runID: s.runState.runID}
}

func (s *Session) correlateVisibleEvent(event *Event) {
	if event == nil || event.ProviderRequestID != "" {
		return
	}
	switch event.Type {
	case TextDelta, ThinkingDelta,
		ToolInputStarted, ToolInputDelta, ToolInputCompleted,
		ToolStarted, ToolFinished, MessageCompleted:
	default:
		return
	}
	if event.ToolCallID != "" {
		if tool, ok := s.toolState(event.ToolCallID); ok {
			event.ProviderRequestID = tool.correlation.requestID
			return
		}
	}
	event.ProviderRequestID = s.activeRequestCorrelation().requestID
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
	if len(s.runState.pendingSteps) > 0 {
		step := s.runState.pendingSteps[0]
		correlation.turnID = step.lifecycleTurnID
		correlation.stepID = step.stepID
		correlation.requestID = step.requestID
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
