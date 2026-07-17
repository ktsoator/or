// Package bootstrap is the composition root shared by the coding product
// adapters. It constructs one consistently configured coding.Session while the
// CLI and Web packages provide only their transport-specific dependencies.
package bootstrap

import (
	"context"
	"strings"

	"github.com/ktsoator/or/coding"
	"github.com/ktsoator/or/coding/internal/app/config"
	"github.com/ktsoator/or/coding/policy"
	"github.com/ktsoator/or/coding/store"
)

// Dependencies are adapter-specific inputs needed to assemble a session.
type Dependencies struct {
	Confirm policy.Confirm
}

// NewSession builds the product's standard coding session. Keeping this wiring
// in one place prevents CLI and Web behavior from drifting apart.
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
	})
}

// detailsFile derives the tool-details side-car path from the transcript path,
// keeping the two files together under the session directory.
func detailsFile(sessionFile string) string {
	return strings.TrimSuffix(sessionFile, ".jsonl") + ".details.jsonl"
}
