# 事件与状态

一次运行在推进时会发出一串事件，agent 把这些事件折叠进一份可随时读取的实时状态。两者
配合，能让 UI 实时渲染一次运行——流式文本、工具进度、回合边界——而无需轮询。

## 订阅

`Subscribe` 注册一个按顺序接收每个事件的监听器，并返回一个用于移除它的函数。`Prompt` 会
阻塞到运行结束，所以监听器是在运行期间触发的；请在调用 `Prompt` 之前订阅。

```go
unsubscribe := assistant.Subscribe(func(event agent.AgentEvent) {
	switch event.Type {
	case agent.MessageUpdate:
		if event.LLMEvent != nil && event.LLMEvent.Type == llm.EventTextDelta {
			fmt.Print(event.LLMEvent.Delta) // 逐 token 流式输出回答
		}
	case agent.ToolStart:
		fmt.Printf("\n[tool] %s %v\n", event.ToolName, event.Args)
	case agent.ToolEnd:
		fmt.Printf("[done] %s (error=%v)\n", event.ToolName, event.IsError)
	}
})
defer unsubscribe()
```

监听器在驱动运行的那个 goroutine 上、按事件顺序同步执行。阻塞的监听器会拖住整个运行，
包括工具执行，所以要让它们足够快——把重活交给另一个 goroutine 或一个带缓冲的 channel。

## 事件类型

```go
type AgentEvent struct {
	Type        AgentEventType
	Message     AgentMessage         // 生命周期事件指向的消息
	LLMEvent    *llm.Event           // 底层 llm 事件，在 MessageUpdate 上设置
	ToolResults []llm.ToolResultMessage // 在 TurnEnd 上设置
	ToolCallID  string
	ToolName    string
	Args        any                  // 校验后的工具参数，在工具事件上
	Result      any                  // （部分）ToolResult，在工具事件上
	IsError     bool
	Messages    []AgentMessage       // 追加的消息，在 AgentEnd 上设置
}
```

字段按 `Type` 填充；无关字段为零值。

| 事件 | 含义 | 关键字段 |
|---|---|---|
| `AgentStart` / `AgentEnd` | 运行边界 | `AgentEnd.Messages` —— 本次运行追加的全部内容 |
| `TurnStart` / `TurnEnd` | 一轮 assistant 响应及其工具 | `TurnEnd.ToolResults` |
| `MessageStart` / `MessageUpdate` / `MessageEnd` | 一条消息进入、流式、完成 | `MessageUpdate.LLMEvent` —— 底层 `llm.Event` |
| `ToolStart` / `ToolUpdate` / `ToolEnd` | 一个工具在执行 | `ToolName`、`Args`、`Result`、`IsError` |

`MessageUpdate` 在 `LLMEvent` 里携带原始 `llm.Event`，所以你能区分文本增量、推理增量和
工具调用增量，并从 `event.Message` 读取目前为止拼装出的部分消息。

## 运行的生命周期

事件以可预测的顺序到达：

```
AgentStart
  TurnStart
    MessageStart / MessageEnd        （用户提示）
    MessageStart / MessageUpdate* / MessageEnd   （assistant 轮次，流式）
    ToolStart / ToolUpdate* / ToolEnd            （该轮调用的每个工具）
    MessageStart / MessageEnd        （每个工具结果）
  TurnEnd
  ... 模型继续调用工具时，又一个 TurnStart ...
AgentEnd
```

一个不调用任何工具、也没有留下引导消息的回合，会在 `TurnEnd` 之后结束运行。
`AgentEnd.Messages` 与无状态 `RunLoop` 返回的切片相同——即本次运行追加进 transcript 的
全部内容。

## 读取状态

`Snapshot` 返回 agent 当前状态的只读副本。在运行进行时从另一个 goroutine 调用它是安全的。

```go
type State struct {
	SystemPrompt     string
	Model            llm.Model
	ThinkingLevel    llm.ModelThinkingLevel
	Tools            []AgentTool
	Messages         []AgentMessage // 随每条消息完成而增长
	IsStreaming      bool           // 一次 prompt 或 continuation 正在进行
	StreamingMessage AgentMessage   // 在途响应，或 nil
	PendingToolCalls []string       // 当前正在执行的工具调用 id
	ErrorMessage     string         // 最近一次失败回合的文本
}
```

agent 在通知监听器之前，会先把每个事件折叠进这份状态，所以监听器看到的总是更新后的状态：

- `Messages` 随每条消息到达 `MessageEnd` 而增长。
- `StreamingMessage` 随增量到达而追踪响应，完成时清空。
- `PendingToolCalls` 列出处于 `ToolStart` 与 `ToolEnd` 之间的工具调用。

```go
state := assistant.Snapshot()
if state.IsStreaming {
	fmt.Print(state.StreamingMessage) // 渲染部分回答
}
```

## 下一步

- 在[引导与追加](steering.md)里于运行流式进行时注入消息，或在它停止后继续。
- 在[生命周期钩子](hooks.md)里拦截工具调用、在回合之间切换模型。
