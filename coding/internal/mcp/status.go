package mcp

type ServerState string

const (
	StateConnected  ServerState = "connected"
	StateDisabled   ServerState = "disabled"
	StateError      ServerState = "error"
	StateOutOfScope ServerState = "out_of_scope"
)

// ServerStatus is a secret-free connection diagnostic returned by mcp_status.
type ServerStatus struct {
	Name      string      `json:"name"`
	Transport string      `json:"transport,omitempty"`
	State     ServerState `json:"state"`
	ToolCount int         `json:"toolCount,omitempty"`
	Error     string      `json:"error,omitempty"`
}
