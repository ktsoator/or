package harness

import (
	"fmt"
	"strings"

	"github.com/ktsoator/or/agent"
)

// SetTools replaces the full tool registry. The model is advertised the active
// subset; any active name no longer in the registry drops out. Changes apply
// from the next run.
func (h *Harness) SetTools(tools []agent.AgentTool) {
	h.cfgMu.Lock()
	h.toolset = append([]agent.AgentTool(nil), tools...)
	active := h.activeToolsLocked()
	h.cfgMu.Unlock()
	h.agent.SetTools(active)
}

// SetActiveTools restricts the tools advertised to the model to the named subset
// of the registry. Passing no names re-activates the whole registry. It returns
// an error naming any tool that is not registered, leaving the active set
// unchanged. Changes apply from the next run.
func (h *Harness) SetActiveTools(names ...string) error {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()

	if len(names) == 0 {
		h.activeNames = nil
		h.agent.SetTools(h.activeToolsLocked())
		return nil
	}

	registered := make(map[string]bool, len(h.toolset))
	for _, tool := range h.toolset {
		registered[tool.Definition.Name] = true
	}
	var unknown []string
	active := make(map[string]bool, len(names))
	for _, name := range names {
		if !registered[name] {
			unknown = append(unknown, name)
			continue
		}
		active[name] = true
	}
	if len(unknown) > 0 {
		return fmt.Errorf("harness: unknown tool(s): %s", strings.Join(unknown, ", "))
	}

	h.activeNames = active
	h.agent.SetTools(h.activeToolsLocked())
	return nil
}

// Tools returns a copy of the full tool registry.
func (h *Harness) Tools() []agent.AgentTool {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	return append([]agent.AgentTool(nil), h.toolset...)
}

// ActiveTools returns the tools currently advertised to the model, in registry
// order.
func (h *Harness) ActiveTools() []agent.AgentTool {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	return h.activeToolsLocked()
}

// activeToolsLocked returns the active subset in registry order. The caller must
// hold cfgMu, except at construction before the Harness is shared.
func (h *Harness) activeToolsLocked() []agent.AgentTool {
	if h.activeNames == nil {
		return append([]agent.AgentTool(nil), h.toolset...)
	}
	active := make([]agent.AgentTool, 0, len(h.activeNames))
	for _, tool := range h.toolset {
		if h.activeNames[tool.Definition.Name] {
			active = append(active, tool)
		}
	}
	return active
}
