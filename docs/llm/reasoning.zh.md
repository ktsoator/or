# 推理与思考

`StreamOptions.Reasoning` 是一个与厂商无关的推理强度。每个适配器会将它映射到目标
提供方的原生形式——Anthropic 的自适应或预算思考，或 OpenAI 兼容的推理字段——并将其
限制在所选模型支持的级别范围内。非推理模型会忽略它,因此这个选项可以安全地设在任何
模型上。

```go
options := llm.StreamOptions{Reasoning: llm.ModelThinkingHigh}
response, err := llm.Complete(ctx, model, llm.Prompt("..."), options)
```

## 推理强度

级别越高,模型在作答前可用于思考的 token 越多,以延迟和成本换取在难题上的质量。
`Reasoning` 留空则使用模型自身的默认值。

| 级别 | 效果 |
|---|---|
| `ModelThinkingOff` | 完全关闭思考 |
| `ModelThinkingMinimal` | 最小思考预算 |
| `ModelThinkingLow` | 轻量推理 |
| `ModelThinkingMedium` | 均衡推理 |
| `ModelThinkingHigh` | 面向难题的扩展推理 |
| `ModelThinkingXHigh` | 最大思考预算 |

## 检查模型支持哪些级别

并非每个模型都接受每个级别。`SupportedThinkingLevels` 返回某个模型支持的级别,
`ClampThinkingLevel` 会把请求的级别贴合到最接近的受支持级别。`Stream` 和 `Complete`
会自动贴合,但自行调用它有助于驱动 UI,或在模型不能推理时直接跳过该选项。

```go
levels := llm.SupportedThinkingLevels(model)
if len(levels) == 0 {
	// 模型不支持推理;不要提供该控件。
}

// 把用户的选择贴合到模型能接受的级别。
requested := llm.ModelThinkingXHigh
effective := llm.ClampThinkingLevel(model, requested)
if effective != requested {
	log.Printf("model caps thinking at %s", effective)
}

response, err := llm.Complete(ctx, model, input, llm.StreamOptions{
	Reasoning: effective,
})
```

`Model.Reasoning` 是一个快速判断模型是否具备推理能力的布尔值。

## 读取思考内容

流式过程中,推理会在答案文本之前以独立的块到达——`EventThinkingStart`、
`EventThinkingDelta`、`EventThinkingEnd`——因此你可以把它与最终回复分开渲染。

```go
for event := range events {
	switch event.Type {
	case llm.EventThinkingDelta:
		fmt.Fprint(thinkingPane, event.Delta)
	case llm.EventTextDelta:
		fmt.Fprint(answerPane, event.Delta)
	}
}
```

对于已完成的消息,推理是 `response.Content` 中的一个 `ThinkingContent` 块。`Thinking`
保存文本;`ThinkingSignature` 携带在后续轮次重放的提供方签名;`Redacted` 标记被提供方
隐去的思考。

```go
for _, block := range response.Content {
	if t, ok := block.(*llm.ThinkingContent); ok && !t.Redacted {
		fmt.Println("reasoning:", t.Thinking)
	}
}
```

## Anthropic 思考显示

在 Anthropic 协议上，`ThinkingDisplay` 控制推理内容如何返回，但不改变模型是否进行
推理。留空时默认为摘要化思考。

```go
options := llm.StreamOptions{
	Reasoning: llm.ModelThinkingHigh,
	ProtocolOptions: &llm.AnthropicStreamOptions{
		ThinkingDisplay: llm.ThinkingDisplaySummarized,
	},
}
```

`ThinkingDisplayOmitted` 会隐去思考文本，同时保留多轮工具调用所需的签名。当应用不能
展示推理内容、但后续请求仍需有效历史时使用它。

```go
options := llm.StreamOptions{
	Reasoning: llm.ModelThinkingHigh,
	ProtocolOptions: &llm.AnthropicStreamOptions{
		ThinkingDisplay: llm.ThinkingDisplayOmitted,
	},
}
```

使用 `ThinkingDisplayOmitted` 时,不会有 `EventThinkingDelta` 事件到达,且
`ThinkingContent` 块会被标记为 `Redacted`。

## 对话连续性

提供方所需的推理元数据——例如 Anthropic 签名和 OpenRouter 加密推理——会保留在
assistant 消息中，并在后续工具调用需要时重放。当目标模型发生变化时，本库会依据兼容性
保留、降级或省略推理内容。模型切换与持久化详见[对话](conversations.md)。
