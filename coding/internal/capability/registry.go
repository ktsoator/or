// Package capability assembles optional coding-product behavior around the
// reusable agent SDK. It owns registration and composition only; permissions,
// persistence, and execution remain responsibilities of the coding engine.
package capability

import (
	"fmt"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/tools"
)

// Manifest identifies one capability contribution. Version is informational
// for built-in capabilities today and becomes the compatibility surface for a
// future external plugin adapter.
type Manifest struct {
	ID      string
	Version string
}

// ToolContribution is one tool registered by a capability. Replace must be set
// explicitly when a capability intentionally overrides an existing tool.
type ToolContribution struct {
	Tool    tools.Tool
	Replace bool
}

// Definition is the complete, build-time contribution of one capability.
// Hooks execute in capability registration order.
type Definition struct {
	Manifest       Manifest
	Tools          []ToolContribution
	PromptSections []string
	BeforeToolCall func(agent.BeforeToolCallCtx) (block bool, reason string)
	AfterToolCall  func(agent.AfterToolCallCtx) *agent.AfterToolCallResult
}

type registeredTool struct {
	tool   tools.Tool
	source string
}

// Registry collects capabilities while a coding session is being constructed.
// It is intentionally build-time only; callers should finish registration
// before handing its tools and hooks to an Agent.
type Registry struct {
	manifests      []Manifest
	capabilityByID map[string]struct{}
	tools          []registeredTool
	toolIndex      map[string]int
	promptSections []string
	promptSeen     map[string]bool
	beforeHooks    []func(agent.BeforeToolCallCtx) (bool, string)
	afterHooks     []func(agent.AfterToolCallCtx) *agent.AfterToolCallResult
}

// NewRegistry returns an empty capability registry.
func NewRegistry() *Registry {
	return &Registry{
		capabilityByID: make(map[string]struct{}),
		toolIndex:      make(map[string]int),
		promptSeen:     make(map[string]bool),
	}
}

// Register validates and atomically adds one capability definition.
func (r *Registry) Register(def Definition) error {
	id := strings.TrimSpace(def.Manifest.ID)
	if id == "" {
		return fmt.Errorf("capability id is required")
	}
	if _, exists := r.capabilityByID[id]; exists {
		return fmt.Errorf("capability %q is already registered", id)
	}

	seenTools := make(map[string]bool, len(def.Tools))
	for _, contribution := range def.Tools {
		name := strings.TrimSpace(contribution.Tool.Name())
		if name == "" {
			return fmt.Errorf("capability %q registered a tool without a name", id)
		}
		if seenTools[name] {
			return fmt.Errorf("capability %q registered tool %q more than once", id, name)
		}
		seenTools[name] = true
		if existing, exists := r.toolIndex[name]; exists && !contribution.Replace {
			return fmt.Errorf(
				"capability %q registered tool %q already provided by %q",
				id,
				name,
				r.tools[existing].source,
			)
		}
	}

	manifest := def.Manifest
	manifest.ID = id
	r.capabilityByID[id] = struct{}{}
	r.manifests = append(r.manifests, manifest)
	for _, contribution := range def.Tools {
		name := strings.TrimSpace(contribution.Tool.Name())
		if index, exists := r.toolIndex[name]; exists {
			r.tools[index] = registeredTool{tool: contribution.Tool, source: id}
			continue
		}
		r.toolIndex[name] = len(r.tools)
		r.tools = append(r.tools, registeredTool{tool: contribution.Tool, source: id})
	}
	for _, section := range def.PromptSections {
		section = strings.TrimSpace(section)
		if section == "" || r.promptSeen[section] {
			continue
		}
		r.promptSeen[section] = true
		r.promptSections = append(r.promptSections, section)
	}
	if def.BeforeToolCall != nil {
		r.beforeHooks = append(r.beforeHooks, def.BeforeToolCall)
	}
	if def.AfterToolCall != nil {
		r.afterHooks = append(r.afterHooks, def.AfterToolCall)
	}
	return nil
}

// Manifests returns registered capability metadata in registration order.
func (r *Registry) Manifests() []Manifest {
	return append([]Manifest(nil), r.manifests...)
}

// Tools returns active tools in deterministic advertise order. Replacing a tool
// preserves its original position.
func (r *Registry) Tools() []tools.Tool {
	result := make([]tools.Tool, len(r.tools))
	for index, registered := range r.tools {
		result[index] = registered.tool
	}
	return result
}

// ToolSource returns the capability ID that currently provides name.
func (r *Registry) ToolSource(name string) (string, bool) {
	index, ok := r.toolIndex[name]
	if !ok {
		return "", false
	}
	return r.tools[index].source, true
}

// PromptSections returns stable system-prompt sections in registration order.
func (r *Registry) PromptSections() []string {
	return append([]string(nil), r.promptSections...)
}

// BeforeToolCall returns the composed capability veto hook. The first block
// wins; a nil return means no capability registered this hook.
func (r *Registry) BeforeToolCall() func(agent.BeforeToolCallCtx) (bool, string) {
	if len(r.beforeHooks) == 0 {
		return nil
	}
	hooks := append([]func(agent.BeforeToolCallCtx) (bool, string)(nil), r.beforeHooks...)
	return func(ctx agent.BeforeToolCallCtx) (bool, string) {
		for _, hook := range hooks {
			if block, reason := hook(isolatedBeforeContext(ctx)); block {
				return true, reason
			}
		}
		return false, ""
	}
}

// isolatedBeforeContext keeps a veto hook observational. Validated arguments
// are JSON-shaped mutable maps and slices; each hook gets its own copy so it
// cannot rewrite what core permission checks or the tool eventually executes.
func isolatedBeforeContext(ctx agent.BeforeToolCallCtx) agent.BeforeToolCallCtx {
	ctx.Args = cloneJSONValue(ctx.Args)
	ctx.ToolCall.Arguments = cloneJSONMap(ctx.ToolCall.Arguments)
	return ctx
}

func cloneJSONMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = cloneJSONValue(value)
	}
	return cloned
}

func cloneJSONValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		return cloneJSONMap(value)
	case []any:
		cloned := make([]any, len(value))
		for index, item := range value {
			cloned[index] = cloneJSONValue(item)
		}
		return cloned
	default:
		return value
	}
}

// AfterToolCall returns a hook that applies capability overrides in registration
// order. Each hook observes the result produced by earlier hooks.
func (r *Registry) AfterToolCall() func(agent.AfterToolCallCtx) *agent.AfterToolCallResult {
	if len(r.afterHooks) == 0 {
		return nil
	}
	hooks := append([]func(agent.AfterToolCallCtx) *agent.AfterToolCallResult(nil), r.afterHooks...)
	return func(ctx agent.AfterToolCallCtx) *agent.AfterToolCallResult {
		var combined *agent.AfterToolCallResult
		for _, hook := range hooks {
			override := hook(ctx)
			if override == nil {
				continue
			}
			if combined == nil {
				combined = &agent.AfterToolCallResult{}
			}
			if override.Content != nil {
				ctx.Result.Content = override.Content
				combined.Content = override.Content
			}
			if override.Details != nil {
				ctx.Result.Details = override.Details
				combined.Details = override.Details
			}
			if override.IsError != nil {
				ctx.IsError = *override.IsError
				combined.IsError = override.IsError
			}
			if override.Terminate != nil {
				ctx.Result.Terminate = *override.Terminate
				combined.Terminate = override.Terminate
			}
		}
		return combined
	}
}
