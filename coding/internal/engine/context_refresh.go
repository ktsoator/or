package engine

import (
	"sync"

	"github.com/ktsoator/or/coding/internal/contextprojection"
	"github.com/ktsoator/or/coding/internal/prompt"
	"github.com/ktsoator/or/coding/internal/skills"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

// contextManager owns the mutable product context projected at provider request
// boundaries. Its staged revisions become active only through commit, after the
// matching journal checkpoint and committed-request validation have succeeded.
type contextManager struct {
	// Manager methods acquire mu before entering projection. Projection code
	// never calls back into this owner, so the lock order stays one-way.
	mu sync.Mutex

	cwd          string
	instructions string

	skillRegistry        *skills.DynamicRegistry
	skillLoader          func() []skills.Skill
	skillRevision        string
	pendingSkills        *skills.Registry
	pendingSkillRevision string

	contextRevision        string
	pendingContextRevision string
	projection             *contextprojection.Manager
}

type contextManagerState struct {
	Projection             contextprojection.State
	SkillRevision          string
	PendingSkillRevision   string
	ContextRevision        string
	PendingContextRevision string
}

func newContextManager(
	cwd string,
	instructions string,
	skillRegistry *skills.DynamicRegistry,
	skillLoader func() []skills.Skill,
	entries []transcript.Entry,
) *contextManager {
	if skillRegistry == nil {
		skillRegistry = skills.NewDynamicRegistry(skills.NewRegistry(nil))
	}
	registry := skillRegistry.Snapshot()
	skillRevision := registry.Revision()
	env := prompt.DetectEnvironment(cwd)
	files := prompt.LoadContextFiles(cwd)
	baseRendered := prompt.RenderBaseContext(env, files)
	baseRevision := prompt.ContextRevision(env, files)
	projection := contextprojection.New(
		nextContextEpoch(entries),
		baseRevision,
		baseRendered,
		skillRevision,
		prompt.RenderSkillListing(skillRevision, skillInfos(registry.List())),
	)
	projection.RestoreActivatedSkills(restoredActivatedSkills(entries))
	return &contextManager{
		cwd:             cwd,
		instructions:    instructions,
		skillRegistry:   skillRegistry,
		skillLoader:     skillLoader,
		skillRevision:   skillRevision,
		contextRevision: baseRevision,
		projection:      projection,
	}
}

// systemPrompt assembles only the stable coding prompt. Session instructions
// and skill listings are projected at the request boundary and never become
// part of the Agent's canonical system prompt.
func (manager *contextManager) systemPrompt(toolSet []tools.Tool) string {
	infos := make([]prompt.ToolInfo, len(toolSet))
	for i, t := range toolSet {
		infos[i] = prompt.ToolInfo{Name: t.Name(), Guidelines: t.Guidelines}
	}
	return prompt.BuildSystem(prompt.SystemOptions{
		Instructions: manager.instructions,
		Tools:        infos,
	})
}

func (manager *contextManager) currentSkillRegistryLocked() *skills.Registry {
	if manager.pendingSkills != nil {
		return manager.pendingSkills
	}
	return manager.skillRegistry.Snapshot()
}

func (manager *contextManager) hasSkills() bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.currentSkillRegistryLocked().Len() > 0
}

func (manager *contextManager) preparePrompt(text string) (string, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	registry := manager.currentSkillRegistryLocked()
	_, matched, err := registry.ResolveExplicitInvocation(text)
	if err != nil {
		return "", err
	}
	if !matched {
		return text, nil
	}
	if activated, ok := registry.ExplicitInvocationSkill(text); ok {
		manager.projection.StageActivatedSkill(
			activated.Name,
			"",
			skills.FormatActivatedContext(activated),
		)
	}
	return registry.DisplayExplicitInvocation(text), nil
}

func (manager *contextManager) activateSkill(name string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	skill, ok := manager.currentSkillRegistryLocked().Lookup(name)
	if !ok {
		return
	}
	manager.projection.StageActivatedSkill(
		skill.Name,
		"",
		skills.FormatActivatedContext(skill),
	)
}

