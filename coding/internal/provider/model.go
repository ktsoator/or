package provider

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ktsoator/or/llm"
)

// ActiveModel returns the explicitly activated model for new conversations.
// The boolean is false on first launch and after the active provider is reset.
func (s *Store) ActiveModel() (ModelSelection, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeModel == nil {
		return ModelSelection{}, false
	}
	return *s.activeModel, true
}

// ActivateModel validates and persists the application-wide model selection.
// A provider must have an effective credential before its model can be used.
func (s *Store) ActivateModel(selection ModelSelection) (ModelSelection, error) {
	validated, err := validateModelSelection(s.registry, selection)
	if err != nil {
		return ModelSelection{}, err
	}
	status, ok := s.registry.AuthStatus(validated.Provider, nil)
	if !ok || !status.Configured {
		return ModelSelection{}, fmt.Errorf("provider %q is not configured", validated.Provider)
	}

	s.mu.Lock()
	previous := s.activeModel
	s.activeModel = &validated
	if err := s.saveLocked(); err != nil {
		s.activeModel = previous
		s.mu.Unlock()
		return ModelSelection{}, err
	}
	s.mu.Unlock()
	return validated, nil
}

func validateModelSelection(
	registry *llm.ProviderRegistry,
	selection ModelSelection,
) (ModelSelection, error) {
	selection.Provider = strings.TrimSpace(selection.Provider)
	selection.Model = strings.TrimSpace(selection.Model)
	if selection.Provider == "" || selection.Model == "" {
		return ModelSelection{}, errors.New("provider and model are required")
	}
	provider, ok := registry.Get(selection.Provider)
	if !ok {
		return ModelSelection{}, fmt.Errorf("unknown provider %q", selection.Provider)
	}
	var model llm.Model
	for _, candidate := range provider.Models() {
		if candidate.ID == selection.Model {
			model = candidate
			break
		}
	}
	if model.ID == "" || !llm.SupportsProtocol(model.Protocol) {
		return ModelSelection{}, fmt.Errorf("model %q is not available for provider %q", selection.Model, selection.Provider)
	}
	for _, level := range llm.SupportedThinkingLevels(model) {
		if level == selection.ThinkingLevel {
			return selection, nil
		}
	}
	return ModelSelection{}, fmt.Errorf("thinking level %q is not supported by model %q", selection.ThinkingLevel, selection.Model)
}
