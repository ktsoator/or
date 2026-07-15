# LLM 使用指南

按要完成的任务选择页面。这里维护完整程序、接入步骤和应用层处理策略；类型、字段和行为契约以“API 与概念”下的参考页为准。

## 请求与会话

| 要实现的功能 | 对应页面 | 主要接口 |
|---|---|---|
| 发送一次配置完整的请求 | [单次文本生成](basic-completion.md) | `LookupModel`、`Complete` |
| 边生成边展示 | [流式响应](streaming-chat.md) | `Stream`、`Event` |
| 保存并恢复历史 | [保存与恢复对话](conversation-persistence.md) | `Context.Messages`、JSON helpers |
| 发送截图或图片 | [发送图片](vision.md) | `ImageContent`、`Model.Input` |
| 请求并展示推理 | [请求推理内容](reasoning.md) | `Reasoning`、thinking events |
| 执行结构化工具 | [执行工具调用](tool-loop.md) | `MustTool`、`DecodeToolCall` |
| 在轮次间更换模型 | [对话中更换模型](model-switching.md) | `TransformMessages` |

## 接入、配置与测试

| 要实现的功能 | 对应页面 | 主要接口 |
|---|---|---|
| 发现可运行模型和凭证 | [查找模型与检查凭证](provider-discovery.md) | `GetRunnableModels`、`AuthStatus` |
| 接入代理或私有模型服务 | [接入自定义模型服务](custom-gateway.md) | `ProviderOverride`、`Model.BaseURL` |
| 隔离配置或注入 transport | [创建自定义 Client](custom-client.md) | `NewClient`、registries |
| 增加请求追踪或改写 body | [记录和修改请求](observability.md) | `OnRequest`、`RewriteRequest`、`OnResponse` |
| 统一处理失败 | [处理请求失败](error-handling.md) | `StopReason`、`IsContextOverflow` |
| 不使用真实账号测试 | [使用本地模拟服务测试](mock-testing.md) | `httptest`、显式 `Model` |

## 运行示例前

示例要求 Go 1.24 或更高版本：

```sh
go get github.com/ktsoator/or/llm@latest
```

调用前要准备模型和对应的协议 adapter。使用 `openai-completions` 时导入 `llm/openai`；使用 `anthropic-messages` 时导入 `llm/anthropic`；两者都使用时导入 `llm/all`。模型被内置模型清单收录，并不表示当前程序已注册对应 adapter。

真实 provider 示例所需的环境变量见[协议与提供方状态](../support-matrix.md)。生成文本、token 用量和 provider response ID 会随账号与模型版本变化。

## 示例中的通用约定

- 只读取最终 `AssistantMessage` 时，调用 `Complete`。
- 需要实时处理文本、推理内容或工具调用增量时，调用 `Stream`。
- 对话历史的存储和工具调用的实际执行由应用负责。
- `Stream` 返回后应持续消费事件，直到通道关闭；取消 context 后也是如此。
- `Usage.Cost` 根据内置模型清单中的价格估算，不能替代 provider 账单。

`example/llm/` 下的程序可用于快速验证；本组页面补充了接入时的参数、边界和运行约束。
