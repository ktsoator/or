// Package providerconfig manages the coding product's persisted provider
// connection profiles. The llm package remains the only provider runtime;
// this package selects one profile and projects it into llm.ProviderOverride.
package providerconfig

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ktsoator/or/llm"
)

const (
	fileVersion          = 2
	OfficialConnectionID = "official"
)

// Key is one locally stored credential. APIKey is write-only at the HTTP
// boundary and must never be returned to the browser.
type Key struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	APIKey string `json:"apiKey"`
}

// Connection groups one endpoint with multiple named credentials. The
// official connection has no stored BaseURL; requests use the model catalog.
type Connection struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	BaseURL     string `json:"baseURL,omitempty"`
	ActiveKeyID string `json:"activeKeyId,omitempty"`
	Keys        []Key  `json:"keys,omitempty"`
}

// Profile is the persisted configuration for one llm provider.
type Profile struct {
	ActiveConnectionID string       `json:"activeConnectionId"`
	Connections        []Connection `json:"connections"`
}

type profileFile struct {
	Version     int                `json:"version"`
	ActiveModel *ModelSelection    `json:"activeModel,omitempty"`
	Providers   map[string]Profile `json:"providers"`
}

// ModelSelection is the application-wide model used for new conversations.
// It is deliberately absent on first launch; choosing a provider connection
// alone must never silently select a model.
type ModelSelection struct {
	Provider      string                 `json:"provider"`
	Model         string                 `json:"model"`
	ThinkingLevel llm.ModelThinkingLevel `json:"thinkingLevel"`
}

// Update describes an application-level profile change. Blank APIKey values
// preserve existing secrets with the same key ID.
type Update struct {
	ActiveConnectionID string
	Connections        []ConnectionUpdate
}

type ConnectionUpdate struct {
	ID          string
	Name        string
	BaseURL     string
	ActiveKeyID string
	Keys        []KeyUpdate
}

type KeyUpdate struct {
	ID     string
	Name   string
	APIKey string
}

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

// Apply projects each saved profile into the shared llm provider registry.
// Providers without a saved profile retain their catalog URL but remain
// unconfigured; coding never falls back to process environment credentials.
func (s *Store) Apply() {
	s.mu.Lock()
	profiles := cloneProfiles(s.profiles)
	s.mu.Unlock()

	providerIDs := make(map[string]struct{}, len(profiles))
	for _, provider := range s.registry.Providers() {
		providerIDs[provider.ID()] = struct{}{}
	}
	for id := range profiles {
		providerIDs[id] = struct{}{}
	}
	for id := range providerIDs {
		profile, found := profiles[id]
		s.apply(id, profile, found)
	}
}

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

// Replace validates and atomically replaces one provider profile.
func (s *Store) Replace(providerID string, update Update) (Profile, error) {
	providerID = strings.TrimSpace(providerID)
	if _, ok := s.registry.Get(providerID); !ok {
		return Profile{}, fmt.Errorf("unknown provider %q", providerID)
	}
	s.mu.Lock()
	previous, found := s.profiles[providerID]
	existing := normalizeProfile(previous)
	next, err := mergeProfile(existing, update)
	if err != nil {
		s.mu.Unlock()
		return Profile{}, err
	}
	s.profiles[providerID] = next
	if err := s.saveLocked(); err != nil {
		if found {
			s.profiles[providerID] = previous
		} else {
			delete(s.profiles, providerID)
		}
		s.mu.Unlock()
		return Profile{}, err
	}
	s.mu.Unlock()

	s.apply(providerID, next, true)
	return cloneProfile(next), nil
}

// Save updates connection names, endpoints, and credentials without changing
// which connection or credential is currently active. If an active item was
// removed, the profile falls back to the official connection or inherited
// credential as appropriate.
func (s *Store) Save(providerID string, update Update) (Profile, error) {
	providerID = strings.TrimSpace(providerID)
	if _, ok := s.registry.Get(providerID); !ok {
		return Profile{}, fmt.Errorf("unknown provider %q", providerID)
	}

	s.mu.Lock()
	previous, found := s.profiles[providerID]
	existing := normalizeProfile(previous)

	connectionIDs := make(map[string]struct{}, len(update.Connections))
	for index := range update.Connections {
		requested := &update.Connections[index]
		requested.ID = strings.TrimSpace(requested.ID)
		connectionIDs[requested.ID] = struct{}{}
		requested.ActiveKeyID = ""
		if current := FindConnection(existing, requested.ID); current != nil {
			for _, key := range requested.Keys {
				if strings.TrimSpace(key.ID) == current.ActiveKeyID {
					requested.ActiveKeyID = current.ActiveKeyID
					break
				}
			}
		}
	}
	update.ActiveConnectionID = existing.ActiveConnectionID
	if _, kept := connectionIDs[update.ActiveConnectionID]; !kept {
		update.ActiveConnectionID = OfficialConnectionID
	}

	next, err := mergeProfile(existing, update)
	if err != nil {
		s.mu.Unlock()
		return Profile{}, err
	}
	if active := FindConnection(next, next.ActiveConnectionID); active != nil &&
		active.ID != OfficialConnectionID && active.ActiveKeyID == "" {
		next.ActiveConnectionID = OfficialConnectionID
	}
	s.profiles[providerID] = next
	if err := s.saveLocked(); err != nil {
		restoreProfile(s.profiles, providerID, previous, found)
		s.mu.Unlock()
		return Profile{}, err
	}
	s.mu.Unlock()

	s.apply(providerID, next, true)
	return cloneProfile(next), nil
}

