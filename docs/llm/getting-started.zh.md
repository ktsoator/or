# 快速开始

## 安装

新建一个 Go 应用并安装本包：

```sh
mkdir myapp
cd myapp
go mod init myapp
go get github.com/ktsoator/or/llm@latest
```

本包会从进程的环境变量中读取所选提供方的 API key。例如：

```sh
export DEEPSEEK_API_KEY=your-deepseek-api-key
```

本地开发时，可以用 [`godotenv`](https://github.com/joho/godotenv) 这类 `.env` 加载器在首次请求前加载凭证。记得将 `.env` 加入 `.gitignore`；生产环境则应通过部署环境注入凭证。

## 完成一次请求

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai" // 注册 OpenAI 兼容协议（DeepSeek、Groq、xAI…）
)

func main() {
	model, ok := llm.LookupModel("deepseek", "deepseek-v4-flash")
	if !ok || !llm.SupportsProtocol(model.Protocol) {
		log.Fatal("model is not runnable")
	}
	response, err := llm.Complete(
		context.Background(),
		model,
		llm.Prompt("Explain Go channels briefly."),
		llm.StreamOptions{},
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(response.Text())
}
```

运行程序：

```sh
go run .
```

`llm.Complete` 等待生成结束，再返回最终的 `AssistantMessage`。如果需要边生成边处理文本、推理或工具调用，使用 [`llm.Stream`](streaming.md)。

### 注册协议适配器

调用包级 `Complete` 或 `Stream` 前，必须导入目标模型所使用协议的适配器包。示例中的空导入 `llm/openai` 会在程序初始化时注册 OpenAI Chat Completions 兼容接口的适配器。使用 Anthropic Messages 兼容接口时导入 `llm/anthropic`；同时需要两种内置协议时可导入 `llm/all`。

按需导入适配器包，可以避免把未使用的 provider SDK 链接进二进制。

### 选择可运行模型

模型出现在内置模型清单中，只表示框架认识该模型；不表示当前进程已经具备调用它的适配器。

- `LookupModel`：检查 provider 和模型 ID 是否存在于内置模型清单。
- `SupportsProtocol`：检查当前进程是否已注册该模型所需的协议适配器。
- `GetRunnableModels`：只返回模型清单中且当前进程能够调用的模型，适合构建模型选择器。
- `GetModels`：返回完整模型清单，其中可能包含当前没有内置适配器的模型，不能直接作为可调用列表。

当前实现状态见[协议与提供方状态](support-matrix.md)。

## 自定义请求

第一个示例发送的是空的 `StreamOptions{}`。用 `PromptWithSystem` 加上 system 提示，并设置温度、输出上限等常用选项。这些选项适用于任意模型，与协议无关。

```go
temperature := 0.2
response, err := llm.Complete(
	context.Background(),
	model,
	llm.PromptWithSystem("You are a concise Go tutor.", "Explain Go channels."),
	llm.StreamOptions{
		Temperature: &temperature,
		MaxTokens:   512,
	},
)
```

完整的选项集合参见[请求选项](configuration.md)。

## 查看用量与成本

每个响应都会报告它消耗的 token 及其成本：

```go
fmt.Printf("tokens=%d cost=$%.6f\n",
	response.Usage.TotalTokens, response.Usage.Cost.Total)
```

停止原因、用量与诊断详见[响应与用量](results.md)。

## 下一步

- 用[消息与上下文](conversations.md)了解消息结构，再按[保存与恢复对话](recipes/conversation-persistence.md)实现多轮会话。
- 从[模型与提供方](providers.md)中选择一个模型。
- 在[协议与提供方状态](support-matrix.md)中确认协议和提供方状态。
- 用[流式事件](streaming.md)增量渲染响应。
- 用[类型化工具](tools.md)为模型加上结构化能力。
- 在[示例](examples.md)页浏览可运行程序。
- 从 [Recipes](recipes/README.md) 按开发任务查找最小代码。
