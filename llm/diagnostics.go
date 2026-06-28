package llm

import "time"

// Diagnostic records a non-fatal event that occurred while producing an
// assistant response — a failure that was recovered from, or a degraded result
// — without affecting the message content or stop reason. Callers may inspect
// AssistantMessage.Diagnostics to react to recoveries, for example to avoid
// auto-executing a tool call whose arguments were only partially recovered.
type Diagnostic struct {
	// Type categorizes the event, e.g. DiagnosticToolArgumentsRecovered.
	Type string `json:"type"`
	// Timestamp is the Unix millisecond time the diagnostic was recorded.
	Timestamp int64 `json:"timestamp"`
	// Message is an optional human-readable summary.
	Message string `json:"message,omitempty"`
	// Details carries structured, redacted context for the event.
	Details map[string]any `json:"details,omitempty"`
}

// DiagnosticToolArgumentsRecovered is the Type of a diagnostic recorded when a
// tool call's arguments could not be parsed strictly and were repaired,
// partially recovered, or discarded.
const DiagnosticToolArgumentsRecovered = "tool_arguments_recovered"

// ToolArgumentsDiagnostic builds a diagnostic describing how a tool call's
// arguments were recovered. ok is false for a clean (strict) parse, which needs
// no diagnostic.
func ToolArgumentsDiagnostic(toolCallID, toolName string, mode ArgumentsMode) (Diagnostic, bool) {
	if mode == ArgumentsStrict {
		return Diagnostic{}, false
	}
	return Diagnostic{
		Type:      DiagnosticToolArgumentsRecovered,
		Timestamp: time.Now().UnixMilli(),
		Details: map[string]any{
			"toolCallId": toolCallID,
			"toolName":   toolName,
			"mode":       string(mode),
		},
	}, true
}
