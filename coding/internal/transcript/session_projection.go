package transcript

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// SessionProjection is a disposable, deterministic view of one committed
// transcript prefix. AsOfSeq identifies the last event included in the view.
type SessionProjection struct {
	AppliedEntries int
	AsOfSeq        int64
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

	toolIndexes map[string]int
}

// ProjectSession folds entries once, in committed order, into a detached
// session view. The canonical reducer validates ordering and references while
// this projector builds the disposable read model.
func ProjectSession(entries []Entry) (*SessionProjection, error) {
	reducer := newSessionReducer(len(entries))
	projector := sessionProjector{
		runIndex: -1, turnIndex: -1, stepIndex: -1,
		toolIndexes: make(map[string]int),
	}
	for index, entry := range entries {
		transition, err := reducer.Apply(index, entry)
		if err != nil {
			return nil, err
		}
		if err := projector.apply(index, entry, transition); err != nil {
			return nil, err
		}
	}
	projector.projection.AppliedEntries = len(entries)
	projector.projection.AsOfSeq = -1
	if len(entries) > 0 {
		projector.projection.AsOfSeq = entries[len(entries)-1].Seq
	}
	projector.projection.Open = ProjectedLifecycle{
		RunID: reducer.scope.RunID, TurnID: reducer.scope.TurnID, StepID: reducer.scope.StepID,
	}
	return &projector.projection, nil
}

func (p *sessionProjector) apply(
	index int,
	entry Entry,
	transition sessionTransition,
) error {
	switch entry.Type {
	case RunStartEntry:
		p.startRun(index, entry)
	case RunEndEntry:
		p.endRun(index, entry)
	case TurnStartEntry:
		p.startTurn(index, entry)
	case TurnEndEntry:
		p.endTurn(index, entry)
	case StepStartEntry:
		p.startStep(index, entry)
	case StepEndEntry:
		p.endStep(index, entry)
	case MessageEntry:
		return p.applyMessage(index, entry, transition)
	case ToolCallEntry:
		p.applyToolDispatch(index, entry)
	case ToolOutcomeEntry:
		p.applyToolOutcome(index, entry)
	case ContextEntry:
		attachment := *entry.Context
		p.projection.Contexts = append(p.projection.Contexts, ProjectedContext{
			EntryID: entry.ID, EntryIndex: index,
			RunID: transition.Scope.RunID, TurnID: transition.Scope.TurnID,
			StepID: transition.Scope.StepID, Attachment: attachment,
		})
	case CompactionEntry:
		compaction := *entry.Compaction
		compaction.ReadFiles = append([]string(nil), compaction.ReadFiles...)
		compaction.ModifiedFiles = append([]string(nil), compaction.ModifiedFiles...)
		p.projection.Compactions = append(p.projection.Compactions, ProjectedCompaction{
			EntryID: entry.ID, EntryIndex: index,
			RunID: transition.Scope.RunID, TurnID: transition.Scope.TurnID,
			StepID: transition.Scope.StepID, Compaction: compaction,
		})
	}
	return nil
}

func (p *sessionProjector) startRun(index int, entry Entry) {
	p.projection.Runs = append(p.projection.Runs, ProjectedRun{
		ID: entry.Lifecycle.RunID, StartEntryID: entry.ID, StartEntryIndex: index,
		EndEntryIndex: -1, StartedAt: entry.Timestamp,
	})
	p.runIndex = len(p.projection.Runs) - 1
	p.turnIndex = -1
	p.stepIndex = -1
}

func (p *sessionProjector) endRun(index int, entry Entry) {
	run := &p.projection.Runs[p.runIndex]
	run.EndEntryID = entry.ID
	run.EndEntryIndex = index
	run.CompletedAt = entry.Timestamp
	run.Status = entry.Lifecycle.Status
	run.Reason = entry.Lifecycle.Reason
	p.runIndex = -1
}

func (p *sessionProjector) startTurn(index int, entry Entry) {
	run := &p.projection.Runs[p.runIndex]
	run.Turns = append(run.Turns, ProjectedTurn{
		ID: entry.Lifecycle.TurnID, RunID: entry.Lifecycle.RunID,
		StartEntryID: entry.ID, StartEntryIndex: index, EndEntryIndex: -1,
		StartedAt: entry.Timestamp,
	})
	p.turnIndex = len(run.Turns) - 1
	p.stepIndex = -1
}

