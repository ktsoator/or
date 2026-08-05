// Package invocation defines product-owned metadata for resources explicitly
// invoked by a user message. It gives persistence and UI contracts a stable
// identity independent of the user-visible command text.
package invocation

import "fmt"

type Kind string

const PromptTemplate Kind = "prompt_template"

// Record identifies the exact resource the backend resolved for one user
// message. Source and Path describe the resolved resource, not client input.
type Record struct {
	Kind   Kind   `json:"kind"`
	Name   string `json:"name"`
	Source string `json:"source"`
	Path   string `json:"path"`
}

func (r Record) Validate() error {
	if r.Kind != PromptTemplate {
		return fmt.Errorf("invocation: unsupported kind %q", r.Kind)
	}
	if r.Name == "" || r.Source == "" || r.Path == "" {
		return fmt.Errorf("invocation: %s record is incomplete", r.Kind)
	}
	return nil
}
