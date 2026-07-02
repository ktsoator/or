# 测试

无需调用真实 provider，即可测试基于 `llm` 构建的代码。两种技术覆盖大多数需求：手动构造结果值来测试读取响应的代码，以及把模型指向本地 mock server 来测试完整请求路径。

## 用纯值测试响应处理

`AssistantMessage`、`Context` 及内容块都是普通结构体。消费响应的代码（渲染、提取工具调用、按停止原因分支）接收的是 `AssistantMessage`，所以把这类代码抽成函数，并用测试中构造的值来调用。无网络、无 key。

```go
// 被测代码:把响应转成展示文本。
func renderReply(msg llm.AssistantMessage) string {
	if msg.StopReason == llm.StopReasonError {
		return "error: " + msg.ErrorMessage
	}
	return msg.Text()
}

func TestRenderReply(t *testing.T) {
	msg := llm.AssistantMessage{
		StopReason: llm.StopReasonStop,
		Content:    []llm.AssistantContent{&llm.TextContent{Text: "hello"}},
	}
	if got := renderReply(msg); got != "hello" {
		t.Fatalf("got %q", got)
	}
}
```

按需构造任意形态：往 `Content` 加一个 `ToolCall` 测试工具处理，把 `StopReason` 设为 `StopReasonLength` 测试截断，或填充 `Usage` 测试成本核算。输入侧同理——用 `UserText`、 `AssistantText` 与 `ToolResult` 组装 `Context` 来演练历史逻辑。

## 针对 mock server 测试请求路径

要测试真正调用 `Stream` 或 `Complete` 的代码，起一个讲 provider 线格式的 `httptest` 服务器，然后用 `BaseURL` 把 `Model` 指向它。像平常一样注册对应的 provider 包，并传入任意非空 `APIKey` 以满足 key 解析。

OpenAI 兼容端点以 Server-Sent Events 流式返回——每个块一行 `data:`，以 `data: [DONE]` 结尾：

```go
import (
	"net/http"
	"net/http/httptest"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func TestComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, `data: {"choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}`+"\n\n")
			io.WriteString(w, `data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n")
			io.WriteString(w, "data: [DONE]\n\n")
		}))
	defer server.Close()

	model := llm.Model{
		ID:       "test-model",
		Provider: "test",
		Protocol: llm.ProtocolOpenAICompletions,
		BaseURL:  server.URL + "/v1",
	}

	msg, err := llm.Complete(context.Background(), model,
		llm.Prompt("hello"), llm.StreamOptions{APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text() != "hi" {
		t.Fatalf("got %q", msg.Text())
	}
}
```

同一个服务器也适用于 `Stream`：遍历事件，对 `EventTextDelta` 序列与终止的 `EventDone` 做断言。

## 测试工具循环

让 mock server 按请求次数返回不同响应：第一次返回工具调用，第二次返回最终答案。仅靠计数请求就足以驱动一个完整循环：

```go
var calls int
server := httptest.NewServer(http.HandlerFunc(
	func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		calls++
		if calls == 1 {
			// 第 1 轮:请求一次工具调用。
			io.WriteString(w, `data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Paris\",\"days\":3}"}}]},"finish_reason":null}]}`+"\n\n")
			io.WriteString(w, `data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n")
		} else {
			// 第 2 轮:带着工具结果给出答案。
			io.WriteString(w, `data: {"choices":[{"index":0,"delta":{"content":"Pack light."},"finish_reason":null}]}`+"\n\n")
			io.WriteString(w, `data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n")
		}
		io.WriteString(w, "data: [DONE]\n\n")
	}))
defer server.Close()
```

用该循环跑这个服务器，断言它解码了调用、追加了 `ToolResult`，并在第二轮到达最终文本。这样便可端到端演练循环逻辑（即[工具循环清单](tools.md#运行工具循环)中的各项）而无需 provider。

对于 Anthropic 兼容目标，设置 `Protocol: llm.ProtocolAnthropicMessages` 并改为发出该协议的事件形态。
