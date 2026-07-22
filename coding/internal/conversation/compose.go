package conversation

import (
	"context"
	"strings"

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
	approver permission.Approver,
) (*engine.Session, error) {
	return engine.New(ctx, engine.Options{
		Model:         cfg.Model,
		ThinkingLevel: cfg.ThinkingLevel,
		Cwd:           cfg.WorkspacePath,
		Store:         transcript.NewJSONL(cfg.TranscriptPath),
		DetailsStore:  transcript.NewJSONLDetails(detailsFile(cfg.TranscriptPath)),
		Policy:        permission.PolicyForMode(cfg.PermissionMode),
		Approver:      approver,
		Skills:        loadSkills(cfg.WorkspacePath),
	})
}

// loadSkills returns the skills visible from a workspace. Diagnostics for
// malformed skills are dropped here so one bad skill file never blocks a
// session from starting; the API surfaces them separately.
func loadSkills(cwd string) []skills.Skill {
	reg, _ := skills.LoadFor(cwd)
	return reg.List()
}

// detailsFile derives the tool-details side-car path from the transcript path,
// keeping the two files together under the session directory.
func detailsFile(sessionFile string) string {
	return strings.TrimSuffix(sessionFile, ".jsonl") + ".details.jsonl"
}
