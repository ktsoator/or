# Mock Provider 测试

## 本场景实现什么

`httptest` server 发出 OpenAI-compatible SSE，测试在没有 provider 账号和网络访问的情况下验证完整 `Complete` 请求路径。

将代码保存为依赖 `llm` 的 package 中的 `completion_test.go`。

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

运行 `go test ./...`。非空 request API key 只用于通过本地校验，不会发送到测试服务器之外。

## 下一步应该测试什么

- 检查请求 JSON，断言 tools、system prompt、reasoning 和 headers。
- 第一次请求发 `tool_calls`，第二次发最终答案，测试完整工具循环。
- 发错误 status 和畸形 SSE，验证部分消息与错误策略。
- 测试 `Stream` 取消，并继续读取到通道关闭，以发现 goroutine 泄漏。
- 测试注册自定义 adapter 或 provider override 时使用显式 client 和 registry，避免 package-global 状态跨测试污染。

Mock test 验证协议转换，不验证真实 provider 兼容性。生产使用的 provider 仍应保留少量带凭证集成测试。
