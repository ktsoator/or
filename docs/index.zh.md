# or

从意图到行动，自由选择路径。

`or` 是一个模块化的 Go 工具包，用于构建基于语言模型和更高层 agent 的应用。
底层的 provider-neutral（与厂商无关）LLM 包让对话、工具、推理和流式事件保持稳定，
即使底下的模型和线缆协议发生变化也不受影响；而 agent 包在其之上构建工具调用循环、
状态管理和流式事件。

## 为什么用 `or`

- 在 OpenAI 兼容和 Anthropic 兼容的多家提供方之间，使用同一套对话模型。
- 通过类型化事件流式输出文本、推理、工具调用、用量和错误。
- 用 Go 结构体定义工具，并校验模型生成的参数。
- 保留多轮推理与工具调用所需的提供方元数据。
- 在不同轮次之间切换模型，无需重建对话历史。
- 在不扩张共享请求 API 的前提下，新增自定义模型协议。
- 运行自主的多步工具循环，支持流式事件、运行中干预，以及逐轮切换模型。

## 软件包

| 软件包 | 状态 | 说明 |
|---|---|---|
| [`or/llm`](llm/README.md) | 可用 | 统一的模型访问、流式、工具、推理、图像与对话历史 |
| [`or/agent`](agent/README.md) | 可用 | 带工具、流式事件、干预、追加消息与中止的有状态 agent 循环 |

## 安装

```sh
go get github.com/ktsoator/or/llm@latest
```

## 第一个请求

```go
model := llm.GetModel("anthropic", "claude-opus-4-8")
msg, err := llm.Complete(ctx, model, llm.Prompt("Hello"), llm.StreamOptions{})
```

导出的类型和函数，参见
[pkg.go.dev](https://pkg.go.dev/github.com/ktsoator/or/llm) 上的包文档。
