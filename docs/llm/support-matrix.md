# Protocol and provider support matrix

The model catalog and protocol implementations are independent. `GetModels` returns catalog entries. A request is routable only when the current process has registered an adapter for the model's `Protocol`. Use `GetRunnableModels` for runtime model lists.

<!-- catalog-stats: total=821 runnable=743 openai-completions=481 anthropic-messages=262 openai-responses=45 google-generative-ai=12 mistral-conversations=21 -->

## Protocol status

| Protocol | Catalog models | Status | Registration | Meaning |
|---|---:|---|---|---|
| `openai-completions` | 481 | Implemented | `_ "github.com/ktsoator/or/llm/openai"` | OpenAI Chat Completions and compatible endpoints |
| `anthropic-messages` | 262 | Implemented | `_ "github.com/ktsoator/or/llm/anthropic"` | Anthropic Messages and compatible endpoints |
| `openai-responses` | 45 | Catalog only | None | No adapter; official OpenAI catalog models use this protocol |
| `google-generative-ai` | 12 | Catalog only | None | No adapter |
| `mistral-conversations` | 21 | Catalog only | None | No adapter |

Importing `github.com/ktsoator/or/llm/all` registers the two implemented protocols. It does not implement the three catalog-only protocols.

## Catalog versus runtime state

```go
models := llm.GetModels("openai") // returns cataloged openai-responses models
for _, model := range models {
	fmt.Println(model.ID, llm.SupportsProtocol(model.Protocol))
}

runnable := llm.GetRunnableModels("openai") // currently empty
```

`GetModel` and `LookupModel` only check catalog membership. They do not check adapter registration. Call `SupportsProtocol` before sending, or build selection UIs from `GetRunnableModels`.

## Provider catalog

The current catalog contains the following provider IDs. Counts come from `llm/catalog.generated.json`.

| Provider ID | Models | Protocol | Credential variables |
|---|---:|---|---|
| `anthropic` | 12 | Anthropic Messages | `ANTHROPIC_OAUTH_TOKEN` or `ANTHROPIC_API_KEY` |
| `cerebras` | 3 | OpenAI Completions | `CEREBRAS_API_KEY` |
| `deepseek` | 4 | OpenAI Completions | `DEEPSEEK_API_KEY` |
| `fireworks` | 16 | Anthropic Messages | `FIREWORKS_API_KEY` |
| `github-copilot` | 17 | Both implemented protocols | `COPILOT_GITHUB_TOKEN` |
| `google` | 12 | Google Generative AI | `GEMINI_API_KEY` |
| `groq` | 7 | OpenAI Completions | `GROQ_API_KEY` |
| `huggingface` | 49 | OpenAI Completions | `HF_TOKEN` |
| `kimi-coding` | 4 | Anthropic Messages | `KIMI_API_KEY` |
| `minimax` | 7 | Anthropic Messages | `MINIMAX_API_KEY` |
| `minimax-cn` | 7 | Anthropic Messages | `MINIMAX_CN_API_KEY` |
| `mistral` | 21 | Mistral Conversations | `MISTRAL_API_KEY` |
| `moonshotai` | 9 | OpenAI Completions | `MOONSHOT_API_KEY` |
| `moonshotai-cn` | 9 | OpenAI Completions | `MOONSHOT_API_KEY` |
| `nvidia` | 45 | OpenAI Completions | `NVIDIA_API_KEY` |
| `openai` | 45 | OpenAI Responses | `OPENAI_API_KEY` |
| `opencode` | 32 | Both implemented protocols | `OPENCODE_API_KEY` |
| `opencode-go` | 13 | Both implemented protocols | `OPENCODE_API_KEY` |
| `openrouter` | 268 | OpenAI Completions | `OPENROUTER_API_KEY` |
| `together` | 20 | OpenAI Completions | `TOGETHER_API_KEY` |
| `vercel-ai-gateway` | 192 | Anthropic Messages | `AI_GATEWAY_API_KEY` |
| `xai` | 5 | OpenAI Completions | `XAI_API_KEY` |
| `xiaomi` | 3 | OpenAI Completions | `XIAOMI_API_KEY` or `MIMO_API_KEY` |
| `xiaomi-token-plan-ams` | 3 | OpenAI Completions | `XIAOMI_TOKEN_PLAN_AMS_API_KEY` |
| `xiaomi-token-plan-cn` | 3 | OpenAI Completions | `XIAOMI_TOKEN_PLAN_CN_API_KEY` |
| `xiaomi-token-plan-sgp` | 3 | OpenAI Completions | `XIAOMI_TOKEN_PLAN_SGP_API_KEY` |
| `zai` | 6 | OpenAI Completions | `ZAI_API_KEY` |
| `zai-coding-cn` | 6 | OpenAI Completions | `ZAI_CODING_CN_API_KEY` |

The authoritative credential mapping is `llm/keys.go` and `APIKeyEnvVars(provider)`.

## Validation level

Built-in tests validate both protocol adapters with local mock servers. The current project material does not show continuous integration against every live provider in the table. Therefore:

- “implemented protocol” means the adapter can route and decode that wire format;
- catalog membership is not a live-provider support guarantee;
- provider changes to non-standard fields may require a `Model.Compatibility` update or `RewriteRequest`;
- production adoption should test authentication, streaming, tools, usage, and errors against the selected provider account.

## Model capabilities

Read capabilities from each `Model`:

| Capability | Field or API |
|---|---|
| Image input | `slices.Contains(model.Input, llm.Image)` |
| Reasoning | `model.Reasoning`, `SupportedThinkingLevels(model)` |
| Context window | `model.ContextWindow` |
| Maximum output | `model.MaxTokens` |
| Catalog price | `model.Cost` |
| Routable now | `SupportsProtocol(model.Protocol)` |

The catalog is generated from external sources and embedded into the binary. Prices, model status, and limits can lag behind a provider. `Usage.Cost` is a catalog-priced estimate, not an invoice.
