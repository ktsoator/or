package session

import (
	"context"
	"strings"

	"github.com/ktsoator/or/coding"
	"github.com/ktsoator/or/coding/internal/config"
	"github.com/ktsoator/or/coding/policy"
	"github.com/ktsoator/or/coding/skill"
	"github.com/ktsoator/or/coding/store"
)

// This is the one place a coding.Session is assembled. Every conversation the
// product opens goes through it, so the tool set, transcript layout, permission
// gate and skill discovery are decided once rather than per call site.

// newCodingSession builds the product's standard coding session. cfg carries
// the per-conversation values the manager has already resolved — workspace,
// transcript path, model — and confirm is how this session asks its viewer to
// approve a tool call.
func newCodingSession(
	ctx context.Context,
	cfg config.Config,
	confirm policy.Confirm,
) (*coding.Session, error) {
	model, err := cfg.ResolveModel()
	if err != nil {
		return nil, err
	}

	return coding.New(ctx, coding.Options{
		Model:         model,
		ThinkingLevel: cfg.Thinking(),
		Cwd:           cfg.Cwd,
		Store:         store.NewJSONL(cfg.SessionFile),
		DetailsStore:  store.NewJSONLDetails(detailsFile(cfg.SessionFile)),
		Policy:        policy.Gate{Confirm: confirm},
		Skills:        loadSkills(cfg.Cwd),
	})
}

// loadSkills returns the skills visible from a workspace. Diagnostics for
// malformed skills are dropped here so one bad skill file never blocks a
// session from starting; the API surfaces them separately.
func loadSkills(cwd string) []skill.Skill {
	reg, _ := skill.LoadFor(cwd)
	return reg.List()
}

// detailsFile derives the tool-details side-car path from the transcript path,
// keeping the two files together under the session directory.
func detailsFile(sessionFile string) string {
	return strings.TrimSuffix(sessionFile, ".jsonl") + ".details.jsonl"
}
