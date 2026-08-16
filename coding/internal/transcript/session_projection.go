package transcript

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// SessionProjection is a disposable, deterministic view of one committed
// transcript prefix. AppliedEntries is a count, not a durable sequence number.
type SessionProjection struct {
	AppliedEntries int
	Runs           []ProjectedRun
	Messages       []ProjectedMessage
	ToolCalls      []ProjectedToolCall
	Contexts       []ProjectedContext
	Compactions    []ProjectedCompaction
	Open           ProjectedLifecycle
}

// ProjectedLifecycle identifies the currently open durable boundaries. An
// empty value means the committed prefix is at a clean session boundary.
type ProjectedLifecycle struct {
	RunID  string
	TurnID string
	StepID string
}

// ProjectedRun is one run reconstructed from explicit lifecycle boundaries.
type ProjectedRun struct {
	ID              string
	StartEntryID    string
	EndEntryID      string
	StartEntryIndex int
	EndEntryIndex   int
	StartedAt       time.Time
	CompletedAt     time.Time
	Status          LifecycleStatus
	Reason          string
	Turns           []ProjectedTurn
}

// ProjectedTurn is one claimed unit of user or follow-up intent inside a run.
type ProjectedTurn struct {
	ID              string
	RunID           string
	StartEntryID    string
	EndEntryID      string
	StartEntryIndex int
	EndEntryIndex   int
	StartedAt       time.Time
	CompletedAt     time.Time
	Status          LifecycleStatus
	Reason          string
	Steps           []ProjectedStep
}

// ProjectedStep is one assistant request-and-tools cycle inside a turn.
type ProjectedStep struct {
	ID              string
	RunID           string
	TurnID          string
	StartEntryID    string
	EndEntryID      string
	StartEntryIndex int
	EndEntryIndex   int
	StartedAt       time.Time
	CompletedAt     time.Time
	Status          LifecycleStatus
	Reason          string
}

// ProjectedMessage associates a durable model message with the lifecycle
// boundaries open at its position. User and steering messages may have no StepID.
type ProjectedMessage struct {
	EntryID    string
	EntryIndex int
	Timestamp  time.Time
	RunID      string
	TurnID     string
	StepID     string
	Message    agent.AgentMessage
}

// ProjectedToolCall joins the assistant request, optional durable dispatch
// intent, model-facing result, and optional product-facing outcome.
type ProjectedToolCall struct {
	ToolCallID string
	ToolName   string
	Arguments  json.RawMessage
	RunID      string
	TurnID     string
	StepID     string

	AssistantMessageEntryID    string
	AssistantMessageEntryIndex int
	DispatchEntryID            string
	DispatchEntryIndex         int
	ResultMessageEntryID       string
	ResultEntryIndex           int
	OutcomeEntryID             string
	OutcomeEntryIndex          int
	Outcome                    *ToolOutcome
}

// ProjectedContext associates one durable hidden context attachment with the
// lifecycle boundaries open when it was committed.
type ProjectedContext struct {
	EntryID    string
	EntryIndex int
	RunID      string
	TurnID     string
	StepID     string
	Attachment ContextAttachment
}

// ProjectedCompaction records a durable summary boundary and its lifecycle
// ownership without applying model-context presentation rules.
type ProjectedCompaction struct {
	EntryID    string
	EntryIndex int
	RunID      string
	TurnID     string
	StepID     string
	Compaction Compaction
}

type sessionProjector struct {
	projection SessionProjection
	runIndex   int
	turnIndex  int
	stepIndex  int

	entryIDs   map[string]struct{}
	messageIDs map[string]struct{}
	runIDs     map[string]struct{}
	turnIDs    map[string]struct{}
	stepIDs    map[string]struct{}

	toolIndexes  map[string]int
	pendingTools []int
}

