package engine

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

// sessionJournal owns the durable conversation prefix and the structured tool
// outcomes associated with it. Keeping this state behind one boundary prevents
// Session's run, history, and compaction paths from coordinating its locks and
// stores independently.
type sessionJournal struct {
	store transcript.Store

	commitMu     sync.Mutex
	mu           sync.RWMutex
	entries      []transcript.Entry
	persistedLen int
	usageStart   int
	validator    *transcript.SessionValidator
	projections  *transcript.ProjectionRegistry

	outcomeMu sync.Mutex
	outcomes  map[string]agent.ToolOutcome // tool-call ID -> terminal outcome
}

type positionedJournalEntry struct {
	messageIndex int
	entry        transcript.Entry
}

func newSessionJournal(
	ctx context.Context,
	store transcript.Store,
) (*sessionJournal, []agent.AgentMessage, []transcript.Entry, error) {
	var entries []transcript.Entry
	if store != nil {
		loaded, err := store.Load(ctx)
		if err != nil {
			return nil, nil, nil, err
		}
		entries = loaded
	}
	projections := transcript.NewProjectionRegistry()
	sessionView := transcript.NewSessionProjectionUnit()
	if err := projections.Register(sessionView); err != nil {
		return nil, nil, nil, fmt.Errorf("coding: register session projection: %w", err)
	}
	modelContextView := transcript.NewModelContextProjectionUnit()
	if err := projections.Register(modelContextView); err != nil {
		return nil, nil, nil, fmt.Errorf("coding: register model-context projection: %w", err)
	}
	if err := projections.Register(newTodoProjectionUnit()); err != nil {
		return nil, nil, nil, fmt.Errorf("coding: register todo projection: %w", err)
	}
	if err := projections.Register(newPlanModeProjectionUnit()); err != nil {
		return nil, nil, nil, fmt.Errorf("coding: register plan mode projection: %w", err)
	}
	validator, repairs, err := transcript.RecoverSessionWithProjections(entries, projections)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("coding: recover session transcript: %w", err)
	}
	if len(repairs) > 0 {
		if err := store.Append(ctx, repairs...); err != nil {
			return nil, nil, nil, fmt.Errorf("coding: persist session recovery: %w", err)
		}
		entries = append(entries, repairs...)
	}
	seed, err := transcript.BuildContext(entries)
	if err != nil {
		return nil, nil, nil, err
	}
	projectedSeed, err := modelContextView.Snapshot()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("coding: snapshot restored model context: %w", err)
	}
	if err := validateModelContextParity(seed, projectedSeed.Messages); err != nil {
		return nil, nil, nil, fmt.Errorf("coding: restored model-context parity: %w", err)
	}
	usageStart := 0
	for _, entry := range entries {
		if entry.Type == transcript.CompactionEntry {
			usageStart = len(seed)
		}
	}

	outcomes := map[string]agent.ToolOutcome{}
	for _, entry := range entries {
		if entry.Type != transcript.ToolOutcomeEntry || entry.ToolOutcome == nil {
			continue
		}
		if outcome, ok := decodeOutcome(*entry.ToolOutcome); ok {
			outcomes[entry.ToolOutcome.ToolCallID] = outcome
		}
	}

	journal := &sessionJournal{
		store:        store,
		entries:      append([]transcript.Entry(nil), entries...),
		persistedLen: len(seed),
		usageStart:   usageStart,
		validator:    validator,
		projections:  projections,
		outcomes:     outcomes,
	}
	return journal, seed, append([]transcript.Entry(nil), entries...), nil
}

// captureOutcomes subscribes to tool completions and retains each terminal
// outcome until the corresponding tool result is checkpointed. It is
// registered once per session.
func (j *sessionJournal) captureOutcomes(source *agent.Agent) {
	source.Subscribe(func(ev agent.AgentEvent) {
		if ev.Type != agent.ToolEnd {
			return
		}
		j.outcomeMu.Lock()
		j.outcomes[ev.ToolCallID] = ev.Result.Outcome
		j.outcomeMu.Unlock()
	})
}

// outcomesSnapshot returns a copy of the captured outcomes, safe to read while
// a run is appending more.
func (j *sessionJournal) outcomesSnapshot() map[string]agent.ToolOutcome {
	j.outcomeMu.Lock()
	defer j.outcomeMu.Unlock()
	out := make(map[string]agent.ToolOutcome, len(j.outcomes))
	for id, outcome := range j.outcomes {
		out[id] = outcome
	}
	return out
}

