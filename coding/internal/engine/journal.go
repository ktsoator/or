package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/modelcontext"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

// sessionJournal owns the durable conversation prefix and the structured tool
// outcomes associated with it. Keeping this state behind one boundary prevents
// Session's run, history, and compaction paths from coordinating its locks and
// stores independently.
type sessionJournal struct {
	store        transcript.Store
	detailsStore transcript.DetailsStore

	mu           sync.RWMutex
	entries      []transcript.Entry
	persistedLen int
	usageStart   int

	outcomeMu sync.Mutex
	outcomes  map[string]agent.ToolOutcome // tool-call ID -> terminal outcome
}

func newSessionJournal(
	ctx context.Context,
	store transcript.Store,
	detailsStore transcript.DetailsStore,
) (*sessionJournal, []agent.AgentMessage, []transcript.Entry, error) {
	var entries []transcript.Entry
	if store != nil {
		loaded, err := store.Load(ctx)
		if err != nil {
			return nil, nil, nil, err
		}
		entries = loaded
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
	if detailsStore != nil {
		stored, err := detailsStore.Load(ctx)
		if err != nil {
			return nil, nil, nil, err
		}
		for id, raw := range stored {
			if outcome, ok := decodeOutcome(raw); ok {
				outcomes[id] = outcome
			}
		}
	}

	journal := &sessionJournal{
		store:        store,
		detailsStore: detailsStore,
		entries:      append([]transcript.Entry(nil), entries...),
		persistedLen: len(seed),
		usageStart:   usageStart,
		outcomes:     outcomes,
	}
	return journal, seed, append([]transcript.Entry(nil), entries...), nil
}

// captureOutcomes subscribes to tool completions and retains each terminal
// outcome in memory, persisting it to the sidecar so reload restores status,
// error metadata, and structured data. It is registered once per session.
func (j *sessionJournal) captureOutcomes(source *agent.Agent) {
	source.Subscribe(func(ev agent.AgentEvent) {
		if ev.Type != agent.ToolEnd {
			return
		}
		result := ev.Result
		payload, ok := encodeOutcome(result.Outcome)
		if !ok {
			return
		}
		j.outcomeMu.Lock()
		j.outcomes[ev.ToolCallID] = result.Outcome
		j.outcomeMu.Unlock()
		if j.detailsStore != nil {
			// Persist out of band; a failure here must not disrupt the run, and the
			// live event already carried the outcome to any subscriber.
			_ = j.detailsStore.Put(context.Background(), ev.ToolCallID, payload)
		}
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

// modelStreamFn is the product's model-request boundary. It projects hidden
// product context into a detached provider input, checkpoints the canonical
// messages and any newly emitted context attachments, then reaches the
// provider. A persistence failure prevents both context commit and provider I/O.
func (s *Session) modelStreamFn(delegate agent.StreamFn) agent.StreamFn {
	if delegate == nil {
		delegate = llm.Stream
	}
	return func(
		ctx context.Context,
		model llm.Model,
		input llm.Context,
		options llm.StreamOptions,
	) (<-chan llm.Event, error) {
		prepared := s.modelContext.PrepareStep(input)
		if err := s.persistModelInput(ctx, input.Messages, prepared.Pending); err != nil {
			checkpointErr := fmt.Errorf("coding: persist model request checkpoint: %w", err)
			s.recordRunPersistenceError(checkpointErr)
			return nil, checkpointErr
		}
		s.modelContext.Commit(prepared)
		s.commitSkillRefresh(prepared.Pending)
		s.commitContextRefresh(prepared.Pending)
		return delegate(ctx, model, prepared.Input, options)
	}
}

// persistModelInput appends the canonical request prefix and any new hidden
// attachments before the provider is called. input is authoritative here:
// RunLoop may emit MessageEnd before Agent has reduced that event into its live
// snapshot, while input already contains the complete canonical prefix.
func (s *Session) persistModelInput(
	ctx context.Context,
	input []llm.Message,
	attachments []modelcontext.Attachment,
) error {
	messages := make([]agent.AgentMessage, len(input))
	for index, message := range input {
		messages[index] = agent.FromLLM(message)
	}
	contextEntries := make([]transcript.Entry, len(attachments))
	for index, attachment := range attachments {
		contextEntries[index] = transcript.NewContext(transcript.ContextAttachment{
			AttachmentID: attachment.ID,
			Epoch:        attachment.Epoch,
			Kind:         string(attachment.Kind),
			Placement:    string(attachment.Placement),
			Path:         attachment.Path,
			Revision:     attachment.Revision,
			Rendered:     attachment.Rendered,
		})
	}
	return s.journal.persistMessages(
		ctx,
		messages,
		contextEntries,
		0,
		time.Time{},
		time.Time{},
	)
}

// persistNew appends the messages added since the last persist to the Store. It
// runs only while runMu is held, so persistedLen is not racing a run.
func (s *Session) persistNew(ctx context.Context) error {
	return s.persistNewMessages(ctx, 0, time.Time{}, time.Time{})
}

func (s *Session) persistNewRun(
	ctx context.Context,
	runEntryStart int,
	startedAt, completedAt time.Time,
) error {
	return s.persistNewMessages(ctx, runEntryStart, startedAt, completedAt)
}

func (s *Session) persistNewMessages(
	ctx context.Context,
	runEntryStart int,
	startedAt, completedAt time.Time,
) error {
	return s.journal.persistMessages(
		ctx,
		s.agent.Snapshot().Messages,
		nil,
		runEntryStart,
		startedAt,
		completedAt,
	)
}

func (j *sessionJournal) persistMessages(
	ctx context.Context,
	all []agent.AgentMessage,
	contextEntries []transcript.Entry,
	runEntryStart int,
	startedAt, completedAt time.Time,
) error {
	j.mu.RLock()
	persistedLen := j.persistedLen
	existing := append([]transcript.Entry(nil), j.entries...)
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
	entries := make([]transcript.Entry, 0, len(contextEntries)+len(added)+1)
	entries = append(entries, contextEntries...)
	for _, message := range added {
		entries = append(entries, transcript.NewMessage(message))
	}
	if !startedAt.IsZero() && !completedAt.IsZero() {
		candidate := append(existing, entries...)
		firstEntryID := firstMessageFrom(candidate, runEntryStart)
		entries = append(entries, transcript.NewRun(firstEntryID, startedAt, completedAt))
	}
	if len(entries) == 0 {
		return nil
	}
	if j.store != nil {
		if err := j.store.Append(ctx, entries...); err != nil {
			return err
		}
	}
	j.mu.Lock()
	j.entries = append(j.entries, entries...)
	j.persistedLen = len(all)
	j.mu.Unlock()
	return nil
}

func (j *sessionJournal) appendCompaction(ctx context.Context, entry transcript.Entry) error {
	if j.store == nil {
		return nil
	}
	return j.store.Append(ctx, entry)
}

func (j *sessionJournal) applyCompaction(
	entries []transcript.Entry,
	projectedLen int,
) {
	j.mu.Lock()
	j.entries = append([]transcript.Entry(nil), entries...)
	j.usageStart = projectedLen
	j.persistedLen = projectedLen
	j.mu.Unlock()
}

func firstMessageFrom(entries []transcript.Entry, start int) string {
	if start < 0 || start >= len(entries) {
		return ""
	}
	for _, entry := range entries[start:] {
		if entry.Type == transcript.MessageEntry {
			return entry.ID
		}
	}
	return ""
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
