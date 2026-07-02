# 读取响应

`Complete` 返回一个 `AssistantMessage`；流式方式通过 `EventDone.Message` 给出同一个值。本页介绍可以从中读取什么：内容、生成停止的原因、token 用量与成本，以及非致命诊断。

## 内容与元数据

两个访问器覆盖大多数读取需求：

```go
response.Text()      // 拼接所有文本块
response.ToolCalls() // 按顺序返回每个工具调用
```

该消息还携带提供方元数据：`Provider`、`Model`、提供方自身的 `ResponseModel` 与 `ResponseID`，以及 `Timestamp`。失败响应中 `ErrorMessage` 保存提供方或运行时的错误字符串。

```go
fmt.Printf("model=%s response=%s id=%s\n",
	response.Model, response.ResponseModel, response.ResponseID)
if response.ErrorMessage != "" {
	log.Printf("provider error: %s", response.ErrorMessage)
}
```

若想逐块处理而非读取拼接后的文本（例如把思考与文本分别渲染）可对 `response.Content` 做类型分支：

```go
for _, block := range response.Content {
	switch b := block.(type) {
	case *llm.TextContent:
		fmt.Println("text:", b.Text)
	case *llm.ThinkingContent:
		fmt.Println("thinking:", b.Thinking)
	case *llm.ToolCall:
		fmt.Printf("tool call: %s(%v)\n", b.Name, b.Arguments)
	}
}
```

## 停止原因

`StopReason` 说明生成为何停止。在使用响应前先据此分支判断——尤其是在执行工具调用之前。

| `StopReason` | 含义 | 典型处理 |
|---|---|---|
| `StopReasonStop` | 正常完成 | 使用 `response.Text()` |
| `StopReasonToolUse` | 模型需要工具结果 | 运行[工具循环](tools.md#运行工具循环) |
| `StopReasonLength` | 输出触达 `MaxTokens` 上限 | 续写本轮或调高上限 |
| `StopReasonError` | 提供方或运行时失败 | 查看 `ErrorMessage`；不要执行工具调用 |
| `StopReasonAborted` | 请求被取消 | 停止；context 已被取消 |

```go
switch response.StopReason {
case llm.StopReasonStop:
	fmt.Println(response.Text())
case llm.StopReasonToolUse:
	runTools(response.ToolCalls()) // 参见工具循环
case llm.StopReasonLength:
	log.Println("truncated: raise MaxTokens or continue the turn")
case llm.StopReasonError, llm.StopReasonAborted:
	log.Printf("stopped early: %s %s", response.StopReason, response.ErrorMessage)
}
```

## Token 用量与成本

`Usage` 记录响应的 token 消耗。缓存 token 单独统计，使缓存命中可见：

| 字段 | 含义 |
|---|---|
| `Input` | 按完整输入价计费的提示 token |
| `Output` | 生成的 token，含推理 token |
| `CacheRead` | 由提供方缓存返回的输入 token |
| `CacheWrite` | 写入缓存的输入 token |
| `TotalTokens` | 响应报告的总和 |

`Usage.Cost` 是一个 `UsageCost`，以货币单位给出相同的分项（`Input`、`Output`、 `CacheRead`、`CacheWrite` 和 `Total`），在组装响应时按模型定价计算得出。

```go
fmt.Printf("tokens=%d (cached %d) cost=$%.6f\n",
	response.Usage.TotalTokens,
	response.Usage.CacheRead,
	response.Usage.Cost.Total,
)
```

若想自行计价（例如用另一个模型重新核算已存历史的成本）调用 `CalculateCost`：

```go
cost := llm.CalculateCost(model, response.Usage)
fmt.Printf("input=$%.6f output=$%.6f total=$%.6f\n",
	cost.Input, cost.Output, cost.Total)
```

要统计多轮对话的总花费，累加每次响应的 `Cost.Total`：

```go
var spent float64
for _, turn := range responses {
	spent += turn.Usage.Cost.Total
}
fmt.Printf("conversation cost: $%.4f\n", spent)
```

## 检测上下文溢出

`IsContextOverflow` 报告响应是否超出模型的上下文窗口。它既能识别显式的提供方错误，也能识别提供方截断输入而不报错的"静默溢出"。可用它在下一轮之前触发历史压缩或摘要。

```go
if llm.IsContextOverflow(response, model.ContextWindow) {
	// 丢弃或摘要旧消息,然后重试。
}
```

## 诊断

`Diagnostics` 记录生成响应过程中发生的非致命事件，例如从畸形 JSON 中恢复出的工具参数。对于干净的响应，它为 `nil`。每个 `Diagnostic` 携带 `Type`、`Timestamp`、可选的 `Message` 和结构化的 `Details`。

```go
for _, d := range response.Diagnostics {
	if d.Type == llm.DiagnosticToolArgumentsRecovered {
		log.Printf("recovered tool arguments: mode=%v call=%v",
			d.Details["mode"], d.Details["toolCallId"])
	}
}
```

在执行带副作用的工具前请先检查诊断；恢复模式参见[流式诊断](streaming.md#工具调用增量与诊断)。
