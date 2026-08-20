package engine

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ktsoator/or/coding/internal/observability"
	"github.com/ktsoator/or/coding/internal/transcript"
)

// lifecycleCoordinator is the live Run/Turn/Step state machine. Callers submit
// execution facts and persist or project the detached decisions it returns.
type lifecycleCoordinator struct {
	mu sync.RWMutex

	runID         string
	runStartedAt  time.Time
	turnID        string
	turnStartedAt time.Time
	pendingSteps  []stepCorrelationState
	pending       []positionedJournalEntry
	toolCalls     map[string]toolCorrelationState
	lastStep      requestCorrelation
}

type lifecycleRunStarted struct {
	runID     string
	turnID    string
	startedAt time.Time
}

type lifecycleTurnTransition struct {
	runID             string
	previousTurnID    string
	previousStartedAt time.Time
	nextTurnID        string
	nextStartedAt     time.Time
}

type lifecycleStepCheckpoint struct {
	step         stepCorrelationState
	entries      []positionedJournalEntry
	pendingCount int
}

type lifecycleStepCompleted struct {
	step        stepCorrelationState
	status      string
	errorCode   string
	completedAt time.Time
}

type lifecycleStepDiscarded struct {
	correlation requestCorrelation
	reason      string
}

type lifecycleRunTerminal struct {
	runID         string
	turnID        string
	runStartedAt  time.Time
	turnStartedAt time.Time
	completedAt   time.Time
	status        transcript.LifecycleStatus
	reason        string
	entries       []positionedJournalEntry
	pendingCount  int
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

func (c *lifecycleCoordinator) startRun(
	messageIndex int,
	startedAt time.Time,
) lifecycleRunStarted {
	runID := observability.NewID("run")
	turnID := observability.NewID("turn")
	return c.startRunWithIDs(messageIndex, runID, turnID, startedAt)
}

func (c *lifecycleCoordinator) startRunWithIDs(
	messageIndex int,
	runID, turnID string,
	startedAt time.Time,
) lifecycleRunStarted {
	startedAt = startedAt.UTC()
	runStart := transcript.NewRunStart(runID)
	turnStart := transcript.NewTurnStart(runID, turnID)
	runStart.Timestamp = startedAt
	turnStart.Timestamp = startedAt

	c.mu.Lock()
	c.runID = runID
	c.runStartedAt = startedAt
	c.turnID = turnID
	c.turnStartedAt = startedAt
	c.pendingSteps = nil
	c.pending = positionedLifecycle(messageIndex, runStart, turnStart)
	c.toolCalls = nil
	c.lastStep = requestCorrelation{}
	c.mu.Unlock()

	return lifecycleRunStarted{runID: runID, turnID: turnID, startedAt: startedAt}
}

func (c *lifecycleCoordinator) reset() {
	c.mu.Lock()
	c.runID = ""
	c.runStartedAt = time.Time{}
	c.turnID = ""
	c.turnStartedAt = time.Time{}
	c.pendingSteps = nil
	c.pending = nil
	c.toolCalls = nil
	c.lastStep = requestCorrelation{}
	c.mu.Unlock()
}

func (c *lifecycleCoordinator) claimFollowUp(
	messageIndex int,
	startedAt time.Time,
) lifecycleTurnTransition {
	startedAt = startedAt.UTC()
	nextTurnID := observability.NewID("turn")

	c.mu.Lock()
	decision := lifecycleTurnTransition{
		runID:             c.runID,
		previousTurnID:    c.turnID,
		previousStartedAt: c.turnStartedAt,
		nextTurnID:        nextTurnID,
		nextStartedAt:     startedAt,
	}
	if c.runID == "" || c.turnID == "" {
		c.mu.Unlock()
		return lifecycleTurnTransition{}
	}
	turnEnd := transcript.NewTurnEnd(
		c.runID,
		c.turnID,
		transcript.LifecycleCompleted,
		"",
	)
	turnStart := transcript.NewTurnStart(c.runID, nextTurnID)
	turnEnd.Timestamp = startedAt
	turnStart.Timestamp = startedAt
	c.turnID = nextTurnID
	c.turnStartedAt = startedAt
	c.pending = append(c.pending, positionedLifecycle(messageIndex, turnEnd, turnStart)...)
	c.mu.Unlock()
	return decision
}

func (c *lifecycleCoordinator) beginStepCheckpoint(
	messageIndex int,
	startedAt time.Time,
) lifecycleStepCheckpoint {
	stepID := observability.NewID("step")
	return c.beginStepCheckpointWithID(messageIndex, stepID, startedAt)
}

func (c *lifecycleCoordinator) beginStepCheckpointWithID(
	messageIndex int,
	stepID string,
	startedAt time.Time,
) lifecycleStepCheckpoint {
	startedAt = startedAt.UTC()

	c.mu.Lock()
	step := stepCorrelationState{
		runID:           c.runID,
		stepID:          stepID,
		lifecycleTurnID: c.turnID,
		startedAt:       startedAt,
	}
	c.pendingSteps = append(c.pendingSteps, step)
	stepStart := transcript.NewStepStart(step.runID, step.lifecycleTurnID, step.stepID)
	stepStart.Timestamp = startedAt
	pendingCount := len(c.pending)
	entries := append([]positionedJournalEntry(nil), c.pending...)
	entries = append(entries, positionedLifecycle(messageIndex, stepStart)...)
	c.mu.Unlock()

	return lifecycleStepCheckpoint{
		step:         step,
		entries:      entries,
		pendingCount: pendingCount,
	}
}

func (c *lifecycleCoordinator) attachProviderRequest() requestCorrelation {
	requestID := observability.NewID("request")
	return c.attachProviderRequestWithID(requestID)
}

func (c *lifecycleCoordinator) attachProviderRequestWithID(
	requestID string,
) requestCorrelation {
	c.mu.Lock()
	defer c.mu.Unlock()
	correlation := requestCorrelation{runID: c.runID, requestID: requestID}
	if count := len(c.pendingSteps); count > 0 {
		current := &c.pendingSteps[count-1]
		current.requestID = requestID
		correlation.turnID = current.lifecycleTurnID
		correlation.stepID = current.stepID
	}
	return correlation
}

func (c *lifecycleCoordinator) commitStepCheckpoint(
	stepID string,
	pendingCount int,
) {
	c.mu.Lock()
	c.clearPendingLocked(pendingCount)
	for index := range c.pendingSteps {
		if c.pendingSteps[index].stepID == stepID {
			c.pendingSteps[index].lifecycleDurable = true
			break
		}
	}
	c.mu.Unlock()
}

func (c *lifecycleCoordinator) completeStep(
	messageIndex int,
	completedAt time.Time,
	status, errorCode string,
) lifecycleStepCompleted {
	completedAt = completedAt.UTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.pendingSteps) == 0 {
		return lifecycleStepCompleted{}
	}
	step := c.pendingSteps[0]
	c.pendingSteps = c.pendingSteps[1:]
	c.lastStep = requestCorrelation{
		runID:     step.runID,
		turnID:    step.lifecycleTurnID,
		stepID:    step.stepID,
		requestID: step.requestID,
	}
	if step.lifecycleDurable {
		lifecycleStatus, reason := lifecycleTerminal(status, errorCode)
		stepEnd := transcript.NewStepEnd(
			step.runID,
			step.lifecycleTurnID,
			step.stepID,
			lifecycleStatus,
			reason,
		)
		stepEnd.Timestamp = completedAt
		c.pending = append(c.pending, positionedLifecycle(messageIndex, stepEnd)...)
	}
	return lifecycleStepCompleted{
		step:        step,
		status:      status,
		errorCode:   errorCode,
		completedAt: completedAt,
	}
}

