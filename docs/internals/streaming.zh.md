# 流式机制

流式输出围绕 content block 归一化。provider 底层可能是 JSON chunk、SSE event，或
SDK 自己的 union 类型；但包对外报告的是同一套生命周期：流开始，每个内容块开始并接收
增量，每个内容块结束，最后用一个终止事件关闭 channel。

## 事件生命周期

| 内容 | 开始 | 增量 | 结束 |
|---|---|---|---|
| 文本 | `EventTextStart` | `EventTextDelta` | `EventTextEnd` |
| 推理 | `EventThinkingStart` | `EventThinkingDelta` | `EventThinkingEnd` |
| 工具调用 | `EventToolCallStart` | `EventToolCallDelta` | `EventToolCallEnd` |

每个非终止事件都会携带 `Partial`，也就是当前已组装出的 `AssistantMessage` 快照。终止
事件只有两种：带最终消息的 `EventDone`，或带部分消息和错误的 `EventError`。

## StreamWriter

provider 适配器不会直接向 channel 写事件。它们在内存里逐步构造 `AssistantMessage`，
并把指针交给 `NewStreamWriter`。writer 负责维护事件通道的不变量：

- `Start()` 幂等，只发出一个 `EventStart`。
- `Emit()` 为每个非终止事件附上新的 `Partial` 快照。
- `Done()` 发出一个 `EventDone`；如果上下文已取消，则转为失败。
- `Fail()` 发出一个 `EventError`。

writer 用 mutex 和 `finished` 标记保护这些规则，所以 provider 后续又返回错误时，也不能
发出第二个终止事件。上下文取消会报告为 `StopReasonAborted`；其他失败会变成
`StopReasonError`。

## Provider 状态

每个适配器都有自己的 `streamState` 来处理协议差异。OpenAI 适配器同时按 stream index
和 ID 跟踪工具调用，因为不同兼容厂商重复的字段不同。Anthropic 适配器按 provider 的
stream index 跟踪内容块，并记录是否收到 stop 信号；如果连接干净关闭但没有 stop 事件，
则视为错误。

## 工具参数

工具调用参数以原始 JSON 片段流式到达。最终的 `EventToolCallEnd` 会用
`ParseToolArgumentsMode` 解析累计字符串；它可以修复错误转义，也可以把被截断的对象补齐。
恢复出来的参数不会让整个响应失败，而是写入最终 `AssistantMessage.Diagnostics`，记录恢复
模式，让调用方在执行带副作用的工具前自行拒绝不安全参数。

源码：[`internal/llm/stream.go`](https://github.com/ktsoator/or/blob/main/internal/llm/stream.go)、
[`internal/llm/events.go`](https://github.com/ktsoator/or/blob/main/internal/llm/events.go)。
