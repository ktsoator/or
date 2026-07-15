# API 索引

本页用于按模块查找 `github.com/ktsoator/or/llm` 的公开符号，不重复维护字段默认值、事件表或行为规则。完整 Go 声明以当前源码与 [pkg.go.dev](https://pkg.go.dev/github.com/ktsoator/or/llm) 为准；每组符号的语义由表格中的参考页维护。

## 请求入口

| API | 返回 | 用途 |
|---|---|---|
| `Complete(ctx, model, input, options)` | `(AssistantMessage, error)` | 收集整个响应并返回最终或部分消息 |
| `Stream(ctx, model, input, options)` | `(<-chan Event, error)` | 在生成过程中接收统一事件 |
| `NewClient(adapters, providers)` | `*Client` | 使用显式注册表创建独立 Client |
| `(*Client).Complete(...)` | `(AssistantMessage, error)` | 通过指定 Client 完成请求 |
| `(*Client).Stream(...)` | `(<-chan Event, error)` | 通过指定 Client 发起流式请求 |

发送前失败与流式失败的区别见[失败信号](errors.md)。最短调用路径见[快速开始](getting-started.md)。

## 输入、消息与历史

| 分类 | 公开符号 | 参考页 |
|---|---|---|
| Context 构造 | `Prompt`、`PromptWithSystem`、`NewContext` | [消息与上下文](conversations.md#构建消息) |
| 消息构造 | `UserText`、`UserImage`、`AssistantText`、`ToolResult` | [消息与上下文](conversations.md#构建消息) |
| 消息接口 | `Message`、`UserMessage`、`AssistantMessage`、`ToolResultMessage` | [消息与上下文](conversations.md#消息与内容模型) |
| 内容块 | `TextContent`、`ImageContent`、`ThinkingContent`、`ToolCall` | [消息与上下文](conversations.md#消息与内容模型) |
| 响应访问 | `AssistantMessage.Text`、`AssistantMessage.ToolCalls` | [响应与用量](results.md#内容与元数据) |
| 序列化 | `MarshalMessage`、`UnmarshalMessage` | [消息与上下文](conversations.md#json-序列化) |
| 历史转换 | `TransformMessages` | [消息与上下文](conversations.md#历史与模型转换) |

`NewAssistantMessage` 供 adapter 初始化输出消息。`Context` 及具体消息类型实现 JSON marshal/unmarshal。

## 流式接口

| 分类 | 公开符号 | 参考页 |
|---|---|---|
| 事件 | `Event`、`EventType`、`EventStart`、文本/推理/工具调用事件、`EventDone`、`EventError` | [流式事件](streaming.md#事件参考) |
| Adapter 写入 | `StreamWriter`、`NewStreamWriter` | [自定义协议](extending.md) |
| 工具调用复制 | `CloneToolCall` | [流式事件](streaming.md#工具调用增量与诊断) |

事件的有效字段、顺序和终止规则只在[流式事件](streaming.md)中维护。

## 请求选项

| 分类 | 公开符号 | 参考页 |
|---|---|---|
| 通用选项 | `StreamOptions`、`StreamOptions.Validate` | [请求选项](configuration.md) |
| 协议选项接口 | `ProtocolStreamOptions` | [请求选项](configuration.md) |
| OpenAI 选项 | `OpenAICompletionsStreamOptions`、`OpenAIToolChoice` 相关类型与常量 | [工具定义与调用](tools.md#协议特定的工具选择) |
| Anthropic 选项 | `AnthropicStreamOptions`、`AnthropicToolChoice` 相关类型与常量 | [工具定义与调用](tools.md#协议特定的工具选择) |
| 推理显示 | `ThinkingDisplay`、`ThinkingDisplaySummarized`、`ThinkingDisplayOmitted` | [推理配置](reasoning.md#anthropic-思考显示) |

字段默认值、凭证优先级、Hook 与请求改写规则见[请求选项](configuration.md)。

## 工具

| 分类 | 公开符号 | 参考页 |
|---|---|---|
| 定义 | `ToolDefinition`、`NewTool[T]`、`MustTool[T]` | [工具定义与调用](tools.md#类型化工具) |
| 读取与解码 | `ToolCall`、`DecodeToolCall[T]` | [工具定义与调用](tools.md#执行前校验) |
| 通用校验 | `ValidateToolCall`、`ValidateToolArguments` | [工具定义与调用](tools.md#执行前校验) |
| 尽力解析 | `ParseToolArguments`、`ParseToolArgumentsMode`、`ArgumentsMode` 相关常量 | [工具定义与调用](tools.md#执行前校验) |
| 诊断 | `ToolArgumentsDiagnostic`、`DiagnosticToolArgumentsRecovered` | [响应与用量](results.md#诊断) |

工具执行、授权和循环上限不属于这些 API；完整流程见[执行工具调用](recipes/tool-loop.md)。

## 模型与 Provider

| 分类 | 公开符号 | 参考页 |
|---|---|---|
| 内置模型清单 | `LookupModel`、`GetModel`、`GetProviders`、`GetModels`、`GetRunnableModels`、`SupportsProtocol` | [模型与提供方](providers.md#发现模型) |
| 模型能力 | `Model`、`ModelInput`、`ModelThinkingLevel`、`SupportedThinkingLevels`、`ClampThinkingLevel` | [模型与提供方](providers.md#模型元数据)、[推理配置](reasoning.md) |
| 成本估算 | `ModelCost`、`CalculateCost` | [响应与用量](results.md#token-用量与成本) |
| 模型注册表 | `ModelRegistry`、`NewModelRegistry` | [Client 与注册表](clients-and-registries.md#modelregistry) |
| Provider 定义 | `Provider`、`ProviderSpec`、`NewSpecProvider` | [模型与提供方](providers.md#注册自定义-provider) |
| Provider 覆盖 | `ProviderOverride`、`AuthStatus` | [模型与提供方](providers.md#提供方配置与状态) |
| Provider 注册表 | `ProviderRegistry`、`NewProviderRegistry`、`NewBuiltInProviderRegistry`、`DefaultProviderRegistry` | [Client 与注册表](clients-and-registries.md#providerregistry) |
| 凭证辅助 | `APIKeyEnvVars`、`FindEnvAPIKeys`、`FindEnvAPIKeysWithEnv`、`GetEnvAPIKey`、`GetEnvAPIKeyWithEnv`、`MissingAPIKeyError` | [请求选项](configuration.md#按请求提供凭证) |

模型字段、兼容配置和 provider override 行为只在[模型与提供方](providers.md)中维护。线上可用性边界见[协议与提供方状态](support-matrix.md)。

## 协议与注册表

| 分类 | 公开符号 | 参考页 |
|---|---|---|
| 协议 | `Protocol`、`ProtocolOpenAICompletions`、`ProtocolAnthropicMessages` | [协议与提供方状态](support-matrix.md) |
| 兼容配置 | `ModelCompatibility`、`OpenAICompletionsCompatibility`、`AnthropicMessagesCompatibility` | [模型与提供方](providers.md#自定义与兼容端点) |
| Adapter | `ProtocolAdapter` | [自定义协议](extending.md) |
| Adapter 注册表 | `AdapterRegistry`、`NewAdapterRegistry`、`Register` | [Client 与注册表](clients-and-registries.md) |
| 内置 Adapter | `openai.NewAdapter`、`anthropic.NewAdapter`、`llm/all` | [快速开始](getting-started.md#注册协议适配器) |

## 结果、错误与诊断

| 分类 | 公开符号 | 参考页 |
|---|---|---|
| 停止原因 | `StopReason` 及 `StopReasonStop`、`Length`、`ToolUse`、`Error`、`Aborted` | [响应与用量](results.md#停止原因) |
| Token 与成本 | `Usage`、`UsageCost`、`ModelCost` | [响应与用量](results.md#token-用量与成本) |
| 上下文溢出 | `IsContextOverflow`、`OverflowPatterns` | [响应与用量](results.md#检测上下文溢出) |
| 诊断 | `Diagnostic`、`DiagnosticToolArgumentsRecovered` | [响应与用量](results.md#诊断) |

失败发生阶段、返回的 `error`、`EventError` 和失败消息之间的关系见[失败信号](errors.md)。
