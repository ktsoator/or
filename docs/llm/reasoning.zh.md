# 推理配置

本页定义 `StreamOptions.Reasoning`、模型支持级别、思考内容块和协议专用选项的精确语义。如何在应用中展示、保存和控制推理内容，参见[请求推理内容](recipes/reasoning.md)。

```go
options := llm.StreamOptions{Reasoning: llm.ModelThinkingHigh}
response, err := llm.Complete(ctx, model, llm.Prompt("..."), options)
```

## 速览

| 用途 | API |
|---|---|
| 设置推理强度 | `StreamOptions.Reasoning`（`ModelThinkingLevel`） |
| 可选级别 | `ModelThinkingOff` / `Minimal` / `Low` / `Medium` / `High` / `XHigh` |
| 查询模型支持的级别 | `SupportedThinkingLevels(model)`、`ClampThinkingLevel(model, level)` |
| 是否具备推理能力 | `Model.Reasoning`（bool） |
| 流式读回思考 | `EventThinkingStart` / `Delta` / `End` |
| 从最终消息读回思考 | `ThinkingContent`（`Thinking`、`ThinkingSignature`、`Redacted`） |
| 控制思考如何返回（Anthropic） | `AnthropicStreamOptions.ThinkingDisplay` |

强度只决定"思考多少"。思考文本是否随响应返回则是另一个正交的旋钮:在 Anthropic 上由 `ThinkingDisplay` 控制（见 [Anthropic 思考显示](#anthropic-思考显示)）。

## 推理强度

级别越高，模型在作答前可用于思考的 token 越多，以延迟和成本换取在难题上的质量。`Reasoning` 留空则使用模型自身的默认值。

| 级别 | 效果 | 何时使用 |
|---|---|---|
| `ModelThinkingOff` | 完全关闭思考 | 简单任务,或对延迟/成本敏感的路径 |
| `ModelThinkingMinimal` | 最小思考预算 | 只需一点点推理 |
| `ModelThinkingLow` | 轻量推理 | 大多数日常任务 |
| `ModelThinkingMedium` | 均衡推理 | 稳妥的默认选择 |
| `ModelThinkingHigh` | 面向难题的扩展推理 | 数学、规划、多步推理 |
| `ModelThinkingXHigh` | 最大思考预算 | 最难的问题,不计成本 |

在底层,该级别会映射到各提供方自己的控制:在 Anthropic 上是思考 token 预算（或自适应思考）,在 OpenAI 兼容提供方上是 `reasoning_effort` 字段。中立的级别让代码在两者之间保持一致。

思考消耗的 token 计入 `Usage.Output`，与生成文本按同一输出费率计价，因此级别越高、单次请求越贵。用量与成本详见[响应与用量](results.md#token-用量与成本)。

## 检查模型支持哪些级别

并非每个模型都接受每个级别。`SupportedThinkingLevels` 返回某个模型支持的级别，`ClampThinkingLevel` 会把请求的级别贴合到最接近的受支持级别。`Stream` 和 `Complete` 会自动贴合，但自行调用它有助于驱动 UI，或在模型不能推理时直接跳过该选项。

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

流式过程中，推理会在答案文本之前以独立的块到达（`EventThinkingStart`、`EventThinkingDelta`、`EventThinkingEnd`），因此可将其与最终回复分开渲染。

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

对于已完成的消息，推理是 `response.Content` 中的一个 `ThinkingContent` 块。`Thinking` 保存文本；`ThinkingSignature` 携带在后续轮次重放的提供方签名；`Redacted` 标记被提供方隐去的思考。

```go
for _, block := range response.Content {
	if t, ok := block.(*llm.ThinkingContent); ok && !t.Redacted {
		fmt.Println("reasoning:", t.Thinking)
	}
}
```

## Anthropic 思考显示

在 Anthropic 协议上，`ThinkingDisplay` 控制推理内容如何返回，但不改变模型是否进行推理。留空时默认为摘要化思考。

```go
options := llm.StreamOptions{
	Reasoning: llm.ModelThinkingHigh,
	ProtocolOptions: &llm.AnthropicStreamOptions{
		ThinkingDisplay: llm.ThinkingDisplaySummarized,
	},
}
```

`ThinkingDisplayOmitted` 会隐去思考文本，同时保留多轮工具调用所需的签名。当应用不能展示推理内容、但后续请求仍需有效历史时使用它。

```go
options := llm.StreamOptions{
	Reasoning: llm.ModelThinkingHigh,
	ProtocolOptions: &llm.AnthropicStreamOptions{
		ThinkingDisplay: llm.ThinkingDisplayOmitted,
	},
}
```

使用 `ThinkingDisplayOmitted` 时，不会有 `EventThinkingDelta` 事件到达，且 `ThinkingContent` 块会被标记为 `Redacted`。

## 对话连续性

提供方所需的推理元数据（例如 Anthropic 签名和 OpenRouter 加密推理）会保留在 assistant 消息中，并在后续工具调用需要时重放。这一点对带思考的工具调用尤为重要:一些提供方要求把带签名的思考块原样回传，才会接受下一次工具调用，丢失它可能导致该轮失败。即使用 `ThinkingDisplayOmitted` 隐去了文本，本库仍保留该块，以保证历史有效。当目标模型发生变化时，前一个模型产生的推理会被删除，而不会作为普通文本重放。底层消息规则见[消息与上下文](conversations.md)。
