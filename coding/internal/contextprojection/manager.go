package contextprojection

import "sync"

// Manager owns one process context epoch. The base context and initial skill
// listing are projected before canonical conversation messages so the provider
// prompt-cache prefix stays stable. Refreshes are projected after canonical
// messages as self-contained replacements, preserving that cached prefix.
type Manager struct {
	mu sync.Mutex

	epoch     uint64
	base      *trackedAttachment
	listing   *trackedAttachment
	skills    updateSlot
	context   updateSlot
	tasks     updateSlot
	activated map[string]*trackedAttachment
}

// New constructs an epoch from independently rendered base context and initial
// skill listing. Empty renderings produce no message or durable attachment.
func New(
	epoch uint64,
	baseRevision string,
	baseRendered string,
	skillRevision string,
	skillListingRendered string,
) *Manager {
	manager := &Manager{
		epoch:     epoch,
		skills:    updateSlot{kind: SkillsUpdate},
		context:   updateSlot{kind: ContextUpdate},
		tasks:     updateSlot{kind: TaskStatus},
		activated: make(map[string]*trackedAttachment),
	}
	if baseRendered != "" {
		if baseRevision == "" {
			baseRevision = revisionOf(baseRendered)
		}
		manager.base = newTracked(
			epoch,
			BaseContext,
			Prefix,
			baseRevision,
			baseRendered,
		)
	}
	if skillListingRendered != "" {
		if skillRevision == "" {
			skillRevision = revisionOf(skillListingRendered)
		}
		manager.listing = newTracked(
			epoch,
			SkillListing,
			Prefix,
			skillRevision,
			skillListingRendered,
		)
	}
	return manager
}
