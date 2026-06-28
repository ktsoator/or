# 配置

agent 有两层设置：描述运行的 agent 级选项（模型、系统提示、工具、推理），以及每一轮
传给模型的基础 `llm.StreamOptions`，用于请求级配置。

## 推理与请求选项

`ThinkingLevel` 设置推理强度，并被裁剪到模型支持的范围。`StreamOptions` 携带共享的请求
设置——温度、输出上限、请求头，以及 HTTP 观察钩子。

```go
temperature := 0.2
assistant := agent.New(agent.Options{
	Model:         model,
	ThinkingLevel: llm.ModelThinkingHigh,
	StreamOptions: llm.StreamOptions{
		Temperature: &temperature,
		MaxTokens:   4096,
		Headers:     map[string]string{"X-Trace": traceID},
		OnRequest:   func(method, url string, body []byte) { log.Println(method, url) },
		RewriteRequest: func(method, url string, body []byte) []byte {
			return patchVendorField(body) // 用于类型化 API 未暴露的字段
		},
	},
})
```

agent 会用 `ThinkingLevel` 填充 `StreamOptions.Reasoning`、用 `GetAPIKey` 填充
`StreamOptions.APIKey`，所以你在这两个字段里放的任何值都会被忽略。`StreamOptions` 中的
其它一切——包括 `OnRequest`、`OnResponse` 和 `RewriteRequest` 钩子——都作用于每一轮。
每个选项的作用见 llm 的[配置](../llm/configuration.md)指南。

## 动态 API 密钥

`GetAPIKey` 在每一轮之前解析 provider 密钥，用于在长运行中可能过期的短时令牌。非空返回
覆盖密钥；空返回则交由环境变量决定。

```go
GetAPIKey: func(provider string) string {
	return currentOAuthToken(provider) // 带外刷新
},
```

## 自定义传输

`StreamFn` 为一轮触达模型，默认是 `llm.Stream`。它主要作为测试与自定义传输的接缝——
录制的固件、代理，或返回预设轮次的假实现。

```go
StreamFn: func(ctx context.Context, model llm.Model, input llm.Context, opts llm.StreamOptions) (<-chan llm.Event, error) {
	return myRecordingClient.Stream(ctx, model, input, opts)
},
```

## 运行间的重新配置

这些 setter 改变下一次运行的配置；它们不会扰动已在进行的运行——后者在启动时就捕获了自己的
配置。所有 setter 都可安全并发调用。

```go
assistant.SetModel(llm.GetModel("minimax-cn", "MiniMax-M3"))
assistant.SetSystemPrompt("Answer in one sentence.")
assistant.SetThinkingLevel(llm.ModelThinkingHigh)
assistant.SetTools([]agent.AgentTool{weatherTool}) // 切片会被拷贝
assistant.SetToolExecution(agent.ExecutionSequential)
```

若要在**单次运行内**切换模型，改用 `PrepareNextTurn`——见
[生命周期钩子](hooks.md#回合间的模型切换)。

`Reset` 清空 transcript、最近错误和两个队列，同时保留以上全部配置，因此下一次运行会以
相同设置开始一段全新对话。
