package engine

import (
	"github.com/ktsoator/or/coding/internal/modelcontext"
	"github.com/ktsoator/or/coding/internal/prompt"
	"github.com/ktsoator/or/coding/internal/skills"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/transcript"
)

// buildSystemPrompt assembles only the stable coding prompt. Project
// instructions and skill listings are projected by modelContext at the request
// boundary and never become part of the Agent's canonical system prompt.
func (s *Session) buildSystemPrompt(instructions string) string {
	infos := make([]prompt.ToolInfo, len(s.tools))
	for i, t := range s.tools {
		infos[i] = prompt.ToolInfo{Name: t.Name(), Guidelines: t.Guidelines}
	}
	return prompt.BuildSystem(prompt.SystemOptions{
		Instructions:       instructions,
		WorkspaceRoot:      s.cwd,
		Tools:              infos,
		AdditionalSections: s.promptSections,
	})
}

// buildBaseContext renders the session's environment and instruction files, with
// the revision that fingerprints them. prepareContextRefresh compares against
// that revision to decide whether the model's view has gone stale.
func (s *Session) buildBaseContext() (rendered, revision string) {
	env := prompt.DetectEnvironment(s.cwd)
	files := prompt.LoadContextFiles(s.cwd)
	return prompt.RenderBaseContext(env, files), prompt.ContextRevision(env, files)
}

func (s *Session) buildSkillListing() string {
	return prompt.RenderSkillListing(
		s.skillRevision,
		skillInfos(s.skillRegistry.List()),
	)
}

func (s *Session) currentSkillRegistry() *skills.Registry {
	if s.pendingSkills != nil {
		return s.pendingSkills
	}
	return s.skillRegistry.Snapshot()
}

func (s *Session) activateSkill(name string) {
	skill, ok := s.currentSkillRegistry().Lookup(name)
	if !ok {
		return
	}
	s.modelContext.StageActivatedSkill(
		skill.Name,
		"",
		skills.FormatActivatedContext(skill),
	)
}

func (s *Session) setSkillToolAvailable(available bool) {
	next := toolsWithSkillAvailability(s.allTools, available)
	if sameToolNames(s.tools, next) {
		return
	}
	s.tools = next
	s.agent.SetTools(tools.AgentTools(next))
	s.agent.SetSystemPrompt(s.buildSystemPrompt(s.instructions))
}

// prepareSkillRefresh stages one immutable snapshot for the next request. The
// live tool registry is not replaced here; modelStreamFn publishes it only after
// the matching hidden update and canonical request prefix are durable.
func (s *Session) prepareSkillRefresh() {
	if s.skillLoader == nil {
		return
	}
	next := skills.NewRegistry(s.skillLoader())
	nextRevision := next.Revision()

	if s.pendingSkills != nil {
		switch nextRevision {
		case s.pendingSkillRevision:
			s.pendingSkills = next
			return
		case s.skillRevision:
			s.pendingSkills = nil
			s.pendingSkillRevision = ""
			s.modelContext.CancelStagedSkillsUpdate()
			s.skillRegistry.Replace(next)
			return
		}
	} else if nextRevision == s.skillRevision {
		s.skillRegistry.Replace(next)
		return
	}

	delta := skills.Diff(s.skillRegistry.Snapshot(), next)
	rendered := prompt.RenderSkillsUpdate(
		nextRevision,
		skillInfos(next.List()),
		promptSkillDelta(delta),
	)
	s.pendingSkills = next
	s.pendingSkillRevision = nextRevision
	s.modelContext.StageSkillsUpdate(nextRevision, rendered)
}

// prepareContextRefresh restages the environment and instruction files when they
// no longer match what the model has been shown — an edited AGENTS.md, a branch
// switch, or a session that crossed midnight. The refresh is projected after the
// canonical messages, so a change costs one bounded block rather than the
// session's cached request prefix.
func (s *Session) prepareContextRefresh() {
	env := prompt.DetectEnvironment(s.cwd)
	files := prompt.LoadContextFiles(s.cwd)
	nextRevision := prompt.ContextRevision(env, files)

	if s.pendingContextRevision != "" {
		switch nextRevision {
		case s.pendingContextRevision:
			return
		case s.contextRevision:
			// The files reverted before the staged block was ever sent.
			s.pendingContextRevision = ""
			s.modelContext.CancelStagedContextUpdate()
			return
		}
	} else if nextRevision == s.contextRevision {
		return
	}

	s.pendingContextRevision = nextRevision
	s.modelContext.StageContextUpdate(
		nextRevision,
		prompt.RenderContextUpdate(nextRevision, env, files),
	)
}

func (s *Session) commitContextRefresh(attachments []modelcontext.Attachment) {
	if s.pendingContextRevision == "" {
		return
	}
	for _, attachment := range attachments {
		if attachment.Kind != modelcontext.ContextUpdate ||
			attachment.Revision != s.pendingContextRevision {
			continue
		}
		s.contextRevision = s.pendingContextRevision
		s.pendingContextRevision = ""
		return
	}
}

func (s *Session) commitSkillRefresh(attachments []modelcontext.Attachment) {
	if s.pendingSkills == nil {
		return
	}
	for _, attachment := range attachments {
		if attachment.Kind != modelcontext.SkillsUpdate ||
			attachment.Revision != s.pendingSkillRevision {
			continue
		}
		s.skillRegistry.Replace(s.pendingSkills)
		s.skillRevision = s.pendingSkillRevision
		s.pendingSkills = nil
		s.pendingSkillRevision = ""
		return
	}
}

func nextContextEpoch(entries []transcript.Entry) uint64 {
	var latest uint64
	for _, entry := range entries {
		if entry.Type == transcript.ContextEntry &&
			entry.Context != nil &&
			entry.Context.Epoch > latest {
			latest = entry.Context.Epoch
		}
	}
	return latest + 1
}

func restoredActivatedSkills(entries []transcript.Entry) []modelcontext.Attachment {
	result := make([]modelcontext.Attachment, 0)
	for _, entry := range entries {
		if entry.Type != transcript.ContextEntry || entry.Context == nil ||
			entry.Context.Kind != string(modelcontext.ActivatedSkill) {
			continue
		}
		result = append(result, modelcontext.Attachment{
			ID:        entry.Context.AttachmentID,
			Epoch:     entry.Context.Epoch,
			Kind:      modelcontext.ActivatedSkill,
			Placement: modelcontext.Placement(entry.Context.Placement),
			Path:      entry.Context.Path,
			Revision:  entry.Context.Revision,
			Rendered:  entry.Context.Rendered,
		})
	}
	return result
}

// skillInfos projects loaded skills into the prompt's listing entries.
func skillInfos(loaded []skills.Skill) []prompt.SkillInfo {
	if len(loaded) == 0 {
		return nil
	}
	infos := make([]prompt.SkillInfo, len(loaded))
	for i, skill := range loaded {
		infos[i] = prompt.SkillInfo{Name: skill.Name, Description: skill.Description}
	}
	return infos
}

func promptSkillDelta(delta skills.Delta) prompt.SkillsDelta {
	return prompt.SkillsDelta{
		Added:   skillInfos(delta.Added),
		Updated: skillInfos(delta.Updated),
		Removed: append([]string(nil), delta.Removed...),
	}
}
