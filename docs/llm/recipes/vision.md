# Image input

## What this builds

A command reads an image file, verifies that the selected model advertises image input, base64-encodes the bytes, and sends text and image content in one user message.

Use this shape for screenshots, diagrams, photographs, and scanned pages that the provider accepts as an image. `llm` does not fetch image URLs, resize files, or run OCR before the request.

## Prerequisites

```sh
export ANTHROPIC_API_KEY=your-key
go run . ./screenshot.png
```

## Complete program

```go
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/anthropic"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("usage: %s IMAGE", os.Args[0])
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	mimeType := http.DetectContentType(raw)
	if mimeType != "image/png" && mimeType != "image/jpeg" &&
		mimeType != "image/gif" && mimeType != "image/webp" {
		log.Fatalf("unsupported or undetected image type %q", mimeType)
	}

	model := llm.GetModel("anthropic", "claude-sonnet-4-6")
	if !slices.Contains(model.Input, llm.Image) {
		log.Fatalf("model %s does not advertise image input", model.ID)
	}

	input := llm.Context{Messages: []llm.Message{
		&llm.UserMessage{Content: []llm.UserContent{
			&llm.TextContent{Text: "Describe the visible error and suggest the next debugging step."},
			&llm.ImageContent{
				MIMEType: mimeType,
				Data:     base64.StdEncoding.EncodeToString(raw),
			},
		}},
	}}

	response, err := llm.Complete(context.Background(), model, input,
		llm.StreamOptions{MaxTokens: 800})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.Text())
}
```

## Data model and request behavior

`ImageContent.Data` contains base64 text, not raw bytes and not a URL. `MIMEType` and `Data` must both be non-empty. Images are valid only inside `UserMessage` and `ToolResultMessage`.

Before serialization, `TransformMessages` checks the target model. When history containing an image is replayed against a text-only model, the image becomes a text placeholder. This preserves conversation shape but does not give the target model visual information.

## Operational constraints

- Base64 increases payload size by roughly one third and keeps both raw and encoded data in memory in this example.
- Provider limits for dimensions, byte size, animation, and supported MIME types are not normalized by `llm`; consult the selected provider.
- `http.DetectContentType` inspects only a prefix and is not a security scanner. Validate untrusted uploads before storing or forwarding them.
- `Model.Input` is catalog metadata and can lag provider changes. Run an integration test for the selected model.
- Do not log serialized request bodies containing user images.

For multi-turn replay and model changes, see [Conversation persistence](conversation-persistence.md) and [Model switching](model-switching.md).
