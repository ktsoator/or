# Models and protocols

[`model.go`](https://github.com/ktsoator/or/blob/main/llm/model.go)
defines the types needed to describe a model: the protocols, the neutral options
callers set, and the `Model` that ties an endpoint to its capabilities and price.
Where those models live and how they are registered and retrieved is implemented
in [`model_registry.go`](https://github.com/ktsoator/or/blob/main/llm/model_registry.go)
and [`catalog.go`](https://github.com/ktsoator/or/blob/main/llm/catalog.go).
This page covers a single model first — how it is defined, how it decodes by
protocol, and how its capabilities are queried — then how those models are stored
and retrieved together.

## Neutral types

Several settings are small string types, each a closed set of constants. Keeping
them as named types — rather than bare strings — lets the compiler catch typos
and keeps the public API self-documenting.

```go
type Protocol string           // "openai-completions", "anthropic-messages"
type ModelInput string         // "text", "image"
type ModelThinkingLevel string // off, minimal, low, medium, high, xhigh
type ThinkingDisplay string    // summarized, omitted
```

`Protocol` names a wire protocol and, through it, the adapter that speaks it.
`ModelInput` names a modality; a model lists the ones it accepts in `Model.Input`,
and an image sent to a text-only model is downgraded rather than rejected.

`ModelThinkingLevel` is provider-independent. A model declares how each level maps
to its own dialect through `Model.ThinkingLevelMap`, which an adapter consults
when building the request. `ThinkingDisplay` is narrower: it does not change
whether the model reasons or what it is billed, only what comes back —
`summarized` returns readable thinking, `omitted` keeps the signature but drops
the text. Only the Anthropic protocol honors it today.

## Pricing

`ModelCost` stores prices in US dollars per million tokens, split by how each
token is billed:

```go
type ModelCost struct {
	Input      float64 // fresh input tokens
	Output     float64 // generated tokens
	CacheRead  float64 // tokens served from the prompt cache
	CacheWrite float64 // tokens written into the prompt cache
}
```

The four categories line up with the `Usage` counters on a response.
`CalculateCost` works from that: prices are per million tokens, so each category
costs `price ÷ 1,000,000 × that category's tokens`, and the four sum to the total.
Cache reads and writes are priced apart from fresh input because providers bill
them differently.

## The Model

`Model` is grouped into four concerns. The comments in the source mark the
boundaries:

```go
type Model struct {
	// Identity
	ID, Name, Provider string

	// Routing
	Protocol Protocol
	BaseURL  string
	Headers  map[string]string

	// Capabilities
	Reasoning        bool
	ThinkingLevelMap map[ModelThinkingLevel]*string
	Input            []ModelInput
	ContextWindow    int64
	MaxTokens        int64

	// Pricing and per-provider quirks
	Cost          ModelCost
	Compatibility ModelCompatibility
}
```

`Protocol` is the routing discriminator: `Client.Stream` uses it to pick an
adapter. `BaseURL` and `Headers` are what let a compatible vendor reuse a
protocol — point the base URL at the vendor's endpoint, add any required headers,
and the same adapter serves it. `ContextWindow` is the total token budget,
`MaxTokens` the cap on generation; both feed the request and the
[overflow check](transform.md).

`ThinkingLevelMap` uses a pointer value on purpose. A `nil` marks a level as
unsupported; a missing key falls back to the provider default. The two cases are
distinct, and a pointer is what lets the map express both — a plain `string`
could not tell "explicitly off" from "not configured."

## Vendor compatibility

Vendors that implement the same protocol still differ in small ways, and those
differences live on a per-protocol compatibility struct (`Model.Compatibility`).
It is an optional set of overrides: left empty, the adapter follows the reference
behavior throughout; a vendor fills in fields only where it actually deviates.

The Anthropic side is short, because most Anthropic-compatible vendors need no
overrides at all:

```go
type AnthropicMessagesCompatibility struct {
	SupportsTemperature       *bool
	SupportsCacheControl      *bool
	SupportsCacheControlTools *bool
	ForceAdaptiveThinking     *bool
	AllowEmptySignature       *bool
}
```

The OpenAI side carries more, because "OpenAI-compatible" covers a wide range of
endpoints:

```go
type OpenAICompletionsCompatibility struct {
	SupportsStore           *bool
	SupportsDeveloperRole   *bool
	SupportsReasoningEffort *bool
	MaxTokensField          string // "max_tokens" vs "max_completion_tokens"
	SupportsStrictMode      *bool
	RequiresThinkingAsText  *bool  // send thinking as a leading text block
	ThinkingFormat          string
	// ... and a few more
}
```

The booleans are pointers for a reason. A plain `bool` has two states and cannot
tell "the vendor explicitly does not support this" from "unspecified, use the
default." A `*bool` has three: `true`, `false`, and `nil` — and `nil` is the
default path. The string fields (`MaxTokensField`, `ThinkingFormat`) name a
variant directly, with the empty string meaning "use the reference behavior."

## Decoding by protocol

Both compatibility structs satisfy one interface, whose single method reports
which protocol the configuration describes:

```go
type ModelCompatibility interface {
	Protocol() Protocol
	clone() ModelCompatibility
}
```

`Protocol()` reports which protocol the compatibility belongs to; `clone()`
returns a fully independent deep copy. Keeping `clone()` on each concrete type
means `cloneModel` never has to type-switch, and adding a field only touches the
type it belongs to. This keeps `Model` independent of any one protocol. The cost is that the `compat`
field has an interface type, and JSON carries no tag for which concrete struct it
holds — so decoding has to choose. `Model.UnmarshalJSON` makes that choice with
`Protocol` as the discriminator:

```go linenums="1" hl_lines="3 12 16"
func (model *Model) UnmarshalJSON(data []byte) error {
	// Decode every field except compat, capturing compat as raw bytes.
	type modelAlias Model // (1)!
	wire := struct {
		*modelAlias
		Compatibility json.RawMessage `json:"compat"`
	}{modelAlias: (*modelAlias)(model)}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}

	if len(wire.Compatibility) == 0 || isJSONNull(wire.Compatibility) {
		model.Compatibility = nil // no overrides
		return nil
	}
	switch model.Protocol { // (2)!
	case ProtocolOpenAICompletions:
		var c OpenAICompletionsCompatibility
		// unmarshal wire.Compatibility into c, assign &c
	case ProtocolAnthropicMessages:
		var c AnthropicMessagesCompatibility
		// ...
	default:
		return fmt.Errorf("unsupported compatibility protocol %q", model.Protocol)
	}
}
```

1.  The `modelAlias` type drops the `UnmarshalJSON` method, so unmarshalling into
    it does not recurse back into this function. `compat` is held back as
    `json.RawMessage` to decode in a second pass.
2.  `Protocol` was already decoded by the first pass, so it is available to select
    the concrete compatibility type.

The field that drives routing at request time is the same field that selects the
type at decode time. A model serializes to JSON and restores without a separate
type tag, because its protocol already carries that information. This is the
runtime equivalent of a type that would be conditional on the protocol at compile
time in a language with that feature.

## Adapting the thinking level

Callers use a neutral `ModelThinkingLevel`, but models differ in which levels they
support. Two functions bring a requested level down to what the target model
actually accepts.

`SupportedThinkingLevels` gives the set of levels a model accepts: a non-reasoning
model supports only `off`; a reasoning model enumerates them in `off → minimal →
low → medium → high → xhigh` order, where a level explicitly mapped to `nil` in
`ThinkingLevelMap` counts as unsupported, and `xhigh` is included only when
explicitly mapped — the top level stays closed by default unless the model
declares it.

`ClampThinkingLevel` snaps any requested level to the nearest supported one, in
this order: take it directly if supported; otherwise step up through the order to
the first supported level; failing that, step down instead; and finally fall back
to the lowest supported level (or `off`). A caller can thus request any level and
always get one the target model can handle, without comparing each model's
capability table itself.

## The registry

`ModelRegistry` is the storage and lookup hub for models — built-in models are
registered here at startup, and every runtime query goes through it. It has just
two fields: a read-write lock and a two-level "provider → model ID → model" map:

```go
type ModelRegistry struct {
	mu     sync.RWMutex
	models map[string]map[string]Model // provider → model ID → Model
}
```

The outer key is the provider, the inner key a model ID under it. Providers are
grouped independently of protocol — models under the same provider may use
different protocols and still nest side by side:

```text
models ┌─ "anthropic" ─┬─ "claude-opus-4-8"   → Model{protocol: anthropic-messages}
       │               └─ "claude-sonnet-4-6" → Model{protocol: anthropic-messages}
       │
       └─ "deepseek" ──┬─ "deepseek-v4-flash" → Model{protocol: openai-completions}
                       └─ "deepseek-v4-pro"   → Model{protocol: openai-completions}
```

The `sync.RWMutex` guards every read and write of `models` — queries take a read
lock, registrations a write lock — so the registry can be shared across goroutines
without external locking.

### Registration and validation

`Register` writes a model, but runs a fixed sequence before it lands in the table:

1. **Non-empty check** — provider, ID, and protocol must all be present; if any is
   empty it returns an error and registration stops.
2. **Compatibility check** — if the model carries a compatibility configuration
   (`Compatibility`), `validateModelCompatibility` checks it further: the concrete
   type has to be one of the known protocol compatibility structs, and the protocol
   it declares has to match the model's own `Protocol`. In other words, a model
   marked `anthropic-messages` cannot carry an OpenAI compatibility configuration —
   such a mismatch is caught at registration time rather than surfacing later at
   request time.
3. **Acquire the write lock** — `mu.Lock` makes concurrent registrations mutually
   exclusive.
4. **Lazily create the sub-map** — if the provider's inner map does not yet exist,
   it is created first.
5. **Store a deep copy** — what is written is `cloneModel(model)`, not the caller's
   original; registering the same ID again under a provider overwrites the existing
   entry.

All validation comes before the lock and the write: an invalid model is rejected
before the registry is touched, so no half-built state is left behind.

### Retrieving a model

Lookup comes at two levels. A registry instance has three methods, working against
any registry:

```go
func (r *ModelRegistry) Get(provider, modelID string) (Model, bool)
func (r *ModelRegistry) Providers() []string
func (r *ModelRegistry) Models(provider string) []Model
```

- `Get` fetches a single model by provider and model ID: on a hit it returns the
  model and `true`, on a miss the zero value and `false`.
- `Providers` lists every provider ID in the registry, lexically sorted.
- `Models` lists every model under one provider, sorted by model ID.

All three return sorted results, so iteration order is stable and reproducible.

The package-level functions wrap those three methods over the built-in registry
`builtInModelRegistry`, which most callers use directly — sparing them from holding
a registry instance of their own:

```go
func LookupModel(provider, modelID string) (Model, bool) // Get; false when missing
func GetModel(provider, modelID string) Model            // Get; panics when missing
func GetProviders() []string                             // Providers
func GetModels(provider string) []Model                  // Models
```

Fetching a single model comes in two forms, `LookupModel` and `GetModel`,
differing only in how they handle a miss: `GetModel` suits identifiers hard-coded
in source that ought to exist, where a miss is a program error; `LookupModel` suits
identifiers from config or external input, where the caller must handle a miss
itself. `GetProviders` and `GetModels` pass straight through to `Providers` and
`Models` with the same semantics.

Whichever entry point is used, what comes back is a deep copy. A `Model` holds
slices, maps, and pointer fields; handing back the table's original would let a
caller's edits reach other holders, so an independent copy is returned to rule out
that implicit coupling.

## Built-in models

The built-in models in that registry are not fetched at runtime; they ship with
the binary.
[`catalog.generated.json`](https://github.com/ktsoator/or/blob/main/llm/catalog.generated.json)
is produced by `go generate` (`internal/genmodels`) from upstream catalog data —
[Models.dev](https://models.dev) as the primary source, plus the live catalogs and
pricing from OpenRouter and Vercel AI Gateway — and emits only models whose
protocol the package implements (`openai-completions` and `anthropic-messages`).
The result is committed alongside the source and embedded into the binary with
`//go:embed`:

```go
//go:embed catalog.generated.json
var generatedCatalogJSON []byte
```

So neither the build nor startup depends on the network or the working directory.
The registry is populated during program startup: `builtInModelRegistry` is a
package-level variable initialized before `main` — it first decodes the embedded
JSON, then `Register`s each model into the table:

```go
var builtInModelRegistry = newBuiltInModelRegistry()

func builtInModels() []Model {
	var models []Model
	if err := json.Unmarshal(generatedCatalogJSON, &models); err != nil {
		panic(...) // a broken catalog means there is nothing to start from
	}
	return models
}
```

A decode or registration failure panics rather than returns an error. The embedded
catalog is a build-time artifact: at runtime it is either intact or the build
itself is broken, with no degraded state to run in between — so the program is made
to fail early.

Source: [`model.go`](https://github.com/ktsoator/or/blob/main/llm/model.go),
[`model_registry.go`](https://github.com/ktsoator/or/blob/main/llm/model_registry.go),
and [`catalog.go`](https://github.com/ktsoator/or/blob/main/llm/catalog.go).
