package contextprojection

// State returns a detached diagnostic snapshot of the active projection.
func (manager *Manager) State() State {
	if manager == nil {
		return State{}
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()

	state := State{
		Epoch:                 manager.epoch,
		BaseCommitted:         manager.base == nil,
		SkillListingCommitted: manager.listing == nil,
	}
	if manager.base != nil {
		state.HasBase = true
		state.BaseRevision = manager.base.Revision
		state.BaseCommitted = manager.base.committed
	}
	if manager.listing != nil {
		state.HasSkillListing = true
		state.SkillListingRevision = manager.listing.Revision
		state.SkillListingCommitted = manager.listing.committed
	}
	state.ActiveSkillsRevision, state.StagedSkillsRevision = manager.skills.revisions()
	state.ActiveContextRevision, state.StagedContextRevision = manager.context.revisions()
	state.ActiveTaskRevision, state.StagedTaskRevision = manager.tasks.revisions()
	for _, activated := range manager.activated {
		state.ActivatedSkillCount++
		if !activated.committed {
			state.PendingSkillCount++
		}
	}
	return state
}
