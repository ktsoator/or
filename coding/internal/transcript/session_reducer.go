package transcript

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

type sessionScope struct {
	RunID  string
	TurnID string
	StepID string
}

type reducedToolRequest struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type reducedToolState struct {
	Request reducedToolRequest
	Scope   sessionScope

	AssistantEntryID    string
	AssistantEntryIndex int
	DispatchEntryID     string
	DispatchEntryIndex  int
	ResultEntryID       string
	ResultEntryIndex    int
	OutcomeEntryID      string
	OutcomeEntryIndex   int
}

type sessionTransition struct {
	Scope        sessionScope
	Message      llm.Message
	ToolRequests []*reducedToolState
	Tool         *reducedToolState
}

// sessionReducer is the canonical state machine for committed transcript
// events. Projection, validation, and interrupted-tail repair all drive this
// reducer so event ordering rules have one owner.
type sessionReducer struct {
	scope sessionScope

	entryIDs   map[string]struct{}
	messageIDs map[string]struct{}
	runIDs     map[string]struct{}
	turnIDs    map[string]struct{}
	stepIDs    map[string]struct{}
	toolIDs    map[string]struct{}

	tools        map[string]*reducedToolState
	pendingTools []string
}

func newSessionReducer(capacity int) *sessionReducer {
	return &sessionReducer{
		entryIDs:   make(map[string]struct{}, capacity),
		messageIDs: make(map[string]struct{}),
		runIDs:     make(map[string]struct{}),
		turnIDs:    make(map[string]struct{}),
		stepIDs:    make(map[string]struct{}),
		toolIDs:    make(map[string]struct{}),
		tools:      make(map[string]*reducedToolState),
	}
}

func (r *sessionReducer) clone() *sessionReducer {
	clone := &sessionReducer{
		scope:        r.scope,
		entryIDs:     cloneSet(r.entryIDs),
		messageIDs:   cloneSet(r.messageIDs),
		runIDs:       cloneSet(r.runIDs),
		turnIDs:      cloneSet(r.turnIDs),
		stepIDs:      cloneSet(r.stepIDs),
		toolIDs:      cloneSet(r.toolIDs),
		tools:        make(map[string]*reducedToolState, len(r.tools)),
		pendingTools: append([]string(nil), r.pendingTools...),
	}
	for id, tool := range r.tools {
		copied := *tool
		copied.Request.Arguments = append(json.RawMessage(nil), tool.Request.Arguments...)
		clone.tools[id] = &copied
	}
	return clone
}

func cloneSet(source map[string]struct{}) map[string]struct{} {
	clone := make(map[string]struct{}, len(source))
	for value := range source {
		clone[value] = struct{}{}
	}
	return clone
}

func (r *sessionReducer) Apply(index int, entry Entry) (sessionTransition, error) {
	if entry.Seq != int64(index) {
		return sessionTransition{}, fmt.Errorf(
			"transcript: entry %s has sequence %d, want %d",
			entry.ID,
			entry.Seq,
			index,
		)
	}
	if err := entry.Validate(); err != nil {
		return sessionTransition{}, err
	}
	if _, exists := r.entryIDs[entry.ID]; exists {
		return sessionTransition{}, fmt.Errorf("transcript: duplicate entry id %s", entry.ID)
	}
	r.entryIDs[entry.ID] = struct{}{}

	transition := sessionTransition{Scope: r.scope}
	switch entry.Type {
	case RunStartEntry, RunEndEntry,
		TurnStartEntry, TurnEndEntry,
		StepStartEntry, StepEndEntry:
		if err := r.applyLifecycle(entry); err != nil {
			return sessionTransition{}, err
		}
		transition.Scope = r.scope
		return transition, nil

	case MessageEntry:
		message, requests, tool, err := r.applyMessage(index, entry)
		if err != nil {
			return sessionTransition{}, err
		}
		transition.Scope = r.scope
		transition.Message = message
		transition.ToolRequests = requests
		transition.Tool = tool
		return transition, nil

	case ToolCallEntry:
		tool, err := r.applyToolDispatch(index, entry)
		if err != nil {
			return sessionTransition{}, err
		}
		transition.Scope = r.scope
		transition.Tool = tool
		return transition, nil

	case ToolOutcomeEntry:
		tool, err := r.applyToolOutcome(index, entry)
		if err != nil {
			return sessionTransition{}, err
		}
		transition.Scope = r.scope
		transition.Tool = tool
		return transition, nil

	case ContextEntry:
		if err := r.requireOpenStep(entry); err != nil {
			return sessionTransition{}, err
		}
		transition.Scope = r.scope
		return transition, nil

	case CompactionEntry:
		if err := r.requireNoPendingTools(entry); err != nil {
			return sessionTransition{}, err
		}
		if _, exists := r.messageIDs[entry.Compaction.FirstKeptEntryID]; !exists {
			return sessionTransition{}, fmt.Errorf(
				"transcript: compaction entry %s first kept entry %s is not a preceding message",
				entry.ID,
				entry.Compaction.FirstKeptEntryID,
			)
		}
		transition.Scope = r.scope
		return transition, nil

	default:
		return sessionTransition{}, fmt.Errorf(
			"transcript: entry %s has unsupported type %q",
			entry.ID,
			entry.Type,
		)
	}
}

