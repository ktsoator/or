package provider

import "github.com/ktsoator/or/llm"

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