// ActivateConnection selects the endpoint used for subsequent requests.
func (s *Store) ActivateConnection(providerID, connectionID string) (Profile, error) {
	providerID = strings.TrimSpace(providerID)
	connectionID = strings.TrimSpace(connectionID)
	if _, ok := s.registry.Get(providerID); !ok {
		return Profile{}, fmt.Errorf("unknown provider %q", providerID)
	}

	s.mu.Lock()
	previous, found := s.profiles[providerID]
	next := normalizeProfile(cloneProfile(previous))
	if FindConnection(next, connectionID) == nil {
		s.mu.Unlock()
		return Profile{}, fmt.Errorf("connection %q was not found", connectionID)
	}
	connection := FindConnection(next, connectionID)
	if connection.ActiveKeyID == "" {
		s.mu.Unlock()
		return Profile{}, errors.New("select a credential before activating this connection")
	}
	next.ActiveConnectionID = connectionID
	s.profiles[providerID] = next
	if err := s.saveLocked(); err != nil {
		restoreProfile(s.profiles, providerID, previous, found)
		s.mu.Unlock()
		return Profile{}, err
	}
	s.mu.Unlock()

	s.apply(providerID, next, true)
	return cloneProfile(next), nil
}

// ActivateKey selects one saved credential for a connection. An empty key ID
// leaves the connection without a credential selection.
func (s *Store) ActivateKey(providerID, connectionID, keyID string) (Profile, error) {
	providerID = strings.TrimSpace(providerID)
	connectionID = strings.TrimSpace(connectionID)
	keyID = strings.TrimSpace(keyID)
	if _, ok := s.registry.Get(providerID); !ok {
		return Profile{}, fmt.Errorf("unknown provider %q", providerID)
	}

	s.mu.Lock()
	previous, found := s.profiles[providerID]
	next := normalizeProfile(cloneProfile(previous))
	connection := FindConnection(next, connectionID)
	if connection == nil {
		s.mu.Unlock()
		return Profile{}, fmt.Errorf("connection %q was not found", connectionID)
	}
	if keyID != "" && FindKey(*connection, keyID) == nil {
		s.mu.Unlock()
		return Profile{}, fmt.Errorf("key %q was not found", keyID)
	}
	connection.ActiveKeyID = keyID
	s.profiles[providerID] = next
	if err := s.saveLocked(); err != nil {
		restoreProfile(s.profiles, providerID, previous, found)
		s.mu.Unlock()
		return Profile{}, err
	}
	s.mu.Unlock()

	s.apply(providerID, next, true)
	return cloneProfile(next), nil
}