func (r *sessionReducer) applyLifecycle(entry Entry) error {
	if err := r.requireNoPendingTools(entry); err != nil {
		return err
	}
	lifecycle := *entry.Lifecycle
	switch entry.Type {
	case RunStartEntry:
		if r.scope.RunID != "" {
			return sessionOrderError(entry, "starts while run %s is open", r.scope.RunID)
		}
		if _, exists := r.runIDs[lifecycle.RunID]; exists {
			return sessionOrderError(entry, "reuses run id %s", lifecycle.RunID)
		}
		r.scope = sessionScope{RunID: lifecycle.RunID}
		r.runIDs[lifecycle.RunID] = struct{}{}

	case RunEndEntry:
		if err := r.requireRun(entry, lifecycle.RunID); err != nil {
			return err
		}
		if r.scope.TurnID != "" || r.scope.StepID != "" {
			return sessionOrderError(entry, "ends before its open turn and step")
		}
		r.scope = sessionScope{}

	case TurnStartEntry:
		if err := r.requireRun(entry, lifecycle.RunID); err != nil {
			return err
		}
		if r.scope.TurnID != "" {
			return sessionOrderError(entry, "starts while turn %s is open", r.scope.TurnID)
		}
		if _, exists := r.turnIDs[lifecycle.TurnID]; exists {
			return sessionOrderError(entry, "reuses turn id %s", lifecycle.TurnID)
		}
		r.scope.TurnID = lifecycle.TurnID
		r.turnIDs[lifecycle.TurnID] = struct{}{}

	case TurnEndEntry:
		if err := r.requireTurn(entry, lifecycle); err != nil {
			return err
		}
		if r.scope.StepID != "" {
			return sessionOrderError(entry, "ends while step %s is open", r.scope.StepID)
		}
		r.scope.TurnID = ""

	case StepStartEntry:
		if err := r.requireTurn(entry, lifecycle); err != nil {
			return err
		}
		if r.scope.StepID != "" {
			return sessionOrderError(entry, "starts while step %s is open", r.scope.StepID)
		}
		if _, exists := r.stepIDs[lifecycle.StepID]; exists {
			return sessionOrderError(entry, "reuses step id %s", lifecycle.StepID)
		}
		r.scope.StepID = lifecycle.StepID
		r.stepIDs[lifecycle.StepID] = struct{}{}

	case StepEndEntry:
		if err := r.requireStep(entry, lifecycle); err != nil {
			return err
		}
		r.scope.StepID = ""
	}
	return nil
}

