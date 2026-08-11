package conversation

import (
	"context"

	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/skills"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

type engineSessionConfig struct {
	WorkspacePath  string
	TranscriptPath string
	Model          llm.Model
	ThinkingLevel  llm.ModelThinkingLevel
	PermissionMode permission.Mode
	MCPConfigPath  string
}

// This is the one place an engine.Session is assembled. Every conversation the
// product opens goes through it, so the tool set, transcript layout, permission
// gate and skill discovery are decided once rather than per call site.

// newEngineSession builds the product's standard agent session. cfg carries
// the per-conversation values the manager has already resolved — workspace,
// transcript path, model — and approval is how this session asks its viewer to
// approve a tool call.
func newEngineSession(
	ctx context.Context,
	cfg engineSessionConfig,
	transport Transport,
) (*engine.Session, error) {
	return engine.New(ctx, engine.Options{
		Model:          cfg.Model,
		ThinkingLevel:  cfg.ThinkingLevel,
		Cwd:            cfg.WorkspacePath,
		Store:          transcript.NewJSONL(cfg.TranscriptPath),
		PermissionMode: cfg.PermissionMode,
		Approver:       transport,
		Browser:        transport,
		Asker:          transport,
		SkillLoader: func() []skills.Skill {
			return loadSkills(cfg.WorkspacePath)
		},
		MCPConfigPath: cfg.MCPConfigPath,
	})
}

// loadSkills returns the skills visible from a workspace. Diagnostics for
// malformed skills are dropped here so one bad skill file never blocks a
// session from starting; the API surfaces them separately.
func loadSkills(cwd string) []skills.Skill {
	reg, _ := skills.LoadFor(cwd)
	return reg.List()
}
