package tools

// This file defines the structured results the mutating tools attach to
// agent.ToolResult.Details. The model-facing Content stays a short text summary
// derived from these values; the product reads Details to render a
// file-change row and an expandable diff instead of parsing the summary text.

// ChangeKind reports whether a write created a new file or updated an existing
// one.
type ChangeKind string

const (
	ChangeCreate ChangeKind = "create"
	ChangeUpdate ChangeKind = "update"
)

// FileChange is the structured result of a successful edit or write. It is the
// single source of truth: the tool's text Content is formatted from it, and UIs
// render it directly.
type FileChange struct {
	// Path is the file path as the model supplied it (relative to the workspace
	// root when the input was relative), so it can be passed straight to read.
	Path string
	// Kind is create for a new file or update for an existing one.
	Kind ChangeKind
	// Additions and Deletions are the line counts of the change.
	Additions int
	Deletions int
	// Hunks is the diff. A newly created non-empty file is represented as a diff
	// from an empty file. Lines within a hunk are prefixed with " ", "+", or "-".
	Hunks []Hunk
	// Bytes is the size of the content written.
	Bytes int
}

// Hunk is one contiguous region of a unified diff. Start lines are 1-based.
type Hunk struct {
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	// Lines carry a leading " ", "+", or "-" marking context, addition, or
	// deletion.
	Lines []string
}

// MutationFailure is the structured result attached to a failed edit or write,
// so a shell can show why the write did not happen rather than only a text blob.
type MutationFailure struct {
	// Path is the file path the model supplied.
	Path string
	// Reason is a stable machine code for the failure class.
	Reason string
	// Detail is the human-facing explanation, matching the text summary.
	Detail string
}

// Failure reason codes for MutationFailure.Reason.
const (
	FailureNotRead   = "not_read"  // an existing file was not read before mutating
	FailureChanged   = "changed"   // the file changed on disk since it was read
	FailureNotFound  = "not_found" // old_string was not present in the file
	FailureAmbiguous = "ambiguous" // old_string matched multiple places
	FailureIO        = "io"        // an I/O or filesystem error
	FailureInput     = "input"     // invalid arguments
)
