// Package modelcontext owns product-generated context projected into model
// requests without adding fake user messages to the agent's canonical
// transcript.
package modelcontext

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/ktsoator/or/llm"
)

type AttachmentKind string
type Placement string

const (
	BaseContext   AttachmentKind = "base"
	SkillListing  AttachmentKind = "skill_listing"
	SkillsUpdate  AttachmentKind = "skills_update"
	ContextUpdate AttachmentKind = "context_update"

	Prefix       Placement = "prefix"
	AfterCurrent Placement = "after-current"
)

// Attachment is one hidden, product-generated context block. Rendered is the
// provider-visible text; the remaining fields are durable product metadata.
type Attachment struct {
	ID        string
	Epoch     uint64
	Kind      AttachmentKind
	Placement Placement
	Path      string
	Revision  string
	Rendered  string
}

// PreparedStep is the immutable model input for one request plus any context
// attachments that must become durable before that request reaches a provider.
type PreparedStep struct {
	Input   llm.Context
	Pending []Attachment
}

// State is a detached diagnostic snapshot.
type State struct {
	Epoch                 uint64
	HasBase               bool
	BaseRevision          string
	BaseCommitted         bool
	HasSkillListing       bool
	SkillListingRevision  string
	SkillListingCommitted bool
	ActiveSkillsRevision  string
	StagedSkillsRevision  string
	ActiveContextRevision string
	StagedContextRevision string
}

type trackedAttachment struct {
	Attachment
	committed bool
}

// updateSlot holds the one current after-current block of a given kind. A new
// revision supersedes the previous one entirely, so projection cost stays bounded
// no matter how often the underlying resource changes. A staged revision only
// becomes active once its transcript checkpoint succeeds.
type updateSlot struct {
	kind   AttachmentKind
	active *trackedAttachment
	staged *trackedAttachment
}

func (u *updateSlot) stage(epoch uint64, revision, rendered string) {
	if revision == "" || rendered == "" {
		return
	}
	if u.staged != nil && u.staged.Revision == revision {
		return
	}
	if u.staged == nil && u.active != nil && u.active.Revision == revision {
		return
	}
	u.staged = newTracked(epoch, u.kind, AfterCurrent, revision, rendered)
}

func (u *updateSlot) cancel() { u.staged = nil }

// current returns the block to project: the staged revision when one is waiting,
// otherwise the last durable one.
func (u *updateSlot) current() *trackedAttachment {
	if u.staged != nil {
		return u.staged
	}
	return u.active
}

// commit promotes the staged block when id identifies it.
func (u *updateSlot) commit(id string) bool {
	if u.staged == nil || u.staged.ID != id {
		return false
	}
	u.staged.committed = true
	u.active = u.staged
	u.staged = nil
	return true
}

func (u *updateSlot) revisions() (active, staged string) {
	if u.active != nil {
		active = u.active.Revision
	}
	if u.staged != nil {
		staged = u.staged.Revision
	}
	return active, staged
}

// Manager owns one process context epoch. The Base Context and the initial skill
// listing are projected before canonical conversation messages so the provider
// prompt-cache prefix stays stable. Refreshes of either resource are projected
// after canonical messages as self-contained blocks that supersede the prefix,
// which keeps the cached prefix intact across a change.
type Manager struct {
	mu sync.Mutex

	epoch   uint64
	base    *trackedAttachment
	listing *trackedAttachment
	skills  updateSlot
	context updateSlot
}

// New constructs an epoch from independently rendered Base Context and initial
// skill listing. Empty renderings produce no message or durable attachment.
func New(
	epoch uint64,
	baseRevision string,
	baseRendered string,
	skillRevision string,
	skillListingRendered string,
) *Manager {
	manager := &Manager{
		epoch:   epoch,
		skills:  updateSlot{kind: SkillsUpdate},
		context: updateSlot{kind: ContextUpdate},
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

// StageSkillsUpdate prepares a complete replacement skill snapshot for the next
// provider request. It replaces an uncheckpointed staged update. A revision
// already active or staged is a no-op.
func (m *Manager) StageSkillsUpdate(revision, rendered string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.skills.stage(m.epoch, revision, rendered)
}

// CancelStagedSkillsUpdate removes an update that has not reached a persistence
// checkpoint. The active, already durable update remains projected.
func (m *Manager) CancelStagedSkillsUpdate() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.skills.cancel()
	m.mu.Unlock()
}

// StageContextUpdate prepares a complete replacement of the environment and
// instruction files for the next provider request. It supersedes the Base
// Context without disturbing the cached request prefix.
func (m *Manager) StageContextUpdate(revision, rendered string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.base != nil && m.staleContextRevision(revision) {
		return
	}
	m.context.stage(m.epoch, revision, rendered)
}

// staleContextRevision reports whether revision is the state the model already
// sees from the Base Context, with no update layered on top. Callers hold mu.
func (m *Manager) staleContextRevision(revision string) bool {
	return m.context.active == nil &&
		m.context.staged == nil &&
		m.base.Revision == revision
}

// CancelStagedContextUpdate removes a context update that has not reached a
// persistence checkpoint.
func (m *Manager) CancelStagedContextUpdate() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.context.cancel()
	m.mu.Unlock()
}

