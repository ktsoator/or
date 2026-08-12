package engine

import (
	"context"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/skills"
	"github.com/ktsoator/or/llm"
)

// Prompt starts a run from a text message and optional images, blocking until it
// completes. Newly appended messages are persisted. It returns ErrBusy if a run
// is already in progress.
func (s *Session) Prompt(ctx context.Context, text string, images ...llm.ImageContent) error {
	return s.PromptWithFiles(ctx, text, nil, images...)
}

// PromptWithFiles starts a run with text files that remain product-owned
// context rather than becoming a new LLM SDK content type.
func (s *Session) PromptWithFiles(
	ctx context.Context,
	text string,
	files []AttachedFile,
	images ...llm.ImageContent,
) error {
	return s.run(ctx, func(ctx context.Context) error {
		message, err := s.promptMessage(text, files, images...)
		if err != nil {
			return err
		}
		return s.agent.Prompt(ctx, message)
	})
}

func (s *Session) promptMessage(
	text string,
	files []AttachedFile,
	images ...llm.ImageContent,
) (agent.AgentMessage, error) {
	registry := s.skillRegistry.Snapshot()
	if s.pendingSkills != nil {
		registry = s.pendingSkills
	}
	_, matched, err := registry.ResolveExplicitInvocation(text)
	if err != nil {
		return nil, err
	}
	if matched {
		if activated, ok := registry.ExplicitInvocationSkill(text); ok {
			s.modelContext.StageActivatedSkill(
				activated.Name,
				"",
				skills.FormatActivatedContext(activated),
			)
		}
		return userMessage(
			registry.DisplayExplicitInvocation(text),
			files,
			images,
		), nil
	}
	return userMessage(text, files, images), nil
}
