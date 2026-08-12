package contextprojection

import (
	"sort"

	"github.com/ktsoator/or/llm"
)

// PrepareStep creates a detached provider input. Prefix attachments come before
// canonical messages; the latest context and skills updates come after them.
// Attachments remain provider-visible on every request but appear in Pending
// only until their transcript checkpoint succeeds.
func (manager *Manager) PrepareStep(input llm.Context) PreparedStep {
	if manager == nil {
		return PreparedStep{Input: cloneContext(input)}
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()

	prepared := PreparedStep{Input: cloneContext(input)}
	prefix := compactAttachments(manager.base, manager.listing)
	suffix := compactAttachments(manager.context.current(), manager.skills.current())
	suffix = append(suffix, manager.activatedSkills()...)
	suffix = append(suffix, compactAttachments(manager.tasks.current())...)

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

// ProjectedAttachments returns the hidden context blocks that the next provider
// request would receive, in projection order. The returned values are detached
// from the Manager's mutable tracking state.
func (manager *Manager) ProjectedAttachments() []Attachment {
	if manager == nil {
		return nil
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()

	projected := compactAttachments(manager.base, manager.listing)
	projected = append(projected, compactAttachments(manager.context.current(), manager.skills.current())...)
	projected = append(projected, manager.activatedSkills()...)
	projected = append(projected, compactAttachments(manager.tasks.current())...)
	result := make([]Attachment, len(projected))
	for index, attachment := range projected {
		result[index] = attachment.Attachment
	}
	return result
}

func (manager *Manager) activatedSkills() []*trackedAttachment {
	names := make([]string, 0, len(manager.activated))
	for name := range manager.activated {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]*trackedAttachment, 0, len(names))
	for _, name := range names {
		result = append(result, manager.activated[name])
	}
	return result
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

func cloneContext(input llm.Context) llm.Context {
	cloned := input
	cloned.Messages = append([]llm.Message(nil), input.Messages...)
	cloned.Tools = append([]llm.ToolDefinition(nil), input.Tools...)
	return cloned
}
