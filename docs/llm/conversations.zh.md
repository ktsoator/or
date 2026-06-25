# 对话

对话消息与厂商无关。同一段历史可以被持久化、扩展，并发送给另一个兼容模型，而无需重建。

## 构建消息

`Context`、`Message` 以及内容块都是完全通用的，但多数调用只是发送一些文本。便捷构造器
为这种场景省去了嵌套：

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

下面这种完整的结构体字面量写法仍然有效；当你需要构造器未覆盖的内容时（例如在一条消息
中混合文本和图像），再使用它。

## 图像输入

多模态模型支持在用户消息中图文并存。以 base64 提供原始字节及其 MIME 类型：

```go
raw, err := os.ReadFile("screenshot.png")
if err != nil {
	log.Fatal(err)
}
input := llm.Context{Messages: []llm.Message{
	&llm.UserMessage{Content: []llm.UserContent{
		&llm.TextContent{Text: "Describe the problem shown in this screenshot."},
		&llm.ImageContent{
			MIMEType: "image/png",
			Data:     base64.StdEncoding.EncodeToString(raw),
		},
	}},
}}
```

模型通过 `Model.Input` 声明是否支持图像。当包含图像的历史被发送给仅支持文本的模型时，
图像会被自动替换为一个简短的占位符。

## 在不同轮次间切换模型

每次请求前，本库都会为目标模型适配已存储的历史：为仅支持文本的模型降级图像、在兼容时
保留推理签名、降级或移除不兼容的推理，并规范化工具调用标识符。

```go
ctx := context.Background()
draft := llm.GetModel("deepseek", "deepseek-v4-flash")
review := llm.GetModel("anthropic", "claude-opus-4-8")

messages := []llm.Message{
	llm.UserText("Compute 25 * 18 and explain the steps."),
}

first, err := llm.Complete(ctx, draft,
	llm.Context{Messages: messages}, llm.StreamOptions{})
if err != nil {
	log.Fatal(err)
}
messages = append(messages, &first)
messages = append(messages, llm.UserText("Check the calculation above for mistakes."))

second, err := llm.Complete(ctx, review,
	llm.Context{Messages: messages}, llm.StreamOptions{})
if err != nil {
	log.Fatal(err)
}
```

`TransformMessages` 执行这项适配，并已对外导出，供需要查看模型实际会收到的确切历史的
调用方使用。

## 保存与恢复对话

`Context` 序列化为自描述的 JSON：消息携带角色，内容块携带类型，因此 JSON 可以无需
手动分派地往返还原成具体的消息和内容类型。

```go
data, err := json.MarshalIndent(llm.Context{Messages: messages}, "", "  ")
if err != nil {
	log.Fatal(err)
}
if err := os.WriteFile("conversation.json", data, 0o644); err != nil {
	log.Fatal(err)
}

raw, err := os.ReadFile("conversation.json")
if err != nil {
	log.Fatal(err)
}
var restored llm.Context
if err := json.Unmarshal(raw, &restored); err != nil {
	log.Fatal(err)
}
```

`restored.Messages` 已可用于扩展，并针对任意模型重放。