// prepareSkillRefresh stages one immutable snapshot for the next request. The
// live tool registry is not replaced here; modelStreamFn publishes it only after
// the matching hidden update and canonical request prefix are durable.
func (manager *contextManager) prepareSkillRefresh() {
	if manager.skillLoader == nil {
		return
	}
	next := skills.NewRegistry(manager.skillLoader())
	nextRevision := next.Revision()

	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.pendingSkills != nil {
		switch nextRevision {
		case manager.pendingSkillRevision:
			manager.pendingSkills = next
			return
		case manager.skillRevision:
			manager.pendingSkills = nil
			manager.pendingSkillRevision = ""
			manager.projection.CancelStagedSkillsUpdate()
			manager.skillRegistry.Replace(next)
			return
		}
	} else if nextRevision == manager.skillRevision {
		manager.skillRegistry.Replace(next)
		return
	}

	delta := skills.Diff(manager.skillRegistry.Snapshot(), next)
	rendered := prompt.RenderSkillsUpdate(
		nextRevision,
		skillInfos(next.List()),
		promptSkillDelta(delta),
	)
	manager.pendingSkills = next
	manager.pendingSkillRevision = nextRevision
	manager.projection.StageSkillsUpdate(nextRevision, rendered)
}

// prepareContextRefresh restages the environment and instruction files when they
// no longer match what the model has been shown — an edited AGENTS.md, a branch
// switch, or a session that crossed midnight. The refresh is projected after the
// canonical messages, so a change costs one bounded block rather than the
// session's cached request prefix.
func (manager *contextManager) prepareContextRefresh() {
	env := prompt.DetectEnvironment(manager.cwd)
	files := prompt.LoadContextFiles(manager.cwd)
	nextRevision := prompt.ContextRevision(env, files)

	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.pendingContextRevision != "" {
		switch nextRevision {
		case manager.pendingContextRevision:
			return
		case manager.contextRevision:
			// The files reverted before the staged block was ever sent.
			manager.pendingContextRevision = ""
			manager.projection.CancelStagedContextUpdate()
			return
		}
	} else if nextRevision == manager.contextRevision {
		return
	}

	manager.pendingContextRevision = nextRevision
	manager.projection.StageContextUpdate(
		nextRevision,
		prompt.RenderContextUpdate(nextRevision, env, files),
	)
}

func (manager *contextManager) prepareStep(input llm.Context) contextprojection.PreparedStep {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.projection.PrepareStep(input)
}

func (manager *contextManager) commit(prepared contextprojection.PreparedStep) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.projection.Commit(prepared)
	for _, attachment := range prepared.Pending {
		switch {
		case attachment.Kind == contextprojection.ContextUpdate &&
			attachment.Revision == manager.pendingContextRevision:
			manager.contextRevision = manager.pendingContextRevision
			manager.pendingContextRevision = ""
		case attachment.Kind == contextprojection.SkillsUpdate &&
			manager.pendingSkills != nil &&
			attachment.Revision == manager.pendingSkillRevision:
			manager.skillRegistry.Replace(manager.pendingSkills)
			manager.skillRevision = manager.pendingSkillRevision
			manager.pendingSkills = nil
			manager.pendingSkillRevision = ""
		}
	}
}

func (manager *contextManager) stageTaskStatus(rendered string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.projection.StageTaskStatus(rendered)
}

func (manager *contextManager) projectedAttachments() []contextprojection.Attachment {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.projection.ProjectedAttachments()
}

func (manager *contextManager) state() contextManagerState {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return contextManagerState{
		Projection:             manager.projection.State(),
		SkillRevision:          manager.skillRevision,
		PendingSkillRevision:   manager.pendingSkillRevision,
		ContextRevision:        manager.contextRevision,
		PendingContextRevision: manager.pendingContextRevision,
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

func restoredActivatedSkills(entries []transcript.Entry) []contextprojection.Attachment {
	result := make([]contextprojection.Attachment, 0)
	for _, entry := range entries {
		if entry.Type != transcript.ContextEntry || entry.Context == nil ||
			entry.Context.Kind != string(contextprojection.ActivatedSkill) {
			continue
		}
		result = append(result, contextprojection.Attachment{
			ID:        entry.Context.AttachmentID,
			Epoch:     entry.Context.Epoch,
			Kind:      contextprojection.ActivatedSkill,
			Placement: contextprojection.Placement(entry.Context.Placement),
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
