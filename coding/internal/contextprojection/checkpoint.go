package contextprojection

// Commit marks exactly the attachments included in prepared as durable. A
// staged update becomes the sole active update of its kind only after its
// checkpoint succeeds.
func (manager *Manager) Commit(prepared PreparedStep) {
	if manager == nil || len(prepared.Pending) == 0 {
		return
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()

	for _, pending := range prepared.Pending {
		switch {
		case manager.base != nil && pending.ID == manager.base.ID:
			manager.base.committed = true
		case manager.listing != nil && pending.ID == manager.listing.ID:
			manager.listing.committed = true
		case manager.skills.commit(pending.ID):
		case manager.context.commit(pending.ID):
		case manager.tasks.commit(pending.ID):
		default:
			for _, activated := range manager.activated {
				if pending.ID == activated.ID {
					activated.committed = true
					break
				}
			}
		}
	}
}
