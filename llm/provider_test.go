package llm

import (
	"reflect"
	"testing"
)

func testProvider() *Provider {
	return NewSpecProvider(ProviderSpec{
		ID:      "acme",
		Name:    "Acme",
		EnvKeys: []string{"ACME_API_KEY", "ACME_TOKEN"},
		Models: []Model{{
			ID:       "acme-1",
			Provider: "acme",
			Protocol: ProtocolOpenAICompletions,
			BaseURL:  "https://api.acme.test/v1",
		}},
		Headers: map[string]string{"X-Spec": "spec"},
	})
}

func registryWithProvider(t *testing.T, provider *Provider) *ProviderRegistry {
	t.Helper()
	registry := NewProviderRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return registry
}

// TestResolveAPIKeyPrecedence pins the credential priority table:
// options.APIKey > override.APIKey > options.Env > override.Env > process env.
func TestResolveAPIKeyPrecedence(t *testing.T) {
	t.Setenv("ACME_API_KEY", "from-process")

	provider := testProvider()
	model := provider.Models()[0]
	overrideKey := "from-override"

	tests := []struct {
		name     string
		options  StreamOptions
		override ProviderOverride
		want     string
	}{
		{
			name:     "explicit options key wins over everything",
			options:  StreamOptions{APIKey: "from-options", Env: ProviderEnv{"ACME_API_KEY": "from-options-env"}},
			override: ProviderOverride{APIKey: &overrideKey, Env: ProviderEnv{"ACME_API_KEY": "from-override-env"}},
			want:     "from-options",
		},
		{
			name:     "override key beats env lookups",
			options:  StreamOptions{Env: ProviderEnv{"ACME_API_KEY": "from-options-env"}},
			override: ProviderOverride{APIKey: &overrideKey, Env: ProviderEnv{"ACME_API_KEY": "from-override-env"}},
			want:     "from-override",
		},
		{
			name:     "options env beats override env",
			options:  StreamOptions{Env: ProviderEnv{"ACME_API_KEY": "from-options-env"}},
			override: ProviderOverride{Env: ProviderEnv{"ACME_API_KEY": "from-override-env"}},
			want:     "from-options-env",
		},
		{
			name:     "override env beats process env",
			override: ProviderOverride{Env: ProviderEnv{"ACME_API_KEY": "from-override-env"}},
			want:     "from-override-env",
		},
		{
			name: "process env is the last resort",
			want: "from-process",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := registryWithProvider(t, provider)
			registry.SetOverride("acme", test.override)

			_, options := registry.ResolveRequest(model, test.options)
			if got := options.APIKey; got != test.want {
				t.Fatalf("APIKey = %q, want %q", got, test.want)
			}
		})
	}
}

func TestResolveWhitespaceAPIKeyFallsBack(t *testing.T) {
	t.Setenv("ACME_API_KEY", "from-process")

	registry := registryWithProvider(t, testProvider())
	model := testProvider().Models()[0]

	_, options := registry.ResolveRequest(model, StreamOptions{APIKey: "   "})
	if options.APIKey != "from-process" {
		t.Fatalf("APIKey = %q, want %q", options.APIKey, "from-process")
	}
}

func TestResolveBaseURLOverride(t *testing.T) {
	registry := registryWithProvider(t, testProvider())
	model := testProvider().Models()[0]

	resolved, _ := registry.ResolveRequest(model, StreamOptions{})
	if resolved.BaseURL != "https://api.acme.test/v1" {
		t.Fatalf("BaseURL without override = %q", resolved.BaseURL)
	}

	proxy := "https://proxy.corp.test/acme/v1"
	registry.SetOverride("acme", ProviderOverride{BaseURL: &proxy})

	resolved, _ = registry.ResolveRequest(model, StreamOptions{})
	if resolved.BaseURL != proxy {
		t.Fatalf("BaseURL with override = %q, want %q", resolved.BaseURL, proxy)
	}

	registry.ClearOverride("acme")
	resolved, _ = registry.ResolveRequest(model, StreamOptions{})
	if resolved.BaseURL != "https://api.acme.test/v1" {
		t.Fatalf("BaseURL after ClearOverride = %q", resolved.BaseURL)
	}
}

