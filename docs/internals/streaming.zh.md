# 流式机制

本页解释 adapter 如何实现公共流式契约。事件名称、字段矩阵和调用方消费规则只在 [LLM 流式响应](../llm/streaming.md)维护。

Provider 可能返回 SSE、JSON 分片或 SDK union。Adapter 一边构建内存中的 `AssistantMessage`，一边通过 `StreamWriter` 发出中立事件。公共层看到相同生命周期，provider 差异留在各 adapter 的 `streamState` 中。

## StreamWriter 的职责

Adapter 不直接向事件通道发送值。它把正在构建的 `AssistantMessage` 交给 `NewStreamWriter`，然后调用 `Emit`、`Done` 或 `Fail`。

| 不变量 | `StreamWriter` 的处理 |
|---|---|
| 流只有一个开始事件 | 第一次 `Emit`、`Done` 或 `Fail` 时按需发送 `EventStart` |
| 非终止事件携带独立快照 | `Emit` 深克隆当前消息到 `Event.Partial` |
| 流只有一个终止事件 | 锁与 `finished` 标志阻止后续 `Done`、`Fail` 和 `Emit` |
| 取消不能被报告为成功 | `Done` 先检查 `ctx.Err()`，已取消时转入失败路径 |
| 失败消息保留部分结果 | `Fail` 克隆当前消息，并设置停止原因与 `ErrorMessage` |

`Partial` 必须深克隆。否则消费者保存的早期事件会随着 adapter 继续追加内容而发生变化。克隆也意味着每个增量都存在分配成本；调用方不应在只需要 `Delta` 时反复序列化整份 `Partial`。

## 终止路径

正常完成时，writer 把最终消息克隆到 `EventDone.Message`。普通运行错误生成 `EventError`，并设置 `StopReasonError`；context 已取消时生成同一事件类型，但停止原因为 `StopReasonAborted`，error 替换为 context error。

终止事件发送后，writer 将 `finished` 设为 true。晚到的 provider 错误或内容块不会产生第二个终止事件。这项保证由 writer 提供，不要求每个 adapter 重复实现。

事件通道是无缓冲的。Writer 在发送期间可能阻塞，因此调用方停止读取会同时阻止 adapter 收尾。取消 context 只能中止 HTTP 工作，不能替代通道消费。

## Adapter 流状态

每个 adapter 维护自己的流状态，但最终都驱动同一个 writer：

- OpenAI adapter 同时按流索引和 tool-call ID 跟踪工具调用。兼容 provider 在分片中重复的标识并不一致。
- Anthropic adapter 按 provider 内容块索引组装事件，并记录是否收到正式停止信号。Socket 干净关闭但缺少停止事件时，adapter 将其视为错误。
- Adapter 在 goroutine 退出时关闭 SDK stream；writer 发送终止事件后，事件通道随后关闭。

这些状态不暴露给调用方。公共代码只根据 `Event.Type`、`ContentIndex` 和终止消息处理结果。

## 工具参数恢复

工具参数以原始 JSON 片段到达。Adapter 累积字符串，在工具块结束时使用 `ParseToolArgumentsMode` 解析。解析器可以修复坏转义或补全截断对象。

恢复不会自动让整条响应失败。最终 `AssistantMessage.Diagnostics` 记录恢复模式，调用方应等待 `EventDone`，再决定是否拒绝 `partial` 或 `invalid` 参数。工具执行约束见 [LLM 工具](../llm/tools.md)。

## 资源与并发

- 单条流由 adapter goroutine 生产，由调用方 goroutine 消费。
- `StreamWriter` 的锁只保护事件顺序和终止状态，不把事件通道改成缓冲通道。
- `Client` 没有 `Close`；单次 provider stream 由 adapter 关闭。
- 传入 adapter 的 `http.Client` 属于应用，应跨请求复用。
- Context 控制整个请求；`StreamOptions.Timeout` 控制单次 HTTP attempt。

源码：[`llm/stream.go`](https://github.com/ktsoator/or/blob/main/llm/stream.go)、[`llm/events.go`](https://github.com/ktsoator/or/blob/main/llm/events.go)。
