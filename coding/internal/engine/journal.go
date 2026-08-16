package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	snapshot, err := j.projections.Snapshot()
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
	runID string,
	completedAt time.Time,
	status transcript.LifecycleStatus,
	reason string,
) error {
	all := s.agent.Snapshot().Messages
	pending := s.pendingLifecycle()
	_, turnID := s.lifecycleIDs()
	turnEnd := transcript.NewTurnEnd(runID, turnID, status, reason)
	runEnd := transcript.NewRunEnd(runID, status, reason)
	turnEnd.Timestamp = completedAt.UTC()
	runEnd.Timestamp = completedAt.UTC()
	terminal := positionedLifecycle(
		len(all),
		turnEnd,
		runEnd,
	)
	positioned := append(append([]positionedJournalEntry(nil), pending...), terminal...)
	err := s.journal.persistMessages(
		ctx,
		all,
		nil,
		positioned,
	)
	if err == nil {
		s.clearPendingLifecycle(len(pending))
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