func (c *lifecycleCoordinator) discardLastStep(
	messageCount int,
	reason string,
) lifecycleStepDiscarded {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastStep.stepID == "" {
		return lifecycleStepDiscarded{}
	}
	for index := range c.pending {
		if c.pending[index].messageIndex > messageCount {
			c.pending[index].messageIndex = messageCount
		}
	}
	return lifecycleStepDiscarded{correlation: c.lastStep, reason: reason}
}

func (c *lifecycleCoordinator) finishRun(
	messageIndex int,
	completedAt time.Time,
	runErr, checkpointErr error,
) lifecycleRunTerminal {
	completedAt = completedAt.UTC()
	status, reason := runLifecycleTerminal(runErr, checkpointErr)

	c.mu.Lock()
	defer c.mu.Unlock()
	for _, step := range c.pendingSteps {
		c.lastStep = requestCorrelation{
			runID:     step.runID,
			turnID:    step.lifecycleTurnID,
			stepID:    step.stepID,
			requestID: step.requestID,
		}
		if !step.lifecycleDurable {
			continue
		}
		stepEnd := transcript.NewStepEnd(
			step.runID,
			step.lifecycleTurnID,
			step.stepID,
			status,
			reason,
		)
		stepEnd.Timestamp = completedAt
		c.pending = append(c.pending, positionedLifecycle(messageIndex, stepEnd)...)
	}
	c.pendingSteps = nil

	turnEnd := transcript.NewTurnEnd(c.runID, c.turnID, status, reason)
	runEnd := transcript.NewRunEnd(c.runID, status, reason)
	turnEnd.Timestamp = completedAt
	runEnd.Timestamp = completedAt
	pendingCount := len(c.pending)
	entries := append([]positionedJournalEntry(nil), c.pending...)
	entries = append(entries, positionedLifecycle(messageIndex, turnEnd, runEnd)...)
	return lifecycleRunTerminal{
		runID:         c.runID,
		turnID:        c.turnID,
		runStartedAt:  c.runStartedAt,
		turnStartedAt: c.turnStartedAt,
		completedAt:   completedAt,
		status:        status,
		reason:        reason,
		entries:       entries,
		pendingCount:  pendingCount,
	}
}

