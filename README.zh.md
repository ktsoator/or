# or

[English](README.md) | 简体中文

[![Go Reference](https://pkg.go.dev/badge/github.com/ktsoator/or/llm.svg)](https://pkg.go.dev/github.com/ktsoator/or/llm)
[![CI](https://github.com/ktsoator/or/actions/workflows/ci.yml/badge.svg)](https://github.com/ktsoator/or/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/ktsoator/or)](https://goreportcard.com/report/github.com/ktsoator/or)
[![Release](https://img.shields.io/github/v/release/ktsoator/or)](https://github.com/ktsoator/or/releases/latest)
[![Go Version](https://img.shields.io/github/go-mod/go-version/ktsoator/or)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

`or` 是一个模块化的 Go 工具集,用于构建基于语言模型和上层 agent 的应用。它的
LLM 包与具体提供方无关,在底层模型和通信协议不断更替的同时,让对话、工具、推理
和流式事件保持稳定;agent 包则在此之上构建工具调用循环、状态管理和流式事件。

## 为什么选择 `or`

- 在 OpenAI 兼容和 Anthropic 兼容的提供方之间复用同一套对话模型。
- 通过类型化事件流式输出文本、推理、工具调用、用量和错误。
- 用 Go 结构体定义工具,并校验模型生成的参数。
- 保留多轮推理与工具调用所需的提供方元数据。
- 在轮次之间切换模型,无需重建对话历史。
- 新增自定义模型协议,而不必扩张共享的请求 API。
- 运行自主的多步工具循环,支持流式事件、运行中引导以及逐轮切换模型。
- 借助 harness,在上层叠加 transcript 持久化、上下文压缩、逐轮系统提示与技能。

## 包

| 包 | 状态 | 说明 |
|---|---|---|
| [`or/llm`](docs/llm/README.zh.md) | 可用 | 统一的模型访问、流式、工具、推理、图像与对话历史 |
| [`or/agent`](docs/agent/README.zh.md) | 可用 | 有状态的 agent 循环,含工具、流式事件、引导、追加与中止 |
| [`or/agent/harness`](https://pkg.go.dev/github.com/ktsoator/or/agent/harness) | 可用 | agent 之上的编排层:transcript 持久化、上下文压缩、逐轮系统提示、技能与提示模板 |

未来的包可以在同样的基础之上构建更上层的编排能力,而无需把根包变成一个庞大的
单一 API。

## 环境要求

- Go 1.24 或更高版本
- 所选托管提供方的 API key,或一个兼容的本地端点

## 安装

安装 LLM 包:

```sh
go get github.com/ktsoator/or/llm@latest
```

设置所选提供方所需的 API key。例如:

```sh
export DEEPSEEK_API_KEY=your-deepseek-api-key
```

支持的提供方 ID、环境变量、目录发现和自定义端点,参见
[提供方与模型](docs/llm/providers.zh.md)。

## 快速开始

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai" // 注册 OpenAI 兼容协议(DeepSeek、Groq、xAI 等)
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

每个协议都位于一个提供方包中,在被导入时自行注册。通过为副作用导入对应的提供方包
(`llm/openai`、`llm/anthropic`),按需引入你用到的协议——以及仅它们的厂商 SDK;
也可以导入 `llm/all` 一次性引入所有内置协议。

用 `llm.Stream` 替代 `llm.Complete`,即可在模型生成过程中消费增量:

```go
events, err := llm.Stream(ctx, model, input, llm.StreamOptions{})
if err != nil {
	log.Fatal(err)
}
for event := range events {
	switch event.Type {
	case llm.EventTextDelta:
		fmt.Print(event.Delta)
	case llm.EventError:
		log.Fatal(event.Err)
	}
}
```

## 文档

两个包的指南都在
**[ktsoator.github.io/or](https://ktsoator.github.io/or/)**。

API 参考:[`or/llm`](https://pkg.go.dev/github.com/ktsoator/or/llm) ·
[`or/agent`](https://pkg.go.dev/github.com/ktsoator/or/agent)

## 支持的协议

内置适配器实现了:

- OpenAI 兼容的 Chat Completions
- Anthropic 兼容的 Messages

模型目录为 DeepSeek、MiniMax、小米 MiMo、Z.AI、Moonshot AI、Kimi、Anthropic、
OpenRouter 等兼容提供方提供了明确的兼容性元数据。目录中存在并不保证每个模型都经过
实测;两个通信适配器都有自动化的 mock server 测试覆盖。

## 项目状态

`v0.5.x` 在 `or/agent` 包之上新增了 `or/agent/harness`——一个有状态的编排层
(transcript 持久化、上下文压缩、逐轮系统提示与技能),是新接入的推荐基线版本。
项目仍处于 1.0 之前,因此 API 在次要版本之间可能继续演进。破坏性变更会在发布说明
中标注。

## 致谢

本项目受 [earendil-works/pi](https://github.com/earendil-works/pi)(由 Mario
Zechner 创建)启发,并部分改编自该项目。

## 许可证

基于 [MIT License](LICENSE) 发布。
