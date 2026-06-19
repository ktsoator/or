package llm

import "regexp"

// overflowPatterns match error messages returned when the input exceeds a
// model's context window. Providers word these very differently, so each entry
// targets one vendor's phrasing (see the example messages alongside each).
//
//   - Anthropic:        "prompt is too long: 213462 tokens > 200000 maximum"
//   - Anthropic (413):  "request_too_large"
//   - Amazon Bedrock:   "input is too long for requested model"
//   - OpenAI:           "Your input exceeds the context window of this model"
//   - OpenAI/LiteLLM:   "exceeds the model's maximum context length of 131072 tokens"
//   - Google (Gemini):  "input token count (...) exceeds the maximum"
//   - xAI (Grok):       "maximum prompt length is 131072 but the request contains ..."
//   - Groq:             "Please reduce the length of the messages or completion"
//   - OpenRouter:       "maximum context length is X tokens"
//   - OpenRouter/Poolside: "exceeds the maximum allowed input length of Y tokens"
//   - Together AI:      "input (X tokens) is longer than the model's context length (Y tokens)"
//   - GitHub Copilot:   "exceeds the limit of Y"
//   - llama.cpp:        "exceeds the available context size"
//   - LM Studio:        "greater than the context length"
//   - MiniMax:          "context window exceeds limit"
//   - Kimi For Coding:  "exceeded model token limit: X (requested: Y)"
//   - Mistral:          "too large for model with Y maximum context length"
//   - Cerebras:         "400/413 status code (no body)"
var overflowPatterns = compilePatterns([]string{
	`(?i)prompt is too long`,                    // Anthropic token overflow
	`(?i)request_too_large`,                     // Anthropic request byte-size overflow (HTTP 413)
	`(?i)input is too long for requested model`, // Amazon Bedrock
	`(?i)exceeds the context window`,            // OpenAI (Completions & Responses API)
	`(?i)exceeds (?:the )?(?:model'?s )?maximum context length(?: of [\d,]+ tokens?|\s*\([\d,]+\))`, // OpenAI-compatible proxies (LiteLLM)
	`(?i)input token count.*exceeds the maximum`,                                                    // Google (Gemini)
	`(?i)maximum prompt length is \d+`,                                                              // xAI (Grok)
	`(?i)reduce the length of the messages`,                                                         // Groq
	`(?i)maximum context length is \d+ tokens`,                                                      // OpenRouter (most backends)
	`(?i)exceeds (?:the )?maximum allowed input length of [\d,]+ tokens?`,                           // OpenRouter/Poolside
	`(?i)input \(\d+ tokens\) is longer than the model'?s context length \(\d+ tokens\)`,            // Together AI
	`(?i)exceeds the limit of \d+`,                                                                  // GitHub Copilot
	`(?i)exceeds the available context size`,                                                        // llama.cpp server
	`(?i)greater than the context length`,                                                           // LM Studio
	`(?i)context window exceeds limit`,                                                              // MiniMax
	`(?i)exceeded model token limit`,                                                                // Kimi For Coding
	`(?i)too large for model with \d+ maximum context length`,                                       // Mistral
	`(?i)model_context_window_exceeded`,                                                             // z.ai non-standard finish_reason surfaced as error text
	`(?i)prompt too long; exceeded (?:max )?context length`,                                         // Ollama explicit overflow error
	`(?i)context[_ ]length[_ ]exceeded`,                                                             // Generic fallback
	`(?i)too many tokens`,                                                                           // Generic fallback
	`(?i)token limit exceeded`,                                                                      // Generic fallback
	`(?i)^4(?:00|13)\s*(?:status code)?\s*\(no body\)`,                                              // Cerebras: 400/413 with no body
})

// nonOverflowPatterns match errors that are not overflows even when they happen
// to match an overflow pattern (e.g. Bedrock formats throttling as "Too many
// tokens, please wait" which would otherwise hit /too many tokens/).
var nonOverflowPatterns = compilePatterns([]string{
	`(?i)^(Throttling error|Service unavailable):`, // AWS Bedrock non-overflow errors
	`(?i)rate limit`,        // Generic rate limiting
	`(?i)too many requests`, // Generic HTTP 429 style
})

// IsContextOverflow reports whether an assistant message indicates that the
// input exceeded the model's context window. It covers three cases:
//
//  1. Error-based overflow: most providers return StopReasonError with a
//     recognizable error message (matched against overflowPatterns, excluding
//     nonOverflowPatterns such as rate limits).
//  2. Silent overflow (e.g. z.ai): the request succeeds but usage.Input exceeds
//     the context window. Pass a non-zero contextWindow to detect this.
//  3. Length-stop overflow (e.g. Xiaomi MiMo): the server truncates oversized
//     input to fill the window, leaving no room to generate, so it returns
//     StopReasonLength with zero output and input filling the window.
//
// Pass contextWindow as the model's window size to enable cases 2 and 3; pass 0
// to check error messages only.
func IsContextOverflow(message AssistantMessage, contextWindow int64) bool {
	// Case 1: error message patterns.
	if message.StopReason == StopReasonError && message.ErrorMessage != "" {
		if !matchesAny(nonOverflowPatterns, message.ErrorMessage) &&
			matchesAny(overflowPatterns, message.ErrorMessage) {
			return true
		}
	}

	// Case 2: silent overflow (z.ai style) - successful but usage exceeds context.
	if contextWindow > 0 && message.StopReason == StopReasonStop {
		if message.Usage.Input+message.Usage.CacheRead > contextWindow {
			return true
		}
	}

	// Case 3: length-stop overflow (Xiaomi MiMo style) - input truncated to fill
	// the window, leaving no room for output.
	if contextWindow > 0 && message.StopReason == StopReasonLength && message.Usage.Output == 0 {
		inputTokens := message.Usage.Input + message.Usage.CacheRead
		if float64(inputTokens) >= float64(contextWindow)*0.99 {
			return true
		}
	}

	return false
}

// OverflowPatterns returns a copy of the overflow detection patterns, primarily
// for tests.
func OverflowPatterns() []*regexp.Regexp {
	return append([]*regexp.Regexp(nil), overflowPatterns...)
}

func matchesAny(patterns []*regexp.Regexp, text string) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

func compilePatterns(sources []string) []*regexp.Regexp {
	patterns := make([]*regexp.Regexp, len(sources))
	for index, source := range sources {
		patterns[index] = regexp.MustCompile(source)
	}
	return patterns
}
