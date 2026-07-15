# 图片输入

## 本场景实现什么

命令读取图片文件，确认所选模型声明支持图片输入，把字节编码为 base64，再在一条 user 消息中同时发送文字和图片。

该结构适合 provider 接受的截图、图表、照片和扫描页。`llm` 不负责下载图片 URL、调整尺寸或在请求前执行 OCR。

## 前置条件

```sh
export ANTHROPIC_API_KEY=your-key
go run . ./screenshot.png
```

## 完整程序

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

## 数据模型与请求行为

`ImageContent.Data` 保存 base64 文本，不是原始字节或 URL。`MIMEType` 与 `Data` 都不能为空。图片只能出现在 `UserMessage` 和 `ToolResultMessage` 中。

序列化前，`TransformMessages` 会检查目标模型。带图片的历史重放给纯文本模型时，图片会变成文本占位符。这样能维持对话结构，但目标模型不会获得视觉信息。

## 运维约束

- Base64 会让 payload 大约增加三分之一；本示例同时在内存中保留原始与编码数据。
- 图片尺寸、字节数、动画和 MIME 限制没有被 `llm` 统一，应查询所选 provider。
- `http.DetectContentType` 只检查文件前缀，不是安全扫描器。存储或转发不可信上传前仍要校验。
- `Model.Input` 是目录 metadata，可能晚于 provider 更新；应为选定模型运行集成测试。
- 不要记录包含用户图片的序列化请求 body。

多轮重放与更换模型见[对话持久化](conversation-persistence.md)和[模型切换](model-switching.md)。