func (j *sessionJournal) entriesSnapshot() ([]transcript.Entry, int) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return append([]transcript.Entry(nil), j.entries...), j.persistedLen
}

func (j *sessionJournal) projectionSnapshot() (*transcript.SessionProjection, int, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	snapshot, err := j.projections.SnapshotKey(transcript.SessionProjectionKey)
	if err != nil {
		return nil, 0, err
	}
	value, ok := snapshot.Values[transcript.SessionProjectionKey]
	if !ok {
		return nil, 0, fmt.Errorf("coding: session projection is not registered")
	}
	projection, ok := value.(*transcript.SessionProjection)
	if !ok || projection == nil {
		return nil, 0, fmt.Errorf("coding: session projection has type %T", value)
	}
	wantSeq := j.validator.NextSeq() - 1
	if snapshot.AsOfSeq != wantSeq || projection.AsOfSeq != wantSeq {
		return nil, 0, fmt.Errorf(
			"coding: projection registry at sequence %d, session view at %d, validator at %d",
			snapshot.AsOfSeq,
			projection.AsOfSeq,
			wantSeq,
		)
	}
	return projection, j.persistedLen, nil
}

func (j *sessionJournal) modelContextSnapshot() (*transcript.ModelContextProjection, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	snapshot, err := j.projections.SnapshotKey(transcript.ModelContextProjectionKey)
	if err != nil {
		return nil, err
	}
	value, ok := snapshot.Values[transcript.ModelContextProjectionKey]
	if !ok {
		return nil, fmt.Errorf("coding: model-context projection is not registered")
	}
	projection, ok := value.(*transcript.ModelContextProjection)
	if !ok || projection == nil {
		return nil, fmt.Errorf("coding: model-context projection has type %T", value)
	}
	wantSeq := j.validator.NextSeq() - 1
	if snapshot.AsOfSeq != wantSeq || projection.AsOfSeq != wantSeq {
		return nil, fmt.Errorf(
			"coding: projection registry at sequence %d, model context at %d, validator at %d",
			snapshot.AsOfSeq,
			projection.AsOfSeq,
			wantSeq,
		)
	}
	return projection, nil
}

func (j *sessionJournal) todoSnapshot() (*TodoSnapshot, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	snapshot, err := j.projections.SnapshotKey(todoProjectionKey)
	if err != nil {
		return nil, err
	}
	value, ok := snapshot.Values[todoProjectionKey]
	if !ok {
		return nil, fmt.Errorf("coding: todo projection is not registered")
	}
	projection, ok := value.(*TodoSnapshot)
	if !ok {
		return nil, fmt.Errorf("coding: todo projection has type %T", value)
	}
	wantSeq := j.validator.NextSeq() - 1
	if snapshot.AsOfSeq != wantSeq {
		return nil, fmt.Errorf(
			"coding: todo projection at sequence %d, validator at %d",
			snapshot.AsOfSeq,
			wantSeq,
		)
	}
	return projection, nil
}

func (j *sessionJournal) planModeSnapshot() (PlanModeSnapshot, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	snapshot, err := j.projections.SnapshotKey(planModeProjectionKey)
	if err != nil {
		return PlanModeSnapshot{}, err
	}
	value, ok := snapshot.Values[planModeProjectionKey]
	if !ok {
		return PlanModeSnapshot{}, fmt.Errorf("coding: plan mode projection is not registered")
	}
	projection, ok := value.(PlanModeSnapshot)
	if !ok {
		return PlanModeSnapshot{}, fmt.Errorf("coding: plan mode projection has type %T", value)
	}
	wantSeq := j.validator.NextSeq() - 1
	if snapshot.AsOfSeq != wantSeq {
		return PlanModeSnapshot{}, fmt.Errorf(
			"coding: plan mode projection at sequence %d, validator at %d",
			snapshot.AsOfSeq,
			wantSeq,
		)
	}
	return projection, nil
}

func (j *sessionJournal) validateModelContext(expected []agent.AgentMessage) error {
	projection, err := j.modelContextSnapshot()
	if err != nil {
		return err
	}
	if err := validateModelContextParity(expected, projection.Messages); err != nil {
		return fmt.Errorf("coding: committed model-context parity: %w", err)
	}
	return nil
}

