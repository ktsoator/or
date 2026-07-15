# 图像输入

## 用途

在一条用户消息中混合文本和 base64 图像。

## 核心代码

```go
raw, err := os.ReadFile("screenshot.png")
if err != nil {
	log.Fatal(err)
}

input := llm.Context{Messages: []llm.Message{
	&llm.UserMessage{Content: []llm.UserContent{
		&llm.TextContent{Text: "Describe the error in this screenshot."},
		&llm.ImageContent{
			Data:     base64.StdEncoding.EncodeToString(raw),
			MIMEType: "image/png",
		},
	}},
}}

if !slices.Contains(model.Input, llm.Image) {
	log.Print("target is text-only; the image will be replaced by a placeholder")
}

response, err := llm.Complete(ctx, model, input, llm.StreamOptions{})
```

## 约束

- `Data` 必须是 base64 字符串，不能直接放原始字节或 URL。
- `MIMEType` 和 `Data` 均不能为空。
- 图像只能出现在用户消息或工具结果中。
- 文本模型收到占位符，不会收到图像内容。
