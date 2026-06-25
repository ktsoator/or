# Models and protocols

[`types.go`](https://github.com/ktsoator/or/blob/main/internal/llm/types.go)
defines the vocabulary the rest of the package speaks: the protocols, the neutral
knobs callers set, and the `Model` that ties an endpoint to its capabilities and
price.

## Neutral vocabulary

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

The four categories line up with the `Usage` counters on a response, so
`CalculateCost` is a category-by-category multiply. Cache reads and writes are
priced apart from fresh input because providers bill them differently.

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

## Tri-state compatibility

Vendors that implement the same protocol still differ in small ways. Those
differences live on a per-protocol compatibility struct. The Anthropic side is
short, because most Anthropic-compatible vendors need no overrides at all:

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

## Protocol as a decode-time discriminator

Both compatibility structs satisfy one interface, whose single method reports
which protocol the configuration describes:

```go
type ModelCompatibility interface {
	Protocol() Protocol
}
```

This keeps `Model` independent of any one protocol. The cost is that the `compat`
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

Source: [`internal/llm/types.go`](https://github.com/ktsoator/or/blob/main/internal/llm/types.go).
The catalog these models are loaded from is covered in
[`models.go`](https://github.com/ktsoator/or/blob/main/internal/llm/models.go).
