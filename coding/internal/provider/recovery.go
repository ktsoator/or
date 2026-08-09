package provider

import (
	"sort"
	"strings"

	"github.com/ktsoator/or/llm"
)

func (s *Store) restoreActiveModel(saved ModelSelection) (*ModelSelection, *SelectionRepair) {
	previous := activeModelReference(saved)
	if model, ok := s.findRunnableModel(saved.Provider, saved.Model); ok {
		restored := ModelSelection{
			Provider:      strings.TrimSpace(saved.Provider),
			Model:         model.ID,
			ThinkingLevel: llm.ClampThinkingLevel(model, saved.ThinkingLevel),
		}
		if restored == saved {
			return &restored, nil
		}
		replacement := activeModelReference(restored)
		return &restored, repairedSelection(
			SelectionRepairActiveModel,
			SelectionRepairUnsupportedLevel,
			previous,
			&replacement,
		)
	}

	if fallback, ok := s.fallbackActiveModel(saved.Provider, saved.ThinkingLevel); ok {
		replacement := activeModelReference(fallback)
		return &fallback, repairedSelection(
			SelectionRepairActiveModel,
			SelectionRepairUnavailable,
			previous,
			&replacement,
		)
	}
	return nil, repairedSelection(
		SelectionRepairActiveModel,
		SelectionRepairUnavailable,
		previous,
		nil,
	)
}

func (s *Store) fallbackActiveModel(
	preferredProvider string,
	thinking llm.ModelThinkingLevel,
) (ModelSelection, bool) {
	providers := s.registry.Providers()
	sort.SliceStable(providers, func(i, j int) bool {
		leftPreferred := providers[i].ID() == strings.TrimSpace(preferredProvider)
		rightPreferred := providers[j].ID() == strings.TrimSpace(preferredProvider)
		if leftPreferred != rightPreferred {
			return leftPreferred
		}
		return providers[i].ID() < providers[j].ID()
	})
	for _, registered := range providers {
		if !s.profileConfigured(registered.ID()) {
			continue
		}
		for _, model := range registered.Models() {
			if !llm.SupportsProtocol(model.Protocol) {
				continue
			}
			return ModelSelection{
				Provider:      registered.ID(),
				Model:         model.ID,
				ThinkingLevel: llm.ClampThinkingLevel(model, thinking),
			}, true
		}
	}
	return ModelSelection{}, false
}

func (s *Store) findRunnableModel(providerID, modelID string) (llm.Model, bool) {
	registered, ok := s.registry.Get(strings.TrimSpace(providerID))
	if !ok {
		return llm.Model{}, false
	}
	modelID = strings.TrimSpace(modelID)
	for _, model := range registered.Models() {
		if model.ID == modelID && llm.SupportsProtocol(model.Protocol) {
			return model, true
		}
	}
	return llm.Model{}, false
}

func (s *Store) profileConfigured(providerID string) bool {
	profile, ok := s.profiles[providerID]
	if !ok {
		return false
	}
	profile = normalizeProfile(profile)
	connection := FindConnection(profile, profile.ActiveConnectionID)
	if connection == nil {
		return false
	}
	key := FindKey(*connection, connection.ActiveKeyID)
	return key != nil && strings.TrimSpace(key.APIKey) != ""
}

// clearRepairLocked removes a startup notice once the user has explicitly
// replaced that selection. The caller holds s.mu.
func (s *Store) clearRepairLocked(target SelectionRepairTarget) {
	remaining := s.repairs[:0]
	for _, repair := range s.repairs {
		if repair.Target != target {
			remaining = append(remaining, repair)
		}
	}
	s.repairs = remaining
}

func repairedSelection(
	target SelectionRepairTarget,
	reason SelectionRepairReason,
	previous ModelReference,
	replacement *ModelReference,
) *SelectionRepair {
	return &SelectionRepair{Target: target, Reason: reason, Previous: previous, Replacement: replacement}
}

func modelSelectionReference(providerID, modelID string) ModelReference {
	return ModelReference{Provider: strings.TrimSpace(providerID), Model: strings.TrimSpace(modelID)}
}

func activeModelReference(selection ModelSelection) ModelReference {
	reference := modelSelectionReference(selection.Provider, selection.Model)
	reference.ThinkingLevel = selection.ThinkingLevel
	return reference
}