// Delete removes one saved profile. If it supplied the application-wide model,
// that model selection is cleared as part of the same persisted update.
func (s *Store) Delete(providerID string) error {
	s.mu.Lock()
	previous, found := s.profiles[providerID]
	previousModel := s.activeModel
	delete(s.profiles, providerID)
	if s.activeModel != nil && s.activeModel.Provider == providerID {
		s.activeModel = nil
	}
	if err := s.saveLocked(); err != nil {
		if found {
			s.profiles[providerID] = previous
		}
		s.activeModel = previousModel
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()

	s.apply(providerID, Profile{}, false)
	return nil
}

func restoreProfile(profiles map[string]Profile, providerID string, previous Profile, found bool) {
	if found {
		profiles[providerID] = previous
	} else {
		delete(profiles, providerID)
	}
}

func (s *Store) apply(providerID string, profile Profile, hasProfile bool) {
	override := llm.ProviderOverride{DisableEnv: true}
	if hasProfile {
		profile = normalizeProfile(profile)
		connection := FindConnection(profile, profile.ActiveConnectionID)
		if connection != nil {
			if connection.ID != OfficialConnectionID {
				baseURL := connection.BaseURL
				override.BaseURL = &baseURL
			}
			if key := FindKey(*connection, connection.ActiveKeyID); key != nil {
				apiKey := key.APIKey
				override.APIKey = &apiKey
			}
		}
	}
	s.registry.SetOverride(providerID, override)
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

func normalizeProfile(profile Profile) Profile {
	officialIndex := -1
	for index := range profile.Connections {
		connection := &profile.Connections[index]
		connection.ID = strings.TrimSpace(connection.ID)
		connection.Name = strings.TrimSpace(connection.Name)
		connection.BaseURL = strings.TrimSpace(connection.BaseURL)
		if connection.ID == OfficialConnectionID {
			officialIndex = index
			connection.Name = "Official"
			connection.BaseURL = ""
		}
	}
	if officialIndex < 0 {
		profile.Connections = append([]Connection{{
			ID:   OfficialConnectionID,
			Name: "Official",
		}}, profile.Connections...)
	}
	if profile.ActiveConnectionID == "" || FindConnection(profile, profile.ActiveConnectionID) == nil {
		profile.ActiveConnectionID = OfficialConnectionID
	}
	return profile
}

func mergeProfile(existing Profile, update Update) (Profile, error) {
	existing = normalizeProfile(existing)
	existingKeys := map[string]Key{}
	for _, connection := range existing.Connections {
		for _, key := range connection.Keys {
			existingKeys[key.ID] = key
		}
	}

	next := Profile{ActiveConnectionID: strings.TrimSpace(update.ActiveConnectionID)}
	seenConnections := map[string]struct{}{}
	seenKeys := map[string]struct{}{}
	for _, requested := range update.Connections {
		connection := Connection{
			ID:          strings.TrimSpace(requested.ID),
			Name:        strings.TrimSpace(requested.Name),
			BaseURL:     strings.TrimSpace(requested.BaseURL),
			ActiveKeyID: strings.TrimSpace(requested.ActiveKeyID),
		}
		if connection.ID == "" {
			connection.ID = newItemID("conn")
		}
		if _, duplicate := seenConnections[connection.ID]; duplicate {
			return Profile{}, fmt.Errorf("duplicate connection id %q", connection.ID)
		}
		seenConnections[connection.ID] = struct{}{}
		if connection.ID == OfficialConnectionID {
			connection.Name = "Official"
			connection.BaseURL = ""
		} else {
			if connection.Name == "" {
				return Profile{}, errors.New("custom connection name is required")
			}
			if connection.BaseURL == "" {
				return Profile{}, fmt.Errorf("base URL is required for %q", connection.Name)
			}
		}

		for _, requestedKey := range requested.Keys {
			key := Key{
				ID:     strings.TrimSpace(requestedKey.ID),
				Name:   strings.TrimSpace(requestedKey.Name),
				APIKey: strings.TrimSpace(requestedKey.APIKey),
			}
			if key.ID == "" {
				key.ID = newItemID("key")
			}
			if _, duplicate := seenKeys[key.ID]; duplicate {
				return Profile{}, fmt.Errorf("duplicate key id %q", key.ID)
			}
			seenKeys[key.ID] = struct{}{}
			if key.Name == "" {
				return Profile{}, errors.New("key name is required")
			}
			if key.APIKey == "" {
				key.APIKey = existingKeys[key.ID].APIKey
			}
			if key.APIKey == "" {
				return Profile{}, fmt.Errorf("API key is required for %q", key.Name)
			}
			connection.Keys = append(connection.Keys, key)
		}
		if connection.ActiveKeyID != "" && FindKey(connection, connection.ActiveKeyID) == nil {
			return Profile{}, fmt.Errorf("active key %q was not found", connection.ActiveKeyID)
		}
		next.Connections = append(next.Connections, connection)
	}
	requestedActiveConnectionID := next.ActiveConnectionID
	next = normalizeProfile(next)
	if requestedActiveConnectionID != "" && FindConnection(next, requestedActiveConnectionID) == nil {
		return Profile{}, fmt.Errorf("active connection %q was not found", requestedActiveConnectionID)
	}
	return next, nil
}

func validateStoredProfile(profile Profile) (Profile, error) {
	update := Update{ActiveConnectionID: profile.ActiveConnectionID}
	for _, connection := range profile.Connections {
		if strings.TrimSpace(connection.ID) == "" {
			return Profile{}, errors.New("connection id is required")
		}
		requested := ConnectionUpdate{
			ID:          connection.ID,
			Name:        connection.Name,
			BaseURL:     connection.BaseURL,
			ActiveKeyID: connection.ActiveKeyID,
		}
		for _, key := range connection.Keys {
			if strings.TrimSpace(key.ID) == "" {
				return Profile{}, errors.New("key id is required")
			}
			requested.Keys = append(requested.Keys, KeyUpdate(key))
		}
		update.Connections = append(update.Connections, requested)
	}
	return mergeProfile(Profile{}, update)
}

func FindConnection(profile Profile, id string) *Connection {
	for index := range profile.Connections {
		if profile.Connections[index].ID == id {
			return &profile.Connections[index]
		}
	}
	return nil
}

func FindKey(connection Connection, id string) *Key {
	if id == "" {
		return nil
	}
	for index := range connection.Keys {
		if connection.Keys[index].ID == id {
			return &connection.Keys[index]
		}
	}
	return nil
}

func cloneProfiles(profiles map[string]Profile) map[string]Profile {
	cloned := make(map[string]Profile, len(profiles))
	for id, profile := range profiles {
		cloned[id] = cloneProfile(normalizeProfile(profile))
	}
	return cloned
}

func cloneProfile(profile Profile) Profile {
	clone := profile
	clone.Connections = make([]Connection, len(profile.Connections))
	for index, connection := range profile.Connections {
		clone.Connections[index] = connection
		clone.Connections[index].Keys = append([]Key(nil), connection.Keys...)
	}
	return clone
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

func newItemID(prefix string) string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return prefix + "_" + hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("%s_%d_%d", prefix, os.Getpid(), time.Now().UnixNano())
}
