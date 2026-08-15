package contextprojection

import "github.com/ktsoator/or/llm"

type AttachmentKind string
type Placement string

const (
	BaseContext    AttachmentKind = "base"
	SkillListing   AttachmentKind = "skill_listing"
	SkillsUpdate   AttachmentKind = "skills_update"
	ActivatedSkill AttachmentKind = "activated_skill"
	ContextUpdate  AttachmentKind = "context_update"
	TaskStatus     AttachmentKind = "task_status"

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

// ProjectedAttachment locates one hidden attachment in the prepared provider
// message sequence. MessageIndex refers to PreparedStep.Input.Messages.
type ProjectedAttachment struct {
	Attachment
	MessageIndex int
}

// PreparedStep is the immutable model input for one request plus any context
// attachments that must become durable before that request reaches a provider.
type PreparedStep struct {
	Input       llm.Context
	Pending     []Attachment
	Attachments []ProjectedAttachment
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
	ActiveTaskRevision    string
	StagedTaskRevision    string
	ActivatedSkillCount   int
	PendingSkillCount     int
}
