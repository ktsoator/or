# Agent 包

`github.com/ktsoator/or/agent` 把一个模型变成自主的多步执行者。它在 `or/llm` 包
之上运行工具调用循环——流式生成一轮、执行模型请求的工具、追加结果，并持续到模型停止
——同时把历史存储和上下文压缩留给调用方。

它是一个与厂商无关的编排层：一个无状态引擎（`RunLoop`）外加一个可选的有状态封装
（`Agent`），扩展点以函数字段的形式提供。它不捆绑具体的工具、持久化或 system 提示。

## 安装

```sh
go get github.com/ktsoator/or/agent@latest
```

下面的示例假设有这些 import：

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)
```

## 定义一个工具

一个工具把 `llm.ToolDefinition`（从 Go 结构体派生的 JSON schema）与一个 `Execute`
函数配对。`Execute` 通过返回 error 报告失败，循环会把它转成一个错误结果，而不是中止
整个运行。可选的 `onUpdate` 回调流式输出部分进度。可选的 `PrepareArguments` 在
schema 校验前重写原始参数——用它来容忍提供方的怪癖或填充默认值。

一个发生 panic 的工具不会让进程崩溃：循环会把它恢复成一个错误结果并继续。来自调用方
钩子（`ConvertToLLM`、`TransformContext`、`PrepareNextTurn`、`ShouldStopAfterTurn`
或工具调用钩子）的 panic 同样会被恢复，以一个终止错误事件干净地结束运行。

```go
type weatherArgs struct {
	City string `json:"city" jsonschema:"description=City to look up,minLength=1"`
}

weatherTool := agent.AgentTool{
	Definition: llm.MustTool[weatherArgs]("get_weather", "Get the current weather for a city"),
	Execute: func(ctx context.Context, callID string, args json.RawMessage, onUpdate func(agent.ToolResult)) (agent.ToolResult, error) {
		var in weatherArgs
		if err := json.Unmarshal(args, &in); err != nil {
			return agent.ToolResult{}, err
		}
		onUpdate(agent.ToolResult{Details: "querying weather service"}) // 可选进度
		return agent.ToolResult{
			Content: []llm.ToolResultContent{&llm.TextContent{Text: fmt.Sprintf("Sunny, 24°C in %s.", in.City)}},
		}, nil
	},
}
```

## 快速开始

`New` 构建一个有状态的 agent；`Prompt` 把一个任务运行到完成，并把结果追加到它的
记录（transcript）中。`Prompt` 接受字符串、单个 `AgentMessage` 或一个切片。对于
多模态提示，`agent.UserMessage` 从文本和图像构建一条用户消息：

```go
assistant.Prompt(ctx, agent.UserMessage("What is in this picture?",
	llm.ImageContent{Data: base64PNG, MIMEType: "image/png"}))
```

```go
assistant := agent.New(agent.Options{
	SystemPrompt: "Call get_weather before answering a weather question.",
	Model:        llm.GetModel("deepseek", "deepseek-v4-flash"),
	Tools:        []agent.AgentTool{weatherTool},
})

if err := assistant.Prompt(context.Background(), "What is the weather in Shanghai?"); err != nil {
	log.Fatal(err)
}

