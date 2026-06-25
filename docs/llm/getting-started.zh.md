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
处理，则改用 [`llm.Stream`](streaming.md)。包级函数使用的客户端内置了两种协议适配器；
`llm.NewClient` 可创建一个使用相同适配器、但相互隔离的客户端。

## 下一步

- 从[提供方目录](providers.md)中选择一个模型。
- 用[流式事件](streaming.md)增量渲染响应。
- 用[类型化工具](tools.md)为模型加上结构化能力。
- 浏览可运行的 [`llm` 示例](https://github.com/ktsoator/or/tree/main/example/llm)。
