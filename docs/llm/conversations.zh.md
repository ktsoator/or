# 消息与上下文

本页定义 `Context`、消息接口、内容块、构造器和序列化契约。多轮对话、图片输入、持久化和模型切换的完整实现分别放在对应的[使用指南](recipes/README.md)中。

## 消息与内容模型

一段历史就是一个 `[]llm.Message`。`Message` 是一个接口，有三个实现，每个角色一个。每个实现持有一组*内容块*，且角色限定了允许哪些块类型：

| 消息 | 角色 | 允许的内容块 |
|---|---|---|
| `UserMessage` | 用户输入 | `TextContent`、`ImageContent` |
| `AssistantMessage` | 模型输出 | `TextContent`、`ThinkingContent`、`ToolCall` |
| `ToolResultMessage` | 工具结果 | `TextContent`、`ImageContent` |

内容块是实际读写的叶子类型：

| 块 | 承载内容 |
|---|---|
| `TextContent` | 纯文本（任意消息中均可） |
| `ImageContent` | base64 图像数据加 MIME 类型 |
| `ThinkingContent` | 推理文本及其 provider 签名（仅 assistant） |
| `ToolCall` | 工具名、ID 与解码后的参数（仅 assistant） |

由于消息和块都是带类型的，已存对话可无需手动分派地通过 JSON 往返——见[JSON 序列化](#json-序列化)。

对于常见的“只发文本”场景，请使用下面的便捷构造器。仅当需要构造器覆盖不到的内容时才手写结构体字面量，例如在一条用户消息里混合文本与图像，或预置一条携带工具调用的 assistant 轮次。完整图片输入见[发送图片](recipes/vision.md)。

## 构建消息

`Context`、`Message` 以及内容块都是完全通用的，但多数调用只是发送一些文本。便捷构造器为这种场景省去了嵌套：

```go
llm.Prompt("Explain Go channels briefly.")        // 含一条用户文本消息的 Context
llm.PromptWithSystem("Be concise.", "Explain...") // ……外加一个 system 提示
llm.UserText("hello")                             // *UserMessage
llm.AssistantText("hi there")                     // *AssistantMessage（用于预置历史）
llm.UserImage(data, "image/png")                  // 含一张图像的 *UserMessage
llm.ToolResult(callID, name, "result text")       // *ToolResultMessage
llm.NewContext(msg1, msg2, ...)                   // 由若干消息构成的 Context
```

用 `AssistantMessage` 上对应的访问器读回响应：

```go
response.Text()      // 拼接所有文本块
response.ToolCalls() // 按顺序返回每一个工具调用
```

下面这种完整的结构体字面量写法仍然有效；当需要构造器未覆盖的内容时（例如在一条消息中混合文本和图像），再使用它。

## 历史与模型转换

`llm` 不保存会话状态。调用方维护 `[]llm.Message`，每轮依次追加 `*AssistantMessage` 和新的用户消息，再放回 `Context.Messages`。`SystemPrompt` 是请求上下文的一部分，不会自动写入消息切片。完整的并发控制、存储边界和恢复程序见[保存与恢复对话](recipes/conversation-persistence.md)。

请求发出前，`TransformMessages` 会创建面向目标模型的历史副本：

| 已存内容 | 转换行为 |
|---|---|
| 图片发送给纯文本模型 | 替换为文本占位符 |
| 同一模型产生的推理内容 | 保留兼容的推理内容和签名 |
| 其他模型产生的推理内容 | 删除模型服务专有的推理内容 |
| 工具调用 ID | 按目标协议规范化，并同步更新对应结果 |
| 失败或取消的 assistant 消息 | 从重放副本中删除 |
| 没有结果的工具调用 | 插入合成错误结果 |

“同一模型”要求 provider、协议和模型 ID 均一致。转换不会修改调用方传入的历史；未转换的消息对象可能与原切片共享，调用方应把输入历史视为不可变值。

跨模型使用的完整流程与兼容性检查见[对话中更换模型](recipes/model-switching.md)。

## JSON 序列化

`Context` 实现 JSON 往返。消息在 JSON 中携带角色，内容块携带类型，反序列化后会恢复为具体的消息和内容实现。按记录保存单条消息时，使用 `MarshalMessage` 与 `UnmarshalMessage`：

```go
data, err := llm.MarshalMessage(messages[0])
if err != nil {
	log.Fatal(err)
}

message, err := llm.UnmarshalMessage(data)
if err != nil {
	log.Fatal(err)
}
messages = append(messages, message)
```

遇到未知角色、未知内容类型或畸形 JSON 时，`UnmarshalMessage` 返回错误，不会把不支持的结构静默转换为其他类型。完整的文件与数据库示例、并发写入和版本字段建议见[保存与恢复对话](recipes/conversation-persistence.md)。图片的编码、输入能力检查和安全边界见[发送图片](recipes/vision.md)。

!!! warning "序列化的历史是敏感数据"
    序列化后的 `Context` 可能包含用户输入、工具结果（其中可能嵌入抓取到的文档或凭证）以及提供方的推理签名。请把这份 JSON 当作敏感数据：不要整体打日志，存储或传输时应与其中的底层数据同等对待。