func runLifecycleTerminal(
	runErr, checkpointErr error,
) (transcript.LifecycleStatus, string) {
	if runErr == nil && checkpointErr == nil {
		return transcript.LifecycleCompleted, ""
	}
	switch {
	case errors.Is(runErr, context.Canceled):
		return transcript.LifecycleCancelled, "context_cancelled"
	case errors.Is(runErr, context.DeadlineExceeded):
		return transcript.LifecycleFailed, "deadline_exceeded"
	case checkpointErr != nil:
		return transcript.LifecycleFailed, "checkpoint_failed"
	default:
		return transcript.LifecycleFailed, "run_failed"
	}
}

func lifecycleTerminal(
	status, reason string,
) (transcript.LifecycleStatus, string) {
	switch status {
	case "completed":
		return transcript.LifecycleCompleted, ""
	case "cancelled":
		return transcript.LifecycleCancelled, reason
	default:
		return transcript.LifecycleFailed, reason
	}
}

func positionedLifecycle(
	messageIndex int,
	entries ...transcript.Entry,
) []positionedJournalEntry {
	result := make([]positionedJournalEntry, len(entries))
	for index, entry := range entries {
		result[index] = positionedJournalEntry{messageIndex: messageIndex, entry: entry}
	}
	return result
}

func (c *lifecycleCoordinator) pendingEntries() []positionedJournalEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]positionedJournalEntry(nil), c.pending...)
}

func (c *lifecycleCoordinator) commitPending(count int) {
	if count <= 0 {
		return
	}
	c.mu.Lock()
	c.clearPendingLocked(count)
	c.mu.Unlock()
}

func (c *lifecycleCoordinator) clearPendingLocked(count int) {
	if count >= len(c.pending) {
		c.pending = nil
		return
	}
	c.pending = append([]positionedJournalEntry(nil), c.pending[count:]...)
}

func (c *lifecycleCoordinator) activeRun() (string, time.Time) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.runID, c.runStartedAt
}

func (c *lifecycleCoordinator) activeRequest() requestCorrelation {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for index := len(c.pendingSteps) - 1; index >= 0; index-- {
		step := c.pendingSteps[index]
		if step.requestID != "" {
			return requestCorrelation{
				runID:     c.runID,
				turnID:    step.lifecycleTurnID,
				stepID:    step.stepID,
				requestID: step.requestID,
			}
		}
	}
	return requestCorrelation{runID: c.runID}
}

func (c *lifecycleCoordinator) beginTool(
	toolCallID, toolName string,
	startedAt time.Time,
) (toolCorrelationState, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.toolCalls[toolCallID]; ok {
		return existing, false
	}
	correlation := requestCorrelation{runID: c.runID}
	if len(c.pendingSteps) > 0 {
		step := c.pendingSteps[0]
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
	if c.toolCalls == nil {
		c.toolCalls = make(map[string]toolCorrelationState)
	}
	c.toolCalls[toolCallID] = state
	return state, true
}

func (c *lifecycleCoordinator) finishTool(
	toolCallID string,
) (toolCorrelationState, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state, ok := c.toolCalls[toolCallID]
	if ok {
		delete(c.toolCalls, toolCallID)
	}
	return state, ok
}

func (c *lifecycleCoordinator) toolState(
	toolCallID string,
) (toolCorrelationState, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	state, ok := c.toolCalls[toolCallID]
	return state, ok
}
