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
		toolRepairs, err := transcript.RepairInterruptedToolCalls(entries)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("coding: validate session transcript: %w", err)
		}
		toolRepairs, err = transcript.SequenceEntries(toolRepairs, int64(len(entries)))
		if err != nil {
			return nil, nil, nil, fmt.Errorf("coding: sequence session tool recovery: %w", err)
		}
		candidate := append(append([]transcript.Entry(nil), entries...), toolRepairs...)
		lifecycleRepairs, err := transcript.RepairInterruptedLifecycle(candidate)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("coding: validate session lifecycle: %w", err)
		}
		lifecycleRepairs, err = transcript.SequenceEntries(lifecycleRepairs, int64(len(candidate)))
		if err != nil {
			return nil, nil, nil, fmt.Errorf("coding: sequence session lifecycle recovery: %w", err)
		}
		repairs := append(toolRepairs, lifecycleRepairs...)
		recovered := append(append([]transcript.Entry(nil), entries...), repairs...)
		if _, err := transcript.ProjectSession(recovered); err != nil {
			return nil, nil, nil, fmt.Errorf("coding: project session transcript: %w", err)
		}
		if len(repairs) > 0 {
			if err := store.Append(ctx, repairs...); err != nil {
				return nil, nil, nil, fmt.Errorf("coding: persist session recovery: %w", err)
			}
			entries = append(entries, repairs...)
		}
	}
	validator, err := transcript.ValidateSession(entries)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("coding: validate recovered session: %w", err)
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

// snapshotOutcomes returns a copy of the captured outcomes, safe to read while
// a run is appending more.
func (j *sessionJournal) snapshotOutcomes() map[string]agent.ToolOutcome {
	j.outcomeMu.Lock()
	defer j.outcomeMu.Unlock()
	out := make(map[string]agent.ToolOutcome, len(j.outcomes))
	for id, outcome := range j.outcomes {
		out[id] = outcome
	}
	return out
}

func (j *sessionJournal) snapshot() ([]transcript.Entry, int) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return append([]transcript.Entry(nil), j.entries...), j.persistedLen
}

func (j *sessionJournal) usageStartIndex() int {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.usageStart
}

func (j *sessionJournal) messages(active []agent.AgentMessage) []agent.AgentMessage {
	entries, persistedLen := j.snapshot()
	messages := transcript.Messages(entries)
	if persistedLen < len(active) {
		messages = append(messages, active[persistedLen:]...)
	}
	return messages
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
	outcomes := j.snapshotOutcomes()
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
	sequenced, err := transcript.SequenceEntries(entries, j.validator.NextSeq())
	if err != nil {
		return fmt.Errorf("coding: sequence session append: %w", err)
	}
	nextValidator, err := j.validator.ValidateAppend(sequenced)
	if err != nil {
		return fmt.Errorf("coding: validate session append: %w", err)
	}
	if j.store != nil {
		if err := j.store.Append(ctx, sequenced...); err != nil {
			return err
		}
	}
	j.mu.Lock()
	j.entries = append(j.entries, sequenced...)
	j.persistedLen = len(all)
	j.validator = nextValidator
	j.mu.Unlock()
	return nil
}

func (j *sessionJournal) appendCompaction(ctx context.Context, entry transcript.Entry) error {
	j.commitMu.Lock()
	defer j.commitMu.Unlock()

	sequenced, err := transcript.SequenceEntries([]transcript.Entry{entry}, j.validator.NextSeq())
	if err != nil {
		return fmt.Errorf("coding: sequence compaction append: %w", err)
	}
	nextValidator, err := j.validator.ValidateAppend(sequenced)
	if err != nil {
		return fmt.Errorf("coding: validate compaction append: %w", err)
	}
	if j.store != nil {
		if err := j.store.Append(ctx, sequenced...); err != nil {
			return err
		}
	}
	j.mu.Lock()
	j.entries = append(j.entries, sequenced[0])
	j.validator = nextValidator
	j.mu.Unlock()
	return nil
}

func (j *sessionJournal) applyCompaction(
	projectedLen int,
) {
	j.mu.Lock()
	j.usageStart = projectedLen
	j.persistedLen = projectedLen
	j.mu.Unlock()
}

// Messages returns every original message on the current transcript path. A
// compacted session therefore still exposes its complete history.
func (s *Session) Messages() []agent.AgentMessage {
	return s.journal.messages(s.agent.Snapshot().Messages)
}

// Entries returns a detached snapshot of the durable session log.
func (s *Session) Entries() []transcript.Entry {
	return s.snapshotTranscript()
}

func (s *Session) snapshotTranscript() []transcript.Entry {
	entries, _ := s.snapshotTranscriptState()
	return entries
}

func (s *Session) snapshotTranscriptState() ([]transcript.Entry, int) {
	return s.journal.snapshot()
}

func (s *Session) snapshotOutcomes() map[string]agent.ToolOutcome {
	return s.journal.snapshotOutcomes()
}