func (j *sessionJournal) reconstructCommittedProviderRequest(
	providerRequestID string,
) (transcript.ProviderRequest, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	snapshot, err := j.projections.Snapshot()
	if err != nil {
		return transcript.ProviderRequest{}, err
	}
	wantSeq := j.validator.NextSeq() - 1
	if snapshot.AsOfSeq != wantSeq {
		return transcript.ProviderRequest{}, fmt.Errorf(
			"coding: projection registry at sequence %d, validator at %d",
			snapshot.AsOfSeq,
			wantSeq,
		)
	}
	session, ok := snapshot.Values[transcript.SessionProjectionKey].(*transcript.SessionProjection)
	if !ok || session == nil {
		return transcript.ProviderRequest{}, fmt.Errorf(
			"coding: session projection has type %T",
			snapshot.Values[transcript.SessionProjectionKey],
		)
	}
	modelContext, ok := snapshot.Values[transcript.ModelContextProjectionKey].(*transcript.ModelContextProjection)
	if !ok || modelContext == nil {
		return transcript.ProviderRequest{}, fmt.Errorf(
			"coding: model-context projection has type %T",
			snapshot.Values[transcript.ModelContextProjectionKey],
		)
	}
	return transcript.ReconstructCommittedProviderRequest(
		session,
		modelContext,
		providerRequestID,
	)
}

func validateModelContextParity(
	expected []agent.AgentMessage,
	projected []agent.AgentMessage,
) error {
	if len(projected) != len(expected) {
		return fmt.Errorf(
			"message count %d, want %d",
			len(projected),
			len(expected),
		)
	}
	for index := range expected {
		expectedMessage, expectedOK := agent.ToLLM(expected[index])
		projectedMessage, projectedOK := agent.ToLLM(projected[index])
		if !expectedOK || !projectedOK {
			return fmt.Errorf(
				"message %d is not model-facing: expected %T, projected %T",
				index,
				expected[index],
				projected[index],
			)
		}
		expectedJSON, err := llm.MarshalMessage(expectedMessage)
		if err != nil {
			return fmt.Errorf("encode expected message %d: %w", index, err)
		}
		projectedJSON, err := llm.MarshalMessage(projectedMessage)
		if err != nil {
			return fmt.Errorf("encode projected message %d: %w", index, err)
		}
		if !bytes.Equal(projectedJSON, expectedJSON) {
			return fmt.Errorf(
				"message %d differs: expected %T, projected %T",
				index,
				expectedMessage,
				projectedMessage,
			)
		}
	}
	return nil
}

func (j *sessionJournal) usageStartIndex() int {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.usageStart
}

func (j *sessionJournal) messagesSnapshot(
	active []agent.AgentMessage,
) ([]agent.AgentMessage, error) {
	projection, persistedLen, err := j.projectionSnapshot()
	if err != nil {
		return nil, err
	}
	messages := make([]agent.AgentMessage, 0, len(projection.Messages)+len(active))
	for _, message := range projection.Messages {
		messages = append(messages, message.Message)
	}
	if persistedLen < len(active) {
		messages = append(messages, active[persistedLen:]...)
	}
	return messages, nil
}

// persistNew appends the messages added since the last persist to the Store. It
// runs only while runMu is held, so persistedLen is not racing a run.
func (s *Session) persistNew(ctx context.Context) error {
	return s.persistPendingLifecycle(ctx)
}

func (s *Session) persistRunTerminal(
	ctx context.Context,
	terminal lifecycleRunTerminal,
) error {
	all := s.agent.Snapshot().Messages
	err := s.journal.persistMessages(
		ctx,
		all,
		nil,
		terminal.entries,
	)
	if err == nil {
		s.lifecycle.commitPending(terminal.pendingCount)
	}
	return err
}

