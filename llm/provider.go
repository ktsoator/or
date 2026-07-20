package llm

import (
	"maps"
	"slices"
	"strings"
)

// Provider is the runtime entity for one vendor key: its identity, credential
// sources, model list, and static headers. It complements the ModelRegistry,
// which stores per-model data; the Provider answers configuration questions
// (is a key set, where does it come from) and applies per-provider overrides
// to outgoing requests.
//
// Provider is deliberately a concrete, spec-driven struct rather than an
// interface: every built-in vendor is described by data alone. Vendors that
// need custom behavior (OAuth refresh, multi-field auth) are a later phase
// and can introduce an interface without touching this type.
type Provider struct {
	spec ProviderSpec
}

// ProviderSpec is the data-driven description of a vendor. There is no
// BaseURL field on purpose: endpoint URLs live on each Model in the catalog,
// and the only runtime BaseURL knob is ProviderOverride.BaseURL.
type ProviderSpec struct {
	// ID is the vendor key models reference via Model.Provider, e.g. "deepseek".
	ID string
	// Name is the human-readable display name, e.g. "DeepSeek".
	Name string
	// EnvKeys are the environment variables checked for an API key, in
	// precedence order. Built-in providers take these from APIKeyEnvVars.
	EnvKeys []string
	// Models are the provider's known models.
	Models []Model
	// Headers are merged into every request for this provider, overriding
	// same-named model default headers.
	Headers map[string]string
}

// ProviderOverride adjusts every request routed through a provider. All fields
// are optional; the zero value is a no-op. Overrides are stored per provider
// on the ProviderRegistry via SetOverride.
type ProviderOverride struct {
	// BaseURL, when set, replaces Model.BaseURL for every request.
	BaseURL *string
	// APIKey, when set and non-empty, supplies the credential unless the
	// request passes StreamOptions.APIKey explicitly.
	APIKey *string
	// DisableEnv prevents provider environment variables from supplying a
	// credential. Explicit StreamOptions.APIKey and APIKey above still work.
	// Product shells can use this when credentials must come exclusively from
	// their own persisted settings.
	DisableEnv bool
	// Headers are merged into every request, overriding same-named model and
	// provider spec headers. StreamOptions.Headers still win over these.
	Headers map[string]string
	// Env supplies environment overrides for credential lookup. Non-empty
	// StreamOptions.Env values take precedence over these.
	Env ProviderEnv
}

// AuthStatus reports whether a provider has a usable credential and where it
// comes from. It is a read-only snapshot for CLIs and UIs; request-time
// resolution happens in ProviderRegistry.ResolveRequest.
type AuthStatus struct {
	// Configured reports whether an API key resolves right now.
	Configured bool
	// Source names where the key comes from: "override" or "env:<VAR>".
	// Empty when unconfigured.
	Source string
	// Label is the provider display name.
	Label string
	// Missing lists the environment variables that were checked but empty.
	// Nil when configured or when the provider declares no EnvKeys.
	Missing []string
}

// NewSpecProvider creates a provider from an independent snapshot of spec.
// Callers may safely reuse or mutate the slices, maps, and model configuration
// passed in after this function returns.
func NewSpecProvider(spec ProviderSpec) *Provider {
	return &Provider{spec: cloneProviderSpec(spec)}
}

func cloneProviderSpec(spec ProviderSpec) ProviderSpec {
	clone := spec
	clone.EnvKeys = slices.Clone(spec.EnvKeys)
	clone.Headers = maps.Clone(spec.Headers)
	if spec.Models != nil {
		clone.Models = make([]Model, len(spec.Models))
		for index, model := range spec.Models {
			clone.Models[index] = cloneModel(model)
		}
	}
	return clone
}

func cloneProviderOverride(override ProviderOverride) ProviderOverride {
	clone := override
	clone.BaseURL = clonePointer(override.BaseURL)
	clone.APIKey = clonePointer(override.APIKey)
	clone.Headers = maps.Clone(override.Headers)
	clone.Env = maps.Clone(override.Env)
	return clone
}

// ID returns the vendor key models reference via Model.Provider.
func (provider *Provider) ID() string {
	if provider == nil {
		return ""
	}
	return provider.spec.ID
}

