# 运行循环引擎

`RunLoop` 是驱动工具调用循环的无状态引擎。`Agent` 是其上一层轻量的有状态封装，增加了
保留的 transcript、事件订阅，以及引导与追加队列。当你想自己掌管状态时——自己的持久化、
自己的事件管线，或集成进已有的运行循环——直接使用 `RunLoop`。

## 签名

```go
func RunLoop(ctx context.Context, prompts []AgentMessage, base Context, cfg LoopConfig) <-chan AgentEvent
```

- `prompts` 是启动本次运行的新消息。
- `base` 是它们延续的已有上下文——系统提示、先前的 transcript 和工具。
- `cfg` 配置本次运行；每个扩展点都是函数字段。

它返回一个事件 channel。**调用方必须把它排空到关闭。** 最终的 `AgentEnd` 事件携带本次
运行追加的消息，你把它们折叠进自己的 transcript。

```go
events := agent.RunLoop(ctx,
	[]agent.AgentMessage{agent.FromLLM(llm.UserText("Weather in Shanghai?"))},
	agent.Context{
		SystemPrompt: "Call get_weather before answering.",
		Tools:        []agent.AgentTool{weatherTool},
	},
	agent.LoopConfig{Model: llm.GetModel("deepseek", "deepseek-v4-flash")},
)

var appended []agent.AgentMessage
for event := range events {
	switch event.Type {
	case agent.MessageUpdate:
		// 渲染流式输出
	case agent.AgentEnd:
		appended = event.Messages // 本次运行追加的全部
	}
}
```

## LoopConfig

`LoopConfig` 是完整的配置项集合。给定一个 `Model` 和默认的 `ConvertToLLM`，零值配置即为
无拦截的朴素工具循环。

```go
type LoopConfig struct {
	Model         llm.Model
	StreamOptions llm.StreamOptions
	StreamFn      StreamFn
	ConvertToLLM  func([]AgentMessage) []llm.Message
	GetAPIKey     func(provider string) string
	ToolExecution ExecutionMode

	BeforeToolCall      func(BeforeToolCallCtx) (block bool, reason string)
	AfterToolCall       func(AfterToolCallCtx) *AfterToolCallResult
	ShouldStopAfterTurn func(TurnCtx) bool
	PrepareNextTurn     func(TurnCtx) *TurnUpdate
	TransformContext    func([]AgentMessage) []AgentMessage

	GetSteeringMessages func() []AgentMessage
	GetFollowUpMessages func() []AgentMessage
}
```

钩子字段的行为与 `agent.Options` 上完全一致——见[生命周期钩子](hooks.md)和
[配置](configuration.md)。

## 无 Agent 时的引导与追加

`Agent` 用并发安全的队列支撑 `Steer` 和 `FollowUp`。用 `RunLoop` 时，你自己提供来源
函数：

- `GetSteeringMessages` 在每一轮工具调用结束后被轮询；返回要在下一轮之前注入的消息。
- `GetFollowUpMessages` 在运行本应停止时被轮询；返回消息以让它继续。

```go
agent.LoopConfig{
	Model: model,
	GetSteeringMessages: func() []agent.AgentMessage {
		return drainMyQueue() // 你自己的来源
	},
}
```

## RunLoop 与 Agent 的取舍

| | `RunLoop` | `Agent` |
|---|---|---|
| 状态 | 你掌管 transcript | 内部保留 |
| 事件 | 排空返回的 channel | `Subscribe` 监听器 |
| 引导 / 追加 | 提供 `Get*Messages` 函数 | `Steer` / `FollowUp` 队列 |
| 并发 | 由你决定 | 同时一个运行，方法安全 |

多数应用应选择 `Agent`。当其有状态性成为负担时使用 `RunLoop`——例如 transcript 已存放在
数据库中、agent 不应再保留一份副本时。