// PrepareStep creates a detached provider input. Prefix attachments come before
// canonical messages; the latest context and skills updates come after them.
// Attachments remain provider-visible on every request but appear in Pending only
// until their transcript checkpoint succeeds.
func (m *Manager) PrepareStep(input llm.Context) PreparedStep {
	if m == nil {
		return PreparedStep{Input: cloneContext(input)}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	prepared := PreparedStep{Input: cloneContext(input)}
	prefix := compactAttachments(m.base, m.listing)
	suffix := compactAttachments(m.context.current(), m.skills.current())

	messages := make([]llm.Message, 0, len(prefix)+len(input.Messages)+len(suffix))
	for _, attachment := range prefix {
		messages = append(messages, llm.UserText(attachment.Rendered))
		if !attachment.committed {
			prepared.Pending = append(prepared.Pending, attachment.Attachment)
		}
	}
	messages = append(messages, input.Messages...)
	for _, attachment := range suffix {
		messages = append(messages, llm.UserText(attachment.Rendered))
		if !attachment.committed {
			prepared.Pending = append(prepared.Pending, attachment.Attachment)
		}
	}
	prepared.Input.Messages = messages
	return prepared
}

// Commit marks exactly the attachments included in prepared as durable. A staged
// update becomes the sole active update of its kind only after its checkpoint
// succeeds.
func (m *Manager) Commit(prepared PreparedStep) {
	if m == nil || len(prepared.Pending) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, pending := range prepared.Pending {
		switch {
		case m.base != nil && pending.ID == m.base.ID:
			m.base.committed = true
		case m.listing != nil && pending.ID == m.listing.ID:
			m.listing.committed = true
		case m.skills.commit(pending.ID):
		case m.context.commit(pending.ID):
		}
	}
}

func (m *Manager) State() State {
	if m == nil {
		return State{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	state := State{
		Epoch:                 m.epoch,
		BaseCommitted:         m.base == nil,
		SkillListingCommitted: m.listing == nil,
	}
	if m.base != nil {
		state.HasBase = true
		state.BaseRevision = m.base.Revision
		state.BaseCommitted = m.base.committed
	}
	if m.listing != nil {
		state.HasSkillListing = true
		state.SkillListingRevision = m.listing.Revision
		state.SkillListingCommitted = m.listing.committed
	}
	state.ActiveSkillsRevision, state.StagedSkillsRevision = m.skills.revisions()
	state.ActiveContextRevision, state.StagedContextRevision = m.context.revisions()
	return state
}

func compactAttachments(items ...*trackedAttachment) []*trackedAttachment {
	result := make([]*trackedAttachment, 0, len(items))
	for _, item := range items {
		if item != nil {
			result = append(result, item)
		}
	}
	return result
}

func newTracked(
	epoch uint64,
	kind AttachmentKind,
	placement Placement,
	revision string,
	rendered string,
) *trackedAttachment {
	return &trackedAttachment{Attachment: Attachment{
		ID:        fmt.Sprintf("%s:%d:%s", kind, epoch, revision),
		Epoch:     epoch,
		Kind:      kind,
		Placement: placement,
		Revision:  revision,
		Rendered:  rendered,
	}}
}

func cloneContext(input llm.Context) llm.Context {
	cloned := input
	cloned.Messages = append([]llm.Message(nil), input.Messages...)
	cloned.Tools = append([]llm.ToolDefinition(nil), input.Tools...)
	return cloned
}

func revisionOf(rendered string) string {
	sum := sha256.Sum256([]byte(rendered))
	return hex.EncodeToString(sum[:])
}
