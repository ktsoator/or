# Image input

## Purpose

Mix text and a base64 image in one user message.

## Core code

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

## Constraints

- `Data` must be base64 text, not raw bytes or a URL.
- `MIMEType` and `Data` must both be non-empty.
- Images can only appear in user messages or tool results.
- Text-only models receive a placeholder instead of image content.
