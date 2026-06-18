package llm

import "context"

// StreamOptions contains provider-specific settings for a stream request.
type StreamOptions struct {
	APIKey string
}

// Provider adapts a concrete LLM API to the package streaming interface.
type Provider interface {
	// API returns the registry key used to select this provider.
	API() string

	// Stream emits response events for the given model and conversation context.
	Stream(
		ctx context.Context,
		model Model,
		input Context,
		options StreamOptions,
	) (<-chan Event, error)
}