func (r *sessionReducer) applyMessage(
	index int,
	entry Entry,
) (llm.Message, []*reducedToolState, *reducedToolState, error) {
	message, ok := agent.ToLLM(entry.Message)
	if !ok {
		return nil, nil, nil, fmt.Errorf(
			"transcript: message entry %s has unsupported type %T",
			entry.ID,
			entry.Message,
		)
	}

	var requests []*reducedToolState
	var resolved *reducedToolState
	switch typed := message.(type) {
	case *llm.UserMessage:
		if err := r.requireNoPendingTools(entry); err != nil {
			return nil, nil, nil, err
		}
		if err := r.requireOpenTurn(entry); err != nil {
			return nil, nil, nil, err
		}

	case *llm.AssistantMessage:
		if err := r.requireNoPendingTools(entry); err != nil {
			return nil, nil, nil, err
		}
		if err := r.requireOpenStep(entry); err != nil {
			return nil, nil, nil, err
		}
		for _, call := range typed.ToolCalls() {
			tool, err := r.addToolRequest(index, entry, call)
			if err != nil {
				return nil, nil, nil, err
			}
			requests = append(requests, tool)
		}

	case *llm.ToolResultMessage:
		if err := r.requireOpenStep(entry); err != nil {
			return nil, nil, nil, err
		}
		tool, err := r.resolveTool(index, entry, typed)
		if err != nil {
			return nil, nil, nil, err
		}
		resolved = tool

	default:
		return nil, nil, nil, fmt.Errorf(
			"transcript: message entry %s has unsupported type %T",
			entry.ID,
			message,
		)
	}

	r.messageIDs[entry.ID] = struct{}{}
	return message, requests, resolved, nil
}

func (r *sessionReducer) addToolRequest(
	index int,
	entry Entry,
	call llm.ToolCall,
) (*reducedToolState, error) {
	if call.ID == "" {
		return nil, fmt.Errorf("transcript: assistant entry %s has a tool call without an id", entry.ID)
	}
	if _, exists := r.toolIDs[call.ID]; exists {
		return nil, fmt.Errorf("transcript: assistant entry %s repeats tool call id %s", entry.ID, call.ID)
	}
	arguments, err := json.Marshal(call.Arguments)
	if err != nil {
		return nil, fmt.Errorf(
			"transcript: assistant entry %s encodes tool call %s: %w",
			entry.ID,
			call.ID,
			err,
		)
	}
	tool := &reducedToolState{
		Request:          reducedToolRequest{ID: call.ID, Name: call.Name, Arguments: arguments},
		Scope:            r.scope,
		AssistantEntryID: entry.ID, AssistantEntryIndex: index,
		DispatchEntryIndex: -1, ResultEntryIndex: -1, OutcomeEntryIndex: -1,
	}
	r.toolIDs[call.ID] = struct{}{}
	r.tools[call.ID] = tool
	r.pendingTools = append(r.pendingTools, call.ID)
	return tool, nil
}

func (r *sessionReducer) applyToolDispatch(
	index int,
	entry Entry,
) (*reducedToolState, error) {
	if err := r.requireOpenStep(entry); err != nil {
		return nil, err
	}
	tool, exists := r.tools[entry.ToolCall.ToolCallID]
	if !exists || tool.ResultEntryID != "" {
		return nil, fmt.Errorf(
			"transcript: tool call entry %s has no unresolved assistant call %s",
			entry.ID,
			entry.ToolCall.ToolCallID,
		)
	}
	if tool.DispatchEntryID != "" {
		return nil, fmt.Errorf(
			"transcript: tool call entry %s repeats dispatch intent for %s",
			entry.ID,
			entry.ToolCall.ToolCallID,
		)
	}
	if tool.Request.Name != entry.ToolCall.ToolName {
		return nil, fmt.Errorf(
			"transcript: tool call entry %s names %q, want %q",
			entry.ID,
			entry.ToolCall.ToolName,
			tool.Request.Name,
		)
	}
	if !equalJSON(tool.Request.Arguments, entry.ToolCall.Arguments) {
		return nil, fmt.Errorf(
			"transcript: tool call entry %s arguments differ from assistant call %s",
			entry.ID,
			entry.ToolCall.ToolCallID,
		)
	}
	if tool.Scope != r.scope {
		return nil, fmt.Errorf(
			"transcript: tool call entry %s moved outside assistant call %s lifecycle",
			entry.ID,
			entry.ToolCall.ToolCallID,
		)
	}
	tool.DispatchEntryID = entry.ID
	tool.DispatchEntryIndex = index
	return tool, nil
}

