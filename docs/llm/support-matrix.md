# Protocol and provider status

The model catalog and protocol implementations are independent. `GetModels` returns catalog entries. A request is routable only when the current process has registered an adapter for the model's `Protocol`. Use `GetRunnableModels` for runtime model lists.

<!-- catalog-stats: total=394 runnable=352 openai-completions=226 anthropic-messages=73 openai-responses=53 google-generative-ai=20 mistral-conversations=22 -->

## Protocol status

| Protocol | Catalog models | Status | Registration | Meaning |
|---|---:|---|---|---|
| `openai-completions` | 226 | Implemented | `_ "github.com/ktsoator/or/llm/openai"` | OpenAI Chat Completions and compatible endpoints |
| `anthropic-messages` | 73 | Implemented | `_ "github.com/ktsoator/or/llm/anthropic"` | Anthropic Messages and compatible endpoints |
| `openai-responses` | 53 | Implemented | `_ "github.com/ktsoator/or/llm/openai"` | OpenAI Responses API and compatible gateways |
| `google-generative-ai` | 20 | Catalog only | None | No adapter |
| `mistral-conversations` | 22 | Catalog only | None | No adapter |

Importing `github.com/ktsoator/or/llm/all` registers the three implemented protocols. It does not implement the two catalog-only protocols.

## Catalog versus runtime state

```go
models := llm.GetModels("openai") // returns cataloged openai-responses models
for _, model := range models {
	fmt.Println(model.ID, llm.SupportsProtocol(model.Protocol))
}

runnable := llm.GetRunnableModels("openai") // returns the runnable Responses models
```

`GetModel` and `LookupModel` only check catalog membership. They do not check adapter registration. Call `SupportsProtocol` before sending, or build selection UIs from `GetRunnableModels`.

## Provider catalog

The current catalog contains the following provider IDs. Counts come from `llm/catalog.generated.json`.

| Provider ID | Models | Protocol | Credential variables |
|---|---:|---|---|
| `anthropic` | 13 | Anthropic Messages | `ANTHROPIC_API_KEY` |
| `cerebras` | 3 | OpenAI Completions | `CEREBRAS_API_KEY` |
| `deepseek` | 4 | OpenAI Completions | `DEEPSEEK_API_KEY` |
| `fireworks` | 17 | Anthropic Messages | `FIREWORKS_API_KEY` |
| `github-copilot` | 18 | Both implemented protocols | `COPILOT_GITHUB_TOKEN` |
| `google` | 20 | Google Generative AI | `GEMINI_API_KEY` |
| `groq` | 7 | OpenAI Completions | `GROQ_API_KEY` |
| `huggingface` | 54 | OpenAI Completions | `HF_TOKEN` |
| `kimi-coding` | 4 | Anthropic Messages | `KIMI_API_KEY` |
| `minimax` | 7 | Anthropic Messages | `MINIMAX_API_KEY` |
| `minimax-cn` | 7 | Anthropic Messages | `MINIMAX_CN_API_KEY` |
| `mistral` | 22 | Mistral Conversations | `MISTRAL_API_KEY` |
| `moonshotai` | 10 | OpenAI Completions | `MOONSHOT_API_KEY` |
| `moonshotai-cn` | 10 | OpenAI Completions | `MOONSHOT_API_KEY` |
| `nvidia` | 57 | OpenAI Completions | `NVIDIA_API_KEY` |
| `openai` | 30 | OpenAI Responses | `OPENAI_API_KEY` |
| `opencode` | 55 | All three implemented protocols | `OPENCODE_API_KEY` |
| `opencode-go` | 17 | All three implemented protocols | `OPENCODE_API_KEY` |
| `together` | 17 | OpenAI Completions | `TOGETHER_API_KEY` |
| `xai` | 5 | OpenAI Completions | `XAI_API_KEY` |
| `xiaomi` | 3 | OpenAI Completions | `XIAOMI_API_KEY` or `MIMO_API_KEY` |
| `xiaomi-token-plan-ams` | 2 | OpenAI Completions | `XIAOMI_TOKEN_PLAN_AMS_API_KEY` |
| `xiaomi-token-plan-cn` | 2 | OpenAI Completions | `XIAOMI_TOKEN_PLAN_CN_API_KEY` |
| `xiaomi-token-plan-sgp` | 2 | OpenAI Completions | `XIAOMI_TOKEN_PLAN_SGP_API_KEY` |
| `zai` | 4 | OpenAI Completions | `ZAI_API_KEY` |
| `zai-coding-cn` | 4 | OpenAI Completions | `ZAI_CODING_CN_API_KEY` |

Built-in credential configuration is defined in `llm/keys.go`. At runtime, call
`APIKeyEnvVars(provider)` to query the variable names for a provider.

## Validation scope

Built-in tests cover the adapters for all three implemented protocols with local mock servers. The project does not run continuous live integration tests against every provider in the table. Therefore:

- “implemented” means the built-in adapter handles the protocol's request and response formats;
- “model is listed” means only that its metadata exists in the built-in model catalog; it is not a live-provider compatibility guarantee;
- providers using non-standard fields may require `Model.Compatibility` or `RewriteRequest`;
- before production use, validate authentication, streaming, tools, token usage, and error handling with an account for the target provider.

## Model capabilities

Read capabilities from each `Model`:

| Capability | Field or API |
|---|---|
| Image input | `slices.Contains(model.Input, llm.ModelInputImage)` |
| Reasoning | `model.Reasoning`, `SupportedThinkingLevels(model)` |
| Context window | `model.ContextWindow` |
| Maximum output | `model.MaxTokens` |
| Catalog price | `model.Cost` |
| Routable now | `SupportsProtocol(model.Protocol)` |

The catalog is generated from external sources and embedded into the binary. Prices, model status, and limits can lag behind a provider. `Usage.Cost` is a catalog-priced estimate, not an invoice.