// ProjectSession folds entries once, in committed order, into a detached
// session view. Open lifecycle or tool state is represented in the result;
// invalid ordering or broken references return an error.
func ProjectSession(entries []Entry) (*SessionProjection, error) {
	projector := sessionProjector{
		runIndex:    -1,
		turnIndex:   -1,
		stepIndex:   -1,
		entryIDs:    make(map[string]struct{}, len(entries)),
		messageIDs:  make(map[string]struct{}),
		runIDs:      make(map[string]struct{}),
		turnIDs:     make(map[string]struct{}),
		stepIDs:     make(map[string]struct{}),
		toolIndexes: make(map[string]int),
	}
	for index, entry := range entries {
		if err := projector.apply(index, entry); err != nil {
			return nil, err
		}
	}
	projector.projection.AppliedEntries = len(entries)
	projector.projection.Open = projector.lifecycle()
	return &projector.projection, nil
}

func (p *sessionProjector) apply(index int, entry Entry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	if _, exists := p.entryIDs[entry.ID]; exists {
		return fmt.Errorf("transcript: duplicate entry id %s", entry.ID)
	}
	p.entryIDs[entry.ID] = struct{}{}

	switch entry.Type {
	case RunStartEntry:
		return p.startRun(index, entry)
	case RunEndEntry:
		return p.endRun(index, entry)
	case TurnStartEntry:
		return p.startTurn(index, entry)
	case TurnEndEntry:
		return p.endTurn(index, entry)
	case StepStartEntry:
		return p.startStep(index, entry)
	case StepEndEntry:
		return p.endStep(index, entry)
	case MessageEntry:
		return p.applyMessage(index, entry)
	case ToolCallEntry:
		return p.applyToolDispatch(index, entry)
	case ToolOutcomeEntry:
		return p.applyToolOutcome(index, entry)
	case ContextEntry:
		if err := p.requireOpenStep(entry); err != nil {
			return err
		}
		attachment := *entry.Context
		p.projection.Contexts = append(p.projection.Contexts, ProjectedContext{
			EntryID: entry.ID, EntryIndex: index,
			RunID: p.runID(), TurnID: p.turnID(), StepID: p.stepID(),
			Attachment: attachment,
		})
		return nil
	case CompactionEntry:
		if err := p.requireNoPendingTools(entry); err != nil {
			return err
		}
		if _, exists := p.messageIDs[entry.Compaction.FirstKeptEntryID]; !exists {
			return fmt.Errorf(
				"transcript: compaction entry %s first kept entry %s is not a preceding message",
				entry.ID,
				entry.Compaction.FirstKeptEntryID,
			)
		}
		compaction := *entry.Compaction
		compaction.ReadFiles = append([]string(nil), compaction.ReadFiles...)
		compaction.ModifiedFiles = append([]string(nil), compaction.ModifiedFiles...)
		p.projection.Compactions = append(p.projection.Compactions, ProjectedCompaction{
			EntryID: entry.ID, EntryIndex: index,
			RunID: p.runID(), TurnID: p.turnID(), StepID: p.stepID(),
			Compaction: compaction,
		})
		return nil
	default:
		return fmt.Errorf("transcript: entry %s has unsupported type %q", entry.ID, entry.Type)
	}
}

func (p *sessionProjector) startRun(index int, entry Entry) error {
	if err := p.requireNoPendingTools(entry); err != nil {
		return err
	}
	lifecycle := *entry.Lifecycle
	if p.runIndex >= 0 {
		return projectionOrderError(entry, "starts while run %s is open", p.runID())
	}
	if _, exists := p.runIDs[lifecycle.RunID]; exists {
		return projectionOrderError(entry, "reuses run id %s", lifecycle.RunID)
	}
	p.projection.Runs = append(p.projection.Runs, ProjectedRun{
		ID: lifecycle.RunID, StartEntryID: entry.ID, StartEntryIndex: index,
		EndEntryIndex: -1, StartedAt: entry.Timestamp,
	})
	p.runIndex = len(p.projection.Runs) - 1
	p.turnIndex = -1
	p.stepIndex = -1
	p.runIDs[lifecycle.RunID] = struct{}{}
	return nil
}