// TestResolveHeaderMerge pins per-key layering: model < spec < override.
// StreamOptions.Headers stay untouched; adapters merge them last.
func TestResolveHeaderMerge(t *testing.T) {
	provider := testProvider()
	registry := registryWithProvider(t, provider)
	registry.SetOverride("acme", ProviderOverride{
		Headers: map[string]string{"X-Override": "override", "X-Spec": "override-wins"},
	})

	model := provider.Models()[0]
	model.Headers = map[string]string{"X-Model": "model", "X-Spec": "model-loses"}

	resolved, options := registry.ResolveRequest(model, StreamOptions{
		Headers: map[string]string{"X-Options": "options"},
	})

	wantModelHeaders := map[string]string{
		"X-Model":    "model",
		"X-Spec":     "override-wins",
		"X-Override": "override",
	}
	if !reflect.DeepEqual(resolved.Headers, wantModelHeaders) {
		t.Fatalf("model headers = %#v, want %#v", resolved.Headers, wantModelHeaders)
	}
	// Options headers pass through untouched so the adapter keeps them highest.
	if !reflect.DeepEqual(options.Headers, map[string]string{"X-Options": "options"}) {
		t.Fatalf("options headers = %#v", options.Headers)
	}
}

func TestResolveUnknownProviderFallsBackToEnv(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "legacy-key")

	registry := NewProviderRegistry() // deepseek not registered
	model := Model{ID: "m", Provider: "deepseek", Protocol: ProtocolOpenAICompletions}

	resolved, options := registry.ResolveRequest(model, StreamOptions{})
	if options.APIKey != "legacy-key" {
		t.Fatalf("APIKey = %q, want legacy env lookup", options.APIKey)
	}
	if resolved.BaseURL != model.BaseURL || resolved.Provider != model.Provider {
		t.Fatalf("model changed on passthrough: %#v", resolved)
	}
}

func TestResolveNilRegistryFallsBackToEnv(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "legacy-key")

	var registry *ProviderRegistry
	model := Model{ID: "m", Provider: "deepseek", Protocol: ProtocolOpenAICompletions}

	_, options := registry.ResolveRequest(model, StreamOptions{})
	if options.APIKey != "legacy-key" {
		t.Fatalf("APIKey = %q, want legacy env lookup", options.APIKey)
	}
}

func TestAuthStatus(t *testing.T) {
	provider := testProvider()

	t.Run("unconfigured lists missing env vars", func(t *testing.T) {
		registry := registryWithProvider(t, provider)
		status, ok := registry.AuthStatus("acme", nil)
		if !ok {
			t.Fatal("provider not found")
		}
		if status.Configured {
			t.Fatal("expected unconfigured")
		}
		if want := []string{"ACME_API_KEY", "ACME_TOKEN"}; !reflect.DeepEqual(status.Missing, want) {
			t.Fatalf("Missing = %v, want %v", status.Missing, want)
		}
		if status.Label != "Acme" {
			t.Fatalf("Label = %q", status.Label)
		}
	})

	t.Run("env var configures with source", func(t *testing.T) {
		t.Setenv("ACME_TOKEN", "tok")
		registry := registryWithProvider(t, provider)
		status, _ := registry.AuthStatus("acme", nil)
		if !status.Configured || status.Source != "env:ACME_TOKEN" {
			t.Fatalf("status = %+v", status)
		}
	})

	t.Run("override key configures with source", func(t *testing.T) {
		registry := registryWithProvider(t, provider)
		key := "sk-x"
		registry.SetOverride("acme", ProviderOverride{APIKey: &key})
		status, _ := registry.AuthStatus("acme", nil)
		if !status.Configured || status.Source != "override" {
			t.Fatalf("status = %+v", status)
		}
	})

	t.Run("request env configures", func(t *testing.T) {
		registry := registryWithProvider(t, provider)
		status, _ := registry.AuthStatus("acme", ProviderEnv{"ACME_API_KEY": "k"})
		if !status.Configured || status.Source != "env:ACME_API_KEY" {
			t.Fatalf("status = %+v", status)
		}
	})

	t.Run("unknown provider reports not found", func(t *testing.T) {
		registry := NewProviderRegistry()
		if _, ok := registry.AuthStatus("nope", nil); ok {
			t.Fatal("expected ok=false")
		}
	})
}

func TestProviderRegistryRegisterValidation(t *testing.T) {
	registry := NewProviderRegistry()
	if err := registry.Register(nil); err == nil {
		t.Fatal("expected error for nil provider")
	}
	if err := registry.Register(NewSpecProvider(ProviderSpec{})); err == nil {
		t.Fatal("expected error for empty provider ID")
	}
}

func TestProviderRegistryProvidersSorted(t *testing.T) {
	registry := NewProviderRegistry()
	for _, id := range []string{"zeta", "alpha", "mid"} {
		if err := registry.Register(NewSpecProvider(ProviderSpec{ID: id})); err != nil {
			t.Fatalf("Register(%s): %v", id, err)
		}
	}

	var got []string
	for _, provider := range registry.Providers() {
		got = append(got, provider.ID())
	}
	if want := []string{"alpha", "mid", "zeta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Providers() order = %v, want %v", got, want)
	}
}

