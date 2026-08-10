package mcpclient

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ProbeTool is the compact discovery information shown by the MCP settings UI.
type ProbeTool struct {
	Name        string `json:"name"`
	Original    string `json:"original"`
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

	connection, cancel, definitions, err := connect(ctx, name, config, workspace)
	if err != nil {
		return ProbeResult{}, err
	}
	defer func() {
		cancel()
		_ = connection.Close()
	}()

	result := ProbeResult{
		Transport: transportName(config),
		Tools:     make([]ProbeTool, 0, len(definitions)),
	}
	for _, definition := range definitions {
		if definition == nil || strings.TrimSpace(definition.Name) == "" {
			continue
		}
		title := ""
		if definition.Annotations != nil {
			title = strings.TrimSpace(definition.Annotations.Title)
		}
		result.Tools = append(result.Tools, ProbeTool{
			Name:        toolName(name, definition.Name),
			Original:    definition.Name,
			Title:       title,
			Description: strings.TrimSpace(definition.Description),
		})
	}
	sort.Slice(result.Tools, func(i, j int) bool { return result.Tools[i].Name < result.Tools[j].Name })
	return result, nil
}
