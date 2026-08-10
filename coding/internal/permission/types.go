// Package permission owns the coding product's tool authorization policy. It
// describes tool effects, resolves filesystem scope, and asks an approver when
// the session mode cannot allow an operation automatically.
package permission

import "context"

// Mode is the session-wide baseline applied before any one-off approval.
type Mode string

const (
	ModeAsk        Mode = "ask"
	ModeAutoEdit   Mode = "auto_edit"
	ModeFullAccess Mode = "full_access"
)

// Valid reports whether mode is accepted from a client or persisted record.
func (mode Mode) Valid() bool {
	switch mode {
	case ModeAsk, ModeAutoEdit, ModeFullAccess:
		return true
	default:
		return false
	}
}

// NormalizeMode keeps missing or unknown persisted values on the conservative
// default. API handlers still reject invalid values supplied by clients.
func NormalizeMode(mode Mode) Mode {
	if mode.Valid() {
		return mode
	}
	return ModeAsk
}

// Action describes the kind of access a tool call performs.
type Action string

const (
	Read     Action = "read"
	Write    Action = "write"
	Execute  Action = "execute"
	Network  Action = "network"
	Internal Action = "internal"
)

// Location describes where a filesystem access resolves relative to the
// session workspace.
type Location string

const (
	LocationUnknown  Location = "unknown"
	Workspace        Location = "workspace"
	OutsideWorkspace Location = "external"
)

// Access is one effect of a validated tool call. Filesystem tools fill Path;
// Service fills ResolvedPath, Location, and Sensitive before authorization.
type Access struct {
	Action       Action
	Path         string
	ResolvedPath string
	Location     Location
	Sensitive    SensitiveKind
}

// Request is the complete permission input for one validated tool call.
type Request struct {
	ToolCallID string
	Tool       string
	Args       map[string]any
	Accesses   []Access
}

// Result is the final authorization outcome after any required approval.
type Result struct {
	Allowed bool
	Reason  string
}

// ApprovalRequest is sent to the product surface when authorization needs the
// user's decision.
type ApprovalRequest struct {
	Request Request
	Reason  string
}

// ApprovalChoice is the user's response to an approval request.
type ApprovalChoice string

const (
	AllowOnce ApprovalChoice = "allow_once"
	Reject    ApprovalChoice = "deny"
)

// ApprovalResponse is deliberately extensible beyond a boolean so later
// milestones can add session grants without replacing the transport contract.
type ApprovalResponse struct {
	Choice ApprovalChoice
}

// Approver obtains a user decision. Implementations must honor ctx
// cancellation so aborting a run cannot leave a tool preflight blocked.
type Approver interface {
	Decide(context.Context, ApprovalRequest) (ApprovalResponse, error)
}