func (p *sessionProjector) endRun(index int, entry Entry) error {
	if err := p.requireNoPendingTools(entry); err != nil {
		return err
	}
	lifecycle := *entry.Lifecycle
	if err := p.requireRun(entry, lifecycle.RunID); err != nil {
		return err
	}
	if p.turnIndex >= 0 || p.stepIndex >= 0 {
		return projectionOrderError(entry, "ends before its open turn and step")
	}
	run := &p.projection.Runs[p.runIndex]
	run.EndEntryID = entry.ID
	run.EndEntryIndex = index
	run.CompletedAt = entry.Timestamp
	run.Status = lifecycle.Status
	run.Reason = lifecycle.Reason
	p.runIndex = -1
	return nil
}

func (p *sessionProjector) startTurn(index int, entry Entry) error {
	if err := p.requireNoPendingTools(entry); err != nil {
		return err
	}
	lifecycle := *entry.Lifecycle
	if err := p.requireRun(entry, lifecycle.RunID); err != nil {
		return err
	}
	if p.turnIndex >= 0 {
		return projectionOrderError(entry, "starts while turn %s is open", p.turnID())
	}
	if _, exists := p.turnIDs[lifecycle.TurnID]; exists {
		return projectionOrderError(entry, "reuses turn id %s", lifecycle.TurnID)
	}
	run := &p.projection.Runs[p.runIndex]
	run.Turns = append(run.Turns, ProjectedTurn{
		ID: lifecycle.TurnID, RunID: lifecycle.RunID,
		StartEntryID: entry.ID, StartEntryIndex: index, EndEntryIndex: -1,
		StartedAt: entry.Timestamp,
	})
	p.turnIndex = len(run.Turns) - 1
	p.stepIndex = -1
	p.turnIDs[lifecycle.TurnID] = struct{}{}
	return nil
}

func (p *sessionProjector) endTurn(index int, entry Entry) error {
	if err := p.requireNoPendingTools(entry); err != nil {
		return err
	}
	lifecycle := *entry.Lifecycle
	if err := p.requireTurn(entry, lifecycle); err != nil {
		return err
	}
	if p.stepIndex >= 0 {
		return projectionOrderError(entry, "ends while step %s is open", p.stepID())
	}
	turn := p.currentTurn()
	turn.EndEntryID = entry.ID
	turn.EndEntryIndex = index
	turn.CompletedAt = entry.Timestamp
	turn.Status = lifecycle.Status
	turn.Reason = lifecycle.Reason
	p.turnIndex = -1
	return nil
}

func (p *sessionProjector) startStep(index int, entry Entry) error {
	if err := p.requireNoPendingTools(entry); err != nil {
		return err
	}
	lifecycle := *entry.Lifecycle
	if err := p.requireTurn(entry, lifecycle); err != nil {
		return err
	}
	if p.stepIndex >= 0 {
		return projectionOrderError(entry, "starts while step %s is open", p.stepID())
	}
	if _, exists := p.stepIDs[lifecycle.StepID]; exists {
		return projectionOrderError(entry, "reuses step id %s", lifecycle.StepID)
	}
	turn := p.currentTurn()
	turn.Steps = append(turn.Steps, ProjectedStep{
		ID: lifecycle.StepID, RunID: lifecycle.RunID, TurnID: lifecycle.TurnID,
		StartEntryID: entry.ID, StartEntryIndex: index, EndEntryIndex: -1,
		StartedAt: entry.Timestamp,
	})
	p.stepIndex = len(turn.Steps) - 1
	p.stepIDs[lifecycle.StepID] = struct{}{}
	return nil
}

func (p *sessionProjector) endStep(index int, entry Entry) error {
	if err := p.requireNoPendingTools(entry); err != nil {
		return err
	}
	lifecycle := *entry.Lifecycle
	if err := p.requireStep(entry, lifecycle); err != nil {
		return err
	}
	step := p.currentStep()
	step.EndEntryID = entry.ID
	step.EndEntryIndex = index
	step.CompletedAt = entry.Timestamp
	step.Status = lifecycle.Status
	step.Reason = lifecycle.Reason
	p.stepIndex = -1
	return nil
}

