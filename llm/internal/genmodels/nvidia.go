package main

var nvidiaVerifiedReasoningRoutes = map[string]struct{}{
	"deepseek-ai/deepseek-v4-flash": {},
	"deepseek-ai/deepseek-v4-pro":   {},
}

// normalizeNVIDIAModel preserves models.dev reasoning controls only for
// verified NVIDIA routes. It does not inherit the native model vendor's wire
// dialect; NVIDIA keeps the protocol declared by its provider rule.
func normalizeNVIDIAModel(candidate *model, source sourceModel) {
	if candidate == nil || candidate.Provider != "nvidia" {
		return
	}
	if _, ok := nvidiaVerifiedReasoningRoutes[candidate.ID]; !ok {
		return
	}
	applyVerifiedGatewayReasoningMetadata(candidate, source.ReasoningOptions)
}
