package transcript

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

const SessionProjectionKey = "session"

// SessionProjection is a deterministic snapshot of one committed transcript
// prefix. AsOfSeq identifies the last event included in the view.
type SessionProjection struct {
	AppliedEntries   int
	AsOfSeq          int64
	Runs             []ProjectedRun
	Messages         []ProjectedMessage
	ToolCalls        []ProjectedToolCall
	Contexts         []ProjectedContext
	Compactions      []ProjectedCompaction
	ProviderRequests []ProjectedProviderRequest
	Open             ProjectedLifecycle
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

// ProjectedProviderRequest associates one complete durable request definition
// with its checkpoint sequence and open lifecycle scope.
type ProjectedProviderRequest struct {
	EntryID    string
	EntryIndex int
	EntrySeq   int64
	Header     RequestHeader
}

type sessionProjector struct {
	projection SessionProjection
	runIndex   int
	turnIndex  int
	stepIndex  int

	toolIndexes map[string]int
}

// SessionProjectionUnit is the registered, incrementally maintained session
// read model.
type SessionProjectionUnit struct {
	projector sessionProjector
}

func NewSessionProjectionUnit() *SessionProjectionUnit {
	return &SessionProjectionUnit{projector: sessionProjector{
		projection:  SessionProjection{AsOfSeq: -1},
		runIndex:    -1,
		turnIndex:   -1,
		stepIndex:   -1,
		toolIndexes: make(map[string]int),
	}}
}

func (*SessionProjectionUnit) ProjectionKey() string { return SessionProjectionKey }

func (u *SessionProjectionUnit) ApplyProjection(event ProjectionEvent) {
	u.projector.apply(event)
}

func (u *SessionProjectionUnit) SnapshotProjection() (any, error) {
	return u.Snapshot()
}

func (u *SessionProjectionUnit) Snapshot() (*SessionProjection, error) {
	if u == nil {
		return nil, fmt.Errorf("transcript: session projection unit is nil")
	}
	return cloneSessionProjection(u.projector.projection)
}

// ProjectSession folds entries once, in committed order, through the same
// registered projection used by live sessions. It remains the deterministic
// replay entry point for offline diagnostics and tests.
func ProjectSession(entries []Entry) (*SessionProjection, error) {
	registry := NewProjectionRegistry()
	unit := NewSessionProjectionUnit()
	if err := registry.Register(unit); err != nil {
		return nil, err
	}
	if _, err := validateSession(entries, registry); err != nil {
		return nil, err
	}
	return unit.Snapshot()
}

func (p *sessionProjector) apply(event ProjectionEvent) {
	index := event.EntryIndex
	entry := event.Entry
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
		p.applyMessage(event)
	case ToolCallEntry:
		p.applyToolDispatch(index, entry)
	case ToolOutcomeEntry:
		p.applyToolOutcome(index, entry)
	case ContextEntry:
		attachment := *entry.Context
		p.projection.Contexts = append(p.projection.Contexts, ProjectedContext{
			EntryID: entry.ID, EntryIndex: index,
			RunID: event.Scope.RunID, TurnID: event.Scope.TurnID,
			StepID: event.Scope.StepID, Attachment: attachment,
		})
	case CompactionEntry:
		compaction := *entry.Compaction
		compaction.ReadFiles = append([]string(nil), compaction.ReadFiles...)
		compaction.ModifiedFiles = append([]string(nil), compaction.ModifiedFiles...)
		p.projection.Compactions = append(p.projection.Compactions, ProjectedCompaction{
			EntryID: entry.ID, EntryIndex: index,
			RunID: event.Scope.RunID, TurnID: event.Scope.TurnID,
			StepID: event.Scope.StepID, Compaction: compaction,
		})
	case RequestHeaderEntry:
		p.projection.ProviderRequests = append(
			p.projection.ProviderRequests,
			ProjectedProviderRequest{
				EntryID: entry.ID, EntryIndex: index, EntrySeq: entry.Seq,
				Header: cloneRequestHeader(*entry.RequestHeader),
			},
		)
	}
	p.projection.AppliedEntries = index + 1
	p.projection.AsOfSeq = entry.Seq
	p.projection.Open = event.Scope
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

func (p *sessionProjector) applyMessage(event ProjectionEvent) {
	index := event.EntryIndex
	entry := event.Entry
	p.projection.Messages = append(p.projection.Messages, ProjectedMessage{
		EntryID: entry.ID, EntryIndex: index, Timestamp: entry.Timestamp,
		RunID: event.Scope.RunID, TurnID: event.Scope.TurnID,
		StepID: event.Scope.StepID, Message: event.message,
	})

	for _, request := range event.toolRequests {
		p.addToolRequest(request)
	}
	if event.toolResultID != "" {
		tool := &p.projection.ToolCalls[p.toolIndexes[event.toolResultID]]
		tool.ResultMessageEntryID = entry.ID
		tool.ResultEntryIndex = index
	}
}

func (p *sessionProjector) addToolRequest(request projectedToolRequest) {
	tool := ProjectedToolCall{
		ToolCallID:                 request.ID,
		ToolName:                   request.Name,
		Arguments:                  append(json.RawMessage(nil), request.Arguments...),
		RunID:                      request.Scope.RunID,
		TurnID:                     request.Scope.TurnID,
		StepID:                     request.Scope.StepID,
		AssistantMessageEntryID:    request.AssistantEntryID,
		AssistantMessageEntryIndex: request.AssistantEntryIndex,
		DispatchEntryIndex:         -1, ResultEntryIndex: -1, OutcomeEntryIndex: -1,
	}
	p.projection.ToolCalls = append(p.projection.ToolCalls, tool)
	p.toolIndexes[request.ID] = len(p.projection.ToolCalls) - 1
}

func (p *sessionProjector) applyToolDispatch(index int, entry Entry) {
	tool := &p.projection.ToolCalls[p.toolIndexes[entry.ToolCall.ToolCallID]]
	tool.DispatchEntryID = entry.ID
	tool.DispatchEntryIndex = index
}

func (p *sessionProjector) applyToolOutcome(index int, entry Entry) {
	tool := &p.projection.ToolCalls[p.toolIndexes[entry.ToolOutcome.ToolCallID]]
	outcome := cloneToolOutcome(*entry.ToolOutcome)
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

func cloneSessionProjection(source SessionProjection) (*SessionProjection, error) {
	clone := source
	clone.Runs = make([]ProjectedRun, len(source.Runs))
	for runIndex, run := range source.Runs {
		clone.Runs[runIndex] = run
		clone.Runs[runIndex].Turns = make([]ProjectedTurn, len(run.Turns))
		for turnIndex, turn := range run.Turns {
			clone.Runs[runIndex].Turns[turnIndex] = turn
			clone.Runs[runIndex].Turns[turnIndex].Steps = append(
				[]ProjectedStep(nil),
				turn.Steps...,
			)
		}
	}
	clone.Messages = make([]ProjectedMessage, len(source.Messages))
	for index, message := range source.Messages {
		clone.Messages[index] = message
		llmMessage, ok := agent.ToLLM(message.Message)
		if !ok {
			return nil, fmt.Errorf("transcript: projected message entry %s is not model-facing", message.EntryID)
		}
		detached, err := cloneProjectedMessage(llmMessage)
		if err != nil {
			return nil, fmt.Errorf("transcript: clone projected message entry %s: %w", message.EntryID, err)
		}
		clone.Messages[index].Message = detached
	}
	clone.ToolCalls = make([]ProjectedToolCall, len(source.ToolCalls))
	for index, tool := range source.ToolCalls {
		clone.ToolCalls[index] = tool
		clone.ToolCalls[index].Arguments = append(json.RawMessage(nil), tool.Arguments...)
		if tool.Outcome != nil {
			outcome := cloneToolOutcome(*tool.Outcome)
			clone.ToolCalls[index].Outcome = &outcome
		}
	}
	clone.Contexts = append([]ProjectedContext(nil), source.Contexts...)
	clone.Compactions = make([]ProjectedCompaction, len(source.Compactions))
	for index, compaction := range source.Compactions {
		clone.Compactions[index] = compaction
		clone.Compactions[index].Compaction.ReadFiles = append(
			[]string(nil),
			compaction.Compaction.ReadFiles...,
		)
		clone.Compactions[index].Compaction.ModifiedFiles = append(
			[]string(nil),
			compaction.Compaction.ModifiedFiles...,
		)
	}
	clone.ProviderRequests = make([]ProjectedProviderRequest, len(source.ProviderRequests))
	for index, request := range source.ProviderRequests {
		clone.ProviderRequests[index] = request
		clone.ProviderRequests[index].Header = cloneRequestHeader(request.Header)
	}
	return &clone, nil
}
