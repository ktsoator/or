# or

从意图到行动，自由选择路径。

`or` 是一个模块化的 Go 工具包，用于构建基于语言模型和上层 agent 的应用。最底层是一个
不绑定特定厂商的 LLM 包：无论底层更换模型还是切换通信协议，对话、工具、推理和流式事件
这套接口都保持稳定。agent 包在其之上实现工具调用循环、状态管理和流式事件。

## 为什么选择 `or`

- 一套对话模型即可覆盖 OpenAI 兼容和 Anthropic 兼容的多家提供方。
- 文本、推理、工具调用、用量和错误均通过类型化事件流式输出。
- 用 Go 结构体定义工具，并自动校验模型生成的参数。
- 自动保留多轮推理和工具调用所需的提供方元数据。
- 在不同轮次之间切换模型，无需重建对话历史。
- 新增自定义模型协议，且无需扩张共享的请求 API。
- 运行自主的多步工具循环，支持流式事件、运行途中干预，以及逐轮切换模型。

## 软件包

| 软件包 | 状态 | 说明 |
|---|---|---|
| [`or/llm`](llm/README.md) | 可用 | 统一的模型访问、流式、工具、推理、图像与对话历史 |
| [`or/agent`](agent/README.md) | 可用 | 带工具、流式事件、干预、追加消息和中止能力的有状态 agent 循环 |

## 安装

```sh
go get github.com/ktsoator/or/llm@latest
```

## 第一个请求

```go
model := llm.GetModel("anthropic", "claude-opus-4-8")
msg, err := llm.Complete(ctx, model, llm.Prompt("Hello"), llm.StreamOptions{})
```

完整的导出类型和函数，参见
[pkg.go.dev](https://pkg.go.dev/github.com/ktsoator/or/llm) 上的包文档。
