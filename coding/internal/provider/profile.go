package provider

import (
	"errors"
	"fmt"
	"strings"
)

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
