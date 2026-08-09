package provider

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/ktsoator/or/llm"
)

var ErrUtilityModelUnavailable = errors.New("no utility model is configured")

// UtilityModel returns the explicitly configured utility-model route.
func (s *Store) UtilityModel() (UtilityModelSelection, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.utilityModel == nil {
		return UtilityModelSelection{}, false
	}
	return *s.utilityModel, true
}

// SetUtilityModel validates and atomically persists a utility-model selection.
func (s *Store) SetUtilityModel(selection UtilityModelSelection) (UtilityModelSelection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	validated, err := s.validateUtilitySelection(selection)
	if err != nil {
		return UtilityModelSelection{}, err
	}
	previous := s.utilityModel
	s.utilityModel = &validated
	if err := s.saveLocked(); err != nil {
		s.utilityModel = previous
		return UtilityModelSelection{}, err
	}
	s.clearRepairLocked(SelectionRepairUtilityModel)
	return validated, nil
}

// ResolveUtilityModel resolves the pinned route into request-scoped settings.
// It never mutates the provider-wide override.
func (s *Store) ResolveUtilityModel() (ResolvedModelRoute, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.utilityModel == nil {
		return ResolvedModelRoute{}, ErrUtilityModelUnavailable
	}
	return s.resolveUtilityRoute(*s.utilityModel)
}

func (s *Store) validateUtilitySelection(selection UtilityModelSelection) (UtilityModelSelection, error) {
	selection.Provider = strings.TrimSpace(selection.Provider)
	selection.Model = strings.TrimSpace(selection.Model)
	selection.ConnectionID = strings.TrimSpace(selection.ConnectionID)
	selection.KeyID = strings.TrimSpace(selection.KeyID)
	if selection.Provider == "" || selection.Model == "" || selection.ConnectionID == "" || selection.KeyID == "" {
		return UtilityModelSelection{}, errors.New("provider, model, connection and key are required")
	}
	if _, err := s.resolveUtilityRoute(selection); err != nil {
		return UtilityModelSelection{}, err
	}
	return selection, nil
}

func (s *Store) resolveUtilityRoute(selection UtilityModelSelection) (ResolvedModelRoute, error) {
	registered, ok := s.registry.Get(selection.Provider)
	if !ok {
		return ResolvedModelRoute{}, fmt.Errorf("unknown provider %q", selection.Provider)
	}
	model, ok := findRunnableUtilityModel(registered.Models(), selection.Model)
	if !ok {
		return ResolvedModelRoute{}, fmt.Errorf("model %q cannot be used as a utility model", selection.Model)
	}
	profile, ok := s.profiles[selection.Provider]
	if !ok {
		return ResolvedModelRoute{}, fmt.Errorf("provider %q is not configured", selection.Provider)
	}
	connection := FindConnection(normalizeProfile(profile), selection.ConnectionID)
	if connection == nil {
		return ResolvedModelRoute{}, fmt.Errorf("connection %q was not found", selection.ConnectionID)
	}
	key := FindKey(*connection, selection.KeyID)
	if key == nil || strings.TrimSpace(key.APIKey) == "" {
		return ResolvedModelRoute{}, fmt.Errorf("key %q was not found", selection.KeyID)
	}
	baseURL := model.BaseURL
	if connection.ID != OfficialConnectionID {
		baseURL = connection.BaseURL
	}
	return ResolvedModelRoute{
		Route: ModelRoute{
			Provider:     model.Provider,
			Model:        model.ID,
			ConnectionID: connection.ID,
			KeyID:        key.ID,
		},
		Model: model,
		Options: llm.StreamOptions{
			APIKey:    key.APIKey,
			BaseURL:   baseURL,
			Reasoning: llm.ModelThinkingOff,
		},
	}, nil
}

// fallbackUtilityRoute selects a configured utility route when the persisted
// selection is no longer runnable.
func (s *Store) fallbackUtilityRoute() (ResolvedModelRoute, error) {
	type candidate struct {
		selection UtilityModelSelection
		model     llm.Model
		preferred bool
	}
	preferredProvider := ""
	if s.activeModel != nil {
		preferredProvider = s.activeModel.Provider
	}
	var candidates []candidate
	for providerID, rawProfile := range s.profiles {
		registered, ok := s.registry.Get(providerID)
		if !ok {
			continue
		}
		profile := normalizeProfile(rawProfile)
		connection := FindConnection(profile, profile.ActiveConnectionID)
		if connection == nil || connection.ActiveKeyID == "" || FindKey(*connection, connection.ActiveKeyID) == nil {
			continue
		}
		for _, model := range registered.Models() {
			if !IsUtilityModelEligible(model) {
				continue
			}
			candidates = append(candidates, candidate{
				selection: UtilityModelSelection{
					Provider:     providerID,
					Model:        model.ID,
					ConnectionID: connection.ID,
					KeyID:        connection.ActiveKeyID,
				},
				model:     model,
				preferred: providerID == preferredProvider,
			})
		}
	}
	if len(candidates) == 0 {
		return ResolvedModelRoute{}, ErrUtilityModelUnavailable
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if left.preferred != right.preferred {
			return left.preferred
		}
		if left.model.Cost.Output != right.model.Cost.Output {
			return left.model.Cost.Output < right.model.Cost.Output
		}
		if left.model.Cost.Input != right.model.Cost.Input {
			return left.model.Cost.Input < right.model.Cost.Input
		}
		if left.selection.Provider != right.selection.Provider {
			return left.selection.Provider < right.selection.Provider
		}
		return left.selection.Model < right.selection.Model
	})
	return s.resolveUtilityRoute(candidates[0].selection)
}

func findRunnableUtilityModel(models []llm.Model, id string) (llm.Model, bool) {
	for _, model := range models {
		if model.ID == id && IsUtilityModelEligible(model) {
			return model, true
		}
	}
	return llm.Model{}, false
}

// IsUtilityModelEligible reports whether a catalog model can handle the
// product's small text requests with thinking disabled.
func IsUtilityModelEligible(model llm.Model) bool {
	return slices.Contains(llm.SupportedThinkingLevels(model), llm.ModelThinkingOff) &&
		slices.Contains(model.Input, llm.ModelInputText) &&
		llm.SupportsProtocol(model.Protocol)
}

// reconcileUtilityLocked clears a selection whose route was deleted. The
// caller holds s.mu and persists the profile change atomically.
func (s *Store) reconcileUtilityLocked() {
	if s.utilityModel == nil {
		return
	}
	if _, err := s.resolveUtilityRoute(*s.utilityModel); err != nil {
		s.utilityModel = nil
	}
}

func utilitySelectionFromRoute(route ModelRoute) UtilityModelSelection {
	return UtilityModelSelection(route)
}
