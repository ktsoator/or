# 自定义协议

内置适配器覆盖了 OpenAI 兼容和 Anthropic 兼容的端点。要支持一种不同的线缆协议，
实现 `ProtocolAdapter` 并把它注册到客户端上。

一个适配器实现两个方法：`Protocol` 返回它的注册表键，`Stream` 把提供方响应翻译成
包事件。`StreamWriter` 提供与内置适配器相同的生命周期机制：一个 `EventStart`、
非终止事件上的 `Partial` 快照、恰好一个终止事件，以及报告为 `StopReasonAborted`
的取消。

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

把它与内置协议一并注册：

```go
registry := llm.NewRegistry()
llm.RegisterBuiltins(registry)
if err := registry.Register(myAdapter{http: http.DefaultClient}); err != nil {
	log.Fatal(err)
}
client := llm.NewClientWithRegistry(registry)

model := llm.Model{
	ID: "x", Provider: "me", Protocol: "my-protocol", MaxTokens: 1024,
}
message, err := client.Complete(ctx, model, input, llm.StreamOptions{})
```

适配器负责双向翻译：构建线缆请求、对响应分帧、更新用量和停止原因，以及发出增量。
`CloneToolCall` 为事件深拷贝工具调用。`ParseToolArgumentsMode` 提供与内置适配器
相同的不完整 JSON 恢复能力。

## 自定义协议选项

具有协议特定语义的设置可以使用这个共享扩展点，而无需改动 `StreamOptions`：

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

`Client.Stream` 会校验 `ProtocolOptions.Protocol()` 与目标模型匹配，然后在调用
适配器之前调用 `Validate`。
