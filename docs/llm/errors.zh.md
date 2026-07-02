# 错误处理

本库通过两个不同的出口报告失败，拿到哪一个就说明失败发生在哪里：

- `Stream` 或 `Complete` **返回的 `error`**，表示请求在到达提供方之前就被拒绝了——配置错误、缺少 API key，或协议未注册。没有消耗任何 token。
- **失败的响应**，表示请求已到达提供方，但生成未正常完成。`Complete` 会连同错误一起返回部分的 `AssistantMessage`；流式则以 `EventError` 结束。此时消息的 `StopReason` 为 `StopReasonError`（取消时为 `StopReasonAborted`），`ErrorMessage` 保存详情。

```go
response, err := llm.Complete(ctx, model, input, opts)
if err != nil {
	// 请求从未离开进程,或提供方的流失败了。
	// response 仍可能带有部分消息和 StopReason。
	log.Fatalf("request failed: %v", err)
}
```

## 发送前返回的配置错误

`Stream` 与 `Complete` 会在派发给适配器之前校验请求并解析凭证。遇到以下情况时，它们不与提供方通信即返回错误：

- **API key 为空。** 在检查 `StreamOptions.APIKey` 和该提供方的环境变量之后，适配器会返回一个带提供方信息的错误，明确列出检查过哪些变量。见下文。
- **模型协议没有注册适配器。** 通常是漏了空导入（`_ "github.com/ktsoator/or/llm/openai"` 或 `.../llm/anthropic`，或 `llm/all`）。错误为 `no adapter registered for protocol "..."`。
- **选项校验不通过。** `StreamOptions.Validate` 最先运行——最常见的是拒绝与目标协议不匹配的 `ProtocolOptions`（例如把 `AnthropicStreamOptions` 传给 OpenAI 兼容模型）。

## 缺少 API key

找不到 key 时，错误会按优先级顺序列出提供方及其检查过的每个环境变量：

```
API key is empty for provider "anthropic" (set ANTHROPIC_OAUTH_TOKEN or ANTHROPIC_API_KEY or pass StreamOptions.APIKey)
```

key 按以下顺序解析，取第一个非空值：

1. `StreamOptions.APIKey`（若已设置）。
2. `StreamOptions.Env`——一个请求级的 `ProviderEnv` 映射，优先于进程环境检查。适合多租户服务：每个用户的 key 保存在内存中，而非 `os.Environ`。
3. 进程中该提供方的环境变量。

```go
// 显式指定 key,不查环境变量。
opts := llm.StreamOptions{APIKey: userKey}

// 请求级环境,仅对本次调用覆盖进程环境。
opts := llm.StreamOptions{Env: llm.ProviderEnv{"ANTHROPIC_API_KEY": userKey}}
```

若想自己检查 key 的解析（例如在启动时尽早失败，或给出配置提示）可使用 key 辅助函数：

```go
if len(llm.FindEnvAPIKeys(model.Provider)) == 0 {
	log.Printf("no key configured; expected one of %v",
		llm.APIKeyEnvVars(model.Provider))
}
```

`APIKeyEnvVars` 返回某提供方检查的变量，`FindEnvAPIKeys` 返回实际已设置的变量，`MissingAPIKeyError` 则构造出与库内部一致的错误消息。

## 失败与取消的响应

一旦请求到达提供方，请基于 `StopReason` 分支处理，而不要把每个非空 error 都当作致命。完整表格见[读取响应](results.md#停止原因)；两个与错误相关的原因是：

- `StopReasonError`——流中途出现提供方或运行时故障。读取 `ErrorMessage`；**不要**执行该消息上的任何工具调用。
- `StopReasonAborted`——`context` 被取消。干净地停止；这在主动取消请求时是预期行为。

```go
response, err := llm.Complete(ctx, model, input, opts)
switch response.StopReason {
case llm.StopReasonError:
	log.Printf("provider error: %s", response.ErrorMessage)
case llm.StopReasonAborted:
	log.Print("cancelled")
default:
	fmt.Println(response.Text())
}
_ = err
```

流式时，同样的失败会以终止事件的形式到达：

```go
for event := range events {
	if event.Type == llm.EventError {
		log.Printf("stream failed: %v", event.Err)
		break
	}
}
```

取消进行中的流，见[流式 § 取消](streaming.md#取消)。

## 上下文溢出

超出模型上下文窗口的请求，可能显式失败，也可能被提供方静默截断。`IsContextOverflow` 两种都能识别，于是可压缩历史后重试，而不是把原始错误暴露出去：

```go
if llm.IsContextOverflow(response, model.ContextWindow) {
	// 丢弃或摘要旧消息,然后重试本轮。
}
```

参见[读取响应 § 检测上下文溢出](results.md#检测上下文溢出)。

## 重试与超时

暂时性的提供方故障由底层 SDK 重试。可按请求调节：`StreamOptions.MaxRetries`（设为 `0` 关闭）与 `Timeout`（独立于 `context` 截止时间，为每次尝试设上限）。完整选项见[配置](configuration.md)。

## 已恢复的非致命问题

并非每个问题都是错误。畸形或截断的工具调用参数会被尽力恢复，并记录在 `AssistantMessage.Diagnostics` 中，而不会使响应失败——在执行有副作用的工具前，请务必检查诊断。参见[读取响应 § 诊断](results.md#诊断)。
