# 快速开始

## 安装

新建一个 Go 应用并安装 agent 包：

```sh
mkdir myagent
cd myagent
go mod init myagent
go get github.com/ktsoator/or/agent@latest
```

agent 通过 `or/llm` 包驱动模型，因此需要注册一个协议适配器。导入你的模型所用协议对应的
provider 包——通常用空导入——并设置该 provider 所需的 API key：

```sh
export DEEPSEEK_API_KEY=your-deepseek-api-key
```

```go
import (
	_ "github.com/ktsoator/or/llm/openai" // 注册 OpenAI 兼容协议（DeepSeek、Groq、xAI…）
)
```

DeepSeek、Groq、xAI 等 OpenAI 兼容厂商用 `llm/openai`；Anthropic 与 MiniMax 用
`llm/anthropic`。要一次性注册全部内置协议，导入 `llm/all`。完整对照见
[提供方与模型](../llm/providers.md)。

## 创建第一个 agent

`agent.New` 构建一个有状态的 agent；`Prompt` 把一个任务跑到完成，并把结果追加进 agent
的 transcript。一次运行是完整的工具调用循环，而不是单次模型调用：agent 流式生成一轮、
执行模型请求的工具、追加结果，并持续到模型停止。

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

type weatherArgs struct {
	City string `json:"city" jsonschema:"description=City to look up,minLength=1"`
}

func main() {
	weather := agent.AgentTool{
		Definition: llm.MustTool[weatherArgs]("get_weather", "Get the current weather for a city"),
		Execute: func(ctx context.Context, callID string, args json.RawMessage, onUpdate func(agent.ToolResult)) (agent.ToolResult, error) {
			var in weatherArgs
			if err := json.Unmarshal(args, &in); err != nil {
				return agent.ToolResult{}, err
			}
			return agent.ToolResult{
				Content: []llm.ToolResultContent{&llm.TextContent{Text: fmt.Sprintf("Sunny, 24°C in %s.", in.City)}},
			}, nil
		},
	}

	assistant := agent.New(agent.Options{
		SystemPrompt: "Call get_weather before answering a weather question.",
		Model:        llm.GetModel("deepseek", "deepseek-v4-flash"),
		Tools:        []agent.AgentTool{weather},
	})

	if err := assistant.Prompt(context.Background(), "What is the weather in Shanghai?"); err != nil {
		log.Fatal(err)
	}

	fmt.Println(assistant.Snapshot().Messages) // 提示、工具调用、工具结果、回答
}
```

`Prompt` 接受字符串、单个 `AgentMessage` 或一个切片。它会阻塞到运行结束，并且仅在运行
以失败或取消告终时返回错误——见[读取结果](#读取结果)。

## 运行的产出

一次运行会把它生成的每条消息按顺序追加进 transcript：用户提示、每一轮 assistant、以及
每个工具结果。`Snapshot` 返回当前状态的只读副本，其中包含完整 transcript：

```go
for _, message := range assistant.Snapshot().Messages {
	_ = message // 用户消息、assistant 消息或工具结果
}
```

transcript 就是 agent 的记忆。下一次 `Prompt` 会在它之上延续，所以多次调用之间对话会不断
累积，直到你 `Reset`。

## 多模态提示

`agent.UserMessage` 用文本加可选图片构建一条用户消息——文本块在前，随后是按顺序排列的
每张图片：

```go
assistant.Prompt(ctx, agent.UserMessage("What is in this picture?",
	llm.ImageContent{Data: base64PNG, MIMEType: "image/png"}))
```

纯文本模型会自动把图片内容降级成占位文本，因此同一条提示发给任何模型都是安全的。

## 读取结果

错误以消息形式传播：失败的一轮会变成一条带非正常停止原因的 assistant 消息，而不是
panic。`Prompt` 把它暴露为 Go error，`Snapshot().ErrorMessage` 保存着最近一次失败的
文本。

```go
if err := assistant.Prompt(ctx, "..."); err != nil {
	log.Printf("run failed: %v", err) // 同样在 assistant.Snapshot().ErrorMessage 里
}
```

## 下一步

- 在[事件与状态](events.md)里实时观察一次运行——文本增量、工具进度、回合边界。
- 在[工具](tools.md)里定义更丰富的工具、流式输出进度、控制执行顺序。
- 在[引导与追加](steering.md)里运行中注入消息，或继续一个已停止的 agent。
