package llm

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func openAIModel() Model {
	return Model{
		ID:       "demo-o",
		Name:     "Demo OpenAI",
		Protocol: ProtocolOpenAICompletions,
		Provider: "demo",
		BaseURL:  "https://example.com",
		Input:    []ModelInput{Text},
		Headers:  map[string]string{"X-Test": "1"},
		Compatibility: &OpenAICompletionsCompatibility{
			SupportsStore:           ptr(true),
			SupportsDeveloperRole:   ptr(false),
			SupportsReasoningEffort: ptr(true),
		},
		Cost:          ModelCost{Input: 3, Output: 15, CacheRead: 0.3, CacheWrite: 3.75},
		ContextWindow: 128000,
		MaxTokens:     16384,
	}
}

func anthropicReasoningModel() Model {
	low := "low_effort"
	high := "high_effort"
	return Model{
		ID:        "demo-a",
		Name:      "Demo Anthropic",
		Protocol:  ProtocolAnthropicMessages,
		Provider:  "demo",
		BaseURL:   "https://example.com",
		Input:     []ModelInput{Text, Image},
		Reasoning: true,
		ThinkingLevelMap: map[ModelThinkingLevel]*string{
			ModelThinkingLow:    &low,
			ModelThinkingMedium: nil, // explicitly unsupported
			ModelThinkingHigh:   &high,
		},
		Compatibility: &AnthropicMessagesCompatibility{
			SupportsTemperature:  ptr(true),
			SupportsCacheControl: ptr(true),
		},
	}
}

func TestSupportedThinkingLevelsNonReasoningModel(t *testing.T) {
	model := openAIModel()
	if got := SupportedThinkingLevels(model); !slices.Equal(got, []ModelThinkingLevel{ModelThinkingOff}) {
		t.Fatalf("levels = %v, want [off]", got)
	}
}

func TestSupportedThinkingLevelsExcludesMappedNil(t *testing.T) {
	model := anthropicReasoningModel()
	got := SupportedThinkingLevels(model)
	// off, minimal, low, high — medium is explicitly nil (unsupported), xhigh is
	// not present in the map.
	want := []ModelThinkingLevel{ModelThinkingOff, ModelThinkingMinimal, ModelThinkingLow, ModelThinkingHigh}
	if !slices.Equal(got, want) {
		t.Fatalf("levels = %v, want %v", got, want)
	}
}

func TestSupportedThinkingLevelsIncludesXHighWhenExplicitlyMapped(t *testing.T) {
	model := anthropicReasoningModel()
	xhighVal := "max"
	model.ThinkingLevelMap[ModelThinkingXHigh] = &xhighVal
	got := SupportedThinkingLevels(model)
	if !slices.Contains(got, ModelThinkingXHigh) {
		t.Fatalf("levels = %v, want xhigh included", got)
	}
}

func TestClampThinkingLevelReturnsRequestedWhenSupported(t *testing.T) {
	model := anthropicReasoningModel()
	if got := ClampThinkingLevel(model, ModelThinkingLow); got != ModelThinkingLow {
		t.Fatalf("clamp(low) = %v, want low", got)
	}
}

func TestClampThinkingLevelStepsUpForUnsupportedLevel(t *testing.T) {
	model := anthropicReasoningModel() // medium is unsupported, high is supported
	if got := ClampThinkingLevel(model, ModelThinkingMedium); got != ModelThinkingHigh {
		t.Fatalf("clamp(medium) = %v, want high (step up)", got)
	}
}

func TestClampThinkingLevelStepsDownWhenNothingAbove(t *testing.T) {
	// Mark everything at or above medium as unsupported (mapped to nil). Clamping
	// medium must then step DOWN to the highest supported level.
	model := anthropicReasoningModel()
	model.ThinkingLevelMap[ModelThinkingMedium] = nil
	model.ThinkingLevelMap[ModelThinkingHigh] = nil
	model.ThinkingLevelMap[ModelThinkingXHigh] = nil
	if got := ClampThinkingLevel(model, ModelThinkingMedium); got != ModelThinkingLow {
		t.Fatalf("clamp(medium) = %v, want low (step down)", got)
	}
}

func TestClampThinkingLevelUnknownLevelFallsBackToLowest(t *testing.T) {
	model := anthropicReasoningModel()
	if got := ClampThinkingLevel(model, ModelThinkingLevel("nope")); got != ModelThinkingOff {
		t.Fatalf("clamp(unknown) = %v, want off (lowest)", got)
	}
}

func TestClampThinkingLevelNonReasoningModelAlwaysOff(t *testing.T) {
	model := openAIModel()
	if got := ClampThinkingLevel(model, ModelThinkingHigh); got != ModelThinkingOff {
		t.Fatalf("clamp(high) on non-reasoning = %v, want off", got)
	}
}

