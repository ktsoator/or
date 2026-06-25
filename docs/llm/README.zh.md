# LLM 包

`github.com/ktsoator/or/llm` 提供统一的 Go API，用于在 OpenAI 兼容与 Anthropic
兼容的模型之间进行流式响应、结构化工具、推理内容、多模态消息和对话历史的处理。

## 安装

```sh
go get github.com/ktsoator/or/llm@latest
```

## 文档

- [快速开始](getting-started.md) — 凭证与你的第一个请求
- [提供方与模型](providers.md) — 目录发现与自定义端点
- [流式](streaming.md) — 事件、部分响应、诊断与取消
- [工具](tools.md) — 类型化工具与协议特定的工具选择
- [推理](reasoning.md) — 努力级别与思考显示
- [对话](conversations.md) — 图像、模型切换与持久化
- [配置](configuration.md) — 重试、超时、请求头与 HTTP 钩子
- [自定义协议](extending.md) — 适配器、注册表与 `StreamWriter`

导出的类型和函数，参见
[pkg.go.dev](https://pkg.go.dev/github.com/ktsoator/or/llm) 上的包文档。
