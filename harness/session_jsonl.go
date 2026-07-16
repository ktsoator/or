package harness

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// maxSessionLine caps a single persisted message line, generous enough for long
// assistant turns while bounding memory on malformed input.
const maxSessionLine = 16 << 20 // 16 MiB

// JSONLSession persists the transcript as JSON Lines — one message per line — in
// a file, so a session resumes across process restarts. Normal runs append; Compact
// rewrites the file atomically via Replace. It is safe for concurrent use.
//
// Only llm-backed messages are supported. A custom (UI-only) AgentMessage has no
// llm projection and makes Append and Replace return an error, since the session
// cannot serialize it.
type JSONLSession struct {
	mu   sync.Mutex
	path string
}

// NewJSONLSession returns a session backed by the file at path. The file is
// created on first write and need not exist yet.
func NewJSONLSession(path string) *JSONLSession {
	return &JSONLSession{path: path}
}

// Load reads the persisted transcript. A missing file is an empty history, not
// an error.
func (s *JSONLSession) Load(_ context.Context) ([]agent.AgentMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("harness: open session %s: %w", s.path, err)
	}
	defer file.Close()

	var messages []agent.AgentMessage
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64<<10), maxSessionLine)
	line := 0
	for scanner.Scan() {
		line++
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}
		message, err := llm.UnmarshalMessage(raw)
		if err != nil {
			return nil, fmt.Errorf("harness: decode session %s line %d: %w", s.path, line, err)
		}
		messages = append(messages, agent.FromLLM(message))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("harness: read session %s: %w", s.path, err)
	}
	return messages, nil
}

// Append writes the given messages to the end of the file, one JSON line each.
func (s *JSONLSession) Append(_ context.Context, messages ...agent.AgentMessage) error {
	if len(messages) == 0 {
		return nil
	}
	encoded, err := encodeSessionLines(messages)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("harness: open session %s: %w", s.path, err)
	}
	defer file.Close()
	if _, err := file.Write(encoded); err != nil {
		return fmt.Errorf("harness: append session %s: %w", s.path, err)
	}
	return nil
}

// Replace overwrites the file with messages, written atomically through a temp
// file and rename so a crash cannot leave a half-written transcript.
func (s *JSONLSession) Replace(_ context.Context, messages []agent.AgentMessage) error {
	encoded, err := encodeSessionLines(messages)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o644); err != nil {
		return fmt.Errorf("harness: write session %s: %w", s.path, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("harness: replace session %s: %w", s.path, err)
	}
	return nil
}

// encodeSessionLines renders each message as one JSON line. It fails on a
// message with no llm projection.
func encodeSessionLines(messages []agent.AgentMessage) ([]byte, error) {
	var buf bytes.Buffer
	for _, message := range messages {
		llmMessage, ok := agent.ToLLM(message)
		if !ok {
			return nil, fmt.Errorf("harness: JSONLSession cannot persist custom message %T: it has no llm projection", message)
		}
		encoded, err := llm.MarshalMessage(llmMessage)
		if err != nil {
			return nil, fmt.Errorf("harness: encode session message: %w", err)
		}
		buf.Write(encoded)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}
