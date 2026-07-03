# 流式机制

流式被归一到内容块上。厂商可能流式返回 JSON 分片、SSE 事件，或 SDK 特有的 union，但本包对外报告同一套生命周期：流开始，每个内容块开始并接收增量，每个块结束，最后恰好一个终止事件关闭通道。

## 事件生命周期

| 内容 | 开始 | 增量 | 结束 |
|---|---|---|---|
| 文本 | `EventTextStart` | `EventTextDelta` | `EventTextEnd` |
| 推理 | `EventThinkingStart` | `EventThinkingDelta` | `EventThinkingEnd` |
| 工具调用 | `EventToolCallStart` | `EventToolCallDelta` | `EventToolCallEnd` |

一条流以一个 `EventStart` 开场，以恰好一个终止事件收尾——成功是 `EventDone`，失败是 `EventError`。

## Event 联合体

`Event` 是一个平铺的联合体：`Type` 选定更新的种类，而对每个 `Type` 只有一部分字段有意义。

```go
type Event struct {
	// Type selects which of the fields below are meaningful; see the table above.
	Type EventType

	// ContentIndex is the position of the affected block within the assembled
	// message content, on the per-block start/delta/end events.
	ContentIndex int

	// Delta is newly streamed text on a *Delta event, or a fragment of argument
	// JSON on EventToolCallDelta.
	Delta string

	// Content is the completed block text on EventTextEnd and EventThinkingEnd.
	Content string

	// ToolCall is the tool call being assembled, on the toolcall events. It holds
	// the best-effort parsed call at EventToolCallEnd.
	ToolCall *ToolCall

	// Partial is a snapshot of the message assembled so far, on every non-terminal
	// event.
	Partial *AssistantMessage

	// Message is the final assistant message, on the terminal EventDone and
	// EventError events.
	Message *AssistantMessage

	// Err is the stream failure, on EventError.
	Err error
}
```

哪些字段被填充由 `Type` 固定决定。读取当前 `Type` 未列出的字段，只会拿到一个毫无意义的零值：

| Type | 有意义的字段（除 `Type` 外） |
|---|---|
| `EventStart` | `Partial` |
| `EventTextStart` | `ContentIndex`、`Partial` |
| `EventTextDelta` | `ContentIndex`、`Delta`、`Partial` |
| `EventTextEnd` | `ContentIndex`、`Content`、`Partial` |
| `EventThinkingStart` | `ContentIndex`、`Partial` |
| `EventThinkingDelta` | `ContentIndex`、`Delta`、`Partial` |
| `EventThinkingEnd` | `ContentIndex`、`Content`、`Partial` |
| `EventToolCallStart` | `ContentIndex`、`ToolCall`、`Partial` |
| `EventToolCallDelta` | `ContentIndex`、`Delta`、`ToolCall`、`Partial` |
| `EventToolCallEnd` | `ContentIndex`、`ToolCall`、`Partial` |
| `EventDone` | `Message` |
| `EventError` | `Message`、`Err` |

`Partial` 挂在每个非终止事件上；终止事件改为携带最终的 `Message`。

## StreamWriter

厂商适配器不直接往通道发送。它们在内存里构建一个 `AssistantMessage`，把指针交给 `NewStreamWriter`，再用 `Emit`、`Done`、`Fail` 驱动它。writer 负责通道的不变量：一个 `EventStart`、每个非终止事件带一份 `Partial` 快照、恰好一个终止事件。

`Emit` 先在需要时发出 `EventStart`，再把此刻已构建的消息克隆进 `Partial`，让每个事件都是独立快照：

```go
func (w *StreamWriter) Emit(event Event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finished { // (1)!
		return
	}
	w.startLocked()
	event.Partial = cloneAssistantMessage(*w.output) // (2)!
	w.events <- event
}
```

1.  一旦终止事件已发出，之后的每次调用都是空操作——单终止事件的保证正是靠这个成立的。
2.  深克隆，因此持有较早 `Partial` 的消费者不会看到它随流的继续而被改动。

`Done` 正常发送 `EventDone`，但被取消的 context 会被改道到失败路径，因此取消永远表现为错误、绝不表现为成功：

```go
func (w *StreamWriter) Done() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finished {
		return
	}
	w.startLocked()
	if err := w.ctx.Err(); err != nil { // (1)!
		w.failLocked(err)
		return
	}
	w.finished = true
	w.events <- Event{Type: EventDone, Message: cloneAssistantMessage(*w.output)}
}
```

1.  一条已经拼出完整消息、却被取消的流，会以错误收尾，因此调用方不会把中止的轮次误当成已完成。

`failLocked` 设置停止原因和错误，并把被取消的 context 与普通失败区分开：

```go
func (w *StreamWriter) failLocked(err error) {
	w.finished = true
	if err == nil {
		err = errors.New("stream failed")
	}
	output := *w.output
	if w.ctx.Err() != nil {
		output.StopReason = StopReasonAborted // (1)!
		err = w.ctx.Err()
	} else {
		output.StopReason = StopReasonError
	}
	output.ErrorMessage = err.Error()
	w.events <- Event{Type: EventError, Message: cloneAssistantMessage(output), Err: err}
}
```

1.  取消被报告为 `StopReasonAborted`，错误被替换为 context 错误；其余任何失败都是 `StopReasonError`。

互斥锁加上 `finished` 标志意味着：在 `Done` 之后姗姗来迟的厂商错误无法再发出第二个终止事件。

## 各适配器的状态

每个适配器为各自协议的怪癖保留自己的 `streamState`。OpenAI 适配器同时按流索引和 ID 跟踪工具调用，因为兼容厂商在分片间重复的是哪一个各不相同。Anthropic 适配器按厂商的流索引跟踪内容块，并记录停止信号是否到达，因此一个没有停止事件的干净 socket 关闭会被当作错误处理。

## 工具参数

工具调用的参数以裸 JSON 分片的形式流式返回。最后的 `EventToolCallEnd` 用 `ParseToolArgumentsMode` 解析累积的字符串，它能修复坏转义或补全被截断的对象。恢复出的参数不会让整个响应失败；相反，最终的 `AssistantMessage.Diagnostics` 会记录恢复模式，因此调用方可以等到 `EventDone`，再拒绝执行那些参数只被部分恢复的调用。

源码：[`llm/stream.go`](https://github.com/ktsoator/or/blob/main/llm/stream.go)、[`llm/events.go`](https://github.com/ktsoator/or/blob/main/llm/events.go)。
