# Model catalog generator

Run from the repository root:

```sh
go generate ./llm
```

The generated `llm/catalog.generated.json` is committed and embedded
by `catalog.go`, so normal builds and application startup do not need network
or filesystem access.

Generation is strict by default: every catalog source must succeed, the result
must pass structural and size checks, and the destination is replaced
atomically only after rendering succeeds. For local source diagnostics, partial
output can be written to a separate file explicitly:

```sh
go run ./llm/internal/genmodels \
  -allow-partial \
  -output /tmp/catalog.partial.json
```

Do not commit a partial catalog.

The generator draws on one public catalog source:

- [Models.dev](https://models.dev) is the primary source. It is an open-source
  database created by OpenCode and maintained as provider/model TOML files in
  [`sst/models.dev`](https://github.com/sst/models.dev).
Models.dev is a catalog aggregator, not an authoritative model vendor. Provider
API documentation remains the source of truth when metadata conflicts. Local
normalization and compatibility overrides live in `provider_*.go` and
`overrides.go` and should stay small and explicit. Source-specific fetching is
kept in `source_*.go`, while `render.go` owns deterministic catalog output.

Compatibility belongs to a routed model, identified by provider endpoint and
model ID. Models with the same ID on a native API and an aggregation gateway do
not inherit controls from each other. Generator-only thinking profiles classify
verified routes as fixed, toggle, or effort-controlled and compile that behavior
into the existing runtime catalog fields. Unverified gateway routes keep the
provider's fixed default instead of advertising a control that may do nothing.

Verified models.dev `reasoning_options` effort values are normalized into the
SDK's thinking levels for standard OpenAI-compatible and Anthropic adaptive-
thinking models. Toggle and token budget controls remain provider-specific, and
local compatibility overrides keep verified provider behavior authoritative
when source metadata is incomplete.

The catalog includes models for the implemented `openai-completions` and
`anthropic-messages` protocols, plus selected catalog-only protocols planned
for future adapters. Use the public runtime model APIs to distinguish catalog
entries from runnable models. The generated JSON is sorted by provider and
model ID.
