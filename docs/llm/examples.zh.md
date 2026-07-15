# 示例索引

本页索引仓库中 `example/llm/` 下的可运行程序。按任务阅读的短示例见 [LLM Recipes](recipes/README.md)。

## 运行方式

每个目录都是独立的 `main` 包，但使用仓库根模块：

```sh
go run ./example/llm/basic
```

真实请求需要对应 provider 的 API key。先查看[协议与提供方状态](support-matrix.md)。

## 示例程序

| 示例 | 源码目录 | 说明 | 相关文档 |
|---|---|---|---|
| Basic | `example/llm/basic` | `LookupModel`、`Complete` 和文本结果 | [单次文本生成](recipes/basic-completion.md) |
| Options | `example/llm/options` | system prompt、temperature、max tokens | [请求选项](configuration.md) |
| Streaming | `example/llm/streaming` | 消费文本增量和终止事件 | [流式响应](recipes/streaming-chat.md) |
| Reasoning | `example/llm/reasoning` | 推理等级与 thinking 事件 | [请求推理内容](recipes/reasoning.md) |
| Tools | `example/llm/tools` | 类型化工具和手写工具循环 | [执行工具调用](recipes/tool-loop.md) |
| Conversation | `example/llm/conversation` | 调用方管理多轮历史 | [消息与上下文](conversations.md) |
| Model switch | `example/llm/model_switch` | 同一历史切换协议和模型 | [对话中更换模型](recipes/model-switching.md) |
| Advanced | `example/llm/advanced` | 请求 hooks 和底层选项 | [请求选项](configuration.md) |
| Providers | `example/llm/providers` | 注册 provider 和设置 override | [Provider](providers.md) |
| Who am I | `example/llm/whoami` | 查询 provider 凭证状态 | [Provider 状态](providers.md#检查-provider-是否已配置) |

## 编译检查

以下命令编译全部已跟踪的 LLM 示例，不会发送网络请求：

```sh
go test ./example/llm/...
```

示例只展示 `llm` 包职责。会话数据库、工具权限、重试策略和上下文压缩需要由应用补充。
