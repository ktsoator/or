# 推理输出

## 用途

设置中立推理等级，并将 thinking 与最终答案分开渲染。

## 核心代码

```go
requested := llm.ModelThinkingHigh
effective := llm.ClampThinkingLevel(model, requested)

events, err := llm.Stream(ctx, model,
	llm.Prompt("Solve 37 * 48 and check the result."),
	llm.StreamOptions{Reasoning: effective})
if err != nil {
	log.Fatal(err)
}

for event := range events {
	switch event.Type {
	case llm.EventThinkingDelta:
		fmt.Fprint(os.Stderr, event.Delta)
	case llm.EventTextDelta:
		fmt.Print(event.Delta)
	case llm.EventError:
		log.Print(event.Err)
	}
}
```

Anthropic 可隐藏返回的 thinking 文本，同时保留后续轮次所需签名：

```go
options := llm.StreamOptions{
	Reasoning: llm.ModelThinkingHigh,
	ProtocolOptions: &llm.AnthropicStreamOptions{
		ThinkingDisplay: llm.ThinkingDisplayOmitted,
	},
}
```

`ThinkingDisplayOmitted` 不关闭推理。thinking token 仍计入输出 usage。