func (r *sessionReducer) resolveTool(
	index int,
	entry Entry,
	result *llm.ToolResultMessage,
) (*reducedToolState, error) {
	if result == nil || len(r.pendingTools) == 0 {
		return nil, fmt.Errorf("transcript: tool result entry %s has no unresolved call", entry.ID)
	}
	wantID := r.pendingTools[0]
	tool := r.tools[wantID]
	if tool.Request.ID != result.ToolCallID {
		return nil, fmt.Errorf(
			"transcript: tool result entry %s resolves call %s out of model order; want %s",
			entry.ID,
			result.ToolCallID,
			tool.Request.ID,
		)
	}
	if tool.Request.Name != result.ToolName {
		return nil, fmt.Errorf(
			"transcript: tool result entry %s names %q, want %q",
			entry.ID,
			result.ToolName,
			tool.Request.Name,
		)
	}
	if tool.Scope != r.scope {
		return nil, fmt.Errorf(
			"transcript: tool result entry %s moved outside assistant call %s lifecycle",
			entry.ID,
			result.ToolCallID,
		)
	}
	tool.ResultEntryID = entry.ID
	tool.ResultEntryIndex = index
	r.pendingTools = r.pendingTools[1:]
	return tool, nil
}

func (r *sessionReducer) applyToolOutcome(
	index int,
	entry Entry,
) (*reducedToolState, error) {
	if err := r.requireOpenStep(entry); err != nil {
		return nil, err
	}
	tool, exists := r.tools[entry.ToolOutcome.ToolCallID]
	if !exists {
		return nil, fmt.Errorf(
			"transcript: tool outcome entry %s has no assistant call %s",
			entry.ID,
			entry.ToolOutcome.ToolCallID,
		)
	}
	if tool.ResultEntryID == "" {
		return nil, fmt.Errorf(
			"transcript: tool outcome entry %s precedes result for %s",
			entry.ID,
			entry.ToolOutcome.ToolCallID,
		)
	}
	if tool.OutcomeEntryID != "" {
		return nil, fmt.Errorf(
			"transcript: tool outcome entry %s repeats outcome for %s",
			entry.ID,
			entry.ToolOutcome.ToolCallID,
		)
	}
	if tool.Scope != r.scope {
		return nil, fmt.Errorf(
			"transcript: tool outcome entry %s moved outside assistant call %s lifecycle",
			entry.ID,
			entry.ToolOutcome.ToolCallID,
		)
	}
	tool.OutcomeEntryID = entry.ID
	tool.OutcomeEntryIndex = index
	return tool, nil
}

func (r *sessionReducer) requireNoPendingTools(entry Entry) error {
	if len(r.pendingTools) == 0 {
		return nil
	}
	return fmt.Errorf(
		"transcript: %s entry %s follows unresolved tool call %s",
		entry.Type,
		entry.ID,
		r.pendingTools[0],
	)
}

func (r *sessionReducer) requireOpenTurn(entry Entry) error {
	if r.scope.RunID == "" || r.scope.TurnID == "" {
		return sessionOrderError(entry, "has no open turn")
	}
	return nil
}

func (r *sessionReducer) requireOpenStep(entry Entry) error {
	if err := r.requireOpenTurn(entry); err != nil {
		return err
	}
	if r.scope.StepID == "" {
		return sessionOrderError(entry, "has no open step")
	}
	return nil
}

func (r *sessionReducer) requireRun(entry Entry, runID string) error {
	if r.scope.RunID == "" {
		return sessionOrderError(entry, "has no open run")
	}
	if r.scope.RunID != runID {
		return sessionOrderError(entry, "belongs to run %s, want %s", runID, r.scope.RunID)
	}
	return nil
}

func (r *sessionReducer) requireTurn(entry Entry, lifecycle Lifecycle) error {
	if err := r.requireRun(entry, lifecycle.RunID); err != nil {
		return err
	}
	if r.scope.TurnID == "" {
		return sessionOrderError(entry, "has no open turn")
	}
	if r.scope.TurnID != lifecycle.TurnID {
		return sessionOrderError(
			entry,
			"belongs to turn %s, want %s",
			lifecycle.TurnID,
			r.scope.TurnID,
		)
	}
	return nil
}

func (r *sessionReducer) requireStep(entry Entry, lifecycle Lifecycle) error {
	if err := r.requireTurn(entry, lifecycle); err != nil {
		return err
	}
	if r.scope.StepID == "" {
		return sessionOrderError(entry, "has no open step")
	}
	if r.scope.StepID != lifecycle.StepID {
		return sessionOrderError(
			entry,
			"belongs to step %s, want %s",
			lifecycle.StepID,
			r.scope.StepID,
		)
	}
	return nil
}

func sessionOrderError(entry Entry, format string, args ...any) error {
	return fmt.Errorf(
		"transcript: entry %s (%s) %s",
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
