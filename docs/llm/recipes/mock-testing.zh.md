# 使用本地模拟服务测试

`httptest.Server` 可以在本机模拟模型服务。测试不需要真实账号或外部网络，即可覆盖请求序列化、流式响应解析和最终消息组装。

下面的测试模拟 OpenAI Chat Completions 的 SSE 响应。将代码保存为使用 `llm` 的 Go package 中的 `completion_test.go`。

## 完整测试

```go
package myapp_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func TestCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/chat/completions" {
				t.Fatalf("path = %q", r.URL.Path)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, `data: {"choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}`+"\n\n")
			io.WriteString(w, `data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n")
			io.WriteString(w, "data: [DONE]\n\n")
		}))
	defer server.Close()

	model := llm.Model{
		ID: "test-model", Provider: "test",
		Protocol: llm.ProtocolOpenAICompletions,
		BaseURL: server.URL + "/v1", MaxTokens: 100,
	}
	message, err := llm.Complete(context.Background(), model,
		llm.Prompt("hello"), llm.StreamOptions{APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if got := message.Text(); got != "hi" {
		t.Fatalf("text = %q, want hi", got)
	}
	if message.StopReason != llm.StopReasonStop {
		t.Fatalf("stop = %q", message.StopReason)
	}
}
```

运行：

```sh
go test ./...
```

示例中的 API 密钥只用于通过请求校验，并仅发送给本地测试服务器。

## 可以继续覆盖的路径

- 读取请求 JSON，断言工具、系统指令、推理选项和请求头是否正确。
- 第一次返回工具调用，第二次返回最终答案，覆盖完整工具调用流程。
- 返回错误状态码或格式错误的 SSE，验证部分消息和错误处理。
- 取消 `Stream`，并继续读取到通道关闭，检查 goroutine 是否能结束。
- 测试自定义协议适配器或提供方覆盖时，使用自定义 Client 和独立注册表，避免包级全局状态影响其他测试。

## 测试范围

本地模拟测试验证应用逻辑和协议转换，不验证真实模型服务的鉴权、限流、字段差异或线上行为。生产使用的模型服务仍应保留少量带凭证的集成测试。
