# 记录和修改请求

`StreamOptions` 提供三个请求回调：`OnRequest` 读取即将发送的请求，`RewriteRequest` 修改序列化后的请求体，`OnResponse` 读取 HTTP 状态和响应头。

示例记录请求方法、地址、请求体大小、响应状态、请求 ID 和耗时，不记录提示词正文或凭证。每次 SDK 请求尝试都会调用这些回调，因此重试也会产生记录。

## 运行前准备

```sh
go get github.com/ktsoator/or/llm@latest
export DEEPSEEK_API_KEY=your-key
```

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

## 回调的执行时机

示例使用 `OnRequest` 观察序列化请求、用 `RewriteRequest` 返回替换后的请求体，并在 `OnResponse` 中读取状态码和响应头。三个回调都会对每次 SDK 尝试执行；精确签名、返回行为和调用约束统一见[请求选项](../configuration.md#观察-http-请求与响应)。

`RewriteRequest` 用于补充类型化选项尚未支持的模型服务字段。优先使用 `StreamOptions`、协议选项、`Model.Compatibility`、请求头或提供方覆盖；这些方式更容易校验和维护。

## 日志与性能边界

- 请求体可能包含提示词、图片、工具定义和工具参数。只记录大小或脱敏后的结构，不要记录原始字节。
- URL 和响应头可能包含租户信息或凭证。导出日志和指标前使用允许列表筛选字段。
- 回调在请求路径中同步执行。慢速日志或遥测写入应交给应用自己的缓冲队列。
- 一次逻辑请求可能因 SDK 重试触发多组回调。需要区分时，由应用记录尝试序号。
- `llm` 不提供日志器、指标导出器或链路追踪后端；回调需要接入应用现有的可观测系统。
