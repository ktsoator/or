package api

import "sync"

// Each session owns one Hub, which fans its event stream out to every browser
// currently watching that session over SSE.

// Hub fans one session's events out to every connected browser over SSE. Each
// client has a buffered channel; a slow client is disconnected so EventSource
// can reconnect and replay from its Last-Event-ID without blocking the run.
type Hub struct {
	mu       sync.Mutex
	sequence uint64
	events   []hubFrame
	clients  map[chan hubFrame]struct{}
}

type hubFrame struct {
	sequence uint64
	data     []byte
}

const hubReplayLimit = 2048

// NewHub returns an empty Hub.
func NewHub() *Hub {
	return &Hub{clients: make(map[chan hubFrame]struct{})}
}

// Broadcast sends data to every connected client, skipping any whose buffer is
// full.
func (h *Hub) Broadcast(data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sequence++
	frame := hubFrame{sequence: h.sequence, data: append([]byte(nil), data...)}
	h.events = append(h.events, frame)
	if len(h.events) > hubReplayLimit {
		kept := make([]hubFrame, hubReplayLimit)
		copy(kept, h.events[len(h.events)-hubReplayLimit:])
		h.events = kept
	}
	for ch := range h.clients {
		select {
		case ch <- frame:
		default:
			// Force a slow client to reconnect with Last-Event-ID so it can replay
			// the complete sequence instead of silently missing a terminal event.
			delete(h.clients, ch)
			close(ch)
		}
	}
}

func (h *Hub) add(after uint64) (chan hubFrame, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if after > h.sequence {
		after = 0
	}
	if len(h.events) > 0 && after+1 < h.events[0].sequence {
		return nil, true
	}
	replay := 0
	for _, frame := range h.events {
		if frame.sequence > after {
			replay++
		}
	}
	ch := make(chan hubFrame, replay+64)
	for _, frame := range h.events {
		if frame.sequence > after {
			ch <- frame
		}
	}
	h.clients[ch] = struct{}{}
	return ch, false
}

func (h *Hub) remove(ch chan hubFrame) {
	h.mu.Lock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
	h.mu.Unlock()
}

// snapshot runs fn while broadcasts are paused, then returns the sequence that
// exactly follows that snapshot. A client can replay every later frame without
// leaving a gap between HTTP history and SSE.
func (h *Hub) snapshot(fn func()) uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	fn()
	return h.sequence
}