// Name returns the provider display name.
func (provider *Provider) Name() string {
	if provider == nil {
		return ""
	}
	return provider.spec.Name
}

// Models returns copies of the provider's known models.
func (provider *Provider) Models() []Model {
	if provider == nil {
		return nil
	}
	models := make([]Model, 0, len(provider.spec.Models))
	for _, model := range provider.spec.Models {
		models = append(models, cloneModel(model))
	}
	return models
}

// EnvKeys returns the environment variables checked for an API key, in
// precedence order. The returned slice is safe for the caller to modify.
func (provider *Provider) EnvKeys() []string {
	if provider == nil {
		return nil
	}
	return append([]string(nil), provider.spec.EnvKeys...)
}

// resolve applies the provider spec and override to one request. It returns
// the model and options the adapter should see. The precedence, from highest
// to lowest, is:
//
//	APIKey:  StreamOptions.APIKey > override.APIKey > EnvKeys via
//	         StreamOptions.Env > override.Env > process environment;
//	         DisableEnv removes the final environment lookup layer
//	BaseURL: override.BaseURL > Model.BaseURL
//	Headers: StreamOptions.Headers > override.Headers > spec.Headers >
//	         Model.Headers (per key; adapters merge StreamOptions.Headers
//	         over Model.Headers, so spec and override fold into the model)
func (provider *Provider) resolve(model Model, options StreamOptions, override ProviderOverride) (Model, StreamOptions) {
	if override.BaseURL != nil {
		model.BaseURL = *override.BaseURL
	}

	model.Headers = mergeHeaders(model.Headers, provider.spec.Headers, override.Headers)

	if strings.TrimSpace(options.APIKey) == "" {
		switch {
		case override.APIKey != nil && *override.APIKey != "":
			options.APIKey = *override.APIKey
		case !override.DisableEnv:
			env := mergeEnv(override.Env, options.Env)
			options.APIKey = envAPIKeyFrom(provider.spec.EnvKeys, env)
		}
	}

	return model, options
}

// authStatus reports the provider's credential state under the given
// request-scoped environment and override.
func (provider *Provider) authStatus(env ProviderEnv, override ProviderOverride) AuthStatus {
	status := AuthStatus{Label: provider.spec.Name}

	if override.APIKey != nil && *override.APIKey != "" {
		status.Configured = true
		status.Source = "override"
		return status
	}
	if override.DisableEnv {
		return status
	}

	merged := mergeEnv(override.Env, env)
	for _, name := range provider.spec.EnvKeys {
		if providerEnvValue(name, merged) != "" {
			status.Configured = true
			status.Source = "env:" + name
			return status
		}
	}

	status.Missing = append([]string(nil), provider.spec.EnvKeys...)
	return status
}

// envAPIKeyFrom returns the first configured value among the environment
// variable names, preferring request-scoped env values over the process
// environment. It mirrors GetEnvAPIKeyWithEnv but takes an explicit key list
// so spec-driven providers share one lookup path.
func envAPIKeyFrom(envKeys []string, env ProviderEnv) string {
	for _, name := range envKeys {
		if value := providerEnvValue(name, env); value != "" {
			return value
		}
	}
	return ""
}

// mergeHeaders merges header maps in order, later maps overriding same-named
// keys of earlier ones. It returns nil when every input is empty.
func mergeHeaders(layers ...map[string]string) map[string]string {
	size := 0
	for _, layer := range layers {
		size += len(layer)
	}
	if size == 0 {
		return nil
	}

	merged := make(map[string]string, size)
	for _, layer := range layers {
		maps.Copy(merged, layer)
	}
	return merged
}

// mergeEnv layers request-scoped env values over base values. It returns the
// dominant map unchanged when the other is empty.
func mergeEnv(base, dominant ProviderEnv) ProviderEnv {
	if len(base) == 0 {
		return dominant
	}
	if len(dominant) == 0 {
		return base
	}

	merged := make(ProviderEnv, len(base)+len(dominant))
	maps.Copy(merged, base)
	maps.Copy(merged, dominant)
	return merged
}
