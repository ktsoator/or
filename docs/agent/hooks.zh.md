# 生命周期钩子

每个扩展点都是 `agent.Options` 上的一个函数字段（无状态引擎则是 `LoopConfig`）。每个的
零值都是“无钩子”，所以裸 agent 就是一个朴素的工具循环。这些钩子让你拦截工具调用、在回合
之间切换模型、提前停止，并在每次请求前重塑上下文。

## 钩子的触发顺序

在一个回合内，钩子按如下顺序触发：

```
TransformContext        （就在构建请求之前）
  → assistant 轮次流式生成
  对每个工具调用：
    BeforeToolCall       （参数校验之后、执行之前）
      → Execute
    AfterToolCall        （执行之后、结果发出之前）
PrepareNextTurn          （该回合及其工具结果追加之后）
ShouldStopAfterTurn      （下一次请求开始之前）
```

## 拦截工具调用

`BeforeToolCall` 在参数校验之后、工具执行之前运行。返回 `block = true` 跳过该工具；
`reason` 会成为模型看到的错误结果文本。

```go
BeforeToolCall: func(c agent.BeforeToolCallCtx) (block bool, reason string) {
	if c.ToolCall.Name == "delete_file" {
		return true, "file deletion is disabled in this session"
	}
	return false, ""
},
```

`AfterToolCall` 在工具完成后运行。非 nil 的返回会逐字段覆盖结果；nil 字段保留执行得到的
值。

```go
AfterToolCall: func(c agent.AfterToolCallCtx) *agent.AfterToolCallResult {
	if c.IsError {
		stop := true
		return &agent.AfterToolCallResult{Terminate: &stop} // 任一工具出错就结束运行
	}
	return nil
},
```

`AfterToolCallResult` 可覆盖 `Content`、`Details`、`IsError` 和 `Terminate`。给一批里
每个结果都设上 `Terminate`，该批之后即停止运行。两个钩子都按源序运行、绝不并发，即使
工具本身并行运行也是如此——见[工具](tools.md#执行顺序)。

## 回合间的模型切换

`PrepareNextTurn` 在每个回合后运行，可为下一回合替换模型、思考强度或上下文。由于历史按
请求重新适配，新模型甚至可以讲不同的 wire 协议。

```go
PrepareNextTurn: func(c agent.TurnCtx) *agent.TurnUpdate {
	// 先用快模型起草，再用更强的模型评审（不同协议）。
	if len(c.NewMessages) == 2 {
		review := llm.GetModel("minimax-cn", "MiniMax-M3")
		return &agent.TurnUpdate{Model: &review}
	}
	return nil
},
```

`TurnUpdate` 携带可选的 `Context`、`Model` 和 `ThinkingLevel`；nil 字段保留当前值。
`TurnCtx` 把该回合的 assistant 消息、它的工具结果、当前上下文，以及 `NewMessages`——
即运行此刻停止会返回的内容——交给钩子。

## 提前停止

`ShouldStopAfterTurn` 在下一次请求开始之前请求一次优雅停止。agent 没有内置的回合上限，
所以这里就是你防止失控循环的地方。

```go
ShouldStopAfterTurn: func(c agent.TurnCtx) bool {
	return len(c.NewMessages) > 20 // 限制一次运行可追加的消息数
},
```

## 重塑上下文

`TransformContext` 在每次请求把 transcript 投影成 `llm` 消息之前调整它。它是上下文压缩
的挂载点——总结或丢弃旧回合以适配窗口——本包对此不提供默认实现。

```go
TransformContext: func(messages []agent.AgentMessage) []agent.AgentMessage {
	if len(messages) <= 40 {
		return messages
	}
	return compact(messages) // 你的总结策略
},
```

它按请求运行，且不修改存储的 transcript，所以压缩只影响模型看到的内容，不影响 agent 的
记忆。

## 安全性

任何钩子的 panic 都会被恢复成一个终止错误事件，所以行为异常的回调会干净地结束运行，而不是
让进程崩溃。`ConvertToLLM` 同理（见[消息与自定义类型](messages.md)）。
