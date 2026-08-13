package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/contextprojection"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

// modelStreamFn is the product's model-request boundary. It projects hidden
// product context into a detached provider input, checkpoints the canonical
// messages and any newly emitted context attachments, then reaches the
// provider. A persistence failure prevents both context commit and provider I/O.
func (s *Session) modelStreamFn(delegate agent.StreamFn) agent.StreamFn {
	if delegate == nil {
		delegate = llm.Stream
	}
	return func(
		ctx context.Context,
		model llm.Model,
		input llm.Context,
		options llm.StreamOptions,
	) (<-chan llm.Event, error) {
		prepared := s.contextProjection.PrepareStep(input)
		if err := s.persistModelInput(ctx, input.Messages, prepared.Pending); err != nil {
			checkpointErr := fmt.Errorf("coding: persist model request checkpoint: %w", err)
			s.recordRunPersistenceError(checkpointErr)
			return nil, checkpointErr
		}
		s.contextProjection.Commit(prepared)
		s.commitSkillRefresh(prepared.Pending)
		s.commitContextRefresh(prepared.Pending)
		return delegate(ctx, model, prepared.Input, options)
	}
}

// persistModelInput appends the canonical request prefix and any new hidden
// attachments before the provider is called. input is authoritative here:
// RunLoop may emit MessageEnd before Agent has reduced that event into its live
// snapshot, while input already contains the complete canonical prefix.
func (s *Session) persistModelInput(
	ctx context.Context,
	input []llm.Message,
	attachments []contextprojection.Attachment,
) error {
	messages := make([]agent.AgentMessage, len(input))
	for index, message := range input {
		messages[index] = agent.FromLLM(message)
	}
	contextEntries := make([]transcript.Entry, len(attachments))
	for index, attachment := range attachments {
		contextEntries[index] = transcript.NewContext(transcript.ContextAttachment{
			AttachmentID: attachment.ID,
			Epoch:        attachment.Epoch,
			Kind:         string(attachment.Kind),
			Placement:    string(attachment.Placement),
			Path:         attachment.Path,
			Revision:     attachment.Revision,
			Rendered:     attachment.Rendered,
		})
	}
	return s.journal.persistMessages(
		ctx,
		messages,
		contextEntries,
		"",
		0,
		time.Time{},
		time.Time{},
	)
}
