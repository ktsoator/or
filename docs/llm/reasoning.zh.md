# 推理与思考

`StreamOptions.Reasoning` 是一个与厂商无关的推理强度。每个适配器会将它映射到目标
提供方的原生形式——Anthropic 的自适应或预算思考，或 OpenAI 兼容的推理字段——并将其
限制在所选模型支持的级别范围内。非推理模型会忽略它。

```go
options := llm.StreamOptions{Reasoning: llm.ModelThinkingHigh}
```

可选的强度级别为：

- `ModelThinkingOff`
- `ModelThinkingMinimal`
- `ModelThinkingLow`
- `ModelThinkingMedium`
- `ModelThinkingHigh`
- `ModelThinkingXHigh`

`SupportedThinkingLevels` 返回某个模型支持的级别。`ClampThinkingLevel` 会将请求的
级别调整为最接近的受支持级别。

## Anthropic 思考显示

在 Anthropic 协议上，`ThinkingDisplay` 控制推理内容如何返回，但不改变模型是否进行
推理。`ThinkingDisplayOmitted` 会隐去思考文本，同时保留多轮工具调用所需的签名，适用于
不能展示推理内容的应用。

```go
options := llm.StreamOptions{
	Reasoning: llm.ModelThinkingHigh,
	ProtocolOptions: &llm.AnthropicStreamOptions{
		ThinkingDisplay: llm.ThinkingDisplayOmitted,
	},
}
```

使用 `ThinkingDisplaySummarized` 可请求摘要化的思考。流式过程中，可见的推理会在答案
文本之前，依次通过 `EventThinkingStart`、`EventThinkingDelta` 和 `EventThinkingEnd`
到达。

## 对话连续性

提供方所需的推理元数据——例如 Anthropic 签名和 OpenRouter 加密推理——会保留在
assistant 消息中，并在后续工具调用需要时重放。当目标模型发生变化时，本库会依据兼容性
保留、降级或省略推理内容。模型切换与持久化详见[对话](conversations.md)。
