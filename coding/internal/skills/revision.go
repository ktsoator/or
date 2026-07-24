package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Delta describes the semantic change between two resolved registries. Added
// and Updated retain full Skill values so the caller can advertise their
// current discovery metadata. Removed contains stable names.
type Delta struct {
	Added   []Skill
	Updated []Skill
	Removed []string
}

func (d Delta) Empty() bool {
	return len(d.Added) == 0 && len(d.Updated) == 0 && len(d.Removed) == 0
}

// Revision fingerprints the complete resolved registry, including skill bodies
// and source paths. A body-only edit therefore advances the revision even when
// its model-visible name and description do not change.
func (r *Registry) Revision() string {
	encoded, _ := json.Marshal(r.List())
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// Diff compares immutable registry snapshots in stable name order.
func Diff(before, after *Registry) Delta {
	if before == nil {
		before = NewRegistry(nil)
	}
	if after == nil {
		after = NewRegistry(nil)
	}

	var delta Delta
	for _, next := range after.List() {
		previous, ok := before.Lookup(next.Name)
		switch {
		case !ok:
			delta.Added = append(delta.Added, next)
		case previous != next:
			delta.Updated = append(delta.Updated, next)
		}
	}
	for _, previous := range before.List() {
		if _, ok := after.Lookup(previous.Name); !ok {
			delta.Removed = append(delta.Removed, previous.Name)
		}
	}
	return delta
}
