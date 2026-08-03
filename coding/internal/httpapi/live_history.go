package httpapi

// activeRunHistory is a compact, replayable UI event log for the run that has
// not reached done yet. Access is serialized by the owning Hub mutex.
type activeRunHistory struct {
	startedAt string
	events    []wireEvent
}

type activeRunSnapshot struct {
	startedAt string
	events    []wireEvent
}

func (h *activeRunHistory) apply(event wireEvent) {
	switch event.Type {
	case wireEventRunStart:
		h.startedAt = event.StartedAt
		h.events = []wireEvent{event}
		return
	case wireEventDone:
		h.startedAt = ""
		h.events = nil
		return
	}
	if h.startedAt == "" {
		return
	}
	// These have dedicated snapshot fields and should not inflate the active
	// run log. Queued messages are reconstructed from the queue snapshot until
	// they are consumed and re-emitted as ordinary user messages.
	if event.Type == wireEventTitleUpdate || event.Type == wireEventTitleGeneration ||
		(event.Type == wireEventUserMessage && event.Queued) {
		return
	}

	last := len(h.events) - 1
	if last >= 0 && compactLiveEvent(&h.events[last], event) {
		return
	}
	h.events = append(h.events, event)
}

func compactLiveEvent(previous *wireEvent, next wireEvent) bool {
	if previous.Type == wireEventDelta && next.Type == wireEventDelta && previous.Kind == next.Kind {
		previous.Delta += next.Delta
		return true
	}
	if previous.Type == wireEventToolInputDelta && next.Type == wireEventToolInputDelta &&
		previous.ID == next.ID && previous.Tool == next.Tool &&
		sameOptionalInt(previous.ToolContentIndex, next.ToolContentIndex) {
		previous.Delta += next.Delta
		previous.Bytes += next.Bytes
		return true
	}
	return false
}

func sameOptionalInt(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func (h *activeRunHistory) snapshot() activeRunSnapshot {
	return activeRunSnapshot{
		startedAt: h.startedAt,
		events:    append([]wireEvent(nil), h.events...),
	}
}

// mergeActiveRunHistory replaces the engine's mutable active-run projection
// with events that have crossed the Hub sequence boundary. The user message
// immediately before run_start is retained; replaying the matching live
// user_message reconciles it in the client reducer.
func mergeActiveRunHistory(history []wireEvent, active activeRunSnapshot) []wireEvent {
	if active.startedAt == "" || len(active.events) == 0 {
		return history
	}
	boundary := -1
	for index, event := range history {
		if event.Type == wireEventRunStart && event.StartedAt == active.startedAt {
			boundary = index
		}
	}
	if boundary >= 0 {
		history = history[:boundary]
	}
	merged := make([]wireEvent, 0, len(history)+len(active.events))
	merged = append(merged, history...)
	return append(merged, active.events...)
}
