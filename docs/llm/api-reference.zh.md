# API 索引

本页按使用场景整理 `github.com/ktsoator/or/llm` 的公开 API。字段语义和完整 Go 签名以当前源码与 [pkg.go.dev](https://pkg.go.dev/github.com/ktsoator/or/llm) 为准。

使用级别：

- **常用**：应用请求路径通常直接使用；
- **配置**：模型选择、凭证、网关或显式 client；
- **扩展**：自定义 provider 或协议；
- **低层**：adapter、诊断或测试辅助。

## 请求入口

| API | 级别 | 参数 | 返回 | 失败行为 |
|---|---|---|---|---|
| `Complete(ctx, model, input, options)` | 常用 | `context.Context`、`Model`、`Context`、`StreamOptions` | `(AssistantMessage, error)` | 发送前错误返回零消息；流错误可同时返回部分消息和 error |
| `Stream(ctx, model, input, options)` | 常用 | 同上 | `(<-chan Event, error)` | 配置错误立即返回；运行期错误通过 `EventError` 发出 |
| `NewClient(adapters, providers)` | 配置 | `*AdapterRegistry`、`*ProviderRegistry` | `*Client` | `adapters` 为 nil 时请求失败 |
| `(*Client).Complete(...)` | 配置 | 与包级 `Complete` 相同 | `(AssistantMessage, error)` | 使用该 client 自己的注册表 |
| `(*Client).Stream(...)` | 配置 | 与包级 `Stream` 相同 | `(<-chan Event, error)` | 协议未注册时返回错误 |

## 输入构造

| API | 返回 | 说明 |
|---|---|---|
| `Prompt(text)` | `Context` | 一条文本用户消息 |
| `PromptWithSystem(system, user)` | `Context` | system prompt + 一条文本用户消息 |
| `NewContext(messages...)` | `Context` | 按顺序保存传入消息 |
| `UserText(text)` | `*UserMessage` | 文本用户消息 |
| `UserImage(data, mimeType)` | `*UserMessage` | 单张 base64 图像消息 |
| `AssistantText(text)` | `*AssistantMessage` | 用于构造历史或测试的文本 assistant 消息 |
| `ToolResult(callID, toolName, text)` | `*ToolResultMessage` | 文本工具结果，默认 `IsError=false` |
| `NewAssistantMessage(model)` | `AssistantMessage` | 初始化协议、provider、模型和时间戳；adapter 作者使用 |

## 消息模型

### `Context`

| 字段 | 类型 | 说明 |
|---|---|---|
| `SystemPrompt` | `string` | 本次请求的系统提示；不自动写入历史 |
| `Messages` | `[]Message` | 用户、assistant 和工具结果历史 |
| `Tools` | `[]ToolDefinition` | 本次请求可调用的工具 |

`Context` 实现 `MarshalJSON` 和 `UnmarshalJSON`，恢复时会重建具体消息和内容块类型。

### 消息类型

| 类型 | 允许的内容 |
|---|---|
| `UserMessage` | `TextContent`、`ImageContent` |
| `AssistantMessage` | `TextContent`、`ThinkingContent`、`ToolCall` |
| `ToolResultMessage` | `TextContent`、`ImageContent` |

`Message`、`UserContent`、`AssistantContent` 和 `ToolResultContent` 是带未导出标记方法的封闭接口。外部包不能实现新的消息或内容块类型。

### 内容块

| 类型 | 主要字段 | 说明 |
|---|---|---|
| `TextContent` | `Text`、`TextSignature` | 文本和可选 provider 签名 |
| `ImageContent` | `Data`、`MIMEType` | base64 图像 |
| `ThinkingContent` | `Thinking`、`ThinkingSignature`、`Redacted` | 推理文本或加密/隐藏推理 |
| `ToolCall` | `ID`、`Name`、`Arguments`、`ThoughtSignature` | 模型请求执行的工具 |

上述具体消息和内容块实现 JSON marshal/unmarshal 方法。使用 `json.Marshal`/`json.Unmarshal` 即可；无需直接调用这些方法。

### `AssistantMessage`

| 字段 | 说明 |
|---|---|
| `Content` | 文本、thinking 和工具调用块 |
| `Protocol`、`Provider`、`Model` | 请求目标身份 |
| `ResponseModel`、`ResponseID` | provider 返回的模型名和响应 ID |
| `Usage` | token 和成本 |
| `StopReason` | 停止原因 |
| `ErrorMessage` | 失败详情 |
| `Diagnostics` | 已恢复的非致命问题 |
| `Timestamp` | 创建消息时的 Unix 毫秒时间 |

方法：

| 方法 | 返回 | 说明 |
|---|---|---|
| `Text()` | `string` | 按顺序拼接全部文本块 |
| `ToolCalls()` | `[]ToolCall` | 按顺序返回工具调用 |
| `MarshalJSON()` | `([]byte, error)` | 自描述 JSON |
| `UnmarshalJSON(data)` | `error` | 恢复内容块具体类型 |

## 消息序列化与转换

| API | 说明 |
|---|---|
| `MarshalMessage(message)` | 将一个 `Message` 编码为带角色和内容判别字段的 JSON |
| `UnmarshalMessage(data)` | 将单条 JSON 恢复为具体消息类型 |
| `TransformMessages(messages, model, normalizer)` | 为目标模型转换历史，不修改调用方的原始切片 |

`Stream` 和 `Complete` 的内置 adapter 会自动调用 `TransformMessages`。普通请求代码不需要手动调用。

## 流式事件

### `EventType`

| 常量 | 主要字段 |
|---|---|
| `EventStart` | `Partial` |
| `EventTextStart` | `ContentIndex`、`Partial` |
| `EventTextDelta` | `ContentIndex`、`Delta`、`Partial` |
| `EventTextEnd` | `ContentIndex`、`Content`、`Partial` |
| `EventThinkingStart` | `ContentIndex`、`Partial` |
| `EventThinkingDelta` | `ContentIndex`、`Delta`、`Partial` |
| `EventThinkingEnd` | `ContentIndex`、`Content`、`Partial` |
| `EventToolCallStart` | `ContentIndex`、`ToolCall`、`Partial` |
| `EventToolCallDelta` | `ContentIndex`、`Delta`、`ToolCall`、`Partial` |
| `EventToolCallEnd` | `ContentIndex`、`ToolCall`、`Partial` |
| `EventDone` | `Message` |
| `EventError` | `Message`、`Err` |

事件通道必须持续读取到关闭。工具只应在收到 `EventDone` 后执行。

### `StreamWriter`

adapter 作者使用 `NewStreamWriter(ctx, events, output)` 创建 writer：

| 方法 | 说明 |
|---|---|
| `Start()` | 幂等发送 `EventStart` |
| `Emit(event)` | 发送非终止事件并附加 `Partial` 快照 |
| `Done()` | 发送唯一 `EventDone` |
| `Fail(err)` | 发送唯一 `EventError` |

辅助函数 `CloneToolCall` 深拷贝工具参数 map。

## 请求配置

### `StreamOptions`

| 字段 | 类型 | 默认行为 |
|---|---|---|
| `APIKey` | `string` | 空值时从 provider override 或环境解析 |
| `Env` | `ProviderEnv` | nil；请求级环境覆盖 |
| `Temperature` | `*float64` | nil；不覆盖 provider 默认值 |
| `MaxTokens` | `int64` | 0；OpenAI 省略，Anthropic 回退到 `Model.MaxTokens` |
| `Headers` | `map[string]string` | nil；覆盖模型/provider 同名 header |
| `Reasoning` | `ModelThinkingLevel` | 空；使用模型/provider 默认值 |
| `ProtocolOptions` | `ProtocolStreamOptions` | nil |
| `MaxRetries` | `*int` | nil；使用 SDK 默认重试次数 |
| `Timeout` | `time.Duration` | 0；使用 SDK 默认请求超时 |
| `OnRequest` | callback | nil；每次尝试序列化后调用 |
| `RewriteRequest` | callback | nil；每次尝试发送前调用 |
| `OnResponse` | callback | nil；每次 HTTP 响应调用 |

`StreamOptions.Validate(protocol, tools)` 校验协议特定选项。`Client.Stream` 会自动调用。

### 协议特定选项

`ProtocolStreamOptions` 要求：

```go
Protocol() Protocol
Validate(tools []ToolDefinition) error
```

内置类型：

| 类型 | 字段 |
|---|---|
| `OpenAICompletionsStreamOptions` | `ToolChoice OpenAIToolChoice` |
| `AnthropicStreamOptions` | `ThinkingDisplay`、`ToolChoice AnthropicToolChoice` |

工具选择常量和类型：

- `OpenAIToolChoiceAuto`、`OpenAIToolChoiceNone`、`OpenAIToolChoiceRequired`；
- `OpenAIToolChoiceFunction{Name: ...}`；
- `AnthropicToolChoiceAuto`、`AnthropicToolChoiceAny`、`AnthropicToolChoiceNone`；
- `AnthropicToolChoiceTool{Name: ...}`。

`OpenAIToolChoice`、`OpenAIToolChoiceMode`、`AnthropicToolChoice` 和 `AnthropicToolChoiceMode` 表示这些封闭联合类型。

## 工具

| API | 参数 | 返回 | 失败行为 |
|---|---|---|---|
| `NewTool[T](name, description)` | 工具名、描述；`T` 为参数结构体 | `(ToolDefinition, error)` | 名称或 Schema 无效时返回 error |
| `MustTool[T](name, description)` | 同上 | `ToolDefinition` | 无效时 panic；适合启动期静态声明 |
| `DecodeToolCall[T](tool, call)` | 定义和模型调用 | `(T, error)` | Schema 校验或 JSON 解码失败 |
| `ValidateToolCall(tools, call)` | 工具列表和调用 | `(map[string]any, error)` | 名称不存在或参数无效 |
| `ValidateToolArguments(tool, call)` | 已知工具和调用 | `(map[string]any, error)` | 参数违反 Schema |
| `ParseToolArguments(raw)` | 原始 JSON 字符串 | `map[string]any` | 失败时返回尽力恢复值或空 map |
| `ParseToolArgumentsMode(raw)` | 原始 JSON 字符串 | `(map[string]any, ArgumentsMode)` | 不返回 error；mode 描述恢复程度 |
| `ToolArgumentsDiagnostic(id, name, mode)` | 调用身份和解析模式 | `(Diagnostic, bool)` | `strict` 时 bool 为 false |

`ArgumentsMode` 常量：`ArgumentsStrict`、`ArgumentsRepaired`、`ArgumentsPartial`、`ArgumentsInvalid`。

## 模型

### 目录函数

| API | 说明 |
|---|---|
| `LookupModel(provider, modelID)` | 返回 `(Model, bool)`；适合动态输入 |
| `GetModel(provider, modelID)` | 返回 `Model`；未知条目时 panic |
| `GetProviders()` | 返回内置目录 provider ID |
| `GetModels(provider)` | 返回 provider 的全部目录模型，包括未实现协议 |
| `GetRunnableModels(provider)` | 仅返回默认 adapter 注册表可路由的模型 |
| `SupportsProtocol(protocol)` | 默认 adapter 注册表是否包含该协议 |

### `Model`

关键字段：`ID`、`Name`、`Provider`、`Protocol`、`BaseURL`、`Headers`、`Reasoning`、`ThinkingLevelMap`、`Input`、`ContextWindow`、`MaxTokens`、`Cost`、`Compatibility`。

`Model.UnmarshalJSON` 按 `Protocol` 恢复具体 compatibility 类型。当前只支持对带 compatibility 的 OpenAI Completions 和 Anthropic Messages 模型解码。

### 推理、输入和成本

| 类型或 API | 说明 |
|---|---|
| `ModelInput` | 输入模态；常量 `Text`、`Image` |
| `ModelThinkingLevel` | `Off`、`Minimal`、`Low`、`Medium`、`High`、`XHigh` |
| `ThinkingDisplay` | `ThinkingDisplaySummarized`、`ThinkingDisplayOmitted` |
| `SupportedThinkingLevels(model)` | 返回模型接受的中立推理等级 |
| `ClampThinkingLevel(model, level)` | 将请求贴合到最近支持等级 |
| `CalculateCost(model, usage)` | 按每百万 token 目录价格计算 `UsageCost` |

### `ModelRegistry`

| API | 说明 |
|---|---|
| `NewModelRegistry()` | 创建空注册表 |
| `Register(model)` | 校验并新增或替换模型 |
| `Get(provider, modelID)` | 返回模型防御性拷贝 |
| `Providers()` | 排序返回 provider ID |
| `Models(provider)` | 排序返回模型 |

## Provider 与凭证

### 环境变量辅助

| API | 说明 |
|---|---|
| `APIKeyEnvVars(provider)` | 返回按优先级检查的变量名 |
| `FindEnvAPIKeys(provider)` | 返回进程环境中已配置的变量名 |
| `FindEnvAPIKeysWithEnv(provider, env)` | 同时考虑请求级 `ProviderEnv` |
| `GetEnvAPIKey(provider)` | 返回第一个可用 key |
| `GetEnvAPIKeyWithEnv(provider, env)` | 请求级环境优先 |
| `MissingAPIKeyError(provider)` | 构造包含 provider 和变量名的错误 |

### Provider 类型

`ProviderSpec` 字段：`ID`、`Name`、`EnvKeys`、`Models`、`Headers`。

`NewSpecProvider(spec)` 返回输入配置的独立快照。`Provider` 提供：

| 方法 | 返回 |
|---|---|
| `ID()` | `string` |
| `Name()` | `string` |
| `Models()` | `[]Model` 防御性拷贝 |
| `EnvKeys()` | `[]string` 防御性拷贝 |

`ProviderOverride` 字段：`BaseURL`、`APIKey`、`Headers`、`Env`。

`AuthStatus` 字段：`Configured`、`Source`、`Label`、`Missing`。

### `ProviderRegistry`

| API | 说明 |
|---|---|
| `NewProviderRegistry()` | 空注册表 |
| `NewBuiltInProviderRegistry()` | 从目录构造内置 provider |
| `DefaultProviderRegistry()` | 包级 client 使用的实例 |
| `Register(provider)` | 新增或替换 provider |
| `Get(providerID)` | 查询 provider |
| `Providers()` | 排序返回 provider |
| `SetOverride(providerID, override)` | 保存 override 快照 |
| `ClearOverride(providerID)` | 删除 override |
| `ResolveRequest(model, options)` | 应用凭证、URL 和 headers 优先级 |
| `AuthStatus(providerID, env)` | 返回凭证状态和存在标志 |

## 协议与兼容配置

协议常量：

- `ProtocolOpenAICompletions`
- `ProtocolAnthropicMessages`

`ModelCompatibility` 要求 `Protocol() Protocol`；具体类型：

### `OpenAICompletionsCompatibility`

字段：`SupportsStore`、`SupportsDeveloperRole`、`SupportsReasoningEffort`、`MaxTokensField`、`SupportsStrictMode`、`RequiresReasoningContentOnAssistantMessages`、`RequiresThinkingAsText`、`ThinkingFormat`、`ZAIToolStream`。

### `AnthropicMessagesCompatibility`

字段：`SupportsTemperature`、`SupportsCacheControl`、`SupportsCacheControlTools`、`ForceAdaptiveThinking`、`AllowEmptySignature`。

除 `MaxTokensField` 和 `ThinkingFormat` 外，兼容开关使用指针区分“未设置”和显式 false。未设置时由 adapter 的默认兼容检测决定。

## 结果、错误与诊断

### Stop reason

`StopReason` 常量：

- `StopReasonStop`
- `StopReasonLength`
- `StopReasonToolUse`
- `StopReasonError`
- `StopReasonAborted`

### Usage

`Usage` 字段：`Input`、`Output`、`CacheRead`、`CacheWrite`、`TotalTokens`、`Cost`。

`UsageCost` 和 `ModelCost` 都按输入、输出、缓存读、缓存写分类；`UsageCost` 另有 `Total`。

### Overflow 和诊断

| API | 说明 |
|---|---|
| `IsContextOverflow(message, contextWindow)` | 判断错误或用量是否表示上下文溢出 |
| `OverflowPatterns()` | 返回内部匹配规则的防御性拷贝 |
| `Diagnostic` | `Type`、`Timestamp`、`Message`、`Details` |
| `DiagnosticToolArgumentsRecovered` | 工具参数恢复诊断类型 |

## Adapter 扩展

`ProtocolAdapter`：

```go
type ProtocolAdapter interface {
	Protocol() Protocol
	Stream(context.Context, Model, Context, StreamOptions) (<-chan Event, error)
}
```

注册接口：

| API | 说明 |
|---|---|
| `NewAdapterRegistry()` | 创建空 adapter 注册表 |
| `(*AdapterRegistry).Register(adapter)` | 新增或替换相同协议 adapter |
| `(*AdapterRegistry).Get(protocol)` | 查询 adapter |
| `Register(adapter)` | 注册到包级默认 adapter 注册表 |

内置子包：

- `openai.NewAdapter(httpClient)`：OpenAI Chat Completions adapter；
- `anthropic.NewAdapter(httpClient)`：Anthropic Messages adapter；
- `llm/all`：通过副作用导入全部内置 adapter。