func (p *sessionProjector) applyMessage(index int, entry Entry) error {
	message, ok := agent.ToLLM(entry.Message)
	if !ok {
		return fmt.Errorf("transcript: message entry %s has unsupported type %T", entry.ID, entry.Message)
	}
	detached, err := cloneProjectedMessage(message)
	if err != nil {
		return fmt.Errorf("transcript: clone message entry %s: %w", entry.ID, err)
	}
	projected := ProjectedMessage{
		EntryID: entry.ID, EntryIndex: index, Timestamp: entry.Timestamp,
		RunID: p.runID(), TurnID: p.turnID(), StepID: p.stepID(), Message: detached,
	}

	switch typed := message.(type) {
	case *llm.UserMessage:
		if err := p.requireNoPendingTools(entry); err != nil {
			return err
		}
		if err := p.requireOpenTurn(entry); err != nil {
			return err
		}
	case *llm.AssistantMessage:
		if err := p.requireNoPendingTools(entry); err != nil {
			return err
		}
		if err := p.requireOpenStep(entry); err != nil {
			return err
		}
		for _, call := range typed.ToolCalls() {
			if err := p.addToolRequest(index, entry, call); err != nil {
				return err
			}
		}
	case *llm.ToolResultMessage:
		if err := p.requireOpenStep(entry); err != nil {
			return err
		}
		if err := p.resolveTool(index, entry, typed); err != nil {
			return err
		}
	default:
		return fmt.Errorf("transcript: message entry %s has unsupported type %T", entry.ID, message)
	}

	p.projection.Messages = append(p.projection.Messages, projected)
	p.messageIDs[entry.ID] = struct{}{}
	return nil
}

func (p *sessionProjector) addToolRequest(index int, entry Entry, call llm.ToolCall) error {
	if call.ID == "" {
		return fmt.Errorf("transcript: assistant entry %s has a tool call without an id", entry.ID)
	}
	if _, exists := p.toolIndexes[call.ID]; exists {
		return fmt.Errorf("transcript: assistant entry %s repeats tool call id %s", entry.ID, call.ID)
	}
	arguments, err := json.Marshal(call.Arguments)
	if err != nil {
		return fmt.Errorf("transcript: assistant entry %s encodes tool call %s: %w", entry.ID, call.ID, err)
	}
	tool := ProjectedToolCall{
		ToolCallID: call.ID, ToolName: call.Name, Arguments: arguments,
		RunID: p.runID(), TurnID: p.turnID(), StepID: p.stepID(),
		AssistantMessageEntryID: entry.ID, AssistantMessageEntryIndex: index,
		DispatchEntryIndex: -1, ResultEntryIndex: -1, OutcomeEntryIndex: -1,
	}
	p.projection.ToolCalls = append(p.projection.ToolCalls, tool)
	toolIndex := len(p.projection.ToolCalls) - 1
	p.toolIndexes[call.ID] = toolIndex
	p.pendingTools = append(p.pendingTools, toolIndex)
	return nil
}

