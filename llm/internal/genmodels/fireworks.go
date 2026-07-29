package main

var fireworksVerifiedReasoningRoutes = map[string]struct{}{
	"accounts/fireworks/models/deepseek-v4-flash": {},
	"accounts/fireworks/models/deepseek-v4-pro":   {},
}

// normalizeFireworksModel preserves models.dev reasoning controls only for
// verified Fireworks routes. It does not inherit the native model vendor's wire
// dialect; Fireworks keeps the protocol declared by its provider rule.
func normalizeFireworksModel(candidate *model, source sourceModel) {
	if candidate == nil || candidate.Provider != "fireworks" {
		return
	}
	if _, ok := fireworksVerifiedReasoningRoutes[candidate.ID]; !ok {
		return
	}
	applyVerifiedGatewayReasoningMetadata(candidate, source.ReasoningOptions)
}
