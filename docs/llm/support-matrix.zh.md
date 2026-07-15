# 协议与提供方状态

内置模型清单和协议实现是两套独立信息。`GetModels` 返回清单中收录的模型；只有模型的 `Protocol` 已在当前进程注册协议适配器时，请求才能被路由。使用 `GetRunnableModels` 构建运行时模型列表。

<!-- catalog-stats: total=821 runnable=743 openai-completions=481 anthropic-messages=262 openai-responses=45 google-generative-ai=12 mistral-conversations=21 -->

## 协议状态

| 协议 | 收录模型数 | 状态 | 注册方式 | 说明 |
|---|---:|---|---|---|
| `openai-completions` | 481 | 已实现 | `_ "github.com/ktsoator/or/llm/openai"` | OpenAI Chat Completions 及兼容服务 |
| `anthropic-messages` | 262 | 已实现 | `_ "github.com/ktsoator/or/llm/anthropic"` | Anthropic Messages 及兼容服务 |
| `openai-responses` | 45 | 仅收录 | 无 | 当前没有协议适配器；内置模型清单中的 OpenAI 模型使用该协议 |
| `google-generative-ai` | 12 | 仅收录 | 无 | 当前没有协议适配器 |
| `mistral-conversations` | 21 | 仅收录 | 无 | 当前没有协议适配器 |

导入 `github.com/ktsoator/or/llm/all` 会注册两个已实现协议。它不会为三个仅收录协议增加实现。

## 模型清单与运行状态

```go
models := llm.GetModels("openai") // 返回模型清单中收录的 openai-responses 模型
for _, model := range models {
	fmt.Println(model.ID, llm.SupportsProtocol(model.Protocol))
}

runnable := llm.GetRunnableModels("openai") // 当前为空
```

`GetModel` 和 `LookupModel` 只验证模型是否收录在清单中，不验证协议适配器。发送前可调用 `SupportsProtocol`，或从一开始只使用 `GetRunnableModels`。

## 提供方清单

当前模型清单包含以下提供方 ID。模型数量来自 `llm/catalog.generated.json`。

| 提供方 ID | 模型数 | 协议 | 凭证变量 |
|---|---:|---|---|
| `anthropic` | 12 | Anthropic Messages | `ANTHROPIC_OAUTH_TOKEN` 或 `ANTHROPIC_API_KEY` |
| `cerebras` | 3 | OpenAI Completions | `CEREBRAS_API_KEY` |
| `deepseek` | 4 | OpenAI Completions | `DEEPSEEK_API_KEY` |
| `fireworks` | 16 | Anthropic Messages | `FIREWORKS_API_KEY` |
| `github-copilot` | 17 | 两种已实现协议 | `COPILOT_GITHUB_TOKEN` |
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
| `opencode` | 32 | 两种已实现协议 | `OPENCODE_API_KEY` |
| `opencode-go` | 13 | 两种已实现协议 | `OPENCODE_API_KEY` |
| `openrouter` | 268 | OpenAI Completions | `OPENROUTER_API_KEY` |
| `together` | 20 | OpenAI Completions | `TOGETHER_API_KEY` |
| `vercel-ai-gateway` | 192 | Anthropic Messages | `AI_GATEWAY_API_KEY` |
| `xai` | 5 | OpenAI Completions | `XAI_API_KEY` |
| `xiaomi` | 3 | OpenAI Completions | `XIAOMI_API_KEY` 或 `MIMO_API_KEY` |
| `xiaomi-token-plan-ams` | 3 | OpenAI Completions | `XIAOMI_TOKEN_PLAN_AMS_API_KEY` |
| `xiaomi-token-plan-cn` | 3 | OpenAI Completions | `XIAOMI_TOKEN_PLAN_CN_API_KEY` |
| `xiaomi-token-plan-sgp` | 3 | OpenAI Completions | `XIAOMI_TOKEN_PLAN_SGP_API_KEY` |
| `zai` | 6 | OpenAI Completions | `ZAI_API_KEY` |
| `zai-coding-cn` | 6 | OpenAI Completions | `ZAI_CODING_CN_API_KEY` |

各提供方使用哪些凭证环境变量，由 `llm/keys.go` 中的内置配置定义。运行时可调用 `APIKeyEnvVars(provider)` 查询指定提供方的变量名。

## 验证范围

内置测试通过本地模拟服务覆盖两种已实现协议的适配器。项目未对表中每个提供方执行持续的线上集成测试，因此：

- “已实现”表示内置协议适配器可以处理该协议的请求与响应格式；
- “模型已收录”只表示模型元数据存在于内置模型清单中，不代表已验证目标提供方的线上兼容性；
- 提供方使用非标准字段时，可能需要配置 `Model.Compatibility` 或使用 `RewriteRequest`；
- 上线前应使用目标提供方的真实账户，验证鉴权、流式响应、工具调用、Token 用量和错误处理。

## 模型能力

单个模型的能力以 `Model` 字段为准：

| 能力 | 字段或接口 |
|---|---|
| 图像输入 | `slices.Contains(model.Input, llm.Image)` |
| 推理 | `model.Reasoning`、`SupportedThinkingLevels(model)` |
| 上下文窗口 | `model.ContextWindow` |
| 最大输出 | `model.MaxTokens` |
| 内置价格 | `model.Cost` |
| 当前可路由 | `SupportsProtocol(model.Protocol)` |

模型清单由外部数据源生成并嵌入二进制。价格、模型状态和限制可能落后于提供方的实际信息；`Usage.Cost` 是基于内置价格的估算，不是账单。