func (p *sessionProjector) applyToolDispatch(index int, entry Entry) error {
	if err := p.requireOpenStep(entry); err != nil {
		return err
	}
	toolIndex, exists := p.toolIndexes[entry.ToolCall.ToolCallID]
	if !exists || !p.toolPending(toolIndex) {
		return fmt.Errorf(
			"transcript: tool call entry %s has no unresolved assistant call %s",
			entry.ID,
			entry.ToolCall.ToolCallID,
		)
	}
	tool := &p.projection.ToolCalls[toolIndex]
	if tool.DispatchEntryID != "" {
		return fmt.Errorf(
			"transcript: tool call entry %s repeats dispatch intent for %s",
			entry.ID,
			entry.ToolCall.ToolCallID,
		)
	}
	if tool.ToolName != entry.ToolCall.ToolName {
		return fmt.Errorf(
			"transcript: tool call entry %s names %q, want %q",
			entry.ID,
			entry.ToolCall.ToolName,
			tool.ToolName,
		)
	}
	if !equalJSON(tool.Arguments, entry.ToolCall.Arguments) {
		return fmt.Errorf(
			"transcript: tool call entry %s arguments differ from assistant call %s",
			entry.ID,
			entry.ToolCall.ToolCallID,
		)
	}
	if tool.RunID != p.runID() || tool.TurnID != p.turnID() || tool.StepID != p.stepID() {
		return fmt.Errorf(
			"transcript: tool call entry %s moved outside assistant call %s lifecycle",
			entry.ID,
			entry.ToolCall.ToolCallID,
		)
	}
	tool.DispatchEntryID = entry.ID
	tool.DispatchEntryIndex = index
	return nil
}

func (p *sessionProjector) resolveTool(
	index int,
	entry Entry,
	result *llm.ToolResultMessage,
) error {
	if result == nil || len(p.pendingTools) == 0 {
		return fmt.Errorf("transcript: tool result entry %s has no unresolved call", entry.ID)
	}
	toolIndex := p.pendingTools[0]
	tool := &p.projection.ToolCalls[toolIndex]
	if tool.ToolCallID != result.ToolCallID {
		return fmt.Errorf(
			"transcript: tool result entry %s resolves call %s out of model order; want %s",
			entry.ID,
			result.ToolCallID,
			tool.ToolCallID,
		)
	}
	if tool.ToolName != result.ToolName {
		return fmt.Errorf(
			"transcript: tool result entry %s names %q, want %q",
			entry.ID,
			result.ToolName,
			tool.ToolName,
		)
	}
	if tool.RunID != p.runID() || tool.TurnID != p.turnID() || tool.StepID != p.stepID() {
		return fmt.Errorf(
			"transcript: tool result entry %s moved outside assistant call %s lifecycle",
			entry.ID,
			result.ToolCallID,
		)
	}
	tool.ResultMessageEntryID = entry.ID
	tool.ResultEntryIndex = index
	p.pendingTools = p.pendingTools[1:]
	return nil
}

func (p *sessionProjector) applyToolOutcome(index int, entry Entry) error {
	if err := p.requireOpenStep(entry); err != nil {
		return err
	}
	toolIndex, exists := p.toolIndexes[entry.ToolOutcome.ToolCallID]
	if !exists {
		return fmt.Errorf(
			"transcript: tool outcome entry %s has no assistant call %s",
			entry.ID,
			entry.ToolOutcome.ToolCallID,
		)
	}
	tool := &p.projection.ToolCalls[toolIndex]
	if tool.ResultMessageEntryID == "" {
		return fmt.Errorf(
			"transcript: tool outcome entry %s precedes result for %s",
			entry.ID,
			entry.ToolOutcome.ToolCallID,
		)
	}
	if tool.OutcomeEntryID != "" {
		return fmt.Errorf(
			"transcript: tool outcome entry %s repeats outcome for %s",
			entry.ID,
			entry.ToolOutcome.ToolCallID,
		)
	}
	if tool.RunID != p.runID() || tool.TurnID != p.turnID() || tool.StepID != p.stepID() {
		return fmt.Errorf(
			"transcript: tool outcome entry %s moved outside assistant call %s lifecycle",
			entry.ID,
			entry.ToolOutcome.ToolCallID,
		)
	}
	outcome := *entry.ToolOutcome
	outcome.Data = append(json.RawMessage(nil), outcome.Data...)
	tool.OutcomeEntryID = entry.ID
	tool.OutcomeEntryIndex = index
	tool.Outcome = &outcome
	return nil
}

