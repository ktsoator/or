# Agent 包

`github.com/ktsoator/or/agent` 把模型变成一个能自主执行多步的行动者。它在 `or/llm`
之上运行工具调用循环——流式跑一轮、执行模型请求的工具、把结果追加回去，如此往复直到
模型停止——而把历史存储与上下文压缩留给调用方。

它是 provider 无关的编排层：无状态引擎（`RunLoop`）加上可选的有状态封装层
（`Agent`），所有扩展点均为函数字段。它不内置任何具体工具、持久化或系统提示词。

## 安装

```sh
go get github.com/ktsoator/or/agent@latest
```

## 文档

- [快速开始](getting-started.md) — 第一个 agent 与工具循环
- [工具](tools.md) — 定义工具、结果、流式进度与执行顺序
- [事件与状态](events.md) — 运行事件流、订阅与快照
- [引导与追加](steering.md) — 运行中注入消息、继续与中止
- [生命周期钩子](hooks.md) — 拦截工具、切换模型、停止与压缩
- [消息与自定义类型](messages.md) — transcript、仅 UI 消息与投影
- [配置](configuration.md) — 请求选项、推理、动态密钥与 setter
- [运行循环引擎](loop.md) — `RunLoop`、`LoopConfig` 与自建封装

完整的导出类型和函数，参见
[pkg.go.dev](https://pkg.go.dev/github.com/ktsoator/or/agent) 上的包文档。
