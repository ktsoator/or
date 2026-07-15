# 失败信号

本页定义请求失败的报告方式以及各信号之间的关系。重试、降级、对外返回和部分结果保存等应用策略见[处理请求失败](recipes/error-handling.md)。

本库通过两个不同的出口报告失败，拿到哪一个就说明失败发生在哪里：

- `Stream` 或 `Complete` **返回的 `error`**，表示请求在到达提供方之前就被拒绝了——配置错误、缺少 API key，或协议未注册。没有消耗任何 token。
- **失败的响应**，表示请求已到达提供方，但生成未正常完成。`Complete` 会连同错误一起返回部分的 `AssistantMessage`；流式则以 `EventError` 结束。此时消息的 `StopReason` 为 `StopReasonError`（取消时为 `StopReasonAborted`），`ErrorMessage` 保存详情。

## 发送前返回的配置错误

`Stream` 与 `Complete` 会在派发给适配器之前校验请求并解析凭证。遇到以下情况时，它们不与提供方通信即返回错误：

- **API key 为空。** 请求级配置、provider override 和环境变量都不能解析出凭证时，会返回带 provider 信息的错误。见下文。
- **模型协议没有注册适配器。** 通常是漏了空导入（`_ "github.com/ktsoator/or/llm/openai"` 或 `.../llm/anthropic`，或 `llm/all`）。错误为 `no adapter registered for protocol "..."`。
- **选项校验不通过。** `StreamOptions.Validate` 最先运行——最常见的是拒绝与目标协议不匹配的 `ProtocolOptions`（例如把 `AnthropicStreamOptions` 传给 OpenAI 兼容模型）。

## 缺少 API key

找不到 key 时，错误会按优先级顺序列出提供方及其检查过的每个环境变量：

```
API key is empty for provider "anthropic" (set ANTHROPIC_OAUTH_TOKEN or ANTHROPIC_API_KEY or pass StreamOptions.APIKey)
```

凭证可能来自 `StreamOptions`、provider override 或进程环境。完整优先级只在[请求选项 § 按请求提供凭证](configuration.md#按请求提供凭证)维护。

若想自己检查 key 的解析（例如在启动时尽早失败，或给出配置提示）可使用 key 辅助函数：

```go
if len(llm.FindEnvAPIKeys(model.Provider)) == 0 {
	log.Printf("no key configured; expected one of %v",
		llm.APIKeyEnvVars(model.Provider))
}
```

`APIKeyEnvVars` 返回某提供方检查的变量，`FindEnvAPIKeys` 返回实际已设置的变量，`MissingAPIKeyError` 则构造出与库内部一致的错误消息。`AuthStatus` 还可以报告 override 或环境来源，但不会验证凭证是否仍有效。

## 失败与取消的响应

一旦请求到达提供方，请基于 `StopReason` 分支处理，而不要把每个非空 error 都当作致命。完整表格见[响应与用量](results.md#停止原因)；两个与错误相关的原因是：

- `StopReasonError`——流中途出现提供方或运行时故障。读取 `ErrorMessage`；**不要**执行该消息上的任何工具调用。
- `StopReasonAborted`——`context` 被取消。干净地停止；这在主动取消请求时是预期行为。

`Complete` 会同时返回该消息和非空 `error`；`Stream` 通过终止的 `EventError` 返回 `Message` 与 `Err`。应用如何统一分支见[处理请求失败](recipes/error-handling.md)，取消进行中的流见[流式事件 § 取消](streaming.md#取消)。

## 上下文溢出

超出模型上下文窗口的请求，可能显式失败，也可能被提供方静默截断。`IsContextOverflow` 可识别这两类信号；判定规则见[响应与用量 § 检测上下文溢出](results.md#检测上下文溢出)，压缩历史后的应用重试流程见[处理请求失败](recipes/error-handling.md#上下文溢出)。

## 重试与超时

暂时性的提供方故障由底层 SDK 重试。可按请求调节：`StreamOptions.MaxRetries`（设为 `0` 关闭）与 `Timeout`（独立于 `context` 截止时间，为每次尝试设上限）。完整选项见[请求选项](configuration.md)。

## 已恢复的非致命问题

并非每个问题都是错误。畸形或截断的工具调用参数会被尽力恢复，并记录在 `AssistantMessage.Diagnostics` 中，而不会使响应失败——在执行有副作用的工具前，请务必检查诊断。参见[响应与用量 § 诊断](results.md#诊断)。