func (p *sessionProjector) requireNoPendingTools(entry Entry) error {
	if len(p.pendingTools) == 0 {
		return nil
	}
	tool := p.projection.ToolCalls[p.pendingTools[0]]
	return fmt.Errorf(
		"transcript: %s entry %s follows unresolved tool call %s",
		entry.Type,
		entry.ID,
		tool.ToolCallID,
	)
}

func (p *sessionProjector) requireOpenTurn(entry Entry) error {
	if p.runIndex < 0 || p.turnIndex < 0 {
		return projectionOrderError(entry, "has no open turn")
	}
	return nil
}

func (p *sessionProjector) requireOpenStep(entry Entry) error {
	if err := p.requireOpenTurn(entry); err != nil {
		return err
	}
	if p.stepIndex < 0 {
		return projectionOrderError(entry, "has no open step")
	}
	return nil
}

func (p *sessionProjector) toolPending(index int) bool {
	for _, pending := range p.pendingTools {
		if pending == index {
			return true
		}
	}
	return false
}

func (p *sessionProjector) requireRun(entry Entry, runID string) error {
	if p.runIndex < 0 {
		return projectionOrderError(entry, "has no open run")
	}
	if p.runID() != runID {
		return projectionOrderError(entry, "belongs to run %s, want %s", runID, p.runID())
	}
	return nil
}

func (p *sessionProjector) requireTurn(entry Entry, lifecycle Lifecycle) error {
	if err := p.requireRun(entry, lifecycle.RunID); err != nil {
		return err
	}
	if p.turnIndex < 0 {
		return projectionOrderError(entry, "has no open turn")
	}
	if p.turnID() != lifecycle.TurnID {
		return projectionOrderError(
			entry,
			"belongs to turn %s, want %s",
			lifecycle.TurnID,
			p.turnID(),
		)
	}
	return nil
}

func (p *sessionProjector) requireStep(entry Entry, lifecycle Lifecycle) error {
	if err := p.requireTurn(entry, lifecycle); err != nil {
		return err
	}
	if p.stepIndex < 0 {
		return projectionOrderError(entry, "has no open step")
	}
	if p.stepID() != lifecycle.StepID {
		return projectionOrderError(
			entry,
			"belongs to step %s, want %s",
			lifecycle.StepID,
			p.stepID(),
		)
	}
	return nil
}

func (p *sessionProjector) lifecycle() ProjectedLifecycle {
	return ProjectedLifecycle{RunID: p.runID(), TurnID: p.turnID(), StepID: p.stepID()}
}

func (p *sessionProjector) runID() string {
	if p.runIndex < 0 {
		return ""
	}
	return p.projection.Runs[p.runIndex].ID
}

func (p *sessionProjector) turnID() string {
	if p.runIndex < 0 || p.turnIndex < 0 {
		return ""
	}
	return p.projection.Runs[p.runIndex].Turns[p.turnIndex].ID
}

func (p *sessionProjector) stepID() string {
	if p.runIndex < 0 || p.turnIndex < 0 || p.stepIndex < 0 {
		return ""
	}
	return p.currentTurn().Steps[p.stepIndex].ID
}

func (p *sessionProjector) currentTurn() *ProjectedTurn {
	return &p.projection.Runs[p.runIndex].Turns[p.turnIndex]
}

func (p *sessionProjector) currentStep() *ProjectedStep {
	return &p.currentTurn().Steps[p.stepIndex]
}

func projectionOrderError(entry Entry, format string, args ...any) error {
	return fmt.Errorf(
		"transcript: projection entry %s (%s) %s",
		entry.ID,
		entry.Type,
		fmt.Sprintf(format, args...),
	)
}

func equalJSON(left, right json.RawMessage) bool {
	var leftValue any
	var rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func cloneProjectedMessage(message llm.Message) (agent.AgentMessage, error) {
	encoded, err := llm.MarshalMessage(message)
	if err != nil {
		return nil, err
	}
	decoded, err := llm.UnmarshalMessage(encoded)
	if err != nil {
		return nil, err
	}
	return agent.FromLLM(decoded), nil
}
