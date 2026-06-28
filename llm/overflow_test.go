package llm

import "testing"

func TestIsContextOverflowMatchesVendorErrorMessages(t *testing.T) {
	// One representative phrasing per vendor entry in overflowPatterns.
	cases := []struct {
		name string
		msg  string
	}{
		{"Anthropic prompt", "prompt is too long: 213462 tokens > 200000 maximum"},
		{"Anthropic 413", "request_too_large"},
		{"Bedrock", "input is too long for requested model"},
		{"OpenAI explicit", "Your input exceeds the context window of this model"},
		{"OpenAI/LiteLLM", "exceeds the model's maximum context length of 131072 tokens"},
		{"Google Gemini", "input token count (250000) exceeds the maximum"},
		{"xAI Grok", "maximum prompt length is 131072 but the request contains more"},
		{"Groq", "Please reduce the length of the messages or completion"},
		{"OpenRouter", "maximum context length is 200000 tokens"},
		{"OpenRouter/Poolside", "exceeds the maximum allowed input length of 16384 tokens"},
		{"Together AI", "input (12345 tokens) is longer than the model's context length (8192 tokens)"},
		{"GitHub Copilot", "exceeds the limit of 16384"},
		{"llama.cpp", "exceeds the available context size"},
		{"LM Studio", "greater than the context length"},
		{"MiniMax", "context window exceeds limit"},
		{"Kimi", "exceeded model token limit: 200000 (requested: 250000)"},
		{"Mistral", "too large for model with 32000 maximum context length"},
		{"z.ai", "model_context_window_exceeded"},
		{"Ollama", "prompt too long; exceeded max context length"},
		{"generic context_length_exceeded", "context_length_exceeded"},
		{"generic too many tokens", "this request has too many tokens"},
		{"generic token limit", "token limit exceeded"},
		{"Cerebras 413", "413 (no body)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := AssistantMessage{StopReason: StopReasonError, ErrorMessage: tc.msg}
			if !IsContextOverflow(msg, 0) {
				t.Fatalf("IsContextOverflow(%q) = false, want true", tc.msg)
			}
		})
	}
}

func TestIsContextOverflowSkipsNonOverflowErrors(t *testing.T) {
	cases := []struct {
		name string
		msg  string
	}{
		// Bedrock returns throttling using "too many tokens"-like phrasing; the
		// non-overflow filter must keep these out.
		{"Bedrock throttling", "Throttling error: too many tokens, please wait"},
		{"service unavailable", "Service unavailable: please retry"},
		{"rate limit", "rate limit exceeded for token bucket"},
		{"429 too many requests", "too many requests, please slow down"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := AssistantMessage{StopReason: StopReasonError, ErrorMessage: tc.msg}
			if IsContextOverflow(msg, 0) {
				t.Fatalf("IsContextOverflow(%q) = true, want false", tc.msg)
			}
		})
	}
}

func TestIsContextOverflowIgnoresErrorMessageOnSuccess(t *testing.T) {
	// An overflow-shaped error message attached to a successful response is not
	// itself an overflow: the response succeeded.
	msg := AssistantMessage{StopReason: StopReasonStop, ErrorMessage: "prompt is too long"}
	if IsContextOverflow(msg, 0) {
		t.Fatalf("IsContextOverflow over successful stop = true, want false")
	}
}

func TestIsContextOverflowIgnoresEmptyError(t *testing.T) {
	msg := AssistantMessage{StopReason: StopReasonError}
	if IsContextOverflow(msg, 0) {
		t.Fatalf("IsContextOverflow with empty error = true, want false")
	}
}

func TestIsContextOverflowSilentOverflowWhenUsageExceedsWindow(t *testing.T) {
	// Case 2: z.ai-style — request succeeds, but tokens consumed beat the window.
	msg := AssistantMessage{
		StopReason: StopReasonStop,
		Usage:      Usage{Input: 199000, CacheRead: 2000},
	}
	if !IsContextOverflow(msg, 200000) {
		t.Fatalf("silent overflow = false, want true")
	}
}

func TestIsContextOverflowSilentOverflowRequiresPositiveWindow(t *testing.T) {
	msg := AssistantMessage{StopReason: StopReasonStop, Usage: Usage{Input: 1_000_000}}
	if IsContextOverflow(msg, 0) {
		t.Fatalf("silent overflow with zero window = true, want false (window unknown)")
	}
}

func TestIsContextOverflowSilentOverflowBelowWindow(t *testing.T) {
	msg := AssistantMessage{StopReason: StopReasonStop, Usage: Usage{Input: 1000, CacheRead: 500}}
	if IsContextOverflow(msg, 200000) {
		t.Fatalf("silent overflow when under window = true, want false")
	}
}

func TestIsContextOverflowLengthStopOverflowAtFullFill(t *testing.T) {
	// Case 3: Xiaomi MiMo-style — input truncated to fill the window, leaving no
	// room for output. 99% fill counts as overflow.
	msg := AssistantMessage{
		StopReason: StopReasonLength,
		Usage:      Usage{Input: 198000, CacheRead: 2000, Output: 0},
	}
	if !IsContextOverflow(msg, 200000) {
		t.Fatalf("length-stop overflow = false, want true")
	}
}

func TestIsContextOverflowLengthStopIgnoredWhenOutputNonZero(t *testing.T) {
	// Normal length-stop (model spoke but ran out of room) is not an overflow.
	msg := AssistantMessage{
		StopReason: StopReasonLength,
		Usage:      Usage{Input: 199000, Output: 100},
	}
	if IsContextOverflow(msg, 200000) {
		t.Fatalf("length stop with non-zero output = true, want false")
	}
}

func TestIsContextOverflowLengthStopBelowThreshold(t *testing.T) {
	// 90% fill is below the 99% threshold — not an overflow signal.
	msg := AssistantMessage{
		StopReason: StopReasonLength,
		Usage:      Usage{Input: 180000, Output: 0},
	}
	if IsContextOverflow(msg, 200000) {
		t.Fatalf("length stop at 90%% fill = true, want false")
	}
}

func TestIsContextOverflowRejectsRandomError(t *testing.T) {
	msg := AssistantMessage{StopReason: StopReasonError, ErrorMessage: "internal server error"}
	if IsContextOverflow(msg, 0) {
		t.Fatalf("IsContextOverflow on unrelated error = true, want false")
	}
}

func TestOverflowPatternsReturnsDefensiveCopy(t *testing.T) {
	got := OverflowPatterns()
	if len(got) == 0 {
		t.Fatalf("OverflowPatterns returned empty slice")
	}
	got[0] = nil
	again := OverflowPatterns()
	if again[0] == nil {
		t.Fatalf("OverflowPatterns leaked its internal slice")
	}
}
