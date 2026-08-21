package engine

import (
	"context"

	"github.com/ktsoator/or/coding/internal/transcript"
)

const planModeProjectionKey = "plan-mode"

// PlanModeSnapshot is the latest committed planning mode for a session.
type PlanModeSnapshot struct {
	Active bool `json:"active"`
}

type planModeProjectionUnit struct {
	active bool
}

func newPlanModeProjectionUnit() *planModeProjectionUnit { return &planModeProjectionUnit{} }

func (*planModeProjectionUnit) ProjectionKey() string { return planModeProjectionKey }

func (p *planModeProjectionUnit) ApplyProjection(event transcript.ProjectionEvent) {
	if event.Entry.Type == transcript.PlanModeEntry && event.Entry.PlanMode != nil {
		p.active = event.Entry.PlanMode.Active
	}
}

func (p *planModeProjectionUnit) SnapshotProjection() (any, error) {
	return PlanModeSnapshot{Active: p.active}, nil
}

// PlanMode returns the latest committed plan-mode state.
func (s *Session) PlanMode() PlanModeSnapshot {
	snapshot, err := s.journal.planModeSnapshot()
	if err != nil {
		return PlanModeSnapshot{}
	}
	return snapshot
}

// PlanModeActive implements tools.PlanModeState.
func (s *Session) PlanModeActive() bool { return s.PlanMode().Active }

// SetPlanMode changes plan mode while the session is idle.
func (s *Session) SetPlanMode(ctx context.Context, active bool) error {
	if !s.runMu.TryLock() {
		return ErrBusy
	}
	defer s.runMu.Unlock()
	return s.setPlanMode(ctx, active)
}

// ExitPlanMode implements tools.PlanModeState for the in-run review tool.
func (s *Session) ExitPlanMode(ctx context.Context) error {
	return s.setPlanMode(ctx, false)
}

func (s *Session) setPlanMode(ctx context.Context, active bool) error {
	if s.PlanModeActive() == active {
		return nil
	}
	if err := s.journal.appendPlanMode(ctx, active); err != nil {
		return err
	}
	if s.agent != nil && s.toolRuntime != nil {
		s.agent.SetSystemPrompt(s.toolRuntime.stableSystemPrompt())
	}
	s.dispatchEvent(Event{Type: PlanModeChanged, PlanMode: active})
	return nil
}
