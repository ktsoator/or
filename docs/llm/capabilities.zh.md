# 功能总览

`github.com/ktsoator/or/llm` 是一个无状态的 LLM 协议翻译层。它统一请求、消息、流式事件、工具调用、推理内容和使用量，但不保存会话，也不执行工具。

本页从开发任务出发，列出可直接使用的功能和对应接口。完整的类型与方法索引见 [API 索引](api-reference.md)。

## 功能与接口

| 开发任务 | 主要接口 | 说明 | 完整指南 |
|---|---|---|---|
| 单次文本生成 | `Complete`、`Prompt` | 收集完整流并返回 `AssistantMessage` | [基础生成](recipes/basic-completion.md) |
| 带系统提示词生成 | `PromptWithSystem` | 设置本次请求的 `Context.SystemPrompt` | [对话](conversations.md) |
| 流式聊天界面 | `Stream`、`EventTextDelta` | 按增量渲染文本、推理和工具参数 | [流式聊天](recipes/streaming-chat.md) |
| 多轮对话 | `Context.Messages`、`UserText` | 调用方保存并在下一轮重发历史 | [对话持久化](recipes/conversation-persistence.md) |
| 图像理解 | `UserImage`、`ImageContent` | 发送 base64 图像；纯文本模型自动降级 | [图片输入](recipes/vision.md) |
| 切换模型或协议 | `LookupModel`、`TransformMessages` | 复用同一段历史，库按目标模型重新适配 | [模型切换](recipes/model-switching.md) |
| 展示推理过程 | `StreamOptions.Reasoning`、thinking events | 使用中立推理等级并单独读取思考内容 | [推理输出](recipes/reasoning.md) |
| 定义结构化工具 | `NewTool`、`MustTool` | 从 Go 结构体生成工具 JSON Schema | [工具](tools.md) |
| 解析工具调用 | `DecodeToolCall`、`ValidateToolCall` | 校验、强制转换并解码模型参数 | [工具](tools.md#执行前校验) |
| 自行运行工具循环 | `ToolCalls`、`ToolResult`、`StopReasonToolUse` | 应用执行工具后把结果追加到历史 | [工具循环](recipes/tool-loop.md) |
| 保存与恢复对话 | `json.Marshal(Context)`、`MarshalMessage` | 使用自描述 JSON 保存消息及内容块 | [对话持久化](recipes/conversation-persistence.md) |
| 查看 token 和成本 | `AssistantMessage.Usage`、`CalculateCost` | 读取输入、输出、缓存 token 和目录估算成本 | [读取响应](results.md) |
| 观察或改写请求 | `OnRequest`、`RewriteRequest`、`OnResponse` | 追踪每次 SDK attempt，或修改 provider-specific JSON | [可观测性 Hook](recipes/observability.md) |
| 检测上下文溢出 | `IsContextOverflow` | 结合错误文本、停止原因和 usage 判断 | [错误处理](recipes/error-handling.md) |
| 构建模型选择器 | `GetProviders`、`GetRunnableModels` | 只展示当前进程已注册协议的模型 | [模型发现](recipes/provider-discovery.md) |
| 检查 provider 凭证 | `AuthStatus`、`APIKeyEnvVars` | 不发请求地检查 key 来源和缺失变量 | [鉴权发现](recipes/provider-discovery.md) |
| 接入代理或网关 | `ProviderRegistry.SetOverride` | 按 provider 覆盖 URL、key、headers 和环境 | [自定义网关](recipes/custom-gateway.md) |
| 使用兼容 endpoint | 手动构造 `Model` | 接入 OpenAI Chat Completions 或 Anthropic Messages 兼容服务 | [自定义网关](recipes/custom-gateway.md) |
| 注入自定义 HTTP client | `openai.NewAdapter`、`anthropic.NewAdapter` | 配置 Transport、代理、TLS、连接池或 mock | [显式 Client](recipes/custom-client.md) |
| 隔离全局状态 | `NewClient`、`NewAdapterRegistry` | 为测试、租户或子系统构造独立 client | [显式 Client](recipes/custom-client.md) |
| 新增 provider | `NewSpecProvider`、`ProviderRegistry.Register` | 注册凭证来源、headers 和关联模型 | [自定义 Provider](providers.md#注册自定义-provider) |
| 新增线协议 | `ProtocolAdapter`、`StreamWriter` | 实现请求转换和统一流式事件 | [自定义协议](extending.md) |
| 无真实 API 测试 | 手工 `AssistantMessage`、`httptest.Server` | 测试结果处理或完整协议路径 | [Mock Provider 测试](recipes/mock-testing.md) |

## 最小使用路径

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func main() {
	model, ok := llm.LookupModel("deepseek", "deepseek-v4-flash")
	if !ok || !llm.SupportsProtocol(model.Protocol) {
		log.Fatal("model is not runnable")
	}

	response, err := llm.Complete(
		context.Background(),
		model,
		llm.Prompt("Explain Go channels in one sentence."),
		llm.StreamOptions{},
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(response.Text())
}
```

设置 `DEEPSEEK_API_KEY` 后运行：

```sh
go run .
```

## 功能边界

`llm` 当前不提供以下功能：

- 会话数据库或自动历史管理；
- 上下文摘要、裁剪或压缩；
- 工具执行器、工具权限控制或自动工具循环；
- Agent 规划、任务调度或运行状态机；
- RAG、向量数据库或文档索引；
- provider fallback、负载均衡或多模型竞速；
- OpenAI Responses、Google Generative AI 或 Mistral Conversations adapter。

这些能力可以在 `llm` 之上实现。当前材料没有说明内置实现。

## 选择入口

- 固定模型、只取最终消息：使用 `Complete`。
- 需要首 token 延迟或分块渲染：使用 `Stream`，并持续消费事件通道直到关闭。
- 需要独立 HTTP client、注册表或测试隔离：使用显式 `Client`。
- endpoint 仍兼容现有协议：构造 `Model` 或注册 provider，不要实现新 adapter。
- endpoint 使用不同线协议：实现 `ProtocolAdapter`。
