# 消息类型系统

[`conversation.go`](https://github.com/ktsoator/or/blob/main/internal/llm/conversation.go)
定义了对话模型——所有适配器共同读写的一组中立类型，不与任何厂商绑定。出站时，适配器将它们翻译为某一厂商的通信格式；入站时，再从响应流中重建出相同的类型。

## 结构总览

[`Context`](#context) 由系统提示、对话消息和可用工具组成。每条消息是对话历史中的一个条目——用户输入、助手回复或工具返回——条目内部由若干内容块承载具体片段。

```text
Context
├── SystemPrompt: string
├── Messages: []Message
│       ├── UserMessage        → []UserContent
│       ├── AssistantMessage   → []AssistantContent
│       └── ToolResultMessage  → []ToolResultContent
└── Tools: []ToolDefinition
```

## 用标记接口封闭类型

Go 没有原生的联合类型（sum type），无法直接声明一个封闭的类型集合。这里的替代做法是给接口加一个未导出方法：包外类型无从实现，可选的实现便被限定在当前包内的几种：

```go
type Message interface {
	isMessage()
}

func (*UserMessage) isMessage()       {}
func (*AssistantMessage) isMessage()  {}
func (*ToolResultMessage) isMessage() {}
```

消息的种类因此构成一个封闭集合：对它做 type switch 时，你能确信分支已经列全——包外无法
再添新类型。这些方法定义在指针接收者上，所以包内流转的具体值始终是 `*UserMessage`、
`*AssistantMessage` 与 `*ToolResultMessage`。

同一套接口对内容块还多一层用途：内容块按角色分成三个接口，每种块实现哪个接口，就声明了
自己能出现在哪种消息里——没实现的那种，放进去就编译不过：

```go
// UserContent 可出现在用户消息中
type UserContent interface {
	isUserContent()
}

// AssistantContent 可出现在助手消息中
type AssistantContent interface {
	isAssistantContent()
}

// ToolResultContent 可出现在工具结果消息中
type ToolResultContent interface {
	isToolResultContent()
}

func (*TextContent) isUserContent()       {}
func (*TextContent) isAssistantContent()  {} // 三种消息都可
func (*TextContent) isToolResultContent() {}

func (*ImageContent) isUserContent()       {} // 仅用户与工具结果
func (*ImageContent) isToolResultContent() {}

func (*ThinkingContent) isAssistantContent() {} // 仅助手
func (*ToolCall) isAssistantContent()        {} // 仅助手
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

有几个字段的实际分量重于字面：

- `ToolCall.ID` 是关联键。`ToolResultMessage` 在 `ToolCallID` 中回填它来应答一次调用，
  结果与请求正是借此在一轮内对应起来。
- `ToolCall.Arguments` 是已解码的 JSON 对象（`map[string]any`），而非原始字符串。流式
  传来的参数文本会先经过一次尽力解析，即便流被截断也能得出一个值，再存入此字段。
- `ThinkingContent.Redacted` 标记厂商以删节形式返回的推理：文本被隐去，但内容块予以保留，
  使该轮保持完整，其签名也得以回传。

!!! note "关于 signature 字段"
    `TextSignature`、`ThinkingSignature`、`ThoughtSignature` 是不透明的厂商元数据。
    本包不解读其内容，仅原样保存，并在后续轮次中原样回传，使厂商得以跨请求验证其推理与
    工具调用的连贯性。目标模型变化时这些字段如何被保留或丢弃，参见
    [模型切换](transform.md)。

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

## token 用量与停止原因

`AssistantMessage` 上的 `Usage` 与 `StopReason` 各是一个小型值类型。`Usage` 按类别统计
token，并携带算得的 `UsageCost`；其类别与 [`ModelCost`](models.md#定价) 对应，故成本即
逐类相乘：

```go
type Usage struct {
	Input, Output, CacheRead, CacheWrite, TotalTokens int64
	Cost UsageCost
}

type UsageCost struct {
	Input, Output, CacheRead, CacheWrite, Total float64
}
```

`StopReason` 是一组固定取值，将各厂商不同的停止信号统一映射为同一套中立取值：

| 取值 | 含义 |
|---|---|
| `stop` | 正常完成 |
| `length` | 因输出 token 上限被截断 |
| `toolUse` | 为让调用方执行工具调用而停止 |
| `error` | 厂商或运行时故障 |
| `aborted` | 请求被取消 |

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
