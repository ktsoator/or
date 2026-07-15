# Sending images

An image is sent as a content block in a `UserMessage`. The caller reads the image file, base64-encodes its bytes, and places it beside text instructions in one user message.

Use this for screenshots, diagrams, photographs, and scanned pages. `llm` does not download image URLs, resize images, or run OCR before a request; the application owns those operations.

## Scope

| Source or use | Handling |
|---|---|
| Local image file | Read bytes, inspect type, and base64-encode |
| User upload | Enforce size and validate content before constructing `ImageContent` |
| Remote image URL | Download and validate in application code; `ImageContent` does not accept URLs |
| Multiple images in one question | Add multiple image blocks to one `UserMessage.Content` in order |
| Screenshot or chart returned by a tool | Use an image block in `ToolResultMessage` |
| Text-only model | The image becomes placeholder text and is not sent to the model service |

## Before running the example

The example uses an Anthropic model and accepts an image file path:

```sh
go get github.com/ktsoator/or/llm@latest
export ANTHROPIC_API_KEY=your-key
go run . ./screenshot.png
```

## Complete program

The program checks the file type and model capability, then sends text and an image together. It writes the model response text to standard output.

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
	"time"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/anthropic"
)

func main() {
	const maxImageBytes = 10 << 20 // example limit: 10 MiB

	if len(os.Args) != 2 {
		log.Fatalf("usage: %s IMAGE", os.Args[0])
	}
	info, err := os.Stat(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	if info.Size() <= 0 || info.Size() > maxImageBytes {
		log.Fatalf("image size must be between 1 byte and %d bytes", maxImageBytes)
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

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	response, err := llm.Complete(ctx, model, input,
		llm.StreamOptions{MaxTokens: 800})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.Text())
}
```

The 10 MiB value is an application safeguard, not a model-service limit. Use the smaller of the application limit and the target service limit.

## Image content in a request

`ImageContent.Data` holds base64 text; it does not accept raw bytes or a URL. Both `MIMEType` and `Data` must be non-empty. Image content blocks are valid only in `UserMessage` and `ToolResultMessage`.

`llm.UserImage(data, mimeType)` quickly creates a user message containing only an image. To send text with an image, or multiple images, construct `UserMessage.Content` as in the complete program:

```go
message := &llm.UserMessage{Content: []llm.UserContent{
	&llm.TextContent{Text: "Compare these screenshots."},
	&llm.ImageContent{MIMEType: "image/png", Data: first},
	&llm.ImageContent{MIMEType: "image/jpeg", Data: second},
}}
```

Content block order is preserved. Describe the meaning of each image in text; `llm` does not generate file names, captions, or OCR text.

## Selecting an image-capable model

The example checks whether the built-in model catalog marks a model as accepting images with `slices.Contains(model.Input, llm.Image)`. This preflight check does not replace compatibility testing against the target model service.

For a model picker, filter the result of `GetRunnableModels(provider)` by `Model.Input`. `GetRunnableModels` checks whether a protocol adapter is registered; it does not filter by text or image capability.

Even when `Model.Input` contains `llm.Image`, a model service can impose separate limits on format, pixels, file size, image count, or animation. The built-in model catalog does not normalize these limits.

## Switching to a text-only model

Protocol adapters check the target model while converting messages. When history containing images is sent to a text-only model, images are replaced with text placeholders. This preserves conversation order, but the target model does not receive the visual content.

If an image contains information required for later answers, select another image-capable model before switching, or extract and persist the needed text in application code.

Downgrading creates a message copy for the current request and does not modify stored history. The original image remains available if a later turn switches back to an image-capable model.

## Results and multi-turn use

- Image requests return ordinary `AssistantMessage` values through `Complete` or `Stream`; result handling is the same as for text requests.
- Images can affect input-token accounting. `Usage` uses values returned by the model service; `llm` does not calculate image tokens separately.
- Serializing `Context` to JSON persists base64 images. Repeatedly carrying images through a long conversation can substantially increase storage and request size.
- If later turns need only conclusions from an image, application code can store an approved text summary and decide whether the original image must remain in subsequent requests.

## File and data boundaries

- Base64 increases request data by roughly one third. This example retains both raw bytes and encoded text in memory.
- Image dimensions, file size, animation, and supported MIME types are determined by the model service; `llm` does not normalize these limits.
- `http.DetectContentType` examines only the beginning of a file and is not a security scanner. For untrusted uploads, application code must also validate content and size.
- When downloading remote images, restrict schemes, hosts, redirects, response size, and timeout to prevent SSRF and resource exhaustion.
- `Model.Input` comes from the built-in model catalog and can lag model-service changes. Test image requests against the selected model before production use.
- Do not log serialized request bodies containing user images.
- Images can contain personal data, document content, or location metadata. Apply EXIF handling, access controls, and retention rules required by the application.

See [Saving and restoring conversations](conversation-persistence.md) for multi-turn storage, and [Changing models in a conversation](model-switching.md) for history conversion when changing models.
