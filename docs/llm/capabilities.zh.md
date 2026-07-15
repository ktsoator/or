# 功能总览

本页用于判断 `github.com/ktsoator/or/llm` 能否覆盖当前开发任务，以及下一步应从哪个入口开始。完整程序放在[使用指南](recipes/README.md)，类型和字段定义放在“API 与概念”下的参考页。

`llm` 是无状态的模型调用层。它负责把应用中的消息、工具和请求选项转换成目标模型服务能够接收的请求，再将返回内容整理成统一消息或流式事件。对话存储、工具执行和业务重试仍由应用负责。

## 选择调用入口

| 当前需求 | 使用方式 | 后续阅读 |
|---|---|---|
| 等请求结束后一次性读取结果 | `Complete` | [单次文本生成](recipes/basic-completion.md) |
| 文本生成时立即展示增量 | `Stream` | [流式响应](recipes/streaming-chat.md) |
| 隔离注册表、网络配置或测试状态 | 独立 `Client` | [创建自定义 Client](recipes/custom-client.md) |
| 接入兼容现有协议的代理或私有服务 | `ProviderOverride` 或显式 `Model` | [接入自定义模型服务](recipes/custom-gateway.md) |
| 接口的请求和响应格式尚未被框架支持 | `ProtocolAdapter` | [自定义协议](extending.md) |

选择 `Complete` 或 `Stream` 只改变应用如何读取结果，不改变消息、工具、用量和停止原因的公共模型。包级函数使用默认注册表；独立 `Client` 使用应用传入的注册表。

## 生成与响应处理

### 获取完整结果

`Complete` 会消费底层事件流并返回一个 `AssistantMessage`。应用可以读取文本、内容块、停止原因、Token 用量、成本估算、响应 ID 和诊断信息。

适合批处理、后台任务，以及不需要显示生成过程的请求。返回的 `error` 可能同时伴随部分消息，不能只按 `err != nil` 丢弃响应。字段和失败语义分别见[响应与用量](results.md)和[失败信号](errors.md)。

### 处理流式结果

`Stream` 返回统一事件通道。文本、推理内容和工具参数使用各自的 start、delta、end 事件；请求最终以 `EventDone` 或 `EventError` 结束。

适合终端输出、聊天界面、首段延迟统计或工具调用进度。事件通道无缓冲，调用方必须读取到关闭。完整事件契约见[流式事件](streaming.md)。

## 消息、图片与对话

`Context` 保存本次请求的系统指令、消息历史和工具定义。消息由带角色的类型和内容块组成，可通过 JSON 保存并恢复。

| 能力 | `llm` 提供 | 应用负责 |
|---|---|---|
| 多轮对话 | 类型化消息、历史转换和 JSON 序列化 | 会话 ID、数据库、轮次并发和上下文裁剪 |
| 图片输入 | `ImageContent`、模型输入能力和纯文本降级 | 文件读取、MIME 校验、大小限制和访问控制 |
| 更换模型 | 按目标模型转换历史副本 | 选择目标、检查凭证、评估语义差异和回退策略 |
| 保存历史 | `Context`、`MarshalMessage`、`UnmarshalMessage` | 存储版本、加密、保留期限和租户隔离 |

对应的完整流程见[保存与恢复对话](recipes/conversation-persistence.md)、[发送图片](recipes/vision.md)和[对话中更换模型](recipes/model-switching.md)。消息类型和转换规则见[消息与上下文](conversations.md)。

## 推理与工具调用

### 请求推理内容

`StreamOptions.Reasoning` 使用统一等级表达推理投入，协议适配器会将其转换为目标接口能够理解的选项。模型是否支持某个等级由模型元数据决定；可见推理内容通过独立事件和 `ThinkingContent` 返回。

应用决定是否展示、记录或隐藏推理内容。等级、签名和对话连续性见[推理配置](reasoning.md)，完整接入程序见[请求推理内容](recipes/reasoning.md)。

### 执行工具调用

`NewTool` 和 `MustTool` 可从 Go 结构体生成工具 JSON Schema。模型返回 `ToolCall` 后，应用使用 `DecodeToolCall` 或校验函数读取参数，再执行实际操作并用 `ToolResult` 回传结果。

`llm` 不执行工具，也不提供权限控制或自动循环。应用必须处理授权、超时、幂等、并发和循环上限。工具类型见[工具定义与调用](tools.md)，完整循环见[执行工具调用](recipes/tool-loop.md)。

## 模型与服务接入

内置模型清单提供模型 ID、协议、输入能力、上下文窗口、输出上限和价格等元数据。`GetRunnableModels` 只返回当前进程已经注册相应协议适配器的模型；凭证是否存在可通过 `AuthStatus` 检查。

开发者可以：

- 用 `LookupModel` 读取内置模型；
- 直接构造 `Model` 接入单个兼容服务；
- 用 `ProviderRegistry.SetOverride` 为某个提供方设置网关、凭证或请求头；
- 用 `NewSpecProvider` 注册新的提供方配置；
- 注入自定义 `http.Client`，控制代理、TLS、连接池和 HTTP 传输配置；
- 实现 `ProtocolAdapter` 支持新的请求与响应格式。

模型元数据和提供方配置见[模型与提供方](providers.md)。模型清单不等于线上兼容保证，部署前应使用真实目标服务验证鉴权、流式、工具和错误路径。

## 请求观察与测试

`OnRequest`、`RewriteRequest` 和 `OnResponse` 可观察或修改底层 SDK 的每次请求尝试。回调在请求路径中同步执行，原始请求可能包含提示词、工具参数和其他敏感内容。完整示例见[记录和修改请求](recipes/observability.md)。

业务结果处理可以直接构造 `AssistantMessage` 测试；请求序列化和事件转换可以使用 `httptest.Server` 验证，无需真实账号。测试分层见[测试策略](testing.md)，完整本地服务示例见[使用本地模拟服务测试](recipes/mock-testing.md)。

## 不包含的能力

`llm` 不提供：

- 会话数据库和自动历史管理；
- 上下文摘要、裁剪或压缩；
- 工具执行器、权限系统和自动工具循环；
- 智能体规划、任务调度或运行状态机；
- 检索增强生成、向量数据库或文档索引；
- 提供方故障转移、负载均衡或多模型竞速；
- [协议与提供方状态](support-matrix.md)中标记为“仅收录”的协议实现。

这些能力需要由应用或更上层模块实现。公开符号的查找入口见 [API 索引](api-reference.md)。
