package mcpclient

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ProbeTool is compact protocol metadata discovered during a connection probe.
type ProbeTool struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

// ProbeResult reports a successful initialize and tools/list round trip.
type ProbeResult struct {
	Transport string      `json:"transport"`
	Tools     []ProbeTool `json:"tools"`
}

// Probe connects to one saved server long enough to discover its tools, then
// closes the connection. Calling it is an explicit user action in settings.
func Probe(ctx context.Context, name string, config ServerConfig, workspace string) (ProbeResult, error) {
	if err := config.Validate(); err != nil {
		return ProbeResult{}, err
	}
	if strings.TrimSpace(workspace) == "" {
		var err error
		workspace, err = os.Getwd()
		if err != nil {
			return ProbeResult{}, err
		}
	}
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return ProbeResult{}, err
	}
	workspace = filepath.Clean(absWorkspace)
	applies, err := config.AppliesTo(workspace)
	if err != nil {
		return ProbeResult{}, err
	}
	if !applies {
		return ProbeResult{}, fmt.Errorf("server is not available in workspace %s", workspace)
	}

	connection, err := Connect(ctx, name, config, workspace)
	if err != nil {
		return ProbeResult{}, err
	}
	defer connection.Close()

	result := ProbeResult{
		Transport: connection.Transport(),
		Tools:     make([]ProbeTool, 0, len(connection.Tools())),
	}
	for _, tool := range connection.Tools() {
		definition := tool.Definition()
		if definition == nil || strings.TrimSpace(definition.Name) == "" {
			continue
		}
		result.Tools = append(result.Tools, ProbeTool{
			Name:        definition.Name,
			Title:       protocolToolTitle(definition),
			Description: strings.TrimSpace(definition.Description),
		})
	}
	sort.Slice(result.Tools, func(i, j int) bool { return result.Tools[i].Name < result.Tools[j].Name })
	return result, nil
}

func protocolToolTitle(definition *protocol.Tool) string {
	if title := strings.TrimSpace(definition.Title); title != "" {
		return title
	}
	if definition.Annotations != nil {
		return strings.TrimSpace(definition.Annotations.Title)
	}
	return ""
}
