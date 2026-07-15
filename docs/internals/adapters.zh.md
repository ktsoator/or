# 协议适配器

适配器是 `llm` 包的边界。在适配器之前，所有类型都是与厂商无关的；进入适配器之后，代码就可以面向某一种具体线路协议。内置适配器有两个：

- `openai-completions`：面向 OpenAI 兼容的 Chat Completions 端点。
- `anthropic-messages`：面向 Anthropic 兼容的 Messages 端点。

## 适配器契约

```go
type ProtocolAdapter interface {
	// Protocol returns the registry key used to select this adapter.
	Protocol() Protocol

	// Stream emits response events for the given model and conversation context.
	Stream(ctx context.Context, model Model, input Context, options StreamOptions) (<-chan Event, error)
}
```

`Protocol()` 是注册表里的键。`Stream()` 负责一种协议的一次完整请求：校验模型，迁移并转换历史，构造 SDK 参数，启动 goroutine，并在厂商的流到达时发出统一的包内事件。

## 注册表分派

`Client.Stream` 本身并不知道 OpenAI 或 Anthropic 的细节。它只按 `model.Protocol` 从注册表取出适配器，然后委托出去。注册表是一个并发安全的 map；`Register` 为某个协议添加或替换适配器：

```go
func (registry *AdapterRegistry) Register(adapter ProtocolAdapter) error {
	if adapter == nil {
		return errors.New("protocol adapter is nil")
	}

	protocol := adapter.Protocol()
	if protocol == "" {
		return errors.New("protocol adapter protocol is empty")
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.adapters[protocol] = adapter
	return nil
}
```

## 导入即注册

内置的厂商包在 `init` 函数里把各自的适配器注册进包级默认注册表，因此只要导入某个厂商（或导入 `llm/all` 一次性注册全部），它的协议就能被 `llm.Stream` 和 `llm.Complete` 使用：

```go
func init() {
	if err := llm.Register(NewAdapter(nil)); err != nil {
		panic(err)
	}
}
```

偏好显式接线的调用方可以跳过默认注册表，用 `NewAdapterRegistry` 自建一个，把适配器注册进去，再传给 `NewClient`。

## 构建 SDK client

`Stream` 把中立的 `StreamOptions` 映射到厂商 SDK 的请求选项上。Anthropic 适配器里的 `buildClient` 展示了这个形状：base URL、重试、超时变成 SDK 选项，而观测钩子变成 middleware：

```go
func buildClient(httpClient *http.Client, model llm.Model, options llm.StreamOptions) sdk.Client {
	clientOptions := []option.RequestOption{
		option.WithAPIKey(options.APIKey),
	}
	if httpClient != nil {
		clientOptions = append(clientOptions, option.WithHTTPClient(httpClient))
	}
	if model.BaseURL != "" { // (1)!
		clientOptions = append(clientOptions, option.WithBaseURL(model.BaseURL))
	}
	if options.MaxRetries != nil {
		clientOptions = append(clientOptions, option.WithMaxRetries(*options.MaxRetries))
	}
	if options.Timeout > 0 {
		clientOptions = append(clientOptions, option.WithRequestTimeout(options.Timeout))
	}
	if options.OnRequest != nil { // (2)!
		clientOptions = append(clientOptions, option.WithMiddleware(onRequestMiddleware(options.OnRequest)))
	}
	if options.RewriteRequest != nil {
		clientOptions = append(clientOptions, option.WithMiddleware(rewriteRequestMiddleware(options.RewriteRequest)))
	}
	if options.OnResponse != nil {
		clientOptions = append(clientOptions, option.WithMiddleware(onResponseMiddleware(options.OnResponse)))
	}
	for name, value := range mergedHeaders(model, options) { // (3)!
		clientOptions = append(clientOptions, option.WithHeader(name, value))
	}
	return sdk.NewClient(clientOptions...)
}
```

1.  `BaseURL` 正是让兼容厂商能把这个适配器复用到自己端点上的东西。
2.  `OnRequest`、`RewriteRequest`、`OnResponse` 作为 SDK middleware 安装，因此每次尝试各触发一次——包括重试——而 `RewriteRequest` 可以在请求体发送前对其打补丁。
3.  请求头会覆盖同名的模型默认头。

## 适配器翻译什么

两个内置适配器都遵循同一条路径：

1. 校验 `model.Protocol` 和 `model.Compatibility` 是否匹配当前适配器。
2. 序列化历史前先调用 `TransformMessages`，为目标模型适配历史。
3. 把中立的内容块转成厂商请求消息格式。
4. 把 `ToolDefinition` 包装成厂商原生工具 schema。
5. 把推理、最大 token 等中立选项映射到厂商字段。
6. 消费厂商流，并重建 `AssistantMessage` 与事件流。

协议特有的开关都放在 `StreamOptions.ProtocolOptions` 里。OpenAI 兼容模型接受 `OpenAICompletionsStreamOptions`；Anthropic 模型接受 `AnthropicStreamOptions`。共享的 options 校验会在发出任何 HTTP 请求之前拒绝协议不匹配的配置。

## 兼容厂商

`Model.BaseURL`、`Model.Headers` 和 `Model.Compatibility` 让非参考厂商也能复用同一个适配器。例如，OpenAI 兼容厂商可以把 `BaseURL` 指向自己的端点，并用兼容性字段描述 `max_tokens` 与 `max_completion_tokens`、严格工具支持、reasoning 字段名等差异。

因此，新增一个兼容厂商通常只是更新模型清单，而不是新增适配器。只有请求与响应格式真正不同的服务才需要新的 `ProtocolAdapter`。

源码：[`llm/adapters.go`](https://github.com/ktsoator/or/blob/main/llm/adapters.go)、[`llm/anthropic/adapter.go`](https://github.com/ktsoator/or/blob/main/llm/anthropic/adapter.go)、[`llm/openai/`](https://github.com/ktsoator/or/tree/main/llm/openai)。