func TestBuiltInProviderRegistry(t *testing.T) {
	registry := NewBuiltInProviderRegistry()

	provider, ok := registry.Get("anthropic")
	if !ok {
		t.Fatal("anthropic not registered")
	}
	if want := APIKeyEnvVars("anthropic"); !reflect.DeepEqual(provider.EnvKeys(), want) {
		t.Fatalf("EnvKeys = %v, want %v", provider.EnvKeys(), want)
	}
	if len(provider.Models()) == 0 {
		t.Fatal("anthropic has no models")
	}

	// Every catalog provider must be present.
	for _, id := range GetProviders() {
		if _, ok := registry.Get(id); !ok {
			t.Fatalf("catalog provider %q missing from built-in registry", id)
		}
	}
}

func TestProviderModelsReturnsCopies(t *testing.T) {
	provider := testProvider()
	models := provider.Models()
	models[0].BaseURL = "mutated"

	if provider.Models()[0].BaseURL != "https://api.acme.test/v1" {
		t.Fatal("Models() leaked internal state")
	}
}

func TestNewSpecProviderCopiesSpec(t *testing.T) {
	supportsStore := true
	spec := ProviderSpec{
		ID:      "snapshot",
		Name:    "Snapshot",
		EnvKeys: []string{"SNAPSHOT_API_KEY"},
		Models: []Model{{
			ID:       "snapshot-1",
			Provider: "snapshot",
			Protocol: ProtocolOpenAICompletions,
			BaseURL:  "https://snapshot.test/v1",
			Headers:  map[string]string{"X-Model": "original"},
			Input:    []ModelInput{Text},
			Compatibility: &OpenAICompletionsCompatibility{
				SupportsStore: &supportsStore,
			},
		}},
		Headers: map[string]string{"X-Spec": "original"},
	}

	provider := NewSpecProvider(spec)

	spec.EnvKeys[0] = "MUTATED_API_KEY"
	spec.Models[0].BaseURL = "https://mutated.test/v1"
	spec.Models[0].Headers["X-Model"] = "mutated"
	spec.Models[0].Input[0] = Image
	*spec.Models[0].Compatibility.(*OpenAICompletionsCompatibility).SupportsStore = false
	spec.Headers["X-Spec"] = "mutated"

	if got := provider.EnvKeys(); !reflect.DeepEqual(got, []string{"SNAPSHOT_API_KEY"}) {
		t.Fatalf("EnvKeys = %v, want original snapshot", got)
	}
	model := provider.Models()[0]
	if model.BaseURL != "https://snapshot.test/v1" ||
		model.Headers["X-Model"] != "original" ||
		!reflect.DeepEqual(model.Input, []ModelInput{Text}) {
		t.Fatalf("model snapshot was mutated: %#v", model)
	}
	compatibility, ok := model.Compatibility.(*OpenAICompletionsCompatibility)
	if !ok || compatibility.SupportsStore == nil || !*compatibility.SupportsStore {
		t.Fatalf("compatibility snapshot was mutated: %#v", model.Compatibility)
	}
	resolved, _ := provider.resolve(model, StreamOptions{}, ProviderOverride{})
	if resolved.Headers["X-Spec"] != "original" {
		t.Fatalf("spec headers = %v, want original snapshot", resolved.Headers)
	}
}

func TestSetOverrideCopiesInput(t *testing.T) {
	t.Setenv("ACME_API_KEY", "")
	registry := registryWithProvider(t, testProvider())
	model := testProvider().Models()[0]

	baseURL := "https://proxy.test/v1"
	apiKey := "override-key"
	headers := map[string]string{"X-Override": "original"}
	override := ProviderOverride{
		BaseURL: &baseURL,
		APIKey:  &apiKey,
		Headers: headers,
	}
	registry.SetOverride("acme", override)

	baseURL = "https://mutated.test/v1"
	apiKey = "mutated-key"
	headers["X-Override"] = "mutated"

	resolved, options := registry.ResolveRequest(model, StreamOptions{})
	if resolved.BaseURL != "https://proxy.test/v1" {
		t.Fatalf("BaseURL = %q, want stored snapshot", resolved.BaseURL)
	}
	if options.APIKey != "override-key" {
		t.Fatalf("APIKey = %q, want stored snapshot", options.APIKey)
	}
	if resolved.Headers["X-Override"] != "original" {
		t.Fatalf("headers = %v, want stored snapshot", resolved.Headers)
	}

	env := ProviderEnv{"ACME_API_KEY": "env-key"}
	registry.SetOverride("acme", ProviderOverride{Env: env})
	env["ACME_API_KEY"] = "mutated-env-key"
	_, options = registry.ResolveRequest(model, StreamOptions{})
	if options.APIKey != "env-key" {
		t.Fatalf("Env APIKey = %q, want stored snapshot", options.APIKey)
	}
}
