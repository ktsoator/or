# 工具定义与调用

工具是一类经声明后可供模型请求调用的函数——用于获取数据、执行计算，或完成模型自身无法完成的操作。本库本身不执行任何操作：它将 Go 类型转换为模型可见的 schema，把模型发起的调用交回调用方，再由调用方将结果回传。

一次完整的往返如下：**模型发起工具调用 → 解码并执行 → 将结果回传 → 模型在上下文中带着该结果继续。**

举例来说，模型要查天气时不会自己去查，而是发出一个 `get_weather(city=…)` 调用；调用方查到结果后回传，模型再据此作答。

本页说明工具类型、Schema 生成、参数校验和协议选项。完整的“请求 → 执行 → 回传”程序及生产环境策略见[执行工具调用](recipes/tool-loop.md)。

## 速览

| 任务 | API |
|---|---|
| 从结构体定义工具 | `NewTool[T]` / `MustTool[T]` → `ToolDefinition` |
| 把工具挂到请求 | `Context.Tools` |
| 读回模型的调用 | `AssistantMessage.ToolCalls()` → `[]ToolCall` |
| 解码调用的参数 | `DecodeToolCall[T]` |
| 回传结果 | `ToolResult(id, name, text)` → `ToolResultMessage` |
| 无 Go 类型时校验 | `ValidateToolCall` / `ValidateToolArguments` / `ParseToolArguments` |
| 强制或限制选择 | `StreamOptions.ProtocolOptions` |

`ToolDefinition` 只包含 `Name`、`Description` 与一段 `Parameters` JSON Schema。模型返回的 `ToolCall` 携带 `ID`、`Name` 与解码后的 `Arguments`；回传 `ToolResult` 时需回填其 `ID` 与 `Name`。

## 类型化工具

从 Go 结构体生成与提供方兼容的 JSON Schema，而无需手写工具参数。同一个类型既用于校验、强制转换，也用于解码模型返回的工具调用。

**1. 用结构体描述参数。** `jsonschema` 标签会转化为 schema 约束。没有 `omitempty` 的字段为必填。生成的 schema 完全内联，并省略了 `$schema`、`$id`、`$ref`、`$defs` 等文档元数据。

```go
type WeatherArgs struct {
	City  string `json:"city" jsonschema:"description=City name,minLength=1"`
	Units string `json:"units,omitempty" jsonschema:"enum=celsius,enum=fahrenheit"`
	Days  int    `json:"days" jsonschema:"minimum=1,maximum=10"`
}
```

`jsonschema` 标签支持以下约束，库会据此校验模型返回的参数：

| 约束 | 标签 | 适用类型 |
|---|---|---|
| 必填 | 省略 `omitempty`（加上即为可选） | 全部 |
| 描述 | `description=...` | 全部 |
| 枚举 | `enum=celsius,enum=fahrenheit` | string、number |
| 数值范围 | `minimum=` · `maximum=` · `exclusiveMinimum=` · `exclusiveMaximum=` | number、integer |
| 字符串长度 | `minLength=` · `maxLength=` | string |
| 正则 | `pattern=^[A-Z]` | string |
| 数组长度 | `minItems=` · `maxItems=` | array |

从类型构建工具后，将定义放入 `Context.Tools`：

```go
weatherTool := llm.MustTool[WeatherArgs]("get_weather", "Get a weather forecast")

input.Tools = []llm.ToolDefinition{weatherTool}
```

`response.ToolCalls()` 按顺序返回模型发起的调用。`DecodeToolCall` 会先按工具 Schema 校验参数，再解码为指定的 Go 类型：

```go
arguments, err := llm.DecodeToolCall[WeatherArgs](weatherTool, toolCall)
result := llm.ToolResult(toolCall.ID, toolCall.Name, resultText)
```

从工具声明到多轮执行的完整程序见[执行工具调用](recipes/tool-loop.md)。本页只维护类型、Schema、校验和消息对应关系。

当类型无法生成有效 schema 时，`MustTool` 会 panic，适合在启动阶段声明的工具。若工具是动态构建的、需要处理失败而非崩溃，请改用返回 error 的 `NewTool`。

## 调用与结果的对应关系

`StopReasonToolUse` 表示模型等待工具结果。每个 `ToolCall` 都必须对应一个 `ToolResultMessage`，且结果中的 `ToolCallID` 和工具名称必须与调用一致。历史顺序为 assistant 消息在前，其工具结果随后；之后才能发起下一轮请求。

单轮可能包含多个调用，也可能在收到结果后继续调用其他工具。循环上限、分派、执行失败回传和停止条件属于应用流程，完整实现只在[执行工具调用](recipes/tool-loop.md)中维护。

## 执行前校验

`DecodeToolCall` 会按工具 schema 校验参数并一步解码进目标结构体，这是大多数应用采用的路径。当参数没有对应的 Go 类型时，可改为校验成通用 map:

- `ValidateToolCall(tools, call)` — 按名称匹配工具，然后校验并强制转换；以 `map[string]any` 返回参数。
- `ValidateToolArguments(tool, call)` — 针对一个已知工具进行校验。
- `ParseToolArguments(raw)` — 对原始参数 JSON 做尽力解析，不做 schema 校验；搭配 `ParseToolArgumentsMode` 可得知 JSON 是严格、已修复、部分还是无效。

提供方流式传来的工具参数可能从不完整的 JSON 中恢复而来。稳妥的应用会拒绝 `partial` 和 `invalid` 的参数，并返回一个工具错误让模型重试。在执行带副作用的工具前，请先阅读[流式诊断](streaming.md#工具调用增量与诊断)。

## 协议特定的工具选择

工具选择保留各协议自身的原生写法。通过 `ProtocolOptions` 提供；客户端会校验它的类型与所选模型协议是否匹配，以及被命名的工具是否存在于请求 context 中。

OpenAI 兼容的 Chat Completions 使用 `required` 和 function 选择：

```go
options := llm.StreamOptions{
	ProtocolOptions: &llm.OpenAICompletionsStreamOptions{
		ToolChoice: llm.OpenAIToolChoiceRequired,
		// 若要强制调用某一个 function：
		// ToolChoice: llm.OpenAIToolChoiceFunction{Name: "get_weather"},
	},
}
```

Anthropic Messages 使用 `any` 和 tool 选择：

```go
options := llm.StreamOptions{
	ProtocolOptions: &llm.AnthropicStreamOptions{
		ToolChoice: llm.AnthropicToolChoiceAny,
		// 若要强制调用某一个工具：
		// ToolChoice: llm.AnthropicToolChoiceTool{Name: "get_weather"},
	},
}
```

两种协议都提供 `Auto` 和 `None` 常量。任何显式的工具选择都要求 `Context.Tools` 中至少有一个工具。

## 执行边界

`llm` 返回工具调用，但从不执行工具。应用或独立的编排层必须负责
“请求 → 执行 → 回传”循环。编排层的生命周期、状态与事件接口不属于本 LLM
文档的范围。
