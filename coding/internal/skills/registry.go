package skills

import "sort"

// Registry is an immutable, name-indexed set of loaded skills. It is built by
// Load and read concurrently; it is never mutated after construction.
type Registry struct {
	order  []string // skill names in stable (sorted) order
	byName map[string]Skill
}

// NewRegistry builds a Registry from an explicit set of skills, for callers that
// assemble or filter skills themselves rather than loading from disk. On a name
// collision the last skill wins.
func NewRegistry(skills []Skill) *Registry {
	byName := make(map[string]Skill, len(skills))
	for _, s := range skills {
		byName[s.Name] = s
	}
	return newRegistry(byName)
}

// newRegistry builds a Registry from resolved skills, ordering names
// deterministically so listings are stable across loads.
func newRegistry(byName map[string]Skill) *Registry {
	order := make([]string, 0, len(byName))
	for name := range byName {
		order = append(order, name)
	}
	sort.Strings(order)
	return &Registry{order: order, byName: byName}
}

// List returns the skills in stable name order.
func (r *Registry) List() []Skill {
	if r == nil {
		return nil
	}
	out := make([]Skill, len(r.order))
	for i, name := range r.order {
		out[i] = r.byName[name]
	}
	return out
}

// ModelList returns only skills the model may discover and invoke.
func (r *Registry) ModelList() []Skill {
	if r == nil {
		return nil
	}
	out := make([]Skill, 0, len(r.order))
	for _, name := range r.order {
		s := r.byName[name]
		if !s.DisableModelInvocation {
			out = append(out, s)
		}
	}
	return out
}

// Lookup returns the skill registered under name.
func (r *Registry) Lookup(name string) (Skill, bool) {
	if r == nil {
		return Skill{}, false
	}
	s, ok := r.byName[name]
	return s, ok
}

// ModelLookup returns a skill only when model invocation is allowed.
func (r *Registry) ModelLookup(name string) (Skill, bool) {
	s, ok := r.Lookup(name)
	if !ok || s.DisableModelInvocation {
		return Skill{}, false
	}
	return s, true
}

// Len reports how many skills are registered.
func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.order)
}

// names returns the registered names in stable order, for error messages.
func (r *Registry) names() []string {
	if r == nil {
		return nil
	}
	return append([]string(nil), r.order...)
}

// modelNames returns the names the model may pass to the skill tool.
func (r *Registry) modelNames() []string {
	list := r.ModelList()
	names := make([]string, len(list))
	for i, skill := range list {
		names[i] = skill.Name
	}
	return names
}
