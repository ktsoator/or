# LLM 场景手册

这些页面不是孤立的 API 片段。每个场景都会说明适用条件、请求如何经过 `llm`、完整可运行程序、关键参数、预期结果、失败行为和生产约束。

## 从这里开始

| 目标 | 场景 | 主要接口 |
|---|---|---|
| 发送一次配置完整的请求 | [基础生成](basic-completion.md) | `LookupModel`、`Complete` |
| 边生成边展示 | [流式聊天](streaming-chat.md) | `Stream`、`Event` |
| 维护并持久化历史 | [对话持久化](conversation-persistence.md) | `Context.Messages`、JSON helpers |
| 发送截图或图片 | [图片输入](vision.md) | `ImageContent`、`Model.Input` |
| 请求并展示推理 | [推理输出](reasoning.md) | `Reasoning`、thinking events |
| 执行结构化工具 | [工具循环](tool-loop.md) | `MustTool`、`DecodeToolCall` |
| 在轮次间更换模型 | [模型切换](model-switching.md) | `TransformMessages` |

## 配置与运维

| 目标 | 场景 | 主要接口 |
|---|---|---|
| 发现可运行模型和凭证 | [模型与鉴权发现](provider-discovery.md) | `GetRunnableModels`、`AuthStatus` |
| 接入代理或私有模型服务 | [自定义网关](custom-gateway.md) | `ProviderOverride`、`Model.BaseURL` |
| 隔离配置或注入 transport | [显式 Client](custom-client.md) | `NewClient`、registries |
| 增加请求追踪或改写 body | [可观测性 Hook](observability.md) | `OnRequest`、`RewriteRequest`、`OnResponse` |
| 统一处理失败 | [错误处理](error-handling.md) | `StopReason`、`IsContextOverflow` |
| 不使用真实账号测试 | [Mock Provider 测试](mock-testing.md) | `httptest`、显式 `Model` |

## 通用准备

示例要求 Go 1.24 或更高版本，并使用当前 module：

```sh
go get github.com/ktsoator/or/llm@latest
```

每次请求同时需要模型和已注册的协议 adapter。`openai-completions` 导入 `llm/openai`，`anthropic-messages` 导入 `llm/anthropic`，两者都需要时导入 `llm/all`。模型出现在目录中不代表可以直接运行。

真实 provider 示例使用[协议支持矩阵](../support-matrix.md)中的环境变量。生成文本、usage 和 provider response ID 会随账号和模型版本变化。

## 如何阅读示例

- 只关心最终 `AssistantMessage` 时使用 `Complete`。
- 需要实时展示文本、推理或工具增量时使用 `Stream`。
- 消息持久化和工具执行由应用负责。
- 即使已经取消，也要持续消费流直到通道关闭。
- 目录成本只是估算，账单以 provider 为准。

`example/llm/` 下的程序仍适合作为简短 smoke test；本组场景手册补充实际接入所需的设计与运维说明。
