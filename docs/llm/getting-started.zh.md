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

本地开发时，可以用 [`godotenv`](https://github.com/joho/godotenv) 这类 `.env`
加载器在首次请求前加载凭证。记得将 `.env` 加入 `.gitignore`；生产环境则应通过部署
环境注入凭证。

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
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
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

`llm.Complete` 会将整个流收集成一个 `AssistantMessage`。当应用需要在增量到达时即时
处理，则改用 [`llm.Stream`](streaming.md)。包级函数通过一个默认注册表分发；上面那行空导入
`llm/openai` 会把 OpenAI 兼容协议注册进去。你用到哪个协议就导入对应的 provider 包——或者
导入 `llm/all` 一次性注册全部——这样二进制里只链接你需要的 SDK。

## 自定义请求

第一个示例发送的是空的 `StreamOptions{}`。用 `PromptWithSystem` 加上 system 提示,
并设置温度、输出上限等常用选项。这些选项适用于任意模型,与协议无关。

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

完整的选项集合参见[请求配置](configuration.md)。

## 查看用量与成本

每个响应都会报告它消耗的 token 及其成本:

```go
fmt.Printf("tokens=%d cost=$%.6f\n",
	response.Usage.TotalTokens, response.Usage.Cost.Total)
```

停止原因、用量与诊断详见[读取响应](results.md)。

## 下一步

- 用[对话](conversations.md)把交流延续到多轮。
- 从[提供方目录](providers.md)中选择一个模型。
- 用[流式事件](streaming.md)增量渲染响应。
- 用[类型化工具](tools.md)为模型加上结构化能力。
- 在[示例](examples.md)页浏览可运行程序。
