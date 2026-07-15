# 提供方与模型

本包包含两种协议适配器：

- `openai-completions`
- `anthropic-messages`

模型目录还包含三个没有 adapter 的 catalog-only 协议。官方 OpenAI 目录模型当前使用 `openai-responses`，不能通过内置 adapter 调用。完整数量、provider ID 和凭证变量见[协议与 Provider 支持矩阵](support-matrix.md)。

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

目录中还包含其他兼容提供方和模型的元数据。这些条目可供查询，并且可能通过两种协议适配器之一正常工作，但它们尚未全部针对线上提供方 API 验证过，不构成支持保证。自动化测试通过本地 mock 服务器覆盖两种适配器，而非对每个提供方进行线上集成测试。

本包只读取 `llm.GetModel` 所选提供方的 key。也可以通过 `StreamOptions.APIKey` 或 `StreamOptions.Env` 提供请求级别的凭证。

## 发现模型

与其硬编码动态提供的模型 ID，不如直接查询目录：

```go
for _, provider := range llm.GetProviders() {
	fmt.Println(provider)
	for _, model := range llm.GetModels(provider) {
		fmt.Printf("  %s: %s\n", model.ID, model.Name)
	}
}

model, ok := llm.LookupModel("xiaomi", "mimo-v2.5")
if !ok {
	log.Fatal("model not found")
}
```

`LookupModel` 返回模型和一个表示是否找到的标志。`GetModel` 适用于已知的目录条目，在提供方或模型 ID 不存在时会 panic。

目录也包含尚无内置适配器的协议模型。可用 `SupportsProtocol` 检查单个协议，或用
`GetRunnableModels` 只列出当前应用已导入适配器、因而能够实际调用的模型：

```go
if !llm.SupportsProtocol(model.Protocol) {
	log.Fatalf("协议 %q 尚未注册", model.Protocol)
}

for _, model := range llm.GetRunnableModels("deepseek") {
	fmt.Println(model.ID)
}
```

这两个函数查询包级默认 adapter 注册表。使用前应以副作用方式导入对应协议包，或导入
`llm/all`。`GetModels` 仍然返回未经筛选的完整目录。

## 模型元数据

`Model` 同时是一份只读的元数据记录。可在请求前读取它来驱动 UI、施加限制或估算成本：

| 字段 | 类型 | 含义 |
|---|---|---|
| `ID` | `string` | 发送给提供方的标识符 |
| `Name` | `string` | 可读的展示名 |
| `Provider` | `string` | 厂商键，如 `anthropic` |
| `Protocol` | `Protocol` | 由哪个适配器处理 |
| `BaseURL` | `string` | 端点基础 URL |
| `Headers` | `map[string]string` | 合并进每次请求的默认请求头 |
| `Reasoning` | `bool` | 模型能否产生思考内容 |
| `Input` | `[]ModelInput` | 接受的模态：`Text`、`Image` |
| `ContextWindow` | `int64` | 最大总 token 数（输入 + 输出） |
| `MaxTokens` | `int64` | 模型可生成的最大 token 数 |
| `Cost` | `ModelCost` | 每百万 token 的定价 |
| `Compatibility` | `ModelCompatibility` | 协议特定的覆盖项（见下文） |

`Reasoning` 只表明是否支持思考；要读取模型实际接受的精确等级，请用[`SupportedThinkingLevels`](reasoning.md)，而不是直接读 `ThinkingLevelMap`。

`Cost` 中的价格是**每百万 token** 的单价，与 `CalculateCost` 的计费方式一致：

| 字段 | 含义 |
|---|---|
| `Input` | 每百万输入 token 的价格 |
| `Output` | 每百万输出 token 的价格 |
| `CacheRead` | 每百万缓存读取 token 的价格 |
| `CacheWrite` | 每百万缓存写入 token 的价格 |

```go
model, _ := llm.LookupModel("deepseek", "deepseek-v4-flash")
fmt.Printf("%s: %d-token window, $%.2f/M in, $%.2f/M out\n",
model.Name, model.ContextWindow, model.Cost.Input, model.Cost.Output)
```

目录由外部数据源生成并嵌入二进制。价格、模型状态和限制可能晚于 provider 更新。`CalculateCost` 和 `Usage.Cost` 是目录估算，不是 provider 账单。

已完成请求上对应的 `Usage` 与 `UsageCost` 记录参见[读取响应](results.md)。

## 自定义与兼容端点

任何实现了内置协议之一的端点，都可以通过直接构造一个 `Model` 并设置 `BaseURL` 来使用。这涵盖 Ollama、vLLM、LM Studio 等本地服务器，以及私有模型网关：

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

端点特定的行为（推理字段名、cache-control 支持以及类似差异）通过 `Model.Compatibility` 配合 `OpenAICompletionsCompatibility` 或 `AnthropicMessagesCompatibility` 配置。只需设置与默认不同的字段；每个字段都是指针，未设置时保持适配器原有行为不变。

