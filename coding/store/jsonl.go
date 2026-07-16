package store

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// maxLine caps a single persisted message line: generous for long assistant
// turns while bounding memory on malformed input.
const maxLine = 16 << 20 // 16 MiB

// JSONL persists the transcript as JSON Lines, one message per line, in a file.
// Runs append; compaction rewrites the file atomically via Replace. It is safe
// for concurrent use.
//
// Only llm-backed messages are supported; a custom (UI-only) message has no llm
// projection and makes Append and Replace fail, since it cannot be serialized.
type JSONL struct {
	mu   sync.Mutex
	path string
}

// NewJSONL returns a Store backed by the file at path. The file is created on
// first write and need not exist yet.
func NewJSONL(path string) *JSONL {
	return &JSONL{path: path}
}

// Load reads the persisted transcript. A missing file is an empty history, not
// an error.
func (s *JSONL) Load(_ context.Context) ([]agent.AgentMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", s.path, err)
	}
	defer file.Close()

	var messages []agent.AgentMessage
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64<<10), maxLine)
	line := 0
	for scanner.Scan() {
		line++
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}
		message, err := llm.UnmarshalMessage(raw)
		if err != nil {
			return nil, fmt.Errorf("store: decode %s line %d: %w", s.path, line, err)
		}
		messages = append(messages, agent.FromLLM(message))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("store: read %s: %w", s.path, err)
	}
	return messages, nil
}

// Append writes the given messages to the end of the file, one JSON line each.
func (s *JSONL) Append(_ context.Context, messages ...agent.AgentMessage) error {
	if len(messages) == 0 {
		return nil
	}
	encoded, err := encodeLines(messages)
	if err != nil {
		return err
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
	if _, err := file.Write(encoded); err != nil {
		return fmt.Errorf("store: append %s: %w", s.path, err)
	}
	return nil
}

// Replace overwrites the file with messages, written atomically through a temp
// file and rename so a crash cannot leave a half-written transcript.
func (s *JSONL) Replace(_ context.Context, messages []agent.AgentMessage) error {
	encoded, err := encodeLines(messages)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("store: create session dir: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o644); err != nil {
		return fmt.Errorf("store: write %s: %w", s.path, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("store: replace %s: %w", s.path, err)
	}
	return nil
}

// encodeLines renders each message as one JSON line. It fails on a message with
// no llm projection.
func encodeLines(messages []agent.AgentMessage) ([]byte, error) {
	var buf bytes.Buffer
	for _, message := range messages {
		llmMessage, ok := agent.ToLLM(message)
		if !ok {
			return nil, fmt.Errorf("store: cannot persist custom message %T: it has no llm projection", message)
		}
		encoded, err := llm.MarshalMessage(llmMessage)
		if err != nil {
			return nil, fmt.Errorf("store: encode message: %w", err)
		}
		buf.Write(encoded)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}
