package client

import (
	"fmt"
	"strings"
)

// Config contains only the transport fields needed to connect one MCP server.
// Product concerns such as enablement and workspace visibility stay above the
// protocol client.
type Config struct {
	Command        string
	Args           []string
	Env            map[string]string
	Cwd            string
	URL            string
	Headers        map[string]string
	TimeoutSeconds int
}

// Validate checks the transport fields that do not require interpolation or
// opening a connection.
func (config Config) Validate() error {
	hasCommand := strings.TrimSpace(config.Command) != ""
	hasURL := strings.TrimSpace(config.URL) != ""
	if hasCommand == hasURL {
		return fmt.Errorf("configure exactly one of command or url")
	}
	if config.TimeoutSeconds < 0 {
		return fmt.Errorf("timeoutSeconds must not be negative")
	}
	return nil
}
