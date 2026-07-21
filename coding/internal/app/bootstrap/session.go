// Package bootstrap is the composition root for the coding product. It holds
// the one place a coding.Session is assembled, so the session manager passes
// only its transport-specific dependencies and never repeats the wiring.
package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/ktsoator/or/coding"
	"github.com/ktsoator/or/coding/internal/app/config"
	"github.com/ktsoator/or/coding/policy"
	"github.com/ktsoator/or/coding/skill"
	"github.com/ktsoator/or/coding/store"
)

// Dependencies are the caller-supplied inputs needed to assemble a session.
type Dependencies struct {
	Confirm policy.Confirm
}

// NewSession builds the product's standard coding session.
func NewSession(ctx context.Context, cfg config.Config, deps Dependencies) (*coding.Session, error) {
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
		Policy:        policy.Gate{Confirm: deps.Confirm},
		Skills:        loadSkills(cfg.Cwd),
	})
}

// loadSkills discovers skills from the user root (~/.or/skills) and the
// workspace root (<cwd>/.or/skills). A project skill overrides a user skill of
// the same name. Diagnostics for malformed skills are ignored here so a bad
// skill file never blocks session start; the rest still load.
func loadSkills(cwd string) []skill.Skill {
	var userDir string
	if home, err := os.UserHomeDir(); err == nil {
		userDir = filepath.Join(home, ".or", "skills")
	}
	var projectDir string
	if strings.TrimSpace(cwd) != "" {
		projectDir = filepath.Join(cwd, ".or", "skills")
	}
	reg, _ := skill.Load(skill.LoadOptions{UserDir: userDir, ProjectDir: projectDir})
	return reg.List()
}

// detailsFile derives the tool-details side-car path from the transcript path,
// keeping the two files together under the session directory.
func detailsFile(sessionFile string) string {
	return strings.TrimSuffix(sessionFile, ".jsonl") + ".details.jsonl"
}
