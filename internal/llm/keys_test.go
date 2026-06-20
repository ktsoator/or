package llm

import (
	"reflect"
	"slices"
	"testing"
)

func TestAPIKeyEnvVarsReturnsConfiguredOrder(t *testing.T) {
	// Anthropic intentionally checks the OAuth token before the API key.
	got := APIKeyEnvVars("anthropic")
	want := []string{"ANTHROPIC_OAUTH_TOKEN", "ANTHROPIC_API_KEY"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("APIKeyEnvVars = %v, want %v", got, want)
	}
}

func TestAPIKeyEnvVarsReturnsDefensiveCopy(t *testing.T) {
	got := APIKeyEnvVars("xiaomi")
	if len(got) < 2 {
		t.Fatalf("expected multi-entry provider, got %v", got)
	}
	got[0] = "MUTATED"
	again := APIKeyEnvVars("xiaomi")
	if again[0] == "MUTATED" {
		t.Fatalf("APIKeyEnvVars leaked its internal slice")
	}
}

func TestAPIKeyEnvVarsUnknownProviderReturnsEmpty(t *testing.T) {
	if got := APIKeyEnvVars("does-not-exist"); len(got) != 0 {
		t.Fatalf("APIKeyEnvVars = %v, want empty", got)
	}
}

func TestGetEnvAPIKeyWithEnvPrefersFirstConfigured(t *testing.T) {
	env := ProviderEnv{
		"ANTHROPIC_OAUTH_TOKEN": "oauth-token",
		"ANTHROPIC_API_KEY":     "api-key",
	}
	if got := GetEnvAPIKeyWithEnv("anthropic", env); got != "oauth-token" {
		t.Fatalf("GetEnvAPIKey = %q, want oauth-token", got)
	}
}

func TestGetEnvAPIKeyWithEnvFallsThroughEmpty(t *testing.T) {
	// First var is set but empty, second is the real one — the lookup must skip
	// the empty value and fall through.
	env := ProviderEnv{
		"ANTHROPIC_OAUTH_TOKEN": "",
		"ANTHROPIC_API_KEY":     "real-key",
	}
	if got := GetEnvAPIKeyWithEnv("anthropic", env); got != "real-key" {
		t.Fatalf("GetEnvAPIKey = %q, want real-key", got)
	}
}

func TestGetEnvAPIKeyWithEnvFallsBackToProcessEnv(t *testing.T) {
	// Per-request override is empty; process env should win.
	t.Setenv("DEEPSEEK_API_KEY", "from-process")
	if got := GetEnvAPIKeyWithEnv("deepseek", ProviderEnv{}); got != "from-process" {
		t.Fatalf("GetEnvAPIKey = %q, want from-process", got)
	}
}

func TestGetEnvAPIKeyWithEnvProviderEnvOverridesProcess(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "from-process")
	env := ProviderEnv{"DEEPSEEK_API_KEY": "from-override"}
	if got := GetEnvAPIKeyWithEnv("deepseek", env); got != "from-override" {
		t.Fatalf("GetEnvAPIKey = %q, want from-override", got)
	}
}

func TestGetEnvAPIKeyEmptyWhenNoSource(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "")
	if got := GetEnvAPIKeyWithEnv("deepseek", nil); got != "" {
		t.Fatalf("GetEnvAPIKey = %q, want empty", got)
	}
}

func TestGetEnvAPIKeyDelegatesToWithEnv(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "process-groq")
	if got := GetEnvAPIKey("groq"); got != "process-groq" {
		t.Fatalf("GetEnvAPIKey = %q, want process-groq", got)
	}
}

func TestFindEnvAPIKeysOnlyReturnsConfiguredVars(t *testing.T) {
	// xiaomi accepts XIAOMI_API_KEY or MIMO_API_KEY; only the second is set.
	t.Setenv("XIAOMI_API_KEY", "")
	env := ProviderEnv{"MIMO_API_KEY": "yes"}
	got := FindEnvAPIKeysWithEnv("xiaomi", env)
	if !slices.Equal(got, []string{"MIMO_API_KEY"}) {
		t.Fatalf("FindEnvAPIKeys = %v, want [MIMO_API_KEY]", got)
	}
}

func TestFindEnvAPIKeysReportsBothWhenBothPresent(t *testing.T) {
	env := ProviderEnv{
		"XIAOMI_API_KEY": "a",
		"MIMO_API_KEY":   "b",
	}
	got := FindEnvAPIKeysWithEnv("xiaomi", env)
	if !slices.Equal(got, []string{"XIAOMI_API_KEY", "MIMO_API_KEY"}) {
		t.Fatalf("FindEnvAPIKeys = %v, want both in declared order", got)
	}
}

func TestFindEnvAPIKeysWrapsProcessLookup(t *testing.T) {
	t.Setenv("CEREBRAS_API_KEY", "cb")
	if got := FindEnvAPIKeys("cerebras"); !slices.Equal(got, []string{"CEREBRAS_API_KEY"}) {
		t.Fatalf("FindEnvAPIKeys = %v, want [CEREBRAS_API_KEY]", got)
	}
}

func TestFindEnvAPIKeysReturnsNilForUnknownProvider(t *testing.T) {
	if got := FindEnvAPIKeysWithEnv("does-not-exist", nil); got != nil {
		t.Fatalf("FindEnvAPIKeys = %v, want nil", got)
	}
}
