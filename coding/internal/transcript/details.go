package transcript

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// DetailsStore persists per-tool structured results out of band from the
// transcript, keyed by tool-call ID. The transcript itself carries only the
// model-facing text; recognized structured Details live here so a reloaded
// session can restore the same rich rendering and preview targets it showed
// live. A nil DetailsStore disables this and history replays as plain text.
type DetailsStore interface {
	// Load returns every stored payload keyed by tool-call ID.
	Load(ctx context.Context) (map[string]json.RawMessage, error)
	// Put records one tool call's payload. A later Put for the same ID wins.
	Put(ctx context.Context, callID string, payload json.RawMessage) error
}

// JSONLDetails persists tool-call payloads as JSON Lines, one record per line,
// appended as tools finish. On Load the last record for an ID wins, so a
// re-emitted result supersedes an earlier one. It is safe for concurrent use.
type JSONLDetails struct {
	mu   sync.Mutex
	path string
}

// NewJSONLDetails returns a DetailsStore backed by the file at path. The file is
// created on first write and need not exist yet.
func NewJSONLDetails(path string) *JSONLDetails {
	return &JSONLDetails{path: path}
}

type detailsRecord struct {
	ID      string          `json:"id"`
	Payload json.RawMessage `json:"payload"`
}

// Load reads all persisted payloads. A missing file is an empty map, not an
// error.
func (s *JSONLDetails) Load(_ context.Context) (map[string]json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]json.RawMessage{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", s.path, err)
	}
	defer file.Close()

	out := map[string]json.RawMessage{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64<<10), maxLine)
	line := 0
	for scanner.Scan() {
		line++
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}
		var rec detailsRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			return nil, fmt.Errorf("store: decode %s line %d: %w", s.path, line, err)
		}
		if rec.ID != "" {
			out[rec.ID] = rec.Payload
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("store: read %s: %w", s.path, err)
	}
	return out, nil
}

// Put appends one payload record to the file.
func (s *JSONLDetails) Put(_ context.Context, callID string, payload json.RawMessage) error {
	if callID == "" {
		return nil
	}
	encoded, err := json.Marshal(detailsRecord{ID: callID, Payload: payload})
	if err != nil {
		return fmt.Errorf("store: encode details: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("store: create session dir: %w", err)
	}
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("store: open %s: %w", s.path, err)
	}
	defer file.Close()
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("store: append %s: %w", s.path, err)
	}
	return nil
}

// MemoryDetails is an in-process DetailsStore for tests and ephemeral sessions.
type MemoryDetails struct {
	mu      sync.Mutex
	entries map[string]json.RawMessage
}

// NewMemoryDetails returns an empty in-memory DetailsStore.
func NewMemoryDetails() *MemoryDetails {
	return &MemoryDetails{entries: map[string]json.RawMessage{}}
}

func (m *MemoryDetails) Load(context.Context) (map[string]json.RawMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]json.RawMessage, len(m.entries))
	for k, v := range m.entries {
		out[k] = v
	}
	return out, nil
}

func (m *MemoryDetails) Put(_ context.Context, callID string, payload json.RawMessage) error {
	if callID == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[callID] = payload
	return nil
}
