package skills

import (
	"sync/atomic"

	"github.com/ktsoator/or/agent"
)

// DynamicRegistry exposes one stable skill tool while atomically replacing the
// immutable Registry snapshot consulted by tool calls. Provider-visible tool
// definitions therefore stay byte-stable across skill additions, updates, and
// removals.
type DynamicRegistry struct {
	current atomic.Pointer[Registry]
}

func NewDynamicRegistry(initial *Registry) *DynamicRegistry {
	dynamic := &DynamicRegistry{}
	dynamic.Replace(initial)
	return dynamic
}

// Replace atomically publishes next. A nil registry means an empty snapshot.
func (d *DynamicRegistry) Replace(next *Registry) {
	if d == nil {
		return
	}
	if next == nil {
		next = NewRegistry(nil)
	}
	d.current.Store(next)
}

// Snapshot returns the current immutable registry.
func (d *DynamicRegistry) Snapshot() *Registry {
	if d == nil {
		return NewRegistry(nil)
	}
	if current := d.current.Load(); current != nil {
		return current
	}
	return NewRegistry(nil)
}

func (d *DynamicRegistry) List() []Skill {
	return d.Snapshot().List()
}

func (d *DynamicRegistry) Lookup(name string) (Skill, bool) {
	return d.Snapshot().Lookup(name)
}

func (d *DynamicRegistry) names() []string {
	return d.Snapshot().names()
}

// Tool returns a stable tool whose execution reads the registry snapshot that
// is current at call time.
func (d *DynamicRegistry) Tool() agent.AgentTool {
	return newTool(d.Lookup, d.names)
}
