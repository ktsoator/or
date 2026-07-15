# LLM Recipes

这些页面按开发任务提供最小路径。完整的可运行程序仍位于仓库的 `example/llm/`。

| 任务 | Recipe | 主要接口 |
|---|---|---|
| 发送一次请求 | [基础生成](basic-completion.md) | `LookupModel`、`Complete` |
| 构建流式聊天 | [流式聊天](streaming-chat.md) | `Stream`、`Event` |
| 发送截图或图像 | [图像输入](vision.md) | `ImageContent`、`Model.Input` |
| 展示推理过程 | [推理输出](reasoning.md) | `Reasoning`、thinking events |
| 自行运行工具循环 | [工具循环](tool-loop.md) | `MustTool`、`DecodeToolCall` |
| 在轮次间切换模型 | [模型切换](model-switching.md) | `Context.Messages`、`TransformMessages` |
| 接入代理或私有网关 | [自定义网关](custom-gateway.md) | `ProviderOverride` |
| 注入 HTTP client | [显式 Client](custom-client.md) | `NewClient`、`NewAdapterRegistry` |

所有调用真实 provider 的示例都需要对应 API key。凭证变量见[支持矩阵](../support-matrix.md)。
