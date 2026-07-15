# 自定义协议

内置适配器覆盖了 OpenAI 兼容和 Anthropic 兼容的端点。要支持另一种通信协议，实现 `ProtocolAdapter` 并将它注册到客户端上即可。

一个适配器实现两个方法：`Protocol` 返回它的注册表键，`Stream` 将提供方响应翻译成包事件。`StreamWriter` 提供与内置适配器相同的生命周期机制：一个 `EventStart`、非终止事件上的 `Partial` 快照、恰好一个终止事件，以及报告为 `StopReasonAborted` 的取消。

**1. 声明适配器及其注册表键。** `Protocol` 返回客户端用来把模型路由到本适配器的键。

```go
type myAdapter struct{ http *http.Client }

func (myAdapter) Protocol() llm.Protocol { return "my-protocol" }
```

**2. 准备响应消息和一个 `StreamWriter`。** `Stream` 会立即返回通道，实际工作在 goroutine 中进行。`NewStreamWriter` 会发出开头的 `EventStart` 并跟踪 `Partial` 快照。

```go
events := make(chan llm.Event)
go func() {
	defer close(events)

	message := llm.AssistantMessage{
		Protocol: model.Protocol,
		Provider: model.Provider,
		Model:    model.ID,
	}
	writer := llm.NewStreamWriter(ctx, events, &message)
```

**3. 调用端点；失败时通过 writer 上报。** `writer.Fail` 会发出唯一的终止事件 `EventError`，使失败的请求仍能正确关闭流。

```go
	reply, usage, err := callMyEndpoint(ctx, a.http, model, input, options)
	if err != nil {
		writer.Fail(err)
		return
	}
```

**4. 发出内容块的生命周期事件。** 把内容块追加进 `message.Content`，再依次发出 start 事件、每个分片的 delta，以及携带最终文本的 end 事件。

```go
	text := &llm.TextContent{}
	message.Content = append(message.Content, text)
	writer.Emit(llm.Event{Type: llm.EventTextStart, ContentIndex: 0})
	for chunk := range reply {
		text.Text += chunk
		writer.Emit(llm.Event{
			Type: llm.EventTextDelta, ContentIndex: 0, Delta: chunk,
		})
	}
	writer.Emit(llm.Event{
		Type: llm.EventTextEnd, ContentIndex: 0, Content: text.Text,
	})
```

**5. 记录用量与停止原因，然后收尾。** `writer.Done` 发出唯一的终止事件 `EventDone`，携带组装好的消息。

```go
	message.Usage = usage
	message.StopReason = llm.StopReasonStop
	writer.Done()
}()
return events, nil
```

<details>
<summary>完整适配器</summary>

```go
type myAdapter struct{ http *http.Client }

func (myAdapter) Protocol() llm.Protocol { return "my-protocol" }

func (a myAdapter) Stream(
	ctx context.Context,
	model llm.Model,
	input llm.Context,
	options llm.StreamOptions,
) (<-chan llm.Event, error) {
	events := make(chan llm.Event)
	go func() {
		defer close(events)

		message := llm.AssistantMessage{
			Protocol: model.Protocol,
			Provider: model.Provider,
			Model:    model.ID,
		}
		writer := llm.NewStreamWriter(ctx, events, &message)

		reply, usage, err := callMyEndpoint(ctx, a.http, model, input, options)
		if err != nil {
			writer.Fail(err)
			return
		}

		text := &llm.TextContent{}
		message.Content = append(message.Content, text)
		writer.Emit(llm.Event{Type: llm.EventTextStart, ContentIndex: 0})
		for chunk := range reply {
			text.Text += chunk
			writer.Emit(llm.Event{
				Type: llm.EventTextDelta, ContentIndex: 0, Delta: chunk,
			})
		}
		writer.Emit(llm.Event{
			Type: llm.EventTextEnd, ContentIndex: 0, Content: text.Text,
		})

		message.Usage = usage
		message.StopReason = llm.StopReasonStop
		writer.Done()
	}()
	return events, nil
}
```

</details>

注册它并构建 client：

```go
registry := llm.NewAdapterRegistry()
if err := registry.Register(myAdapter{http: http.DefaultClient}); err != nil {
	log.Fatal(err)
}
// 第二个参数传 nil 保留环境变量查 key 的行为；
// 传 llm.NewBuiltInProviderRegistry() 则同时应用 provider override。
client := llm.NewClient(registry, nil)

model := llm.Model{
	ID: "x", Provider: "me", Protocol: "my-protocol", MaxTokens: 1024,
}
message, err := client.Complete(ctx, model, input, llm.StreamOptions{})
```

若想让同一个 client 也支持内置协议，把 `openai.NewAdapter(nil)` 和 `anthropic.NewAdapter(nil)`（来自 `github.com/ktsoator/or/llm/openai` 和 `github.com/ktsoator/or/llm/anthropic`）一并注册进注册表。

适配器负责双向翻译：构建底层请求、切分响应、更新用量和停止原因，以及发出增量。 `CloneToolCall` 为事件深拷贝工具调用。`ParseToolArgumentsMode` 提供与内置适配器相同的不完整 JSON 恢复能力。

## 自定义协议选项

具有协议特定语义的设置，可以使用这个共享扩展点，而无需改动 `StreamOptions`：

```go
type myProtocolOptions struct {
	SafetyMode string
}

func (*myProtocolOptions) Protocol() llm.Protocol { return "my-protocol" }

func (options *myProtocolOptions) Validate(_ []llm.ToolDefinition) error {
	if options.SafetyMode == "" {
		return errors.New("safety mode is required")
	}
	return nil
}

options := llm.StreamOptions{
	ProtocolOptions: &myProtocolOptions{SafetyMode: "strict"},
}
```

`Client.Stream` 会先校验 `ProtocolOptions.Protocol()` 与目标模型匹配，再在调用适配器之前调用 `Validate`。

若服务已经实现 OpenAI Chat Completions 或 Anthropic Messages，不应再创建新
协议 adapter。应构造使用对应内置协议的 `Model`，并设置端点兼容字段。显式
装配、自定义 transport 与独立注册表见 [Client 与注册表](clients-and-registries.md)。
