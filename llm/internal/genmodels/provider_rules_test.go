package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestValidateProviderRuleSet(t *testing.T) {
	usableCatalog := map[string]sourceProvider{
		"shared": {Models: map[string]sourceModel{"model": xiaomiRouteTestModel(1)}},
		"other":  {Models: map[string]sourceModel{"model": xiaomiRouteTestModel(2)}},
	}

	t.Run("allows a source shared by distinct providers", func(t *testing.T) {
		rules := []providerRule{
			validProviderRule("shared", "first"),
			validProviderRule("shared", "second"),
		}
		if err := validateProviderRuleSet(rules, usableCatalog); err != nil {
			t.Fatalf("validateProviderRuleSet: %v", err)
		}
	})

	t.Run("rejects duplicate providers", func(t *testing.T) {
		rules := []providerRule{
			validProviderRule("shared", "duplicate"),
			validProviderRule("other", "duplicate"),
		}
		assertProviderRuleError(t, rules, usableCatalog, `duplicates provider "duplicate"`)
	})

	t.Run("rejects incomplete routing fields", func(t *testing.T) {
		tests := []struct {
			name   string
			mutate func(*providerRule)
			want   string
		}{
			{name: "source", mutate: func(rule *providerRule) { rule.Source = "" }, want: "empty source"},
			{name: "provider", mutate: func(rule *providerRule) { rule.Provider = "" }, want: "empty provider"},
			{name: "protocol", mutate: func(rule *providerRule) { rule.Protocol = "" }, want: "empty protocol"},
			{name: "base URL", mutate: func(rule *providerRule) { rule.BaseURL = "not-a-url" }, want: "invalid base URL"},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				rule := validProviderRule("shared", "provider")
				test.mutate(&rule)
				assertProviderRuleError(t, []providerRule{rule}, usableCatalog, test.want)
			})
		}
	})

	t.Run("rejects a missing source", func(t *testing.T) {
		rules := []providerRule{validProviderRule("missing", "provider")}
		assertProviderRuleError(t, rules, usableCatalog, `references missing models.dev source "missing"`)
	})

	t.Run("rejects sources without usable models", func(t *testing.T) {
		unusable := sourceModel{ToolCall: true, Status: "deprecated"}
		catalog := map[string]sourceProvider{
			"unusable": {Models: map[string]sourceModel{
				"no-tools":   {},
				"deprecated": unusable,
			}},
		}
		rules := []providerRule{validProviderRule("unusable", "provider")}
		assertProviderRuleError(t, rules, catalog, "has no usable tool-calling models")
	})
}

func TestLoadModelsDevModelsPropagatesProviderRuleValidation(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{}`)),
		}, nil
	})}

	_, err := loadModelsDevModels(context.Background(), client)
	if err == nil || !strings.Contains(err.Error(), "validate models.dev provider rules") || !strings.Contains(err.Error(), "missing models.dev source") {
		t.Fatalf("loadModelsDevModels error = %v, want provider rule validation failure", err)
	}
}

func TestXiaomiProviderRulesUseRouteSpecificSources(t *testing.T) {
	want := map[string]string{
		"xiaomi":                "xiaomi",
		"xiaomi-token-plan-cn":  "xiaomi-token-plan-cn",
		"xiaomi-token-plan-ams": "xiaomi-token-plan-ams",
		"xiaomi-token-plan-sgp": "xiaomi-token-plan-sgp",
	}
	for _, rule := range providerRules {
		source, ok := want[rule.Provider]
		if !ok {
			continue
		}
		if rule.Source != source {
			t.Errorf("provider %s source = %q, want %q", rule.Provider, rule.Source, source)
		}
		delete(want, rule.Provider)
	}
	for provider := range want {
		t.Errorf("missing provider rule for %s", provider)
	}
}

func validProviderRule(source, provider string) providerRule {
	return providerRule{
		Source:   source,
		Provider: provider,
		Protocol: "openai-completions",
		BaseURL:  "https://example.com/v1",
	}
}

func assertProviderRuleError(
	t *testing.T,
	rules []providerRule,
	catalog map[string]sourceProvider,
	want string,
) {
	t.Helper()
	err := validateProviderRuleSet(rules, catalog)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("validateProviderRuleSet error = %v, want %q", err, want)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestFromModelsDevKeepsXiaomiRouteMetadataIndependent(t *testing.T) {
	catalog := map[string]sourceProvider{
		"xiaomi": {
			Models: map[string]sourceModel{
				"shared":      xiaomiRouteTestModel(1),
				"direct-only": xiaomiRouteTestModel(11),
			},
		},
		"xiaomi-token-plan-cn": {
			Models: map[string]sourceModel{
				"shared":  xiaomiRouteTestModel(0),
				"cn-only": xiaomiRouteTestModel(21),
			},
		},
		"xiaomi-token-plan-ams": {
			Models: map[string]sourceModel{
				"shared":   xiaomiRouteTestModel(0),
				"ams-only": xiaomiRouteTestModel(31),
			},
		},
		"xiaomi-token-plan-sgp": {
			Models: map[string]sourceModel{
				"shared":   xiaomiRouteTestModel(0),
				"sgp-only": xiaomiRouteTestModel(41),
			},
		},
	}

	models := fromModelsDev(catalog)
	tests := []struct {
		provider    string
		routeOnlyID string
		wantCost    float64
	}{
		{provider: "xiaomi", routeOnlyID: "direct-only", wantCost: 1},
		{provider: "xiaomi-token-plan-cn", routeOnlyID: "cn-only", wantCost: 0},
		{provider: "xiaomi-token-plan-ams", routeOnlyID: "ams-only", wantCost: 0},
		{provider: "xiaomi-token-plan-sgp", routeOnlyID: "sgp-only", wantCost: 0},
	}
	for _, test := range tests {
		t.Run(test.provider, func(t *testing.T) {
			shared, ok := findGeneratedRouteModel(models, test.provider, "shared")
			if !ok {
				t.Fatalf("missing %s/shared", test.provider)
			}
			if shared.InputCost != test.wantCost {
				t.Errorf("%s/shared input cost = %v, want %v", test.provider, shared.InputCost, test.wantCost)
			}
			if _, ok := findGeneratedRouteModel(models, test.provider, test.routeOnlyID); !ok {
				t.Errorf("missing route-only model %s/%s", test.provider, test.routeOnlyID)
			}
			if test.provider != "xiaomi" {
				if _, leaked := findGeneratedRouteModel(models, test.provider, "direct-only"); leaked {
					t.Errorf("direct-only model leaked into %s", test.provider)
				}
			}
		})
	}
}

func xiaomiRouteTestModel(inputCost float64) sourceModel {
	candidate := sourceModel{ToolCall: true}
	candidate.Limit.Context = 4096
	candidate.Limit.Output = 1024
	candidate.Cost.Input = inputCost
	return candidate
}

func findGeneratedRouteModel(models []model, provider, id string) (model, bool) {
	for _, candidate := range models {
		if candidate.Provider == provider && candidate.ID == id {
			return candidate, true
		}
	}
	return model{}, false
}
