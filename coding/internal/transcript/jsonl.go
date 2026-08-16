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

const maxLine = 16 << 20 // 16 MiB

// JSONL persists a session log: one header followed by typed append-only
// entries.
type JSONL struct {
	mu       sync.Mutex
	path     string
	ready    bool
	nextSeq  int64
	entryIDs map[string]struct{}
}

func NewJSONL(path string) *JSONL { return &JSONL{path: path} }

func (s *JSONL) Load(_ context.Context) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *JSONL) Append(_ context.Context, entries ...Entry) error {
	if len(entries) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ensurePrivateDirectory(s.path); err != nil {
		return fmt.Errorf("store: create session dir: %w", err)
	}
	info, statErr := os.Stat(s.path)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("store: stat %s: %w", s.path, statErr)
	}
	empty := errors.Is(statErr, os.ErrNotExist) || info.Size() == 0
	if empty {
		s.ready = false
		s.nextSeq = 0
		s.entryIDs = nil
	} else if !s.ready {
		if _, err := s.loadLocked(); err != nil {
			return err
		}
	}
	if err := validateAppend(entries, s.nextSeq, s.entryIDs); err != nil {
		return err
	}
	encoded, err := encodeEntries(entries)
	if err != nil {
		return err
	}
	if empty {
		header, err := json.Marshal(NewHeader())
		if err != nil {
			return err
		}
		encoded = append(append(header, '\n'), encoded...)
	}

	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, privateFileMode)
	if err != nil {
		return fmt.Errorf("store: open %s: %w", s.path, err)
	}
	if err := file.Chmod(privateFileMode); err != nil {
		_ = file.Close()
		return fmt.Errorf("store: secure %s: %w", s.path, err)
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
	s.ready = true
	s.nextSeq += int64(len(entries))
	addEntryIDs(&s.entryIDs, entries)
	return nil
}

// Replace atomically installs entries as the complete session log. It is used
// for explicit history rewrites while the owning session is idle.
func (s *JSONL) Replace(_ context.Context, entries []Entry) error {
	if err := validateEntries(entries); err != nil {
		return err
	}
	encoded, err := encodeSession(entries)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ensurePrivateDirectory(s.path); err != nil {
		return fmt.Errorf("store: create session dir: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".transcript-*.tmp")
	if err != nil {
		return fmt.Errorf("store: create replacement for %s: %w", s.path, err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(privateFileMode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("store: secure replacement for %s: %w", s.path, err)
	}
	if _, err := temporary.Write(encoded); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("store: write replacement for %s: %w", s.path, err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("store: sync replacement for %s: %w", s.path, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("store: close replacement for %s: %w", s.path, err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("store: replace %s: %w", s.path, err)
	}
	removeTemporary = false
	s.ready = true
	s.nextSeq = int64(len(entries))
	s.entryIDs = collectEntryIDs(entries)
	return nil
}

func (s *JSONL) loadLocked() ([]Entry, error) {
	if _, err := secureExistingFile(s.path); err != nil {
		return nil, fmt.Errorf("store: secure %s: %w", s.path, err)
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		s.ready = false
		s.nextSeq = 0
		s.entryIDs = nil
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
		s.ready = false
		s.nextSeq = 0
		s.entryIDs = nil
		return nil, nil
	}

	var header Header
	if err := json.Unmarshal(lines[0], &header); err != nil {
		return nil, fmt.Errorf("store: decode session header: %w", err)
	}
	if header.Type != "session" {
		return nil, fmt.Errorf("store: invalid session header type %q", header.Type)
	}
	if header.Version != CurrentVersion {
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
	s.nextSeq = int64(len(entries))
	s.entryIDs = collectEntryIDs(entries)
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

func decodeEntries(lines [][]byte) ([]Entry, error) {
	entries := make([]Entry, 0, len(lines))
	for index, line := range lines {
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("store: decode session line %d: %w", index+2, err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func encodeEntries(entries []Entry) ([]byte, error) {
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

func encodeSession(entries []Entry) ([]byte, error) {
	header, err := json.Marshal(NewHeader())
	if err != nil {
		return nil, err
	}
	encoded, err := encodeEntries(entries)
	if err != nil {
		return nil, err
	}
	return append(append(header, '\n'), encoded...), nil
}

func validateEntries(entries []Entry) error {
	seen := make(map[string]bool, len(entries))
	for index, entry := range entries {
		if entry.Seq != int64(index) {
			return fmt.Errorf(
				"store: entry %s has sequence %d, want %d",
				entry.ID,
				entry.Seq,
				index,
			)
		}
		if seen[entry.ID] {
			return fmt.Errorf("store: duplicate entry id %s", entry.ID)
		}
		seen[entry.ID] = true
	}
	return nil
}

func validateAppend(entries []Entry, firstSeq int64, existingIDs map[string]struct{}) error {
	seen := make(map[string]bool, len(entries))
	for index, entry := range entries {
		want := firstSeq + int64(index)
		if entry.Seq != want {
			return fmt.Errorf(
				"store: append entry %s has sequence %d, want %d",
				entry.ID,
				entry.Seq,
				want,
			)
		}
		if _, exists := existingIDs[entry.ID]; exists {
			return fmt.Errorf("store: append repeats existing entry id %s", entry.ID)
		}
		if seen[entry.ID] {
			return fmt.Errorf("store: append repeats entry id %s", entry.ID)
		}
		seen[entry.ID] = true
	}
	return nil
}

func collectEntryIDs(entries []Entry) map[string]struct{} {
	ids := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		ids[entry.ID] = struct{}{}
	}
	return ids
}

func addEntryIDs(target *map[string]struct{}, entries []Entry) {
	if *target == nil {
		*target = make(map[string]struct{}, len(entries))
	}
	for _, entry := range entries {
		(*target)[entry.ID] = struct{}{}
	}
}