func (p *sessionProjector) endTurn(index int, entry Entry) {
	turn := p.currentTurn()
	turn.EndEntryID = entry.ID
	turn.EndEntryIndex = index
	turn.CompletedAt = entry.Timestamp
	turn.Status = entry.Lifecycle.Status
	turn.Reason = entry.Lifecycle.Reason
	p.turnIndex = -1
}

func (p *sessionProjector) startStep(index int, entry Entry) {
	turn := p.currentTurn()
	turn.Steps = append(turn.Steps, ProjectedStep{
		ID: entry.Lifecycle.StepID, RunID: entry.Lifecycle.RunID,
		TurnID:       entry.Lifecycle.TurnID,
		StartEntryID: entry.ID, StartEntryIndex: index, EndEntryIndex: -1,
		StartedAt: entry.Timestamp,
	})
	p.stepIndex = len(turn.Steps) - 1
}

func (p *sessionProjector) endStep(index int, entry Entry) {
	step := p.currentStep()
	step.EndEntryID = entry.ID
	step.EndEntryIndex = index
	step.CompletedAt = entry.Timestamp
	step.Status = entry.Lifecycle.Status
	step.Reason = entry.Lifecycle.Reason
	p.stepIndex = -1
}

func (p *sessionProjector) applyMessage(
	index int,
	entry Entry,
	transition sessionTransition,
) error {
	detached, err := cloneProjectedMessage(transition.Message)
	if err != nil {
		return fmt.Errorf("transcript: clone message entry %s: %w", entry.ID, err)
	}
	p.projection.Messages = append(p.projection.Messages, ProjectedMessage{
		EntryID: entry.ID, EntryIndex: index, Timestamp: entry.Timestamp,
		RunID: transition.Scope.RunID, TurnID: transition.Scope.TurnID,
		StepID: transition.Scope.StepID, Message: detached,
	})

	for _, request := range transition.ToolRequests {
		p.addToolRequest(request)
	}
	if transition.Tool != nil {
		if _, ok := transition.Message.(*llm.ToolResultMessage); ok {
			tool := &p.projection.ToolCalls[p.toolIndexes[transition.Tool.Request.ID]]
			tool.ResultMessageEntryID = entry.ID
			tool.ResultEntryIndex = index
		}
	}
	return nil
}

func (p *sessionProjector) addToolRequest(request *reducedToolState) {
	tool := ProjectedToolCall{
		ToolCallID:                 request.Request.ID,
		ToolName:                   request.Request.Name,
		Arguments:                  append(json.RawMessage(nil), request.Request.Arguments...),
		RunID:                      request.Scope.RunID,
		TurnID:                     request.Scope.TurnID,
		StepID:                     request.Scope.StepID,
		AssistantMessageEntryID:    request.AssistantEntryID,
		AssistantMessageEntryIndex: request.AssistantEntryIndex,
		DispatchEntryIndex:         -1, ResultEntryIndex: -1, OutcomeEntryIndex: -1,
	}
	p.projection.ToolCalls = append(p.projection.ToolCalls, tool)
	p.toolIndexes[request.Request.ID] = len(p.projection.ToolCalls) - 1
}

func (p *sessionProjector) applyToolDispatch(index int, entry Entry) {
	tool := &p.projection.ToolCalls[p.toolIndexes[entry.ToolCall.ToolCallID]]
	tool.DispatchEntryID = entry.ID
	tool.DispatchEntryIndex = index
}

func (p *sessionProjector) applyToolOutcome(index int, entry Entry) {
	tool := &p.projection.ToolCalls[p.toolIndexes[entry.ToolOutcome.ToolCallID]]
	outcome := *entry.ToolOutcome
	outcome.Data = append(json.RawMessage(nil), outcome.Data...)
	tool.OutcomeEntryID = entry.ID
	tool.OutcomeEntryIndex = index
	tool.Outcome = &outcome
}

func (p *sessionProjector) currentTurn() *ProjectedTurn {
	return &p.projection.Runs[p.runIndex].Turns[p.turnIndex]
}

func (p *sessionProjector) currentStep() *ProjectedStep {
	return &p.currentTurn().Steps[p.stepIndex]
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
