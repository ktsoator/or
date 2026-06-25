# 消息类型系统

[`conversation.go`](https://github.com/ktsoator/or/blob/main/internal/llm/conversation.go)
定义了对话模型，即每个适配器都要读写的那套类型。其中不含任何与具体厂商绑定的内容：出站时，
适配器将这些中立类型翻译为某家厂商的通信格式；入站时，再从响应流中重建出相同的类型。

## 两级结构

一个 [`Context`](#context) 持有一组消息，每条消息再持有一组内容块。消息对应对话的「一轮」，
内容块则是这一轮中的具体内容。

```text
Context
└── Messages: []Message
        ├── UserMessage        → []UserContent
        ├── AssistantMessage   → []AssistantContent
        └── ToolResultMessage  → []ToolResultContent
```

## 以标记接口约束固定集合

Go 没有 sum type，因此这里为接口附加一个未导出方法，将其实现集合限定为固定的几种：

```go
type Message interface {
	isMessage()
}

func (*UserMessage) isMessage()       {}
func (*AssistantMessage) isMessage()  {}
func (*ToolResultMessage) isMessage() {}
```

由于 `isMessage` 未导出，包外的任何类型都无法满足 `Message`。消息的种类因此构成一个封闭
集合，对其进行 type switch 在实践中即是穷尽的。这些方法定义在指针接收者上，因此包内流转的
具体值始终是 `*UserMessage`、`*AssistantMessage` 与 `*ToolResultMessage`。

同一手法对内容块还承担了第二重作用：内容块通过实现对应的角色接口，声明自己可以出现在哪种
消息中——并且只实现被允许的那几个：

```go
func (*TextContent) isUserContent()       {}
func (*TextContent) isAssistantContent()  {}
func (*TextContent) isToolResultContent() {}

func (*ThinkingContent) isAssistantContent() {} // 仅限 assistant 消息
```

## 放置规则

角色接口将「哪个块可放入哪种消息」转化为一条编译期约束。`ThinkingContent` 未实现
`UserContent`，因此若将其放入 `UserMessage`，将无法通过编译。

| 内容块 | UserMessage | AssistantMessage | ToolResultMessage |
|---|:---:|:---:|:---:|
| `TextContent` | ✓ | ✓ | ✓ |
| `ImageContent` | ✓ | — | ✓ |
| `ThinkingContent` | — | ✓ | — |
| `ToolCall` | — | ✓ | — |

## 四种内容块

```go
type TextContent struct {
	Text          string `json:"text"`
	TextSignature string `json:"textSignature,omitempty"`
}

type ImageContent struct {
	Data     string `json:"data"`     // base64 编码的字节
	MIMEType string `json:"mimeType"`
}

type ThinkingContent struct {
	Thinking          string `json:"thinking"`
	ThinkingSignature string `json:"thinkingSignature,omitempty"`
	Redacted          bool   `json:"redacted,omitempty"`
}

type ToolCall struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Arguments        map[string]any `json:"arguments"`
	ThoughtSignature string         `json:"thoughtSignature,omitempty"`
}
```

有几个字段的分量超出表面所载的内容：

- `ToolCall.ID` 是关联键。`ToolResultMessage` 通过在 `ToolCallID` 中回填它来应答一次
  调用，结果与请求正是借此在一轮内对应起来。
- `ToolCall.Arguments` 是已解码的 JSON 对象（`map[string]any`），而非原始字符串。流式
  传来的参数文本会先经过解析——尽力而为，因此即便流被截断也能得出一个值——再落入此处。
- `ThinkingContent.Redacted` 标记厂商以删节形式返回的推理：文本被隐去，但内容块予以保留，
  以使该轮保持完整，其签名也得以回传。

!!! note "关于 signature 字段"
    `TextSignature`、`ThinkingSignature`、`ThoughtSignature` 是不透明的厂商元数据。
    本包不解读其内容，仅将其原样保存，并在后续轮次中原样回传，以便厂商验证其推理与工具
    调用在多次请求之间的连贯性。目标模型变化时这些字段如何被保留或丢弃，参见
    [模型切换](transform.md)。

## token 用量与停止原因

每条 assistant 响应上都附带两个小型值类型。`Usage` 按类别统计 token，并携带算得的
`UsageCost`；其类别与 [`ModelCost`](models.md#定价) 对应，故成本即逐类相乘：

```go
type Usage struct {
	Input, Output, CacheRead, CacheWrite, TotalTokens int64
	Cost UsageCost
}

type UsageCost struct {
	Input, Output, CacheRead, CacheWrite, Total float64
}
```

`StopReason` 是一个封闭集合，将各厂商的停止信号归一为同一套中立词汇：

| 取值 | 含义 |
|---|---|
| `stop` | 正常完成 |
| `length` | 因输出 token 上限被截断 |
| `toolUse` | 为让调用方执行工具调用而停止 |
| `error` | 厂商或运行时故障 |
| `aborted` | 请求被取消 |

## 三种消息

`UserMessage` 与 `ToolResultMessage` 结构精简。用户消息仅是一个内容列表；工具结果则多出
将其关联回对应 `ToolCall` 的调用 ID 与错误标志：

```go
type UserMessage struct {
	Content []UserContent
}

type ToolResultMessage struct {
	ToolCallID string
	ToolName   string
	Content    []ToolResultContent
	IsError    bool
}
```

`AssistantMessage` 是其中较大的一个——既有模型输出，也有由适配器填写的响应元数据：

```go
type AssistantMessage struct {
	Content []AssistantContent

	Protocol     Protocol     // 本次响应所用的通信协议
	Provider     string       // 厂商标识
	Model        string       // 请求的模型 ID
	Usage        Usage        // token 数与算得的成本
	StopReason   StopReason   // 停止生成的原因
	Diagnostics  []Diagnostic // 非致命事件，无异常时为 nil
	Timestamp    int64        // Unix 毫秒
	// …… ResponseModel、ResponseID、ErrorMessage 略
}
```

适配器并不从零填写这些字段。`NewAssistantMessage(model)` 会先植入与厂商无关的元数据——
`Protocol`、`Provider`、`Model` 与 `Timestamp`——因此适配器是从一条半成品消息起步，
只需追加内容以及与本次响应相关的字段。

## 读取响应

两个辅助方法代调用方遍历 `Content`，无需手动逐一进行类型断言。二者均对 nil 安全，并保持
内容块的原有顺序：

```go
func (message *AssistantMessage) Text() string          // 拼接全部文本块
func (message *AssistantMessage) ToolCalls() []ToolCall // 按顺序返回全部工具调用
```

`Text()` 跳过思考块与工具调用块；`ToolCalls()` 在模型未请求工具时返回 `nil`，与
`toolUse` 这一 `StopReason` 相呼应。`ToolCalls()` 返回的是值而非指针——即调用方可直接
交给工具执行器、而不会与消息自身内容块产生别名的副本。

## Context

一次请求由三个字段组成：

```go
type Context struct {
	SystemPrompt string
	Messages     []Message
	Tools        []ToolDefinition
}
```

`ToolDefinition` 将参数 schema 以原始 JSON 保存（`json.RawMessage`），因此别处生成的
schema 可原样透传。

这些类型如何序列化为自描述的 JSON、又如何无需手写分派表即可解码还原，参见
[`messages.go`](https://github.com/ktsoator/or/blob/main/internal/llm/messages.go)。
