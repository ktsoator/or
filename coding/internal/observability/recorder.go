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
	ApplicationStarted  = "application.started"
	ApplicationStopped  = "application.stopped"
	RunStarted          = "run.started"
	RunCompleted        = "run.completed"
	RunFailed           = "run.failed"
	TurnStarted         = "turn.started"
	TurnCompleted       = "turn.completed"
	TurnDiscarded       = "turn.discarded"
	ProviderStarted     = "provider.request.started"
	ProviderCompleted   = "provider.request.completed"
	ProviderFailed      = "provider.request.failed"
	HTTPAttemptStarted  = "provider.http_attempt.started"
	HTTPAttemptResponse = "provider.http_attempt.response"
	CheckpointCompleted = "checkpoint.completed"
	CheckpointFailed    = "checkpoint.failed"
)

// Event is one bounded, privacy-safe observability record. It intentionally
// has no arbitrary attribute bag: adding a field requires an explicit schema
// decision, which keeps prompts, tool arguments, and provider payloads out of
// the local diagnostic log.
type Event struct {
	Name      string
	Level     slog.Level
	Timestamp time.Time

	SessionID         string
	RunID             string
	TurnID            string
	RequestID         string
	Status            string
	ErrorCode         string
	Reason            string
	StartedAt         time.Time
	Duration          time.Duration
	TimeToFirstOutput time.Duration

	Provider           string
	Model              string
	ResponseModel      string
	ProviderResponseID string
	StopReason         string
	Attempt            int
	HTTPStatus         int
	MessageCount       int
	AttachmentCount    int

	InputTokens      int64
	InputUnknown     bool
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	TotalTokens      int64
	CostInput        float64
	CostOutput       float64
	CostCacheRead    float64
	CostCacheWrite   float64
	CostTotal        float64
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
	if event.TurnID != "" {
		record.AddAttrs(slog.String("turn_id", event.TurnID))
	}
	if event.RequestID != "" {
		record.AddAttrs(slog.String("provider_request_id", event.RequestID))
	}
	if event.Status != "" {
		record.AddAttrs(slog.String("status", event.Status))
	}
	if event.ErrorCode != "" {
		record.AddAttrs(slog.String("error_code", event.ErrorCode))
	}
	if event.Reason != "" {
		record.AddAttrs(slog.String("reason", event.Reason))
	}
	if !event.StartedAt.IsZero() {
		record.AddAttrs(slog.Time("started_at", event.StartedAt.UTC()))
	}
	if event.Duration > 0 {
		record.AddAttrs(slog.Int64("duration_ms", event.Duration.Milliseconds()))
	}
	if event.TimeToFirstOutput > 0 {
		record.AddAttrs(slog.Int64("time_to_first_output_ms", event.TimeToFirstOutput.Milliseconds()))
	}
	if event.Provider != "" {
		record.AddAttrs(slog.String("provider", event.Provider))
	}
	if event.Model != "" {
		record.AddAttrs(slog.String("model", event.Model))
	}
	if event.ResponseModel != "" {
		record.AddAttrs(slog.String("response_model", event.ResponseModel))
	}
	if event.ProviderResponseID != "" {
		record.AddAttrs(slog.String("provider_response_id", event.ProviderResponseID))
	}
	if event.StopReason != "" {
		record.AddAttrs(slog.String("stop_reason", event.StopReason))
	}
	if event.Attempt > 0 {
		record.AddAttrs(slog.Int("attempt", event.Attempt))
	}
	if event.HTTPStatus > 0 {
		record.AddAttrs(slog.Int("http_status", event.HTTPStatus))
	}
	if event.MessageCount > 0 {
		record.AddAttrs(slog.Int("message_count", event.MessageCount))
	}
	if event.AttachmentCount > 0 {
		record.AddAttrs(slog.Int("attachment_count", event.AttachmentCount))
	}
	if event.InputTokens != 0 {
		record.AddAttrs(slog.Int64("input_tokens", event.InputTokens))
	}
	if event.InputUnknown {
		record.AddAttrs(slog.Bool("input_unknown", true))
	}
	if event.OutputTokens != 0 {
		record.AddAttrs(slog.Int64("output_tokens", event.OutputTokens))
	}
	if event.CacheReadTokens != 0 {
		record.AddAttrs(slog.Int64("cache_read_tokens", event.CacheReadTokens))
	}
	if event.CacheWriteTokens != 0 {
		record.AddAttrs(slog.Int64("cache_write_tokens", event.CacheWriteTokens))
	}
	if event.TotalTokens != 0 {
		record.AddAttrs(slog.Int64("total_tokens", event.TotalTokens))
	}
	if event.CostInput != 0 {
		record.AddAttrs(slog.Float64("cost_input_usd", event.CostInput))
	}
	if event.CostOutput != 0 {
		record.AddAttrs(slog.Float64("cost_output_usd", event.CostOutput))
	}
	if event.CostCacheRead != 0 {
		record.AddAttrs(slog.Float64("cost_cache_read_usd", event.CostCacheRead))
	}
	if event.CostCacheWrite != 0 {
		record.AddAttrs(slog.Float64("cost_cache_write_usd", event.CostCacheWrite))
	}
	if event.CostTotal != 0 {
		record.AddAttrs(slog.Float64("cost_total_usd", event.CostTotal))
	}
	// Handlers return write errors, but recording is deliberately fail-open.
	_ = handler.Handle(context.Background(), record)
}