func TestModelRegistryRegisterAndGet(t *testing.T) {
	reg := NewModelRegistry()
	if err := reg.Register(openAIModel()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, ok := reg.Get("demo", "demo-o")
	if !ok {
		t.Fatalf("Get returned !ok")
	}
	if got.ID != "demo-o" {
		t.Fatalf("ID = %q, want demo-o", got.ID)
	}
}

func TestModelRegistryGetReturnsDeepCopy(t *testing.T) {
	reg := NewModelRegistry()
	if err := reg.Register(anthropicReasoningModel()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, _ := reg.Get("demo", "demo-a")
	got.Headers = map[string]string{"X": "mutated"}
	got.ThinkingLevelMap[ModelThinkingLow] = nil
	got.Input[0] = Image

	again, _ := reg.Get("demo", "demo-a")
	if again.Headers != nil {
		t.Fatalf("Headers mutation leaked: %v", again.Headers)
	}
	if again.ThinkingLevelMap[ModelThinkingLow] == nil {
		t.Fatalf("ThinkingLevelMap mutation leaked: low became nil")
	}
	if again.Input[0] != Text {
		t.Fatalf("Input mutation leaked: %v", again.Input)
	}
}

func TestModelRegistryRegisterRejectsInvalidInputs(t *testing.T) {
	cases := []struct {
		name  string
		model Model
		want  string
	}{
		{"missing provider", Model{ID: "x", Protocol: ProtocolOpenAICompletions}, "provider is empty"},
		{"missing id", Model{Provider: "p", Protocol: ProtocolOpenAICompletions}, "ID is empty"},
		{"missing protocol", Model{Provider: "p", ID: "x"}, "protocol is empty"},
		{"compatibility protocol mismatch", Model{
			Provider:      "p",
			ID:            "x",
			Protocol:      ProtocolOpenAICompletions,
			Compatibility: &AnthropicMessagesCompatibility{},
		}, "does not match"},
		{"typed nil compatibility", Model{
			Provider:      "p",
			ID:            "x",
			Protocol:      ProtocolOpenAICompletions,
			Compatibility: (*OpenAICompletionsCompatibility)(nil),
		}, "typed nil"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := NewModelRegistry().Register(tc.model)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Register error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestModelRegistryRegisterRejectsUnknownCompatibilityType(t *testing.T) {
	err := NewModelRegistry().Register(Model{
		Provider:      "p",
		ID:            "x",
		Protocol:      ProtocolOpenAICompletions,
		Compatibility: bogusCompatibility{},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported model compatibility") {
		t.Fatalf("Register error = %v, want unsupported compatibility", err)
	}
}

type bogusCompatibility struct{}

func (bogusCompatibility) Protocol() Protocol { return ProtocolOpenAICompletions }

func TestModelRegistryNilReceiverIsSafe(t *testing.T) {
	var reg *ModelRegistry
	if err := reg.Register(openAIModel()); err == nil {
		t.Fatalf("Register on nil = nil, want error")
	}
	if got, ok := reg.Get("demo", "demo-o"); ok || got.ID != "" {
		t.Fatalf("Get on nil = (%v, %v), want zero", got, ok)
	}
	if got := reg.Providers(); got != nil {
		t.Fatalf("Providers on nil = %v, want nil", got)
	}
	if got := reg.Models("demo"); got != nil {
		t.Fatalf("Models on nil = %v, want nil", got)
	}
}

func TestModelRegistryProvidersAndModelsAreSorted(t *testing.T) {
	reg := NewModelRegistry()
	for _, m := range []Model{
		{Provider: "zeta", ID: "a", Protocol: ProtocolOpenAICompletions},
		{Provider: "alpha", ID: "b", Protocol: ProtocolOpenAICompletions},
		{Provider: "alpha", ID: "a", Protocol: ProtocolOpenAICompletions},
	} {
		if err := reg.Register(m); err != nil {
			t.Fatalf("Register %s/%s: %v", m.Provider, m.ID, err)
		}
	}
	if got := reg.Providers(); !slices.Equal(got, []string{"alpha", "zeta"}) {
		t.Fatalf("Providers = %v, want sorted", got)
	}
	models := reg.Models("alpha")
	if len(models) != 2 || models[0].ID != "a" || models[1].ID != "b" {
		t.Fatalf("Models = %v, want sorted by ID", models)
	}
}

func TestModelRegistryRegisterReplacesExisting(t *testing.T) {
	reg := NewModelRegistry()
	model := openAIModel()
	if err := reg.Register(model); err != nil {
		t.Fatalf("Register: %v", err)
	}
	model.Name = "Updated"
	if err := reg.Register(model); err != nil {
		t.Fatalf("re-Register: %v", err)
	}
	got, _ := reg.Get("demo", "demo-o")
	if got.Name != "Updated" {
		t.Fatalf("Name = %q, want Updated", got.Name)
	}
}

func TestCalculateCost(t *testing.T) {
	model := Model{Cost: ModelCost{
		Input:      3.0,  // $3 per million
		Output:     15.0, // $15 per million
		CacheRead:  0.3,  // $0.30 per million
		CacheWrite: 3.75, // $3.75 per million
	}}
	usage := Usage{
		Input:      1_000_000,
		Output:     500_000,
		CacheRead:  2_000_000,
		CacheWrite: 100_000,
	}
	got := CalculateCost(model, usage)
	want := UsageCost{
		Input:      3.0,
		Output:     7.5,
		CacheRead:  0.6,
		CacheWrite: 0.375,
		Total:      11.475,
	}
	if !floatsClose(got.Input, want.Input) ||
		!floatsClose(got.Output, want.Output) ||
		!floatsClose(got.CacheRead, want.CacheRead) ||
		!floatsClose(got.CacheWrite, want.CacheWrite) ||
		!floatsClose(got.Total, want.Total) {
		t.Fatalf("cost = %#v, want %#v", got, want)
	}
}

func TestCalculateCostZeroUsageIsZero(t *testing.T) {
	model := Model{Cost: ModelCost{Input: 999, Output: 999}}
	got := CalculateCost(model, Usage{})
	if got != (UsageCost{}) {
		t.Fatalf("cost = %#v, want zero", got)
	}
}

func TestLookupModelAndGetModelFromBuiltInCatalog(t *testing.T) {
	providers := GetProviders()
	if len(providers) == 0 {
		t.Fatalf("built-in catalog has no providers")
	}
	var provider string
	var model Model
	for _, p := range providers {
		models := GetModels(p)
		if len(models) > 0 {
			provider = p
			model = models[0]
			break
		}
	}
	if provider == "" {
		t.Fatalf("no provider has any models in the built-in catalog")
	}

	got, ok := LookupModel(provider, model.ID)
	if !ok {
		t.Fatalf("LookupModel(%q, %q) not found", provider, model.ID)
	}
	if got.Provider != provider || got.ID != model.ID {
		t.Fatalf("LookupModel = %q/%q, want %q/%q", got.Provider, got.ID, provider, model.ID)
	}
	if again := GetModel(provider, model.ID); !reflect.DeepEqual(again, got) {
		t.Fatalf("GetModel and LookupModel disagree:\n  Get    = %#v\n  Lookup = %#v", again, got)
	}
}

func TestLookupModelUnknownPair(t *testing.T) {
	if _, ok := LookupModel("nope-provider", "nope-model"); ok {
		t.Fatalf("LookupModel for unknown pair returned ok")
	}
}

func TestGetModelPanicsForUnknownPair(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("GetModel did not panic on unknown pair")
		}
	}()
	GetModel("nope-provider", "nope-model")
}

func TestGetModelsUnknownProviderReturnsEmpty(t *testing.T) {
	if got := GetModels("nope-provider"); len(got) != 0 {
		t.Fatalf("GetModels(unknown) = %v, want empty", got)
	}
}

func floatsClose(a, b float64) bool {
	const epsilon = 1e-9
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < epsilon
}

func TestModelRegistryClonesRequiresThinkingAsText(t *testing.T) {
	requiresThinkingAsText := true
	registry := NewModelRegistry()
	err := registry.Register(Model{
		Provider: "test-provider",
		ID:       "test-model",
		Protocol: ProtocolOpenAICompletions,
		Compatibility: &OpenAICompletionsCompatibility{
			RequiresThinkingAsText: &requiresThinkingAsText,
		},
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Mutating the source after registration must not alter the stored model.
	requiresThinkingAsText = false
	first, ok := registry.Get("test-provider", "test-model")
	if !ok {
		t.Fatal("Get() model not found")
	}
	firstCompatibility := first.Compatibility.(*OpenAICompletionsCompatibility)
	if firstCompatibility.RequiresThinkingAsText == nil || !*firstCompatibility.RequiresThinkingAsText {
		t.Fatalf("stored RequiresThinkingAsText = %v, want true", firstCompatibility.RequiresThinkingAsText)
	}

	// Mutating a returned model must not leak back into the registry.
	*firstCompatibility.RequiresThinkingAsText = false
	second, ok := registry.Get("test-provider", "test-model")
	if !ok {
		t.Fatal("Get() model not found on second lookup")
	}
	secondCompatibility := second.Compatibility.(*OpenAICompletionsCompatibility)
	if secondCompatibility.RequiresThinkingAsText == nil || !*secondCompatibility.RequiresThinkingAsText {
		t.Fatalf("second RequiresThinkingAsText = %v, want true", secondCompatibility.RequiresThinkingAsText)
	}
}
