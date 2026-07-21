package store

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

	"github.com/ktsoator/or/coding/transcript"
)

const maxLine = 16 << 20 // 16 MiB

// JSONL persists a v2 session log: one header followed by typed append-only
// entries.
type JSONL struct {
	mu    sync.Mutex
	path  string
	ready bool
}

func NewJSONL(path string) *JSONL { return &JSONL{path: path} }

func (s *JSONL) Load(_ context.Context) ([]transcript.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *JSONL) Append(_ context.Context, entries ...transcript.Entry) error {
	if len(entries) == 0 {
		return nil
	}
	encoded, err := encodeEntries(entries)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("store: create session dir: %w", err)
	}
	info, statErr := os.Stat(s.path)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("store: stat %s: %w", s.path, statErr)
	}
	if errors.Is(statErr, os.ErrNotExist) || info.Size() == 0 {
		header, err := json.Marshal(transcript.NewHeader())
		if err != nil {
			return err
		}
		encoded = append(append(header, '\n'), encoded...)
		s.ready = true
	} else if !s.ready {
		if _, err := s.loadLocked(); err != nil {
			return err
		}
	}

	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("store: open %s: %w", s.path, err)
	}
	if _, err := file.Write(encoded); err != nil {
		_ = file.Close()
		return fmt.Errorf("store: append %s: %w", s.path, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("store: sync %s: %w", s.path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("store: close %s: %w", s.path, err)
	}
	return nil
}

func (s *JSONL) loadLocked() ([]transcript.Entry, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: read %s: %w", s.path, err)
	}
	lines, err := splitLines(data)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, nil
	}

	var header transcript.Header
	if err := json.Unmarshal(lines[0], &header); err != nil {
		return nil, fmt.Errorf("store: decode session header: %w", err)
	}
	if header.Type != "session" {
		return nil, fmt.Errorf("store: invalid session header type %q", header.Type)
	}
	if header.Version != transcript.CurrentVersion {
		return nil, fmt.Errorf("store: unsupported session version %d", header.Version)
	}
	entries, err := decodeEntries(lines[1:])
	if err != nil {
		return nil, err
	}
	if err := validateEntries(entries); err != nil {
		return nil, err
	}
	s.ready = true
	return entries, nil
}

func splitLines(data []byte) ([][]byte, error) {
	var lines [][]byte
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64<<10), maxLine)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) > 0 {
			lines = append(lines, append([]byte(nil), line...))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("store: read JSONL: %w", err)
	}
	return lines, nil
}

func decodeEntries(lines [][]byte) ([]transcript.Entry, error) {
	entries := make([]transcript.Entry, 0, len(lines))
	for index, line := range lines {
		var entry transcript.Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("store: decode v2 line %d: %w", index+2, err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func encodeEntries(entries []transcript.Entry) ([]byte, error) {
	var buffer bytes.Buffer
	for _, entry := range entries {
		encoded, err := json.Marshal(entry)
		if err != nil {
			return nil, fmt.Errorf("store: encode entry: %w", err)
		}
		buffer.Write(encoded)
		buffer.WriteByte('\n')
	}
	return buffer.Bytes(), nil
}

func validateEntries(entries []transcript.Entry) error {
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if seen[entry.ID] {
			return fmt.Errorf("store: duplicate entry id %s", entry.ID)
		}
		if entry.ParentID != "" && !seen[entry.ParentID] {
			return fmt.Errorf("store: entry %s has unknown or forward parent %s", entry.ID, entry.ParentID)
		}
		seen[entry.ID] = true
	}
	return nil
}
