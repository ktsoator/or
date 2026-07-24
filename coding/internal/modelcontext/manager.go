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
	SessionContext AttachmentKind = "session"
	Prefix         Placement      = "prefix"
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
	Epoch         uint64
	HasBase       bool
	BaseRevision  string
	BaseCommitted bool
}

// Manager owns one session process's model-context epoch. Phase one has a
// single stable Base Context attachment; nested and update attachments can be
// added without changing the projection boundary.
type Manager struct {
	mu        sync.Mutex
	epoch     uint64
	base      *Attachment
	committed bool
}

// New constructs an epoch from the currently rendered Base Context. An empty
// rendering produces no hidden message and no durable attachment.
func New(epoch uint64, rendered string) *Manager {
	manager := &Manager{epoch: epoch}
	if rendered == "" {
		manager.committed = true
		return manager
	}
	revision := revisionOf(rendered)
	manager.base = &Attachment{
		ID:        fmt.Sprintf("session:%d:%s", epoch, revision),
		Epoch:     epoch,
		Kind:      SessionContext,
		Placement: Prefix,
		Revision:  revision,
		Rendered:  rendered,
	}
	return manager
}

// PrepareStep prepends Base Context to a detached copy of input. Until Commit
// confirms its checkpoint, the attachment is returned in Pending on every
// preparation attempt.
func (m *Manager) PrepareStep(input llm.Context) PreparedStep {
	if m == nil {
		return PreparedStep{Input: cloneContext(input)}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	prepared := PreparedStep{Input: cloneContext(input)}
	if m.base == nil {
		return prepared
	}

	messages := make([]llm.Message, 0, len(input.Messages)+1)
	messages = append(messages, llm.UserText(m.base.Rendered))
	messages = append(messages, input.Messages...)
	prepared.Input.Messages = messages
	if !m.committed {
		prepared.Pending = []Attachment{*m.base}
	}
	return prepared
}

// Commit marks the pending Base Context durable. A stale or unrelated prepared
// step cannot commit a different attachment.
func (m *Manager) Commit(prepared PreparedStep) {
	if m == nil || len(prepared.Pending) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.base != nil && prepared.Pending[0].ID == m.base.ID {
		m.committed = true
	}
}

func (m *Manager) State() State {
	if m == nil {
		return State{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state := State{Epoch: m.epoch, BaseCommitted: m.committed}
	if m.base != nil {
		state.HasBase = true
		state.BaseRevision = m.base.Revision
	}
	return state
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
