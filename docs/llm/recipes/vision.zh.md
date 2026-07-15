# 发送图片

图片作为 `UserMessage` 的内容块发送。调用方读取图片文件，将字节编码为 base64，并与文字说明放入同一条用户消息。

此方式适用于截图、图表、照片和扫描页。`llm` 不会下载图片 URL、调整图片尺寸或在请求前执行 OCR；这些工作由应用完成。

## 适用范围

| 输入来源或用途 | 处理方式 |
|---|---|
| 本地图片文件 | 读取字节、检查类型、编码为 base64 |
| 用户上传图片 | 应用先限制大小并校验内容，再构造 `ImageContent` |
| 远程图片 URL | 应用负责下载和安全检查；`ImageContent` 不接受 URL |
| 同一问题包含多张图片 | 在一条 `UserMessage.Content` 中按顺序放入多个图片块 |
| 工具返回截图或图表 | 在 `ToolResultMessage` 中使用图片内容块 |
| 模型只支持文本 | 图片会被替换为占位文本，不会传给模型服务 |

## 运行前准备

示例使用 Anthropic 模型，并接收一个图片文件路径：

```sh
go get github.com/ktsoator/or/llm@latest
export ANTHROPIC_API_KEY=your-key
go run . ./screenshot.png
```

## 完整程序

程序检查文件类型和模型能力，然后将文字与图片一起发送。模型返回的文本会输出到标准输出。

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
	const maxImageBytes = 10 << 20 // 示例限制为 10 MiB

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

示例中的 10 MiB 是应用保护值，不代表模型服务的上限。实际限制应取应用策略与目标模型服务限制中的较小值。

## 请求中的图片内容

`ImageContent.Data` 保存 base64 文本，不接受原始字节或 URL。`MIMEType` 与 `Data` 都不能为空。图片内容块只能放在 `UserMessage` 或 `ToolResultMessage` 中。

`llm.UserImage(data, mimeType)` 可以快速构造一条只含图片的用户消息。需要同时发送文字和图片，或一次发送多张图片时，应像完整程序一样构造 `UserMessage.Content`：

```go
message := &llm.UserMessage{Content: []llm.UserContent{
	&llm.TextContent{Text: "Compare these screenshots."},
	&llm.ImageContent{MIMEType: "image/png", Data: first},
	&llm.ImageContent{MIMEType: "image/jpeg", Data: second},
}}
```

内容块的顺序会保留。应在文字中明确每张图片的含义；`llm` 不会自动生成图片名称、说明或 OCR 文本。

## 选择支持图片的模型

示例通过 `slices.Contains(model.Input, llm.Image)` 检查内置模型清单是否标注该模型支持图片。这是调用前检查，不替代目标模型服务的实际兼容性测试。

构建模型选择列表时，可从 `GetRunnableModels(provider)` 的结果中继续筛选 `Model.Input`。`GetRunnableModels` 只检查协议适配器是否已注册，不会按文本或图片能力过滤模型。

即使 `Model.Input` 包含 `llm.Image`，模型服务仍可能对格式、像素、文件大小、图片数量或动画另有限制。内置模型清单没有统一表达这些限制。

## 切换到纯文本模型

协议适配器在转换消息时会检查目标模型。包含图片的历史发送给只支持文本的模型时，图片会被替换为文本占位符。这样可以保留对话顺序，但目标模型不会看到图片内容。

如果图片是后续回答所必需的信息，应在切换模型前重新选择支持图片的模型，或由应用先提取并保存所需文本信息。

降级转换针对本次请求创建消息副本，不会修改应用保存的原始历史。之后重新切换到支持图片的模型时，原始图片仍可使用。

## 结果与多轮对话

- 图片请求仍通过 `Complete` 或 `Stream` 返回普通 `AssistantMessage`，读取方式与文本请求相同。
- 图片可能影响模型服务计算的输入 token。`Usage` 采用模型服务返回的用量数据，`llm` 不单独计算图片 token。
- 将 `Context` 序列化为 JSON 时，base64 图片会一并保存。长对话反复携带图片会显著增加存储和请求体积。
- 如果后续轮次只需要图片中的结论，可由应用保存经过确认的文字摘要，并按业务规则决定是否继续携带原图。

## 文件与数据边界

- Base64 会使请求数据约增加三分之一。本示例同时在内存中保留原始字节和编码后的文本。
- 图片尺寸、文件大小、动画和支持的 MIME 类型由模型服务决定，`llm` 不会统一限制。
- `http.DetectContentType` 只检查文件开头，不能作为安全扫描。处理不可信上传时，应用还应验证文件内容和大小。
- 下载远程图片时应限制协议、主机、重定向、响应大小和超时，防止 SSRF 与资源耗尽。
- `Model.Input` 来自内置模型清单，可能晚于模型服务的更新。上线前应使用目标模型完成图片请求测试。
- 不要在日志中记录包含用户图片的序列化请求体。
- 图片可能包含个人信息、文档内容或位置数据。存储前应根据业务要求处理 EXIF、访问权限和保留期限。

多轮会话的保存见[保存与恢复对话](conversation-persistence.md)，更换模型时的历史转换见[对话中更换模型](model-switching.md)。
