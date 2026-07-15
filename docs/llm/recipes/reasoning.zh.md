# 推理输出

## 本场景实现什么

这个流式命令请求最接近的受支持推理等级，把 thinking 与答案文本分开展示，并读取最终 usage。

推理等级是请求提示，不保证 provider 会返回可见 thinking 文本。不同 provider 使用不同原生控制；模型可以完成推理但不展示过程。

## 完整程序

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func main() {
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	requested := llm.ModelThinkingHigh
	effective := llm.ClampThinkingLevel(model, requested)
	fmt.Fprintf(os.Stderr, "supported=%v requested=%s effective=%s\n",
		llm.SupportedThinkingLevels(model), requested, effective)

	events, err := llm.Stream(context.Background(), model,
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
		log.Fatal(streamErr)
	}
	if final != nil {
		fmt.Printf("\noutput tokens=%d\n", final.Usage.Output)
	}
}
```

## 中立等级如何映射

`ModelThinkingLevel` 包含 `off`、`minimal`、`low`、`medium`、`high` 和 `xhigh`。`SupportedThinkingLevels` 根据模型声明返回等级，`ClampThinkingLevel` 选择最近的可用值；非推理模型实际上会忽略请求。

Adapter 把有效等级转换为协议原生字段。除非类型化映射无法表达目标服务必需的功能，否则不要通过 `RewriteRequest` 手写 provider reasoning JSON。

## Anthropic 展示控制

Anthropic 协议可以隐藏可见 thinking，同时保留推理行为和继续对话所需的签名。选项定义、代码和 token 语义见[推理与思考 § Anthropic 思考显示](../reasoning.md#anthropic-思考显示)，本场景不再复制协议配置。

## 边界

- Thinking 文本和签名可能含敏感信息，默认不要记录。
- 更换模型时，provider-specific reasoning block 会被删除，而不是重放给另一个模型。
- 某些 provider 的输出上限包含推理消耗。
- 目录中的 reasoning metadata 可能过期，应对选定的模型服务做实际验证。
