package llm

import "context"

// The package keeps a default protocol-adapter registry and a client bound to
// it. Provider packages register themselves into this registry from an init
// function, so importing a provider — typically for side effects — makes its
// protocol available to the package-level Stream and Complete helpers:
//
//	import (
//		"github.com/ktsoator/or/llm"
//		_ "github.com/ktsoator/or/llm/openai" // registers the OpenAI-compatible protocol
//	)
//
//	msg, err := llm.Complete(ctx, model, input, llm.StreamOptions{})
//
// Import github.com/ktsoator/or/llm/all to register every built-in protocol at
// once. A caller that prefers explicit wiring can skip the default registry and
// build its own with NewAdapterRegistry, AdapterRegistry.Register, and NewClient.
var (
	defaultRegistry         = NewAdapterRegistry()
	defaultProviderRegistry = NewBuiltInProviderRegistry()
	defaultClient           = NewClient(defaultRegistry, defaultProviderRegistry)
)

// DefaultProviderRegistry returns the provider registry backing the package
// default client. Use it to check provider auth status, apply per-provider
// overrides, or register custom providers:
//
//	llm.DefaultProviderRegistry().SetOverride("deepseek", llm.ProviderOverride{
//		BaseURL: &proxyURL,
//	})
func DefaultProviderRegistry() *ProviderRegistry {
	return defaultProviderRegistry
}

// Register adds adapter to the package default registry, replacing any adapter
// already registered for its protocol. Provider packages call it from init; most
// applications register a provider by importing its package rather than calling
// Register directly.
func Register(adapter ProtocolAdapter) error {
	return defaultRegistry.Register(adapter)
}

// Stream starts a streaming model request using the default client. The model's
// protocol must have an adapter registered (import the matching provider
// package); otherwise it returns a "no adapter registered" error.
func Stream(ctx context.Context, model Model, input Context, options StreamOptions) (<-chan Event, error) {
	return defaultClient.Stream(ctx, model, input, options)
}

// Complete runs a request on the default client and returns the final assistant
// message. Like Stream, it requires the model's protocol to be registered.
func Complete(ctx context.Context, model Model, input Context, options StreamOptions) (AssistantMessage, error) {
	return defaultClient.Complete(ctx, model, input, options)
}
