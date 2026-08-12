package contextprojection

import "fmt"

// updateSlot holds the one current after-current block of a given kind. A new
// revision supersedes the previous one entirely, so projection cost stays
// bounded. A staged revision becomes active only after its checkpoint succeeds.
type updateSlot struct {
	kind   AttachmentKind
	active *trackedAttachment
	staged *trackedAttachment
}

func (slot *updateSlot) stage(epoch uint64, revision, rendered string) {
	if revision == "" || rendered == "" {
		return
	}
	if slot.staged != nil && slot.staged.Revision == revision {
		return
	}
	if slot.staged == nil && slot.active != nil && slot.active.Revision == revision {
		return
	}
	slot.staged = newTracked(epoch, slot.kind, AfterCurrent, revision, rendered)
}

func (slot *updateSlot) cancel() { slot.staged = nil }

func (slot *updateSlot) current() *trackedAttachment {
	if slot.staged != nil {
		return slot.staged
	}
	return slot.active
}

func (slot *updateSlot) commit(id string) bool {
	if slot.staged == nil || slot.staged.ID != id {
		return false
	}
	slot.staged.committed = true
	slot.active = slot.staged
	slot.staged = nil
	return true
}

func (slot *updateSlot) revisions() (active, staged string) {
	if slot.active != nil {
		active = slot.active.Revision
	}
	if slot.staged != nil {
		staged = slot.staged.Revision
	}
	return active, staged
}

// RestoreActivatedSkills installs durable Skill instructions recovered from the
// transcript. The first activation of a name wins so a Skill edit cannot change
// the instructions already governing an in-flight conversation.
func (manager *Manager) RestoreActivatedSkills(attachments []Attachment) {
	if manager == nil || len(attachments) == 0 {
		return
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	for _, attachment := range attachments {
		if attachment.Kind != ActivatedSkill || attachment.Path == "" || attachment.Rendered == "" {
			continue
		}
		if _, exists := manager.activated[attachment.Path]; exists {
			continue
		}
		copy := attachment
		manager.activated[attachment.Path] = &trackedAttachment{Attachment: copy, committed: true}
	}
}

// StageActivatedSkill protects one loaded Skill body from transcript
// compaction. Repeated activation is a no-op; the session keeps the exact first
// body snapshot until the conversation ends.
func (manager *Manager) StageActivatedSkill(name, revision, rendered string) {
	if manager == nil || name == "" || rendered == "" {
		return
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if _, exists := manager.activated[name]; exists {
		return
	}
	if revision == "" {
		revision = revisionOf(rendered)
	}
	tracked := newTracked(manager.epoch, ActivatedSkill, AfterCurrent, revision, rendered)
	tracked.ID = fmt.Sprintf("%s:%d:%s:%s", ActivatedSkill, manager.epoch, name, revision)
	tracked.Path = name
	manager.activated[name] = tracked
}

// StageSkillsUpdate prepares a complete replacement skill snapshot for the next
// provider request. It replaces an uncheckpointed staged update. A revision
// already active or staged is a no-op.
func (manager *Manager) StageSkillsUpdate(revision, rendered string) {
	if manager == nil {
		return
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.skills.stage(manager.epoch, revision, rendered)
}

// CancelStagedSkillsUpdate removes an update that has not reached a persistence
// checkpoint. The active, already durable update remains projected.
func (manager *Manager) CancelStagedSkillsUpdate() {
	if manager == nil {
		return
	}
	manager.mu.Lock()
	manager.skills.cancel()
	manager.mu.Unlock()
}

// StageContextUpdate prepares a complete replacement of the environment and
// instruction files for the next provider request. It supersedes the base
// context without disturbing the cached request prefix.
func (manager *Manager) StageContextUpdate(revision, rendered string) {
	if manager == nil {
		return
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.base != nil && manager.staleContextRevision(revision) {
		return
	}
	manager.context.stage(manager.epoch, revision, rendered)
}

// staleContextRevision reports whether revision is the state the model already
// sees from the base context, with no update layered on top. Callers hold mu.
func (manager *Manager) staleContextRevision(revision string) bool {
	return manager.context.active == nil &&
		manager.context.staged == nil &&
		manager.base.Revision == revision
}

// CancelStagedContextUpdate removes a context update that has not reached a
// persistence checkpoint.
func (manager *Manager) CancelStagedContextUpdate() {
	if manager == nil {
		return
	}
	manager.mu.Lock()
	manager.context.cancel()
	manager.mu.Unlock()
}

// StageTaskStatus prepares the latest bounded background-task snapshot for the
// next provider request. Each snapshot replaces the previous one entirely.
func (manager *Manager) StageTaskStatus(rendered string) {
	if manager == nil || rendered == "" {
		return
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.tasks.stage(manager.epoch, revisionOf(rendered), rendered)
}
