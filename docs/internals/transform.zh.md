# 模型切换

规范的对话历史与厂商无关，但并不是每个模型都能原样接受其中的每一种块。`TransformMessages` 会在适配器转换线路格式之前，把已存储的历史投影成目标模型可安全接收的请求形式。

关键点是：转换发生在每次请求前，不会修改已存储的历史。因此，切到纯文本模型不会永久丢掉图像；从推理模型切走，也不会抹掉原历史里的签名。`Stream` 和 `Complete` 会自动应用它。

## 转换顺序

`TransformMessages` 串起三轮处理，每一轮都返回一个新切片：

```go
func TransformMessages(messages []Message, model Model, normalizeToolCallID func(string) string) []Message {
	transformed := downgradeUnsupportedImages(messages, model)      // (1)!
	transformed = reconcileAssistantHistory(transformed, model, normalizeToolCallID) // (2)!
	return synthesizeOrphanedToolResults(transformed)               // (3)!
}
```

1.  当目标模型的 `Model.Input` 不包含 `Image` 时，把图像块替换成占位文本。连续的图像会合并为一个占位符。
2.  按目标模型重写历史 assistant 轮次：同一模型保留推理和签名，跨模型时删除推理，跨厂商时归一化工具调用 ID。
3.  修复存在 assistant 工具调用但缺少对应工具结果的转录。

## 推理与签名

推理是模型相关的。签名或 redacted 载荷只有在 provider、protocol 和 model ID 都匹配时才适合重放——也就是 `sameModel` 条件。`reconcileThinking` 把单个推理块的完整决策编码在一处：

```go
func reconcileThinking(content *ThinkingContent, sameModel bool) AssistantContent {
	if content == nil || !sameModel { // (1)!
		return nil
	}
	if content.Redacted || content.ThinkingSignature != "" { // (2)!
		return content
	}
	if strings.TrimSpace(content.Thinking) == "" { // (3)!
		return nil
	}
	return content // (4)!
}
```

1.  来自其他模型的推理直接删除，避免把它作为普通文本暴露给另一个模型或提供方。
2.  带签名的块对同一模型原样重放，即便它的文本为空；同模型的 redacted 推理也基于相同原因保留。
3.  空的、无签名的推理什么都没带，直接丢弃。
4.  非空、无签名的推理也只对同一模型保留。

文本签名和工具调用签名遵循同样原则：不透明的厂商元数据只保留给产出它的模型。

## 工具调用 ID

不同协议接受的工具调用 ID 形状不同。当 assistant 轮次跨模型重放时，适配器会传入 `normalizeToolCallID` 函数。如果 ID 被改写，这次改写会被记录下来，同一次正向遍历中后续匹配的 `ToolResultMessage.ToolCallID` 也会同步重映射，让转录保持一致。

最后一轮会维护工具协议的不变量：一个 assistant 工具调用批次在进入下一条 user 或 assistant 轮次前，必须每个调用都有一个结果。缺失的结果会被合成为携带 `"No result provided"` 的错误 tool result；而以错误或取消结尾的 assistant 轮次会被整个丢弃，因为它们可能含有半截推理或半流式的工具调用。

## 上下文溢出检测

`IsContextOverflow` 独立于转换逻辑。它检查一次完成或失败的 `AssistantMessage`，识别三种形态的厂商上下文溢出：

```go
func IsContextOverflow(message AssistantMessage, contextWindow int64) bool {
	// Case 1: error message patterns.
	if message.StopReason == StopReasonError && message.ErrorMessage != "" { // (1)!
		if !matchesAny(nonOverflowPatterns, message.ErrorMessage) &&
			matchesAny(overflowPatterns, message.ErrorMessage) {
			return true
		}
	}

	// Case 2: silent overflow (z.ai style) - successful but usage exceeds context.
	if contextWindow > 0 && message.StopReason == StopReasonStop { // (2)!
		if message.Usage.Input+message.Usage.CacheRead > contextWindow {
			return true
		}
	}

	// Case 3: length-stop overflow (Xiaomi MiMo style) - input truncated to fill
	// the window, leaving no room for output.
	if contextWindow > 0 && message.StopReason == StopReasonLength && message.Usage.Output == 0 { // (3)!
		inputTokens := message.Usage.Input + message.Usage.CacheRead
		if float64(inputTokens) >= float64(contextWindow)*0.99 {
			return true
		}
	}

	return false
}
```

1.  多数厂商返回的错误文本会匹配已知的溢出措辞。`nonOverflowPatterns` 排除了限流之类的相似串。
2.  有些厂商（如 z.ai）请求成功，却报告 usage 超过窗口；传入非零的 `contextWindow` 才能捕获这种情况。
3.  另一些厂商（如小米 MiMo）把超长输入截断以填满窗口，然后以 length 停止且输出为零。

把 `contextWindow` 传 `0` 时，只检查错误文本（第一种情况）。

源码：[`llm/transform.go`](https://github.com/ktsoator/or/blob/main/llm/transform.go)、[`llm/overflow.go`](https://github.com/ktsoator/or/blob/main/llm/overflow.go)。
