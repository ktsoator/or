# 请求推理内容

`StreamOptions.Reasoning` 用于请求模型以指定等级进行推理。流式响应可将推理内容与最终文本分开处理。

推理等级只表达请求偏好，模型服务不保证返回可见的推理文本。模型可能完成推理，但只返回最终答案。

## 适用范围

| 需求 | 处理方式 |
|---|---|
| 请求模型投入更多或更少推理 | 设置 `StreamOptions.Reasoning` |
| 实时区分推理内容与最终答案 | 分别处理 thinking 和 text 事件 |
| 只需要最终答案 | 使用 `Complete`；仍可设置 `Reasoning` |
| 隐藏 Anthropic 的可见推理 | 设置 `AnthropicStreamOptions.ThinkingDisplay` |
| 获取精确推理 token | 当前统一 `Usage` 没有独立字段；以模型服务返回的数据为准 |

推理适合数学、代码分析、复杂规划和多步判断，但更高等级通常会增加延迟和 token 消耗。简单提取、格式转换或分类任务不一定受益。

## 运行前准备

```sh
go get github.com/ktsoator/or/llm@latest
export DEEPSEEK_API_KEY=your-key
```

## 完整程序

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func main() {
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	requested := llm.ModelThinkingHigh
	effective := llm.ClampThinkingLevel(model, requested)
	fmt.Fprintf(os.Stderr, "supported=%v requested=%s effective=%s\n",
		llm.SupportedThinkingLevels(model), requested, effective)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	events, err := llm.Stream(ctx, model,
		llm.Prompt("Solve 37 * 48 and verify the result."),
		llm.StreamOptions{Reasoning: effective, MaxTokens: 1000})
	if err != nil {
		log.Fatal(err)
	}

	var final *llm.AssistantMessage
	var streamErr error
	for event := range events {
		switch event.Type {
		case llm.EventThinkingStart:
			fmt.Fprintln(os.Stderr, "--- thinking ---")
		case llm.EventThinkingDelta:
			fmt.Fprint(os.Stderr, event.Delta)
		case llm.EventTextStart:
			fmt.Println("--- answer ---")
		case llm.EventTextDelta:
			fmt.Print(event.Delta)
		case llm.EventDone:
			final = event.Message
		case llm.EventError:
			final, streamErr = event.Message, event.Err
		}
	}
	if streamErr != nil {
		if final != nil {
			log.Printf("partial answer: %q", final.Text())
		}
		log.Fatal(streamErr)
	}
	if final == nil {
		log.Fatal("stream closed without a terminal message")
	}
	fmt.Printf("\nstop=%s output tokens=%d\n",
		final.StopReason, final.Usage.Output)
}
```

推理增量写入标准错误，最终答案写入标准输出，便于调用方分别展示或重定向。程序仍会读取事件直到通道关闭。

## 选择推理等级

`SupportedThinkingLevels` 返回所选模型声明的可用等级；`ClampThinkingLevel` 将用户选择转换为该模型可接受的值。应用应同时记录请求值和实际值，避免界面显示与最终请求不一致。等级列表、贴合顺序和 token 语义统一见[推理配置](../reasoning.md#推理强度)。

协议适配器会将有效等级转换为对应接口的字段。只有类型化配置无法表达目标服务必需的功能时，才使用 `RewriteRequest` 修改请求体。

## 读取推理事件

示例在 `EventThinkingDelta` 中输出新增推理文本，在 `EventTextDelta` 中输出最终答案，并通过 `EventDone` 或 `EventError` 保存消息。推理块与文本块可能出现多个；需要分别更新界面区域时，使用 `ContentIndex` 关联内容块。

最终 `AssistantMessage.Content` 中的 `ThinkingContent` 还可能包含签名或脱敏标记，不应只保存屏幕上显示的推理文本。完整事件字段见[流式事件](../streaming.md#事件参考)。

## 控制可见推理内容

Anthropic Messages 可隐藏可见的推理内容，同时保留继续对话所需的签名。选项、代码和 token 语义见[推理配置 § Anthropic 思考显示](../reasoning.md#anthropic-思考显示)。

`ThinkingDisplayOmitted` 只控制返回什么内容，不改变模型是否推理或如何计费。当前只有 Anthropic 协议实现此显示选项；把 Anthropic 专用选项传给其他协议会在请求开始前失败。

## Token、结果与历史

- `Usage.Output` 记录模型服务报告的输出用量。某些服务把推理消耗计入其中，但统一 `Usage` 没有单独的推理 token 字段。
- `MaxTokens` 与推理预算的关系由模型服务决定。推理消耗较大时，最终答案可能更短或因输出上限结束。
- 后续仍使用同一模型时，应保存完整 `AssistantMessage`，以保留继续对话或工具调用所需的推理签名。
- 更换模型时，`TransformMessages` 会删除前一模型专有的推理内容，不会把它作为普通文本发送给新模型。

## 使用边界

- Thinking 文本和签名可能含敏感信息，默认不要记录。
- 更换模型时，模型服务专有的推理内容会被删除，不会重放给另一个模型。
- 部分模型服务会将推理消耗计入输出上限。
- 内置模型清单中的推理信息可能过期，应对选定模型进行实际验证。
- 不要将可见推理当作事实来源或审计依据；应用应验证最终答案和工具参数。
- 面向终端用户展示推理内容前，应明确产品策略、访问权限和保留期限。
