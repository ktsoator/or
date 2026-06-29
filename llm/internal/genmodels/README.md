# Model catalog generator

Run from the repository root:

```sh
go generate ./llm
```

The generated `llm/catalog.generated.json` is committed and embedded
by `catalog.go`, so normal builds and application startup do not need network
or filesystem access.

The generator draws on several public catalog layers:

- [Models.dev](https://models.dev) is the primary source. It is an open-source
  database created by OpenCode and maintained as provider/model TOML files in
  [`sst/models.dev`](https://github.com/sst/models.dev).
- [OpenRouter](https://openrouter.ai/api/v1/models) supplies its live routed
  model catalog and pricing.
- [Vercel AI Gateway](https://ai-gateway.vercel.sh/v1/models) supplies its live
  gateway catalog and pricing.

These are catalog aggregators, not authoritative model vendors. Provider API
documentation remains the source of truth when metadata conflicts. Local
normalization and compatibility overrides live in `main.go` and should stay
small and explicit.

Only models whose protocol is implemented by the Go package are emitted.
Currently those protocols are `openai-completions` and `anthropic-messages`.
The generated JSON is grouped by provider at the top level.
