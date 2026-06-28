# 消息与自定义类型

一次运行操作的是 `AgentMessage` 值，而不是原始 `llm` 消息。这让应用能把自己的仅 UI 条目
保留在 transcript 里——通知、分隔符、状态横幅——与模型真正交换的消息并存。

## 两类消息

`AgentMessage` 是一个 sealed 接口，有两种实现：

- **适配后的 `llm` 消息。** `FromLLM` 包装一个标准的 `llm.UserMessage`、
  `llm.AssistantMessage` 或 `llm.ToolResultMessage`。这是常见路径，因为 agent 包无法给
  `llm` 拥有的类型添加方法。
- **你自己的类型。** 一个嵌入了 `agent.Custom` 的结构体即满足 `AgentMessage`，无需引用
  该接口未导出的标记方法。

```go
prompt := agent.FromLLM(llm.UserText("Refactor the parser."))
```

`agent.UserMessage` 是“文本加图片”这一高频场景的快捷方式：

```go
msg := agent.UserMessage("What is in this picture?",
	llm.ImageContent{Data: base64PNG, MIMEType: "image/png"})
```

## 仅 UI 消息

嵌入 `agent.Custom`，定义一种活在 transcript 和事件流里、但不属于模型对话的消息：

```go
type Notice struct {
	agent.Custom
	Text string
}

assistant := agent.New(agent.Options{
	Model:    model,
	Messages: []agent.AgentMessage{Notice{Text: "session resumed"}}, // 保留，不发送
})
```

一个 `Notice` 会出现在 `Snapshot().Messages` 里，并流经 `MessageStart` / `MessageEnd`
事件，所以你的 UI 能渲染它——但默认投影会在模型看到对话之前把它丢弃。

## 投影到模型

`ConvertToLLM` 把 transcript 投影成一次请求所需的 `llm.Message` 值。默认实现解开
`FromLLM` 消息、丢弃其它每一个 `AgentMessage`，所以自定义消息留在历史里却永不抵达模型。

提供你自己的 `ConvertToLLM` 来亲自投影自定义消息——例如把一个 `Notice` 渲染成模型应当
看到的系统备注：

```go
assistant := agent.New(agent.Options{
	Model: model,
	ConvertToLLM: func(messages []agent.AgentMessage) []llm.Message {
		out := make([]llm.Message, 0, len(messages))
		for _, m := range messages {
			switch v := m.(type) {
			case Notice:
				out = append(out, llm.UserText("[system] "+v.Text))
			default:
				if std, ok := agent.ToLLM(m); ok { // 解开 FromLLM 消息
					out = append(out, std)
				}
			}
		}
		return out
	},
})
```

投影在每一轮的请求边界、`TransformContext` 之后运行，所以它总能看到当前 transcript。

## 持久化 transcript

`FromLLM` 包装的消息持有标准 `llm` 消息，它们序列化为自描述 JSON，可对任意模型重放。
自定义消息是你自己的类型：要持久化并恢复它们，给它们一个 `type` 判别字段，并在你应用的
存储层注册一个解码器——agent 包本身不做任何持久化。
