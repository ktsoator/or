// Package permission is the coding agent's permission layer. It decides whether a
// tool call may run, prompting the user for confirmation on borderline calls.
// The gate plugs into the agent loop's BeforeToolCall hook.
package permission

// Decision is the policy outcome for a tool call.
type Decision int

const (
	// Allow runs the call without confirmation.
	Allow Decision = iota
	// Ask runs the call only if the user confirms it.
	Ask
	// Deny blocks the call outright.
	Deny
)

// Request describes a tool call being evaluated.
type Request struct {
	// Tool is the tool's name.
	Tool string
	// Args are the validated call arguments.
	Args map[string]any
	// ReadOnly reports whether the tool leaves the workspace unchanged, taken
	// from the tool's metadata.
	ReadOnly bool
}

// Confirm asks the user to approve a call, returning true to allow it. It is
// invoked only for Ask decisions.
type Confirm func(Request) bool

// Gate decides whether tool calls may run. Classify maps a request to a
// decision; a nil Classify uses DefaultClassify. Confirm is invoked for Ask
// decisions; a nil Confirm treats Ask as Deny, which is the safe default when no
// interactive approver is wired up.
type Gate struct {
	Classify func(Request) Decision
	Confirm  Confirm
}

// DefaultClassify allows read-only tools and asks for everything else. It is the
// conservative baseline: a call that can change the workspace or run a command
// needs confirmation.
func DefaultClassify(req Request) Decision {
	if req.ReadOnly {
		return Allow
	}
	return Ask
}

// Check evaluates req and reports whether the call is blocked, with a reason for
// the model when it is. It matches the shape the agent loop's BeforeToolCall
// hook expects.
func (g Gate) Check(req Request) (block bool, reason string) {
	classify := g.Classify
	if classify == nil {
		classify = DefaultClassify
	}

	switch classify(req) {
	case Allow:
		return false, ""
	case Deny:
		return true, "blocked by policy"
	case Ask:
		if g.Confirm == nil {
			return true, "this tool requires confirmation, but no approver is configured"
		}
		if g.Confirm(req) {
			return false, ""
		}
		return true, "the user declined this action"
	default:
		return false, ""
	}
}
