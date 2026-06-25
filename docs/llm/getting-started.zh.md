# 快速开始

## 安装

创建一个 Go 应用并安装本包：

```sh
mkdir myapp
cd myapp
go mod init myapp
go get github.com/ktsoator/or/llm@latest
```

本包会从进程环境中读取所选提供方的 API key。例如：

```sh
export DEEPSEEK_API_KEY=your-deepseek-api-key
```

本地开发时，可以用 [`godotenv`](https://github.com/joho/godotenv) 这类 `.env`
加载器在首次请求前加载凭证。记得把 `.env` 加入 `.gitignore`；生产环境应通过部署
环境注入凭证。

## 完成一次请求

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
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

`llm.Complete` 会把整个流收集成一个 `AssistantMessage`。当应用需要随到随处理
增量时，请使用 [`llm.Stream`](streaming.md)。包级函数使用的客户端内置了两种协议
适配器；`llm.NewClient` 会创建一个拥有相同适配器的独立客户端。

## 下一步

- 从[提供方目录](providers.md)中选择一个模型。
- 用[流式事件](streaming.md)增量渲染响应。
- 用[类型化工具](tools.md)赋予模型结构化能力。
- 浏览可运行的 [`llm` 示例](https://github.com/ktsoator/or/tree/main/example/llm)。
