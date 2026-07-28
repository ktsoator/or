package main

import "testing"

func TestApplyThinkingProfile(t *testing.T) {
	tests := []struct {
		name                 string
		profile              thinkingProfile
		wantFormat           string
		wantVisibility       string
		wantReplay           bool
		wantSupportsEffort   bool
		wantSupportedLevels  []string
		wantUnsupportedLevel []string
	}{
		{
			name:                 "fixed provider default",
			profile:              fixedProviderDefault(),
			wantSupportedLevels:  []string{"high"},
			wantUnsupportedLevel: []string{"off", "minimal", "low", "medium"},
		},
		{
			name:                 "hidden fixed reasoning",
			profile:              fixedThinking("hidden", false),
			wantVisibility:       "hidden",
			wantSupportedLevels:  []string{"high"},
			wantUnsupportedLevel: []string{"off", "minimal", "low", "medium"},
		},
		{
			name:                 "deepseek toggle",
			profile:              toggleThinking("deepseek", true),
			wantFormat:           "deepseek",
			wantReplay:           true,
			wantSupportedLevels:  []string{"off", "high"},
			wantUnsupportedLevel: []string{"minimal", "low", "medium"},
		},
		{
			name:                 "OpenAI effort",
			profile:              effortThinking("", false, "high", "max"),
			wantSupportsEffort:   true,
			wantSupportedLevels:  []string{"high", "max"},
			wantUnsupportedLevel: []string{"off", "minimal", "low", "medium", "xhigh"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := model{
				Reasoning: true,
				Compat: compatibility{
					Kind:                    "openai",
					SupportsReasoningEffort: boolp(true),
					ThinkingFormat:          "stale",
					RequiresReasoningContentOnAssistantMessages: boolp(true),
				},
			}
			applyThinkingProfile(&candidate, test.profile)

			if candidate.Compat.ThinkingFormat != test.wantFormat {
				t.Fatalf("ThinkingFormat = %q, want %q", candidate.Compat.ThinkingFormat, test.wantFormat)
			}
			if candidate.ThinkingVisibility != test.wantVisibility {
				t.Fatalf("ThinkingVisibility = %q, want %q", candidate.ThinkingVisibility, test.wantVisibility)
			}
			gotReplay := candidate.Compat.RequiresReasoningContentOnAssistantMessages != nil &&
				*candidate.Compat.RequiresReasoningContentOnAssistantMessages
			if gotReplay != test.wantReplay {
				t.Fatalf("ReplayReasoning = %v, want %v", gotReplay, test.wantReplay)
			}
			gotSupportsEffort := candidate.Compat.SupportsReasoningEffort != nil &&
				*candidate.Compat.SupportsReasoningEffort
			if gotSupportsEffort != test.wantSupportsEffort {
				t.Fatalf("SupportsReasoningEffort = %v, want %v", gotSupportsEffort, test.wantSupportsEffort)
			}
			for _, level := range test.wantSupportedLevels {
				value, present := candidate.ThinkingLevelMap[level]
				if present && value == nil {
					t.Errorf("level %q is unsupported", level)
				}
			}
			for _, level := range test.wantUnsupportedLevel {
				if value, present := candidate.ThinkingLevelMap[level]; !present || value != nil {
					t.Errorf("level %q = %v (present %v), want explicit unsupported", level, value, present)
				}
			}
		})
	}
}

func TestValidateThinkingProfile(t *testing.T) {
	key := modelRouteKey{Provider: "opencode-go", ModelID: "test-model"}
	tests := []struct {
		name    string
		profile thinkingProfile
		wantErr bool
	}{
		{name: "fixed", profile: fixedProviderDefault()},
		{name: "toggle", profile: toggleThinking("deepseek", false)},
		{name: "effort", profile: effortThinking("", false, "low", "high")},
		{name: "toggle without wire format", profile: thinkingProfile{Control: thinkingToggle, Levels: []string{"off", "high"}}, wantErr: true},
		{name: "toggle with unknown wire format", profile: toggleThinking("typo", false), wantErr: true},
		{name: "fixed with controls", profile: thinkingProfile{Control: thinkingFixed, Format: "deepseek"}, wantErr: true},
		{name: "effort without levels", profile: thinkingProfile{Control: thinkingEffort}, wantErr: true},
		{name: "invalid level", profile: effortThinking("", false, "extreme"), wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateThinkingProfile(key, test.profile)
			if got := err != nil; got != test.wantErr {
				t.Fatalf("validateThinkingProfile() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestValidateAppliedThinkingProfileRequiresExplicitCompatibility(t *testing.T) {
	profile := fixedProviderDefault()
	candidate := model{
		ID: "test-model", Provider: "opencode-go", Protocol: "openai-completions", Reasoning: true,
		Compat: compatibility{Kind: "openai"},
	}
	applyThinkingProfile(&candidate, profile)
	candidate.Compat.SupportsReasoningEffort = nil

	if err := validateAppliedThinkingProfile(candidate, profile); err == nil {
		t.Fatal("validateAppliedThinkingProfile accepted an implicit endpoint fallback")
	}
}
