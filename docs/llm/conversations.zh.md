# 对话

对话消息与厂商无关。同一段历史可以被持久化、扩展，并发送给另一个兼容模型，而无需重建。

## 消息与内容模型

一段历史就是一个 `[]llm.Message`。`Message` 是一个接口，有三个实现，每个角色一个。每个实现持有一组*内容块*，且角色限定了允许哪些块类型：

| 消息 | 角色 | 允许的内容块 |
|---|---|---|
| `UserMessage` | 用户输入 | `TextContent`、`ImageContent` |
| `AssistantMessage` | 模型输出 | `TextContent`、`ThinkingContent`、`ToolCall` |
| `ToolResultMessage` | 工具结果 | `TextContent`、`ImageContent` |

内容块是实际读写的叶子类型：

| 块 | 承载内容 |
|---|---|
| `TextContent` | 纯文本（任意消息中均可） |
| `ImageContent` | base64 图像数据加 MIME 类型 |
| `ThinkingContent` | 推理文本及其 provider 签名（仅 assistant） |
| `ToolCall` | 工具名、ID 与解码后的参数（仅 assistant） |

由于消息和块都是带类型的，已存对话可无需手动分派地通过 JSON 往返——见[保存与恢复](#保存与恢复对话)。

对于常见的"只发文本"场景，请使用下面的便捷构造器。仅当需要构造器覆盖不到的内容时才手写结构体字面量——例如在一条用户消息里混合文本与图像（见[图像输入](#图像输入)），或播种一条携带工具调用的 assistant 轮次。

## 构建消息

`Context`、`Message` 以及内容块都是完全通用的，但多数调用只是发送一些文本。便捷构造器为这种场景省去了嵌套：

```go
llm.Prompt("Explain Go channels briefly.")        // 含一条用户文本消息的 Context
llm.PromptWithSystem("Be concise.", "Explain...") // ……外加一个 system 提示
llm.UserText("hello")                             // *UserMessage
llm.AssistantText("hi there")                     // *AssistantMessage（用于预置历史）
llm.UserImage(data, "image/png")                  // 含一张图像的 *UserMessage
llm.ToolResult(callID, name, "result text")       // *ToolResultMessage
llm.NewContext(msg1, msg2, ...)                   // 由若干消息构成的 Context
```

用 `AssistantMessage` 上对应的访问器读回响应：

```go
response.Text()      // 拼接所有文本块
response.ToolCalls() // 按顺序返回每一个工具调用
```

下面这种完整的结构体字面量写法仍然有效；当需要构造器未覆盖的内容时（例如在一条消息中混合文本和图像），再使用它。

## 延续对话

多轮对话就是一个不断增长的 `[]llm.Message`。追加助手的回复，再追加下一条用户消息，然后把整个切片再次发送。本库是无状态的，保存的历史就是这段对话。

```go
messages := []llm.Message{
	llm.UserText("Name a Go web framework."),
}

for _, turn := range []string{"And one for CLIs?", "Which is older?"} {
	reply, err := llm.Complete(ctx, model,
		llm.Context{Messages: messages}, llm.StreamOptions{})
	if err != nil {
		log.Fatal(err)
	}
	messages = append(messages, &reply)            // 记录回答
	messages = append(messages, llm.UserText(turn)) // 追问
}
```

追加 `&reply`（指针），让助手这一轮保留本库重放所需的类型。设在 `Context` 上的 `SystemPrompt` 对每一轮都生效，且不会被存进消息历史。

## 图像输入

多模态模型支持在用户消息中图文并存。以 base64 提供原始字节及其 MIME 类型：

```go
raw, err := os.ReadFile("screenshot.png")
if err != nil {
	log.Fatal(err)
}
input := llm.Context{Messages: []llm.Message{
	&llm.UserMessage{Content: []llm.UserContent{
		&llm.TextContent{Text: "Describe the problem shown in this screenshot."},
		&llm.ImageContent{
			MIMEType: "image/png",
			Data:     base64.StdEncoding.EncodeToString(raw),
		},
	}},
}}
```

模型通过 `Model.Input` 声明是否支持图像。当包含图像的历史被发送给仅支持文本的模型时，图像会被自动替换为一个简短的占位符。

## 在不同轮次间切换模型

每次请求前，本库都会为目标模型适配已存储的历史：为仅支持文本的模型降级图像、同模型重放时保留推理签名、删除其他模型产生的推理，并规范化工具调用标识符。

下面这两个模型甚至使用不同的线协议（DeepSeek 是 OpenAI 兼容，MiniMax CN 是 Anthropic 兼容）但历史切片可以原样复用。由于每种协议各有适配器，需同时注册两个 provider 包：

```go
import (
	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/anthropic" // MiniMax CN（Anthropic 兼容）
	_ "github.com/ktsoator/or/llm/openai"    // DeepSeek（OpenAI 兼容）
)

ctx := context.Background()
draft := llm.GetModel("deepseek", "deepseek-v4-flash")   // openai-completions
review := llm.GetModel("minimax-cn", "MiniMax-M2.7")      // anthropic-messages

messages := []llm.Message{
	llm.UserText("Compute 25 * 18 and explain the steps."),
}

first, err := llm.Complete(ctx, draft,
	llm.Context{Messages: messages}, llm.StreamOptions{})
if err != nil {
	log.Fatal(err)
}
messages = append(messages, &first)
messages = append(messages, llm.UserText("Check the calculation above for mistakes."))

second, err := llm.Complete(ctx, review,
	llm.Context{Messages: messages}, llm.StreamOptions{})
if err != nil {
	log.Fatal(err)
}
```

这需要环境中设置 `DEEPSEEK_API_KEY` 与 `MINIMAX_CN_API_KEY`。完整程序见可运行的[`model_switch`](https://github.com/ktsoator/or/tree/main/example/llm/model_switch)示例。

`TransformMessages` 执行这项适配，并已对外导出，供需要查看模型实际会收到的确切历史的调用方使用。

## 保存与恢复对话

`Context` 序列化为自描述的 JSON：消息携带角色，内容块携带类型，因此 JSON 可以无需手动分派地往返还原成具体的消息和内容类型。

```go
data, err := json.MarshalIndent(llm.Context{Messages: messages}, "", "  ")
if err != nil {
	log.Fatal(err)
}
if err := os.WriteFile("conversation.json", data, 0o644); err != nil {
	log.Fatal(err)
}

raw, err := os.ReadFile("conversation.json")
if err != nil {
	log.Fatal(err)
}
var restored llm.Context
if err := json.Unmarshal(raw, &restored); err != nil {
	log.Fatal(err)
}
```

`restored.Messages` 已可用于扩展，并针对任意模型重放。

若存储系统按行或按记录保存单条消息，可使用 `MarshalMessage` 与
`UnmarshalMessage`，无需先包进 `Context`：

```go
data, err := llm.MarshalMessage(messages[0])
if err != nil {
	log.Fatal(err)
}

message, err := llm.UnmarshalMessage(data)
if err != nil {
	log.Fatal(err)
}
messages = append(messages, message)
```

遇到未知角色、未知内容类型或畸形 JSON 时，`UnmarshalMessage` 返回错误，
不会把不支持的消息结构静默转换为其他类型。

!!! warning "序列化的历史是敏感数据"
    序列化后的 `Context` 可能包含用户输入、工具结果（其中可能嵌入抓取到的文档或凭证）以及提供方的推理签名。请把这份 JSON 当作敏感数据：不要整体打日志，存储或传输时应与其中的底层数据同等对待。
