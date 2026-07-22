// Package permission owns the coding product's tool authorization policy. It
// describes tool effects, resolves filesystem scope, and asks an approver when
// the policy cannot allow an operation automatically.
package permission

import "context"

// Action describes the kind of access a tool call performs.
type Action string

const (
	Read     Action = "read"
	Write    Action = "write"
	Execute  Action = "execute"
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

// Access is one effect of a validated tool call. Tools fill Path or Command;
// Service fills ResolvedPath and Location before applying policy.
type Access struct {
	Action          Action
	Path            string
	Command         string
	ResolvedPath    string
	Location        Location
	ResolutionError string
}

// Request is the complete permission input for one validated tool call.
type Request struct {
	ToolCallID string
	Tool       string
	Args       map[string]any
	Accesses   []Access
}

// Behavior is the policy outcome for a tool call.
type Behavior string

const (
	Allow Behavior = "allow"
	Ask   Behavior = "ask"
	Deny  Behavior = "deny"
)

// Decision is a structured authorization result.
type Decision struct {
	Behavior Behavior
	Reason   string
}

// Policy maps a resolved request to an authorization decision.
type Policy func(Request) Decision

// ApprovalRequest is sent to the product surface when policy returns Ask.
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