func (j *sessionJournal) persistMessages(
	ctx context.Context,
	all []agent.AgentMessage,
	contextEntries []transcript.Entry,
	positionedEntries []positionedJournalEntry,
	additionalEntries ...transcript.Entry,
) error {
	j.commitMu.Lock()
	defer j.commitMu.Unlock()

	j.mu.RLock()
	persistedLen := j.persistedLen
	j.mu.RUnlock()
	if persistedLen > len(all) {
		return fmt.Errorf(
			"coding: cannot persist context with %d messages behind durable prefix of %d",
			len(all),
			persistedLen,
		)
	}
	var added []agent.AgentMessage
	if persistedLen < len(all) {
		added = all[persistedLen:]
	}
	outcomes := j.outcomesSnapshot()
	entries := make(
		[]transcript.Entry,
		0,
		len(contextEntries)+2*len(added)+len(positionedEntries)+len(additionalEntries),
	)
	positionedIndex := 0
	for messageIndex := persistedLen; messageIndex <= len(all); messageIndex++ {
		for positionedIndex < len(positionedEntries) &&
			positionedEntries[positionedIndex].messageIndex == messageIndex {
			entries = append(entries, positionedEntries[positionedIndex].entry)
			positionedIndex++
		}
		if messageIndex == len(all) {
			entries = append(entries, contextEntries...)
			break
		}
		message := all[messageIndex]
		entries = append(entries, transcript.NewMessage(message))
		llmMessage, ok := agent.ToLLM(message)
		if !ok {
			continue
		}
		result, ok := llmMessage.(*llm.ToolResultMessage)
		if !ok {
			continue
		}
		outcome, ok := outcomes[result.ToolCallID]
		if !ok {
			continue
		}
		entries = append(entries, transcript.NewToolOutcome(
			encodeOutcome(result.ToolCallID, outcome),
		))
	}
	if positionedIndex != len(positionedEntries) {
		position := positionedEntries[positionedIndex].messageIndex
		return fmt.Errorf(
			"coding: lifecycle entry position %d is outside durable message range %d..%d",
			position,
			persistedLen,
			len(all),
		)
	}
	entries = append(entries, additionalEntries...)
	if len(entries) == 0 {
		return nil
	}
	sequenced, prepared, err := j.persistEntriesLocked(ctx, "session", entries)
	if err != nil {
		return err
	}
	j.mu.Lock()
	prepared.Commit()
	j.entries = append(j.entries, sequenced...)
	j.persistedLen = len(all)
	j.mu.Unlock()
	return nil
}

func (j *sessionJournal) appendCompaction(ctx context.Context, entry transcript.Entry) error {
	j.commitMu.Lock()
	defer j.commitMu.Unlock()

	sequenced, prepared, err := j.persistEntriesLocked(
		ctx,
		"compaction",
		[]transcript.Entry{entry},
	)
	if err != nil {
		return err
	}
	j.mu.Lock()
	prepared.Commit()
	j.entries = append(j.entries, sequenced[0])
	j.mu.Unlock()
	return nil
}

func (j *sessionJournal) appendPlanMode(ctx context.Context, active bool) error {
	j.commitMu.Lock()
	defer j.commitMu.Unlock()

	sequenced, prepared, err := j.persistEntriesLocked(
		ctx,
		"plan mode",
		[]transcript.Entry{transcript.NewPlanMode(active)},
	)
	if err != nil {
		return err
	}
	j.mu.Lock()
	prepared.Commit()
	j.entries = append(j.entries, sequenced[0])
	j.mu.Unlock()
	return nil
}

// persistEntriesLocked prepares and durably writes one batch without changing
// the journal. The caller holds commitMu and installs prepared only after this
// function succeeds.
func (j *sessionJournal) persistEntriesLocked(
	ctx context.Context,
	subject string,
	entries []transcript.Entry,
) ([]transcript.Entry, *transcript.PreparedAppend, error) {
	sequenced, err := transcript.SequenceEntries(entries, j.validator.NextSeq())
	if err != nil {
		return nil, nil, fmt.Errorf("coding: sequence %s append: %w", subject, err)
	}
	prepared, err := j.validator.PrepareAppend(sequenced)
	if err != nil {
		return nil, nil, fmt.Errorf("coding: validate %s append: %w", subject, err)
	}
	if j.store != nil {
		if err := j.store.Append(ctx, sequenced...); err != nil {
			return nil, nil, err
		}
	}
	return sequenced, prepared, nil
}

func (j *sessionJournal) applyCompaction(
	projectedLen int,
) {
	j.mu.Lock()
	j.usageStart = projectedLen
	j.persistedLen = projectedLen
	j.mu.Unlock()
}
