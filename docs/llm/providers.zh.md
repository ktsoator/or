# 提供方与模型

本包包含两种协议适配器：

- `openai-completions`
- `anthropic-messages`

目录与兼容层显式配置了以下提供方：

| 提供方 | Provider ID | 协议 | 环境变量 |
|---|---|---|---|
| DeepSeek | `deepseek` | `openai-completions` | `DEEPSEEK_API_KEY` |
| MiniMax Global | `minimax` | `anthropic-messages` | `MINIMAX_API_KEY` |
| MiniMax China | `minimax-cn` | `anthropic-messages` | `MINIMAX_CN_API_KEY` |
| Xiaomi MiMo | `xiaomi` | `openai-completions` | `XIAOMI_API_KEY` 或 `MIMO_API_KEY` |
| Z.AI Global | `zai` | `openai-completions` | `ZAI_API_KEY` |
| Zhipu Coding Plan China | `zai-coding-cn` | `openai-completions` | `ZAI_CODING_CN_API_KEY` |
| Moonshot AI Global | `moonshotai` | `openai-completions` | `MOONSHOT_API_KEY` |
| Moonshot AI China | `moonshotai-cn` | `openai-completions` | `MOONSHOT_API_KEY` |
| Kimi Coding | `kimi-coding` | `anthropic-messages` | `KIMI_API_KEY` |

目录中还包含其他兼容提供方和模型的元数据。这些条目可供查询，并且可能通过两种协议适配器
之一正常工作，但它们尚未全部针对线上提供方 API 验证过，不构成支持保证。自动化测试通过
本地 mock 服务器覆盖两种适配器，而非对每个提供方进行线上集成测试。

本包只读取 `llm.GetModel` 所选提供方的 key。也可以通过 `StreamOptions.APIKey` 或
`StreamOptions.Env` 提供请求级别的凭证。

## 发现模型

与其硬编码动态提供的模型 ID，不如直接查询目录：

```go
for _, provider := range llm.GetProviders() {
	fmt.Println(provider)
	for _, model := range llm.GetModels(provider) {
		fmt.Printf("  %s: %s\n", model.ID, model.Name)
	}
}

model, ok := llm.LookupModel("xiaomi", "mimo-v2-flash")
if !ok {
	log.Fatal("model not found")
}
```

`LookupModel` 返回模型和一个表示是否找到的标志。`GetModel` 适用于已知的目录条目，
在提供方或模型 ID 不存在时会 panic。模型元数据包含推理与图像支持、上下文窗口、输出
上限和定价信息。

## 自定义与兼容端点

任何实现了内置协议之一的端点，都可以通过直接构造一个 `Model` 并设置 `BaseURL` 来使用。
这涵盖 Ollama、vLLM、LM Studio 等本地服务器，以及私有模型网关：

```go
model := llm.Model{
	ID:            "qwen2.5-coder:7b",
	Name:          "Qwen2.5 Coder 7B",
	Provider:      "ollama",
	Protocol:      llm.ProtocolOpenAICompletions,
	BaseURL:       "http://localhost:11434/v1",
	Input:         []llm.ModelInput{llm.Text},
	ContextWindow: 32768,
	MaxTokens:     4096,
}

events, err := llm.Stream(ctx, model, input, llm.StreamOptions{APIKey: "ollama"})
```

端点特定的行为——推理字段名、cache-control 支持以及类似差异——通过 `Model.Compatibility`
配合 `OpenAICompletionsCompatibility` 或 `AnthropicMessagesCompatibility` 配置。

如果某个通信协议既非 OpenAI 兼容也非 Anthropic 兼容，请实现一个
[自定义协议适配器](extending.md)。
