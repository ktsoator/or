# Agent 包

`github.com/ktsoator/or/agent` 把模型变成一个能自主执行多步的行动者。它在 `or/llm`
之上运行工具调用循环——流式跑一轮、执行模型请求的工具、把结果追加回去，如此往复直到
模型停止——而把历史存储与上下文压缩留给调用方。

它是 provider 无关的编排层：无状态引擎（`RunLoop`）加上可选的有状态封装层
（`Agent`），所有扩展点均为函数字段。它不内置任何具体工具、持久化或系统提示词。

换句话说，`llm` 包负责“向模型发起一次请求”，`agent` 包负责“把一次任务跑完”。如果模型
在回答中请求工具，agent 会验证参数、执行工具、把工具结果作为新消息追加回 transcript，
再继续请求模型。只有当模型给出最终回答、运行被取消、或钩子要求停止时，这次运行才结束。

## 适合什么场景

当你的应用需要下面这些能力时，用 `agent`：

- 让模型在一次用户请求里多轮思考、调用工具、读取结果并继续。
- 把每次运行产生的用户消息、assistant 消息、工具结果保存在同一个 transcript 里。
- 在 UI 或服务端实时观察运行进度：文本增量、工具开始/结束、工具进度、回合边界。
- 在运行中插入新指令、排队追加任务，或中止正在执行的 agent。
- 通过函数钩子控制行为，例如拦截某个工具调用、替换工具结果、切换模型、压缩上下文。

如果你只想手动控制一次模型请求，直接用 [`or/llm`](../llm/README.md)。如果你还需要会话
持久化、自动上下文压缩、按回合构造系统提示词、skills 或 prompt templates，可以在
`agent` 之上使用
[`or/agent/harness`](https://pkg.go.dev/github.com/ktsoator/or/agent/harness)。

## 一次运行发生什么

一次 `Agent.Prompt` 大致按下面的顺序执行：

1. 把用户输入追加到 agent 的 transcript。
2. 将 transcript 投影成 `llm.Message`，调用当前模型并流式接收 assistant 消息。
3. 如果 assistant 请求工具，agent 会按工具 schema 校验参数并执行对应的 `AgentTool`。
4. 把每个工具结果追加回 transcript，让模型读取这些结果。
5. 如此循环，直到模型不再请求工具，或运行被取消、拦截、停止。
6. 运行结束后，`Snapshot().Messages` 保留这次运行追加的完整消息序列。

这个循环不绑定任何具体 provider。只要模型通过 `or/llm` 暴露为同一套消息、工具和流式事件，
`agent` 就可以在其上工作。

## 最小示例

没有工具时，`Agent` 也可以作为一个带 transcript 的有状态模型会话：

```go
assistant := agent.New(agent.Options{
	SystemPrompt: "You are a concise Go tutor.",
	Model:        llm.GetModel("deepseek", "deepseek-v4-flash"),
})

if err := assistant.Prompt(ctx, "Explain goroutines in one sentence."); err != nil {
	log.Fatal(err)
}

messages := assistant.Snapshot().Messages
last, _ := agent.ToLLM(messages[len(messages)-1])
fmt.Println(last.(*llm.AssistantMessage).Text())
```

加入工具后，同一个 `Prompt` 会自动变成完整的工具调用循环：

```go
type weatherArgs struct {
	City string `json:"city" jsonschema:"description=City to look up,minLength=1"`
}

weather := agent.AgentTool{
	Definition: llm.MustTool[weatherArgs](
		"get_weather",
		"Get the current weather for a city",
	),
	Execute: func(ctx context.Context, callID string, args json.RawMessage, onUpdate func(agent.ToolResult)) (agent.ToolResult, error) {
		var in weatherArgs
		if err := json.Unmarshal(args, &in); err != nil {
			return agent.ToolResult{}, err
		}
		return agent.ToolResult{
			Content: []llm.ToolResultContent{
				&llm.TextContent{Text: "Sunny, 24C in " + in.City},
			},
		}, nil
	},
}

assistant := agent.New(agent.Options{
	SystemPrompt: "Call get_weather before answering weather questions.",
	Model:        llm.GetModel("deepseek", "deepseek-v4-flash"),
	Tools:        []agent.AgentTool{weather},
})

err := assistant.Prompt(ctx, "What should I pack for Shanghai today?")
```

完整可运行版本见[快速开始](getting-started.md)和仓库里的
[`example/agent`](https://github.com/ktsoator/or/tree/main/example/agent)。

## 两层 API

| 层级 | 适合谁 | 负责什么 |
| --- | --- | --- |
| `Agent` | 大多数应用 | 保存 transcript、串行运行 prompt、发事件、支持 steer/follow-up/abort |
| `RunLoop` | 自建运行时或已有状态层 | 只执行一次无状态工具循环，把新增消息通过事件返回 |

通常先用 `Agent`。当你的 transcript 已经存放在数据库、队列或自己的运行时里，不希望库再保留
一份状态时，再直接使用 `RunLoop`。

## 它负责什么，不负责什么

`agent` 负责：

- 工具调用循环和工具结果回填。
- 有状态 transcript 与只读快照。
- 流式事件订阅。
- 运行中 steering、follow-up 和 abort。
- 工具执行顺序、工具进度、工具拦截和回合级钩子。
- provider 无关的模型切换、推理等级和动态 API key。

`agent` 不负责：

- 内置搜索、文件系统、浏览器或数据库工具。
- 自动选择系统提示词或安全策略。
- 跨进程持久化 transcript。
- 默认上下文压缩策略。
- 调度任务、部署服务或管理用户权限。

这些边界是刻意保留的：工具、存储、提示词和权限通常都强依赖应用本身。`agent` 提供可组合的
运行内核，让这些策略留在你的应用层。

## 安装

```sh
go get github.com/ktsoator/or/agent@latest
```

## 文档

- [快速开始](getting-started.md) — 第一个 agent 与工具循环
- [工具](tools.md) — 定义工具、结果、流式进度与执行顺序
- [事件与状态](events.md) — 运行事件流、订阅与快照
- [引导与追加](steering.md) — 运行中注入消息、继续与中止
- [生命周期钩子](hooks.md) — 拦截工具、切换模型、停止与压缩
- [消息与自定义类型](messages.md) — transcript、仅 UI 消息与投影
- [配置](configuration.md) — 请求选项、推理、动态密钥与 setter
- [运行循环引擎](loop.md) — `RunLoop`、`LoopConfig` 与自建封装

完整的导出类型和函数，参见
[pkg.go.dev](https://pkg.go.dev/github.com/ktsoator/or/agent) 上的包文档。
