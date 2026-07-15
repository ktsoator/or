# 协议与 Provider 支持矩阵

模型目录和协议实现是两套独立信息。`GetModels` 返回目录条目；只有模型的 `Protocol` 已在当前进程注册 adapter 时，请求才能被路由。使用 `GetRunnableModels` 构建运行时模型列表。

<!-- catalog-stats: total=821 runnable=743 openai-completions=481 anthropic-messages=262 openai-responses=45 google-generative-ai=12 mistral-conversations=21 -->

## 协议状态

| 协议 | 目录模型数 | 状态 | 注册方式 | 说明 |
|---|---:|---|---|---|
| `openai-completions` | 481 | 已实现 | `_ "github.com/ktsoator/or/llm/openai"` | OpenAI Chat Completions 及兼容 endpoint |
| `anthropic-messages` | 262 | 已实现 | `_ "github.com/ktsoator/or/llm/anthropic"` | Anthropic Messages 及兼容 endpoint |
| `openai-responses` | 45 | 仅目录 | 无 | 当前没有 adapter；官方 OpenAI 目录模型使用该协议 |
| `google-generative-ai` | 12 | 仅目录 | 无 | 当前没有 adapter |
| `mistral-conversations` | 21 | 仅目录 | 无 | 当前没有 adapter |

导入 `github.com/ktsoator/or/llm/all` 会注册两个已实现协议。它不会为三个 catalog-only 协议增加实现。

## 目录与运行状态

```go
models := llm.GetModels("openai") // 返回目录中的 openai-responses 模型
for _, model := range models {
	fmt.Println(model.ID, llm.SupportsProtocol(model.Protocol))
}

runnable := llm.GetRunnableModels("openai") // 当前为空
```

`GetModel` 和 `LookupModel` 只验证目录中是否存在条目，不验证 adapter。发送前可调用 `SupportsProtocol`，或从一开始只使用 `GetRunnableModels`。

## Provider 目录

当前目录包含以下 provider ID。模型数量来自 `llm/catalog.generated.json`。

| Provider ID | 模型数 | 协议 | 凭证变量 |
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

凭证变量的权威值来自 `llm/keys.go` 和 `APIKeyEnvVars(provider)`。

## 验证级别

内置测试使用本地 mock server 验证两个协议 adapter。当前材料没有显示对上述每一个线上 provider 执行持续集成测试。因此：

- “已实现协议”表示 adapter 能路由并理解该线格式；
- “目录中存在”不构成线上 provider 兼容保证；
- provider 修改非标准字段时，可能需要更新 `Model.Compatibility` 或使用 `RewriteRequest`；
- 上线前应使用目标 provider 的真实账户完成鉴权、流式、工具、usage 和错误路径测试。

## 模型能力

单个模型的能力以 `Model` 字段为准：

| 能力 | 字段或接口 |
|---|---|
| 图像输入 | `slices.Contains(model.Input, llm.Image)` |
| 推理 | `model.Reasoning`、`SupportedThinkingLevels(model)` |
| 上下文窗口 | `model.ContextWindow` |
| 最大输出 | `model.MaxTokens` |
| 目录价格 | `model.Cost` |
| 当前可路由 | `SupportsProtocol(model.Protocol)` |

目录由外部数据源生成并嵌入二进制。价格、模型状态和限制可能落后于 provider；`Usage.Cost` 是目录价格估算，不是账单。
