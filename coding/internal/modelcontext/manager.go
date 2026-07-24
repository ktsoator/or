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
	BaseContext  AttachmentKind = "base"
	SkillListing AttachmentKind = "skill_listing"
	SkillsUpdate AttachmentKind = "skills_update"

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
}

type trackedAttachment struct {
	Attachment
	committed bool
}

// Manager owns one process context epoch. Stable Base Context and the initial
// skill listing are projected before canonical conversation messages. At most
// one current skills-update block is projected after canonical messages; each
// new update fully supersedes the previous one so projection cost stays bounded.
type Manager struct {
	mu sync.Mutex

	epoch   uint64
	base    *trackedAttachment
	listing *trackedAttachment
	active  *trackedAttachment
	staged  *trackedAttachment
}

// New constructs an epoch from independently rendered Base Context and initial
// skill listing. Empty renderings produce no message or durable attachment.
func New(
	epoch uint64,
	baseRendered string,
	skillRevision string,
	skillListingRendered string,
) *Manager {
	manager := &Manager{epoch: epoch}
	if baseRendered != "" {
		manager.base = newTracked(
			epoch,
			BaseContext,
			Prefix,
			revisionOf(baseRendered),
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
	if m == nil || revision == "" || rendered == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.staged != nil && m.staged.Revision == revision {
		return
	}
	if m.staged == nil && m.active != nil && m.active.Revision == revision {
		return
	}
	m.staged = newTracked(
		m.epoch,
		SkillsUpdate,
		AfterCurrent,
		revision,
		rendered,
	)
}

// CancelStagedSkillsUpdate removes an update that has not reached a persistence
// checkpoint. The active, already durable update remains projected.
func (m *Manager) CancelStagedSkillsUpdate() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.staged = nil
	m.mu.Unlock()
}

// PrepareStep creates a detached provider input. Prefix attachments come before
// canonical messages; the latest skills update comes after them. Attachments
// remain provider-visible on every request but appear in Pending only until
// their transcript checkpoint succeeds.
func (m *Manager) PrepareStep(input llm.Context) PreparedStep {
	if m == nil {
		return PreparedStep{Input: cloneContext(input)}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	prepared := PreparedStep{Input: cloneContext(input)}
	prefix := compactAttachments(m.base, m.listing)
	update := m.active
	if m.staged != nil {
		update = m.staged
	}

	messages := make([]llm.Message, 0, len(prefix)+len(input.Messages)+1)
	for _, attachment := range prefix {
		messages = append(messages, llm.UserText(attachment.Rendered))
		if !attachment.committed {
			prepared.Pending = append(prepared.Pending, attachment.Attachment)
		}
	}
	messages = append(messages, input.Messages...)
	if update != nil {
		messages = append(messages, llm.UserText(update.Rendered))
		if !update.committed {
			prepared.Pending = append(prepared.Pending, update.Attachment)
		}
	}
	prepared.Input.Messages = messages
	return prepared
}

// Commit marks exactly the attachments included in prepared as durable. A
// staged skills update becomes the sole active update only after its checkpoint
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
		case m.staged != nil && pending.ID == m.staged.ID:
			m.staged.committed = true
			m.active = m.staged
			m.staged = nil
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
	if m.active != nil {
		state.ActiveSkillsRevision = m.active.Revision
	}
	if m.staged != nil {
		state.StagedSkillsRevision = m.staged.Revision
	}
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