```go
supports := func(b bool) *bool { return &b }

// OpenAI 兼容端点:其上限字段名为 "max_completion_tokens",并接受推理强度字段。
model.Compatibility = &llm.OpenAICompletionsCompatibility{
	MaxTokensField:          "max_completion_tokens",
	SupportsReasoningEffort: supports(true),
}

// Anthropic 兼容端点:不支持 cache control。
model.Compatibility = &llm.AnthropicMessagesCompatibility{
	SupportsCacheControl: supports(false),
}
```

OpenAI Completions compatibility 字段：

| 字段 | 类型 | 作用 |
|---|---|---|
| `SupportsStore` | `*bool` | 是否发送 `store=false` |
| `SupportsDeveloperRole` | `*bool` | reasoning 模型是否可使用 developer role |
| `SupportsReasoningEffort` | `*bool` | 是否发送标准 reasoning effort 字段 |
| `MaxTokensField` | `string` | `max_tokens` 或 `max_completion_tokens` |
| `SupportsStrictMode` | `*bool` | 工具定义是否发送 strict mode |
| `RequiresReasoningContentOnAssistantMessages` | `*bool` | 回放 assistant 时是否要求 reasoning 字段 |
| `RequiresThinkingAsText` | `*bool` | 是否把 thinking 作为普通文本回放 |
| `ThinkingFormat` | `string` | adapter 识别的 provider reasoning 方言 |
| `ZAIToolStream` | `*bool` | 是否添加 Z.AI `tool_stream` 字段 |

Anthropic Messages compatibility 字段：

| 字段 | 类型 | 作用 |
|---|---|---|
| `SupportsTemperature` | `*bool` | 是否允许 temperature |
| `SupportsCacheControl` | `*bool` | 是否支持消息 cache control |
| `SupportsCacheControlTools` | `*bool` | 是否支持工具 cache control |
| `ForceAdaptiveThinking` | `*bool` | 是否强制使用 adaptive thinking |
| `AllowEmptySignature` | `*bool` | 是否允许回放空 thinking signature |

指针布尔值用于区分“未配置”和显式 false。`ThinkingFormat` 的可用字符串由当前 adapter 实现决定；材料没有定义可供任意 provider 使用的开放枚举。

如果某个通信协议既非 OpenAI 兼容也非 Anthropic 兼容，请实现一个[自定义协议适配器](extending.md)。

## 提供方配置与状态

本包在模型目录之外维护一个 provider 注册表。目录存放 provider 的模型，注册表存放它的配置：提供 key 的环境变量，以及施加到其请求上的 override。包级 `Stream` 和 `Complete` 都经过默认注册表，因此无需自建 client，状态查询和 override 即可生效。

### 检查 provider 是否已配置

`AuthStatus` 无需发送请求，即可报告是否能解析出 key 以及来源。

```go
registry := llm.DefaultProviderRegistry()

status, ok := registry.AuthStatus("deepseek", nil)
if ok && !status.Configured {
	fmt.Printf("%s 未配置；请设置 %v 之一\n", status.Label, status.Missing)
}
// 已配置的 provider 会报告来源，例如 "env:DEEPSEEK_API_KEY"。
```

### 为 provider 的请求改道

`SetOverride` 为发往某个 provider 的每个请求设置 base URL、API key 或 headers，这样接入代理或网关就不必逐个改 `Model`。

```go
proxy := "https://proxy.example.com/deepseek/v1"
registry.SetOverride("deepseek", llm.ProviderOverride{
	BaseURL: &proxy,
	Headers: map[string]string{"X-Team": "infra"},
})
// 此后所有 deepseek 模型都经代理流式请求。
```

`SetOverride` 会保存一份独立快照，因此调用后可以安全复用或修改传入的值、map
和请求级环境变量。条件允许时，override 仍建议在启动阶段设置。完整的凭证优先级见[请求配置](configuration.md)。

### 注册自定义 provider

`Register` 加入目录未内置的 provider。它从自己的环境变量解析 key，也能像内置 provider 一样被 override；这是除了直接传一个裸 `Model` 之外，接入本地服务器的另一种方式。

```go
registry.Register(llm.NewSpecProvider(llm.ProviderSpec{
	ID:      "local",
	Name:    "Local LLM",
	EnvKeys: []string{"LOCAL_API_KEY"},
	Models: []llm.Model{{
		ID:       "qwen2.5-coder:7b",
		Provider: "local",
		Protocol: llm.ProtocolOpenAICompletions,
		BaseURL:  "http://localhost:11434/v1",
		Input:    []llm.ModelInput{llm.Text},
	}},
}))
```

`NewSpecProvider` 会从传入数据（包括模型配置）创建一份独立快照。若 provider 在请求时需要额外逻辑，例如 OAuth 刷新，spec 类型暂不支持。

注册表还提供 `Get`、`Providers`、`ClearOverride` 和 `ResolveRequest`。显式 client、三类注册表的区别及并发语义见 [Client 与注册表](clients-and-registries.md)。
