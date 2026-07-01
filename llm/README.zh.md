# `llm` — 源码导览

[English](README.md) | 简体中文

面向大语言模型的、与厂商无关的统一 API。一套类型同时支持两种 wire 协议
（OpenAI Chat Completions 与 Anthropic Messages）；同一段对话可以发给任一协议下的
任意模型，每次请求都会重新适配。本包是一个无状态的翻译层——只负责决定「发什么」以及
「如何解读流式响应」，把历史存储、上下文压缩和工具循环交给调用方。

本文为阅读或扩展本包的人梳理源码结构。用法请看
[pkg.go.dev 上的包文档](https://pkg.go.dev/github.com/ktsoator/or/llm)（即 [`doc.go`](doc.go)
中的 godoc）与[使用指南](../docs/llm/README.zh.md)。

## 五大块

本包大致分为五层，下面按彼此依赖的顺序排列。自上而下阅读也是一条合理的源码路径。

### 1. 领域类型 —— 词汇表

先搞清楚「一段对话、一个模型、一个流式事件」到底是什么。其余一切都建立在这些类型之上，
从这里开始。

| 文件 | 内容 |
|---|---|
| [`message.go`](message.go) | `Message`、`UserMessage`/`AssistantMessage`/`ToolResultMessage`、内容块（`TextContent`、`ThinkingContent`、`ImageContent`、`ToolCall`）、`Context`、`ToolDefinition`、`Usage`、`StopReason` |
| [`model.go`](model.go) | `Model`、`Protocol`、`ModelThinkingLevel`、`ModelCost`、各协议的兼容性配置，以及单模型操作（`CalculateCost`、`SupportedThinkingLevels`、`ClampThinkingLevel`） |
| [`events.go`](events.go) | `Event` 与 `EventType`——流的基本单位 |

### 2. 入口与派发 —— 请求如何跑起来

从一次调用到 provider adapter 的路径。**对外入口在这里。**

| 文件 | 内容 |
|---|---|
| [`default.go`](default.go) | 包级 `Stream`/`Complete`/`Register`，基于默认 client；说明了「import 触发 init 注册」的用法。**建议从这里开始读。** |
| [`client.go`](client.go) | `Client.Stream`/`Complete`：校验选项、选中 adapter、注入 API key、消费流 |
| [`adapters.go`](adapters.go) | `ProtocolAdapter`（provider 实现的扩展点）与 `AdapterRegistry` |
| [`options.go`](options.go) | `StreamOptions`、协议特化扩展（`AnthropicStreamOptions`、`OpenAICompletionsStreamOptions`）、各家原生 tool-choice 类型及其校验 |

### 3. 模型目录

内置模型从哪里来。

| 文件 | 内容 |
|---|---|
| [`model_registry.go`](model_registry.go) | `ModelRegistry` 及包级 `LookupModel`/`GetModel`/`GetProviders`/`GetModels` |
| [`catalog.go`](catalog.go) | 通过 `//go:embed` 嵌入生成的目录，以及 `go:generate` 指令（数据由 [`internal/genmodels`](internal/genmodels) 生成） |

### 4. 工具调用

一次 tool call 的完整生命周期，从定义到校验后的参数。

| 文件 | 内容 |
|---|---|
| [`tools.go`](tools.go) | `NewTool`/`MustTool`（从 Go 结构体推导 JSON Schema）与 `DecodeToolCall` |
| [`jsonparse.go`](jsonparse.go) | 尽力解析模型流式吐出的参数 JSON（`ParseToolArguments`、`ArgumentsMode`） |
| [`validation.go`](validation.go) | `ValidateToolCall`/`ValidateToolArguments`——薄薄的校验入口 |
| [`jsonschema.go`](jsonschema.go) | 承担校验重活的通用 JSON-Schema 强制转换 + 校验引擎 |
| [`diagnostics.go`](diagnostics.go) | `Diagnostic` 与 `ToolArgumentsDiagnostic`——参数被修复（而非干净解析）时记录 |

### 5. 编解码与辅助 —— 按需阅读

支撑性机制；理解主干流程时都用不到。

| 文件 | 内容 |
|---|---|
| [`message_json.go`](message_json.go) | 所有消息与内容类型的 JSON 编解码（文件大，但职责单一） |
| [`transform.go`](transform.go) | `TransformMessages`：为目标模型适配已存历史——降级不支持的图片、跨模型协调 reasoning、规整 tool-call ID、修复无应答的 tool call |
| [`stream.go`](stream.go) | `StreamWriter`：adapter 用来发射事件的共享机制，保证单一终止事件 |
| [`prompt.go`](prompt.go) | `Prompt`/`UserText`/`ToolResult` 等便捷构造器 |
| [`keys.go`](keys.go) | 从 provider 环境变量查找 API key |
| [`overflow.go`](overflow.go) | `IsContextOverflow` 上下文窗口溢出检测 |
| [`jsonhelpers.go`](jsonhelpers.go) | JSON 深拷贝与 `isJSONNull` |

## 请求流程

```
llm.Stream / llm.Complete            (default.go — 包级门面)
        │
        ▼
Client.Stream                        (client.go)
        │  校验 StreamOptions、解析 API key
        ▼
AdapterRegistry.Get(model.Protocol)  (adapters.go)
        │
        ▼
ProtocolAdapter.Stream               (llm/openai, llm/anthropic)
        │  TransformMessages → 序列化 → HTTP → 解析 SSE
        ▼
StreamWriter 发射 []Event            (stream.go)
        │
        ▼
Complete 消费至 EventDone / EventError → AssistantMessage
```

`Complete` 只是 `Stream` 之上的薄消费层：把事件读完，返回最终消息，或 `EventError`
携带的错误。

## 最短理解路径

`doc.go` → `message.go` + `model.go` → `default.go` → `client.go` →
`adapters.go`，然后读一个 provider（`openai/`）看协议是怎么落地的。第 1–2 块覆盖主干，
第 3–5 块按需展开。

## 子包

| 包 | 作用 |
|---|---|
| [`openai/`](openai) | OpenAI Chat Completions adapter；import 时自注册 |
| [`anthropic/`](anthropic) | Anthropic Messages adapter；import 时自注册 |
| [`all/`](all) | 空白导入两个 provider，一次注册所有内置协议 |
| [`internal/jsonx`](internal/jsonx) | `jsonparse.go` 使用的宽松/部分 JSON 解析 |
| [`internal/genmodels`](internal/genmodels) | `catalog.generated.json` 的生成器 |

一个 provider 包实现 `ProtocolAdapter`，把中立的 `Message`/`StreamOptions` 翻译成自家
wire 格式，并在 `init` 函数里调用 `Register`。要新增一种真正不同的 wire 协议，就实现该接口
并注册——参见[扩展指南](../docs/llm/extending.zh.md)。
