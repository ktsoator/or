package snapshot

import (
	"fmt"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

// FromTranscript builds the inspectable diagnostic view of one committed
// provider request without consulting live agent state or a secondary store.
func FromTranscript(
	sessionID, providerRequestID string,
	entries []transcript.Entry,
) (Snapshot, error) {
	if strings.TrimSpace(sessionID) == "" {
		return Snapshot{}, ErrNotFound
	}
	if !validRequestID(providerRequestID) {
		return Snapshot{}, ErrInvalidID
	}
	projection, err := transcript.ProjectSession(entries)
	if err != nil {
		return Snapshot{}, fmt.Errorf("snapshot: project transcript: %w", err)
	}

	var projected *transcript.ProjectedProviderRequest
	for index := range projection.ProviderRequests {
		candidate := &projection.ProviderRequests[index]
		if candidate.Header.ProviderRequestID == providerRequestID {
			projected = candidate
			break
		}
	}
	if projected == nil {
		return Snapshot{}, ErrNotFound
	}
	reconstructed, err := transcript.ReconstructProviderRequest(entries, providerRequestID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("snapshot: reconstruct provider request: %w", err)
	}

	contexts := make(map[string]transcript.ContextAttachment)
	for _, context := range projection.Contexts {
		if context.EntryIndex <= int(projected.Header.InputSeq) {
			contexts[context.Attachment.AttachmentID] = context.Attachment
		}
	}
	attachments := make([]Attachment, 0, len(projected.Header.Attachments))
	for _, placement := range projected.Header.Attachments {
		context, ok := contexts[placement.AttachmentID]
		if !ok {
			return Snapshot{}, fmt.Errorf(
				"snapshot: request attachment %q was not found",
				placement.AttachmentID,
			)
		}
		attachments = append(attachments, Attachment{
			ID: context.AttachmentID, Kind: context.Kind,
			Placement: context.Placement, Path: context.Path,
			Revision: context.Revision, MessageIndex: placement.MessageIndex,
		})
	}

	header := reconstructed.Header
	result := NewSnapshot(
		sessionID, header.RunID, header.TurnID, header.StepID,
		header.ProviderRequestID, header.Provider, header.Model,
		reconstructed.Input, attachments,
	)
	result.CapturedAt = entries[projected.EntryIndex].Timestamp
	for _, message := range projection.Messages {
		llmMessage, ok := agent.ToLLM(message.Message)
		if !ok {
			continue
		}
		assistant, ok := llmMessage.(*llm.AssistantMessage)
		if !ok || assistant.ProviderRequestID != providerRequestID {
			continue
		}
		result.Output = &Output{
			CapturedAt:   message.Timestamp,
			Message:      sanitizeMessage(assistant),
			StopReason:   string(assistant.StopReason),
			ErrorMessage: sanitizeText(assistant.ErrorMessage),
		}
		break
	}
	return result, nil
}
