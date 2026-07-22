package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ktsoator/or/llm"
)

// Store persists provider profiles and owns their application to a provider
// registry. The coding product deliberately disables environment credential
// lookup, so every usable key comes from this store.
type Store struct {
	path     string
	registry *llm.ProviderRegistry

	mu          sync.Mutex
	profiles    map[string]Profile
	activeModel *ModelSelection
}

func NewStore(
	dataDir string,
	registry *llm.ProviderRegistry,
) (*Store, error) {
	if registry == nil {
		return nil, errors.New("provider registry is nil")
	}
	store := &Store{
		path:     filepath.Join(dataDir, "providers.json"),
		registry: registry,
		profiles: map[string]Profile{},
	}
	data, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, err
	}

	var file profileFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse %s: %w", store.path, err)
	}
	if file.Version != fileVersion {
		return nil, fmt.Errorf("unsupported provider settings version %d", file.Version)
	}
	if file.Providers != nil {
		store.profiles = file.Providers
	}
	if file.ActiveModel != nil {
		selection, err := validateModelSelection(registry, *file.ActiveModel)
		if err != nil {
			return nil, fmt.Errorf("invalid active model: %w", err)
		}
		store.activeModel = &selection
	}
	for id, profile := range store.profiles {
		if strings.TrimSpace(id) == "" {
			return nil, errors.New("provider settings contain an empty provider id")
		}
		validated, err := validateStoredProfile(profile)
		if err != nil {
			return nil, fmt.Errorf("invalid settings for provider %q: %w", id, err)
		}
		store.profiles[id] = validated
	}
	return store, nil
}

func (s *Store) Snapshot() map[string]Profile {
	s.mu.Lock()
	profiles := cloneProfiles(s.profiles)
	s.mu.Unlock()
	for _, provider := range s.registry.Providers() {
		if _, found := profiles[provider.ID()]; !found {
			profiles[provider.ID()] = normalizeProfile(Profile{})
		}
	}
	return profiles
}

func (s *Store) saveLocked() error {
	data, err := json.MarshalIndent(profileFile{
		Version:     fileVersion,
		ActiveModel: s.activeModel,
		Providers:   s.profiles,
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
