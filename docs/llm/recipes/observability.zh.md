# 可观测性 Hook

## 本场景实现什么

请求记录每次尝试的耗时、method、URL、序列化字节数、status 和 provider request ID，但不记录提示正文或凭证。

Hook 每次 SDK attempt 都会运行，因此重试可见。它们在请求路径中同步执行。

## 完整程序

```go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func main() {
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	started := time.Now()
	options := llm.StreamOptions{
		Headers: map[string]string{"X-Request-ID": "example-123"},
		OnRequest: func(method, url string, body []byte) {
			log.Printf("llm request method=%s url=%s bytes=%d",
				method, url, len(body))
		},
		OnResponse: func(status int, headers http.Header) {
			log.Printf("llm response status=%d request_id=%q elapsed=%s",
				status, headers.Get("X-Request-ID"), time.Since(started))
		},
	}
	response, err := llm.Complete(context.Background(), model,
		llm.Prompt("Reply with OK."), options)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.Text())
}
```

## Hook 语义

| Hook | 输入 | 失败行为 |
|---|---|---|
| `OnRequest` | 每次尝试的实际 method、URL 和序列化 body | 不返回 error；阻塞会延迟请求 |
| `RewriteRequest` | 相同请求数据，返回替换字节 | `nil` 保留原值；无效 JSON 会让 provider 调用失败 |
| `OnResponse` | 每次 HTTP 响应的 status 和 headers | 在消费 response body 前运行 |

`RewriteRequest` 是为类型化选项未表达的 provider 字段准备的逃生口。应优先使用 `StreamOptions`、协议选项、模型兼容字段、headers 或 provider override。

## 安全与运维

- Request body 包含提示词、图片、工具和工具参数。应记录大小或脱敏结构，不记录原始字节。
- URL 和 header 可能含租户 ID 或凭证，导出属性前使用 allowlist。
- 慢速遥测写入应放入应用缓冲队列；hook callback 只执行有界工作。
- SDK 重试会让一个逻辑请求产生多次 hook 调用；需要时由应用记录 attempt 编号。
- 本包没有 logger、metrics exporter 或 trace backend，hook 必须接入应用自己的遥测系统。