// 记录中现在保存着提示、工具调用、其结果，以及答案。
for _, message := range assistant.Snapshot().Messages {
	_ = message
}
```

## 观察一次运行

`Subscribe` 注册一个监听器，按顺序接收每一个事件。它返回一个用于移除该监听器的函数。
`Prompt` 会阻塞直到运行结束，因此监听器在运行期间触发。

```go
unsubscribe := assistant.Subscribe(func(event agent.AgentEvent) {
	switch event.Type {
	case agent.ToolStart:
		fmt.Printf("\n[tool] %s %v\n", event.ToolName, event.Args)
	case agent.MessageUpdate:
		if event.LLMEvent != nil && event.LLMEvent.Type == llm.EventTextDelta {
			fmt.Print(event.LLMEvent.Delta) // 流式输出答案
		}
	}
})
defer unsubscribe()
```

各种事件类型：

| 事件 | 含义 | 值得注意的字段 |
|---|---|---|
| `AgentStart` / `AgentEnd` | 运行的边界 | `AgentEnd.Messages` —— 本次运行追加的所有内容 |
| `TurnStart` / `TurnEnd` | 一次 assistant 响应及其工具 | `TurnEnd.ToolResults` |
| `MessageStart` / `MessageUpdate` / `MessageEnd` | 一条消息进入、流式、完成 | `MessageUpdate.LLMEvent` —— 底层的 `llm.Event` |
| `ToolStart` / `ToolUpdate` / `ToolEnd` | 一个工具正在执行 | `Args`、`Result`、`IsError` |

## 无状态引擎

`RunLoop` 是 `Agent` 之下的引擎。它接收新的提示和一个基础 context，返回一个事件
通道，并把记录留给你管理。最终的 `AgentEnd` 事件携带本次运行追加的消息。

```go
events := agent.RunLoop(ctx,
	[]agent.AgentMessage{agent.FromLLM(llm.UserText("Weather in Shanghai?"))},
	agent.Context{Tools: []agent.AgentTool{weatherTool}},
	agent.LoopConfig{Model: llm.GetModel("deepseek", "deepseek-v4-flash")},
)

var appended []agent.AgentMessage
for event := range events {
	if event.Type == agent.AgentEnd {
		appended = event.Messages
	}
}
```

## 控制工具调用

`BeforeToolCall` 在参数校验之后、执行之前运行；返回 `block` 会跳过该工具，并把
`reason` 用作错误结果。

```go
BeforeToolCall: func(c agent.BeforeToolCallCtx) (block bool, reason string) {
	if c.ToolCall.Name == "delete_file" {
		return true, "file deletion is disabled"
	}
	return false, ""
},
```

`AfterToolCall` 在执行之后运行；非 nil 的返回会逐字段覆盖结果。在一个批次中的每个
结果上设置 `Terminate`，会在该批次之后停止运行。

```go
AfterToolCall: func(c agent.AfterToolCallCtx) *agent.AfterToolCallResult {
	stop := true
	return &agent.AfterToolCallResult{Terminate: &stop}
},
```

一个批次**默认并发执行**。可以对整个循环、或对单个工具强制顺序执行：

```go
agent.New(agent.Options{ToolExecution: agent.ExecutionSequential /* ... */})

weatherTool.ExecutionMode = agent.ExecutionSequential // 该工具强制其所在批次顺序执行
```

## 在不同轮次间切换模型

`PrepareNextTurn` 在每一轮之后运行，可以为下一轮替换模型或思考级别。由于历史会按
请求重新适配，新模型甚至可以使用一种不同的线缆协议。

```go
PrepareNextTurn: func(c agent.TurnCtx) *agent.TurnUpdate {
	// 先用快速模型起草，再用更强的模型评审（不同协议）。
	review := llm.GetModel("minimax-cn", "MiniMax-M3")
	return &agent.TurnUpdate{Model: &review}
},
```

## 停止与恢复

`ShouldStopAfterTurn` 在下一次请求之前请求一次优雅停止。

```go
ShouldStopAfterTurn: func(c agent.TurnCtx) bool {
	return len(c.NewMessages) > 20 // 防止循环失控
},
```

`Continue` 在没有新提示的情况下从当前记录恢复——用于重试，或在带外追加消息之后。
提供方需要最新一轮是用户消息或工具结果，因此当记录以一条 assistant 消息结尾时，
`Continue` 会回退到排队的消息：它先排空干预（steering）队列，再排空追加（follow-up）
队列，并运行它找到的任何内容。仅当最后一条消息是 assistant 且两个队列都为空时，它才会
报错。

```go
if err := assistant.Continue(ctx); err != nil {
	log.Fatal(err)
}
```

## 干预与追加

`Steer` 在下一轮之前注入一条消息；`FollowUp` 在 agent 本将停止之后注入一条。在
`Prompt` 运行期间，从另一个 goroutine 调用它们。

```go
go func() {
	_ = assistant.Prompt(ctx, "Summarize the repository")
}()

