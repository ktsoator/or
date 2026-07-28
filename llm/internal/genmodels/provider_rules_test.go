package main

import "testing"

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
