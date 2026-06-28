# 协议适配器

适配器是 `llm` 包的边界。在适配器之前，所有类型都是与厂商无关的；进入适配器之后，
代码就可以面向某一种具体线缆协议。内置适配器有两个：

- `openai-completions`：面向 OpenAI 兼容的 Chat Completions 端点。
- `anthropic-messages`：面向 Anthropic 兼容的 Messages 端点。

## 适配器契约

```go
type ProtocolAdapter interface {
	Protocol() Protocol
	Stream(ctx context.Context, model Model, input Context, options StreamOptions) (<-chan Event, error)
}
```

`Protocol()` 是注册表里的键。`Stream()` 负责一种协议的一次完整请求：校验模型，把消息
和工具转换成厂商格式，构造 SDK 参数，启动 goroutine，并在 provider 的流到达时发出
统一的包内事件。

## 注册表分派

`Client.Stream` 本身并不知道 OpenAI 或 Anthropic 的细节。它只按 `model.Protocol` 从
注册表取出适配器，必要时从 provider 环境变量补 API key，然后委托出去：

```go
adapter, ok := c.registry.Get(model.Protocol)
return adapter.Stream(ctx, model, input, options)
```

`llm.NewClient(registry)` 基于一个注册表构建 client。内置的 provider 包
（`llm/openai`、`llm/anthropic`）在 `init` 函数里把各自的适配器注册进包级默认注册表，
因此只要导入某个 provider（或导入 `llm/all` 一次性注册全部），它的协议就能被
`llm.Stream` 和 `llm.Complete` 使用。自定义 client 可以继续注册新的适配器，而不需要
改变 `StreamOptions` 或共享的对话类型。

## 适配器翻译什么

两个内置适配器都遵循同一条路径：

1. 校验 `model.Protocol` 和 `model.Compatibility` 是否匹配当前适配器。
2. 序列化历史前先调用 `TransformMessages`，为目标模型适配 history。
3. 把中立的 content block 转成 provider 请求消息格式。
4. 把 `ToolDefinition` 包装成 provider 原生工具 schema。
5. 把推理、最大 token 等中立选项映射到 provider 字段。
6. 消费 provider 流，并重建 `AssistantMessage` 与事件流。

协议特有的开关都放在 `StreamOptions.ProtocolOptions` 里。OpenAI 兼容模型接受
`OpenAICompletionsStreamOptions`；Anthropic 模型接受 `AnthropicStreamOptions`。
共享的 options 校验会在发 HTTP 请求之前拒绝协议不匹配的配置。

## 兼容厂商

`Model.BaseURL`、`Model.Headers` 和 `Model.Compatibility` 让非参考厂商也能复用同一个
适配器。例如，OpenAI 兼容厂商可以把 `BaseURL` 指向自己的端点，并用兼容性字段描述
`max_tokens` 与 `max_completion_tokens`、严格工具支持、reasoning 字段名等差异。

因此，新增一个兼容厂商通常只是 catalog 改动，而不是新增适配器。只有真正不同的线缆协议
才需要新的 `ProtocolAdapter`。

源码：[`llm/adapters.go`](https://github.com/ktsoator/or/blob/main/llm/adapters.go)、
[`llm/`](https://github.com/ktsoator/or/tree/main/llm/)。