assistant.Steer(agent.FromLLM(llm.UserText("Focus on the agent package.")))
```

`SteeringMode` 和 `FollowUpMode` 控制每次排空多少条排队消息：`QueueOneAtATime`
（默认）只注入最早的一条，把其余留给后续轮次；`QueueAll` 一次性注入全部。

```go
agent.New(agent.Options{SteeringMode: agent.QueueAll /* ... */})
```

## 动态 API key

`GetAPIKey` 在每一轮之前解析提供方 key，适用于可能在长时间运行中过期的短期令牌。

```go
GetAPIKey: func(provider string) string {
	return currentOAuthToken(provider) // 带外刷新
},
```

## 调优请求

`StreamOptions` 是每轮传给模型的基础请求选项集合——`Temperature`、`MaxTokens`、
`Headers`、`OnRequest` / `OnResponse` 观察器，以及在请求体发送前对其打补丁的
`RewriteRequest` 钩子（用于类型化 API 未暴露的提供方特定字段）。agent 会用
`ThinkingLevel` 填入 `Reasoning`、用 `GetAPIKey` 填入 `APIKey`，因此这两个字段
会被忽略。

```go
temperature := 0.2
assistant := agent.New(agent.Options{
	Model:         model,
	ThinkingLevel: llm.ModelThinkingHigh,
	StreamOptions: llm.StreamOptions{
		Temperature: &temperature,
		MaxTokens:   4096,
		OnRequest:   func(method, url string, body []byte) { log.Println(method, url) },
	},
})
```

## 自定义消息

一次运行操作的是 `AgentMessage`：用 `FromLLM` 适配的标准 `llm` 消息，外加嵌入了
`Custom` 的应用类型。自定义消息会留在记录和事件流中，但会被默认的 `ConvertToLLM`
丢弃，因此它们永远不会到达模型。提供你自己的 `ConvertToLLM` 来投射它们。

```go
type Notice struct {
	agent.Custom
	Text string
}

assistant := agent.New(agent.Options{
	Model:    model,
	Messages: []agent.AgentMessage{Notice{Text: "session resumed"}}, // 保留，不发送
})
```

## 管理状态

```go
state := assistant.Snapshot() // 只读快照，可在运行中从另一个 goroutine 安全调用
// state.Messages 随运行推进而增长；state.StreamingMessage 持有进行中的响应，
// state.PendingToolCalls 列出正在执行的工具调用。

assistant.HasQueuedMessages()  // 是否有排队的干预或追加消息？
assistant.ClearSteeringQueue() // 丢弃排队的干预消息
assistant.ClearQueues()        // 丢弃两个队列
assistant.Abort()              // 取消当前运行
assistant.Reset()              // 清空记录、错误和队列；保留配置
```

要在两次运行之间重新配置 agent，使用 setter。每个 setter 在下一次运行时生效，
且不会扰动已在进行中的运行：

```go
assistant.SetModel(llm.GetModel("minimax-cn", "MiniMax-M3"))
assistant.SetSystemPrompt("Answer in one sentence.")
assistant.SetThinkingLevel(llm.ModelThinkingHigh)
assistant.SetTools([]agent.AgentTool{weatherTool}) // 切片会被复制
assistant.SetToolExecution(agent.ExecutionSequential)
```

可运行的程序在 [`example/agent`](https://github.com/ktsoator/or/tree/main/example/agent)：
`basic`（一个工具、一个提示）和 `tool`（一个带推理、工具进度和会话中模型切换的
交互式会话）。

## 范围

本包提供编排机制，把策略留给调用方。上下文压缩、会话持久化、技能（skills）以及执行
环境都被有意排除在这一层之外；`TransformContext` 钩子是日后接入压缩的地方。

导出的类型和函数，参见
[pkg.go.dev](https://pkg.go.dev/github.com/ktsoator/or/agent) 上的包文档。
