package providerconfig

import (
	"os"
	"testing"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/all"
)

func TestStorePreservesExistingSecret(t *testing.T) {
	store := newTestStore(t, nil)

	first, err := store.Replace("test-provider", Update{
		ActiveConnectionID: "custom",
		Connections: []ConnectionUpdate{
			{ID: OfficialConnectionID},
			{
				ID:          "custom",
				Name:        "Work gateway",
				BaseURL:     "https://gateway.example.com/v1",
				ActiveKeyID: "key-primary",
				Keys: []KeyUpdate{
					{ID: "key-primary", Name: "Primary", APIKey: "secret-primary"},
					{ID: "key-backup", Name: "Backup", APIKey: "secret-backup"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := FindKey(*FindConnection(first, "custom"), "key-primary").APIKey; got != "secret-primary" {
		t.Fatalf("initial key = %q", got)
	}

	updated, err := store.Replace("test-provider", Update{
		ActiveConnectionID: "custom",
		Connections: []ConnectionUpdate{
			{ID: OfficialConnectionID},
			{
				ID:          "custom",
				Name:        "Renamed gateway",
				BaseURL:     "https://gateway.example.com/v2",
				ActiveKeyID: "key-backup",
				Keys: []KeyUpdate{
					{ID: "key-primary", Name: "Primary renamed"},
					{ID: "key-backup", Name: "Backup"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	connection := FindConnection(updated, "custom")
	if connection == nil {
		t.Fatal("custom connection is missing")
	}
	if connection.ActiveKeyID != "key-backup" {
		t.Fatalf("active key = %q", connection.ActiveKeyID)
	}
	if got := FindKey(*connection, "key-primary").APIKey; got != "secret-primary" {
		t.Fatalf("preserved primary key = %q", got)
	}
	if got := FindKey(*connection, "key-backup").APIKey; got != "secret-backup" {
		t.Fatalf("preserved backup key = %q", got)
	}

	info, err := os.Stat(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("provider file mode = %o, want 600", got)
	}
}

func TestStoreRejectsIncompleteCustomConnectionAndKey(t *testing.T) {
	store := newTestStore(t, nil)

	_, err := store.Replace("test-provider", Update{
		ActiveConnectionID: "custom",
		Connections: []ConnectionUpdate{
			{ID: OfficialConnectionID},
			{ID: "custom", Name: "Missing URL"},
		},
	})
	if err == nil {
		t.Fatal("expected custom connection without a base URL to fail")
	}

	_, err = store.Replace("test-provider", Update{
		ActiveConnectionID: OfficialConnectionID,
		Connections: []ConnectionUpdate{
			{
				ID:          OfficialConnectionID,
				ActiveKeyID: "empty",
				Keys:        []KeyUpdate{{ID: "empty", Name: "Empty"}},
			},
		},
	})
	if err == nil {
		t.Fatal("expected a new key without a secret to fail")
	}

	_, err = store.Replace("test-provider", Update{
		ActiveConnectionID: "missing",
		Connections:        []ConnectionUpdate{{ID: OfficialConnectionID}},
	})
	if err == nil {
		t.Fatal("expected an unknown active connection to fail")
	}
}

func TestStoreRestoresCatalogAfterDelete(t *testing.T) {
	registry := registryWithProvider(t)
	store, err := NewStore(t.TempDir(), registry)
	if err != nil {
		t.Fatal(err)
	}
	store.Apply()

	_, err = store.Replace("test-provider", Update{
		ActiveConnectionID: "custom",
		Connections: []ConnectionUpdate{
			{ID: OfficialConnectionID},
			{
				ID:          "custom",
				Name:        "Custom",
				BaseURL:     "https://custom.example.com/v1",
				ActiveKeyID: "work",
				Keys:        []KeyUpdate{{ID: "work", Name: "Work", APIKey: "work-secret"}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved, options := registry.ResolveRequest(testModel(), llm.StreamOptions{})
	if resolved.BaseURL != "https://custom.example.com/v1" {
		t.Fatalf("custom base URL = %q", resolved.BaseURL)
	}
	if options.APIKey != "work-secret" {
		t.Fatalf("custom key = %q", options.APIKey)
	}

	if err := store.Delete("test-provider"); err != nil {
		t.Fatal(err)
	}
	resolved, options = registry.ResolveRequest(testModel(), llm.StreamOptions{})
	if resolved.BaseURL != "https://catalog.example.com/v1" {
		t.Fatalf("restored base URL = %q", resolved.BaseURL)
	}
	if options.APIKey != "" {
		t.Fatalf("restored key = %q", options.APIKey)
	}
}

func TestApplyClearsOverridesOutsideOwnedBaseline(t *testing.T) {
	registry := registryWithProvider(t)
	staleURL := "https://stale.example.com/v1"
	registry.SetOverride("test-provider", llm.ProviderOverride{BaseURL: &staleURL})

	store, err := NewStore(t.TempDir(), registry)
	if err != nil {
		t.Fatal(err)
	}
	store.Apply()

	resolved, _ := registry.ResolveRequest(testModel(), llm.StreamOptions{})
	if resolved.BaseURL != "https://catalog.example.com/v1" {
		t.Fatalf("base URL after Apply = %q", resolved.BaseURL)
	}
}

func TestSaveDoesNotActivateEditedConnectionOrNewKey(t *testing.T) {
	store := newTestStore(t, nil)
	_, err := store.Replace("test-provider", Update{
		ActiveConnectionID: OfficialConnectionID,
		Connections: []ConnectionUpdate{
			{ID: OfficialConnectionID},
			{ID: "custom", Name: "Custom", BaseURL: "https://custom.example.com/v1"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	profile, err := store.Save("test-provider", Update{
		ActiveConnectionID: "custom",
		Connections: []ConnectionUpdate{
			{ID: OfficialConnectionID},
			{
				ID:      "custom",
				Name:    "Renamed custom",
				BaseURL: "https://custom.example.com/v2",
				Keys: []KeyUpdate{
					{ID: "new-key", Name: "New key", APIKey: "new-secret"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if profile.ActiveConnectionID != OfficialConnectionID {
		t.Fatalf("active connection = %q", profile.ActiveConnectionID)
	}
	custom := FindConnection(profile, "custom")
	if custom == nil {
		t.Fatal("custom connection is missing")
	}
	if custom.ActiveKeyID != "" {
		t.Fatalf("new key was activated as %q", custom.ActiveKeyID)
	}
}

func TestActivateConnectionAndKey(t *testing.T) {
	registry := registryWithProvider(t)
	store, err := NewStore(t.TempDir(), registry)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Replace("test-provider", Update{
		ActiveConnectionID: OfficialConnectionID,
		Connections: []ConnectionUpdate{
			{ID: OfficialConnectionID},
			{
				ID:      "custom",
				Name:    "Custom",
				BaseURL: "https://custom.example.com/v1",
				Keys:    []KeyUpdate{{ID: "work", Name: "Work", APIKey: "work-secret"}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ActivateKey("test-provider", "custom", "work"); err != nil {
		t.Fatal(err)
	}
	profile, err := store.ActivateConnection("test-provider", "custom")
	if err != nil {
		t.Fatal(err)
	}
	if profile.ActiveConnectionID != "custom" {
		t.Fatalf("active connection = %q", profile.ActiveConnectionID)
	}
	resolved, options := registry.ResolveRequest(testModel(), llm.StreamOptions{})
	if resolved.BaseURL != "https://custom.example.com/v1" {
		t.Fatalf("active base URL = %q", resolved.BaseURL)
	}
	if options.APIKey != "work-secret" {
		t.Fatalf("active API key = %q", options.APIKey)
	}
}

func TestCustomConnectionRequiresExplicitCredentialSelection(t *testing.T) {
	store := newTestStore(t, nil)
	_, err := store.Replace("test-provider", Update{
		ActiveConnectionID: OfficialConnectionID,
		Connections: []ConnectionUpdate{
			{ID: OfficialConnectionID},
			{ID: "custom", Name: "Custom", BaseURL: "https://custom.example.com/v1"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ActivateConnection("test-provider", "custom"); err == nil {
		t.Fatal("expected custom connection without a credential selection to fail")
	}
	if _, err := store.Save("test-provider", Update{
		Connections: []ConnectionUpdate{
			{ID: OfficialConnectionID},
			{
				ID:      "custom",
				Name:    "Custom",
				BaseURL: "https://custom.example.com/v1",
				Keys:    []KeyUpdate{{ID: "work", Name: "Work", APIKey: "secret"}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ActivateKey("test-provider", "custom", "work"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ActivateConnection("test-provider", "custom"); err != nil {
		t.Fatal(err)
	}
}

func TestOfficialConnectionUsesCatalogURL(t *testing.T) {
	t.Setenv("TEST_PROVIDER_API_KEY", "must-not-be-used")
	registry := registryWithProvider(t)
	store, err := NewStore(t.TempDir(), registry)
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Replace("test-provider", Update{
		ActiveConnectionID: OfficialConnectionID,
		Connections:        []ConnectionUpdate{{ID: OfficialConnectionID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved, options := registry.ResolveRequest(testModel(), llm.StreamOptions{})
	if resolved.BaseURL != "https://catalog.example.com/v1" {
		t.Fatalf("official base URL = %q", resolved.BaseURL)
	}
	if options.APIKey != "" {
		t.Fatalf("official key = %q", options.APIKey)
	}
}

func TestActiveModelStartsEmptyAndPersistsAfterExplicitActivation(t *testing.T) {
	dir := t.TempDir()
	registry := registryWithProvider(t)
	store, err := NewStore(dir, registry)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.ActiveModel(); ok {
		t.Fatal("first launch unexpectedly selected a model")
	}
	_, err = store.Replace("test-provider", Update{
		ActiveConnectionID: OfficialConnectionID,
		Connections: []ConnectionUpdate{{
			ID:          OfficialConnectionID,
			ActiveKeyID: "work",
			Keys:        []KeyUpdate{{ID: "work", Name: "Work", APIKey: "secret"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := store.ActivateModel(ModelSelection{
		Provider:      "test-provider",
		Model:         "test-model",
		ThinkingLevel: llm.ModelThinkingOff,
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Model != "test-model" {
		t.Fatalf("active model = %q", selection.Model)
	}

	restored, err := NewStore(dir, registry)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := restored.ActiveModel(); !ok || got != selection {
		t.Fatalf("restored active model = %#v, %v", got, ok)
	}
}

func TestStoreRejectsUnsupportedFileVersion(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/providers.json", []byte(`{"version":1,"providers":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(dir, registryWithProvider(t)); err == nil {
		t.Fatal("expected unsupported settings version to fail")
	}
}

func TestStoreRejectsInvalidCurrentFile(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{
  "version": 2,
  "providers": {
    "test-provider": {
      "activeConnectionId": "custom",
      "connections": [{"id":"custom","name":"Custom"}]
    }
  }
}`)
	if err := os.WriteFile(dir+"/providers.json", data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(dir, registryWithProvider(t)); err == nil {
		t.Fatal("expected invalid current settings to fail")
	}
}

func newTestStore(t *testing.T, _ map[string]llm.ProviderOverride) *Store {
	t.Helper()
	store, err := NewStore(t.TempDir(), registryWithProvider(t))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func registryWithProvider(t *testing.T) *llm.ProviderRegistry {
	t.Helper()
	registry := llm.NewProviderRegistry()
	if err := registry.Register(llm.NewSpecProvider(llm.ProviderSpec{
		ID:      "test-provider",
		Name:    "Test Provider",
		EnvKeys: []string{"TEST_PROVIDER_API_KEY"},
		Models:  []llm.Model{testModel()},
	})); err != nil {
		t.Fatal(err)
	}
	return registry
}

func testModel() llm.Model {
	return llm.Model{
		ID:       "test-model",
		Provider: "test-provider",
		Protocol: llm.ProtocolOpenAICompletions,
		BaseURL:  "https://catalog.example.com/v1",
	}
}
