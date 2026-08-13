// Package observability records privacy-safe product lifecycle events.
package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"
)

const (
	ApplicationStarted = "application.started"
	ApplicationStopped = "application.stopped"
	RunStarted         = "run.started"
	RunCompleted       = "run.completed"
	RunFailed          = "run.failed"
)

// Event is one bounded, privacy-safe observability record. It intentionally
// has no arbitrary attribute bag: adding a field requires an explicit schema
// decision, which keeps prompts, tool arguments, and provider payloads out of
// the local diagnostic log.
type Event struct {
	Name      string
	Level     slog.Level
	Timestamp time.Time

	SessionID string
	RunID     string
	Status    string
	ErrorCode string
	StartedAt time.Time
	Duration  time.Duration
}

// Recorder accepts structured lifecycle events. Record is best-effort and must
// never make product work fail. Close releases recorder-owned resources.
type Recorder interface {
	Record(Event)
	Close() error
}

// DiscardRecorder ignores every event.
type DiscardRecorder struct{}

func (DiscardRecorder) Record(Event) {}
func (DiscardRecorder) Close() error { return nil }

// OrDiscard replaces a nil recorder with a no-op implementation.
func OrDiscard(recorder Recorder) Recorder {
	if recorder == nil {
		return DiscardRecorder{}
	}
	return recorder
}

var fallbackID atomic.Uint64

// NewID returns a process-local diagnostic ID with 128 bits of randomness.
// The fallback remains unique within the process if the system random source
// is unavailable; observability must not block a run because ID generation did.
func NewID(prefix string) string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return prefix + "_" + hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf(
		"%s_%x_%x_%x",
		prefix,
		os.Getpid(),
		time.Now().UnixNano(),
		fallbackID.Add(1),
	)
}

func recordWithHandler(handler slog.Handler, event Event) {
	if handler == nil || event.Name == "" {
		return
	}
	timestamp := event.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	record := slog.NewRecord(timestamp.UTC(), event.Level, event.Name, 0)
	if event.SessionID != "" {
		record.AddAttrs(slog.String("session_id", event.SessionID))
	}
	if event.RunID != "" {
		record.AddAttrs(slog.String("run_id", event.RunID))
	}
	if event.Status != "" {
		record.AddAttrs(slog.String("status", event.Status))
	}
	if event.ErrorCode != "" {
		record.AddAttrs(slog.String("error_code", event.ErrorCode))
	}
	if !event.StartedAt.IsZero() {
		record.AddAttrs(slog.Time("started_at", event.StartedAt.UTC()))
	}
	if event.Duration > 0 {
		record.AddAttrs(slog.Int64("duration_ms", event.Duration.Milliseconds()))
	}
	// Handlers return write errors, but recording is deliberately fail-open.
	_ = handler.Handle(context.Background(), record)
}
