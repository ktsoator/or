# 流式事件

本页定义 `Stream` 的事件顺序、字段语义、终止条件和取消行为。需要完整的界面接入程序与异常处理策略，参见[流式响应](recipes/streaming-chat.md)。

`Stream` 返回一个只读、无缓冲的事件通道。请求由后台协程执行，调用方必须持续读取到通道关闭；提前停止读取可能阻塞协议适配器发送后续事件。最小的消费结构如下：

```go
for event := range events {
	switch event.Type {
	case llm.EventThinkingDelta, llm.EventTextDelta:
		fmt.Print(event.Delta)
	case llm.EventDone:
		handleDone(event.Message)
	case llm.EventError:
		handleFailure(event.Message, event.Err)
	}
}
```

只有所选模型和提供方返回推理内容时，才会出现推理事件。`EventError.Message` 可能包含已经生成的部分内容和用量。

## 事件参考

流以 `EventStart` 开始，每个内容块（文本、推理或工具调用，可能交错）发出一组 `start → delta… → end`，并以恰好一个终止事件结束：

```mermaid
flowchart LR
    start(["EventStart"]) --> blocks

    subgraph blocks["每个内容块一组"]
        direction LR
        bs["…Start"] --> bd["…Delta<br/><small>× 多次</small>"] --> be["…End"]
    end

    blocks --> outcome{"结果"}
    outcome -->|成功| done(["EventDone<br/><small>Message = 最终 AssistantMessage</small>"])
    outcome -->|失败 / 取消| err(["EventError<br/><small>Err + 部分 Message</small>"])

    classDef ok stroke:#16a34a,stroke-width:2px;
    classDef bad stroke:#dc2626,stroke-width:2px;
    class done ok;
    class err bad;
```

每个非终止事件都带有 `Partial` 快照；`…` 前缀代表 `Text`、`Thinking` 或 `ToolCall`。

| 事件 | 含义 | 主要字段 |
|---|---|---|
| `EventStart` | 提供方流已开始 | `Partial` |
| `EventTextStart` | 文本块开始 | `ContentIndex`、`Partial` |
| `EventTextDelta` | 文本片段到达 | `ContentIndex`、`Delta`、`Partial` |
| `EventTextEnd` | 文本块完成 | `ContentIndex`、`Content`、`Partial` |
| `EventThinkingStart` | 推理块开始 | `ContentIndex`、`Partial` |
| `EventThinkingDelta` | 推理片段到达 | `ContentIndex`、`Delta`、`Partial` |
| `EventThinkingEnd` | 推理块完成 | `ContentIndex`、`Content`、`Partial` |
| `EventToolCallStart` | 工具调用块开始 | `ContentIndex`、`ToolCall`、`Partial` |
| `EventToolCallDelta` | 工具参数 JSON 原始片段到达 | `ContentIndex`、`Delta`、`ToolCall`、`Partial` |
| `EventToolCallEnd` | 工具调用流式结束，参数已尽力解析 | `ContentIndex`、`ToolCall`、`Partial` |
| `EventDone` | 请求成功完成 | `Message` |
| `EventError` | 请求失败或被取消 | `Err`、`Message` |

`EventDone.Message` 是最终的助手消息，包含内容、用量、成本和停止原因。`EventError.Message` 可能包含部分内容和用量。通道只会发出恰好一个终止事件，随后关闭。字段解释参见[响应与用量](results.md)。

来自不同内容块的事件可能交错出现。用 `ContentIndex` 将增量关联到对应的块。每个非终止事件都携带一份当前助手消息的 `Partial` 快照。

## 工具调用增量与诊断

`EventToolCallDelta.Delta` 包含原始的部分 JSON。`EventToolCallEnd` 携带的调用，其参数是尽力解析的：格式错误或被截断的 JSON 会退化为目前已收到的字段，或退化为一个空对象。请在使用前校验参数，在流式过程中收集工具调用，并只在 `EventDone` 之后执行它们。切勿执行来自以 `EventError` 结束的响应中的调用。

当参数无法被严格解析时，响应会在 `Message.Diagnostics` 中记录一条 `tool_arguments_recovered`。其恢复 `mode` 为 `repaired`、`partial` 或 `invalid`。在执行带副作用的工具前请检查诊断。稳妥的做法是拒绝 `partial` 和 `invalid` 的参数，并返回一个工具错误，让模型重试。

## 取消

取消请求上下文会停止正在进行的 HTTP 调用。协议适配器会尝试发出一个 `EventError`，其消息包含 `StopReasonAborted`，随后关闭通道。调用方在取消后仍必须继续读取通道；若消费者已经停止读取，无缓冲发送可能阻止终止事件发出并延迟通道关闭。

传输层的截止时间请使用独立的、按尝试计的 `Timeout` 选项；参见[请求选项](configuration.md)。

`Stream` 不提供独立的 `Close` 或 `Abort` 方法。取消入口是传入的请求上下文。协议适配器的后台协程退出时释放本次请求占用的资源。
