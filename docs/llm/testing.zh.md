# 测试

基于 `llm` 的代码可以分三层测试。只有 adapter 请求路径需要模拟 provider 协议；业务结果处理和多数工具策略都可以使用普通 Go 值。

| 层级 | 测试对象 | 网络 | 推荐方式 |
|---|---|---|---|
| 结果处理 | 文本渲染、停止原因、usage、诊断 | 无 | 手工构造 `AssistantMessage` |
| 应用流程 | 历史、工具循环、重试决策 | 无或本地 | 纯值或有状态 mock |
| Adapter 路径 | 请求序列化、SSE、事件和错误映射 | 本地 HTTP | `httptest.Server` + 显式 `Model` |

## 用纯值测试响应处理

`AssistantMessage`、`Context` 和内容块是普通结构体。将响应处理逻辑提取成接收 `AssistantMessage` 的函数后，可以直接覆盖错误、截断、工具调用和 usage 分支。

```go
func renderReply(msg llm.AssistantMessage) string {
	if msg.StopReason == llm.StopReasonError {
		return "error: " + msg.ErrorMessage
	}
	return msg.Text()
}

func TestRenderReply(t *testing.T) {
	msg := llm.AssistantMessage{
		StopReason: llm.StopReasonStop,
		Content: []llm.AssistantContent{
			&llm.TextContent{Text: "hello"},
		},
	}
	if got := renderReply(msg); got != "hello" {
		t.Fatalf("got %q", got)
	}
}
```

可以通过修改值覆盖其他分支：

- 在 `Content` 中加入 `ToolCall`；
- 使用 `StopReasonLength` 或 `StopReasonAborted`；
- 填充 `Usage` 和 `Diagnostics`；
- 使用 `UserText`、`AssistantText` 和 `ToolResult` 构造输入历史。

## 针对 mock server 测试请求路径

完整的 OpenAI 兼容 SSE 测试只在 [Mock Provider 测试场景](recipes/mock-testing.md)维护。测试路径由以下部分组成：

1. 用 `httptest.NewServer` 返回目标协议的事件；
2. 手动构造 `Model`，把 `BaseURL` 指向 server；
3. 注册对应 adapter，或创建显式 `Client`；
4. 传入非空测试 key，使请求通过本地凭证校验；
5. 断言请求 body、增量顺序、终止事件和最终消息。

不要用 mock 测试声称某个线上 provider 已验证。Mock 只能证明 adapter 对已模拟的线格式行为正确。

## 测试工具循环

工具循环测试应让 mock server 按请求次数返回不同响应：第一轮返回工具调用，第二轮在请求历史包含 `ToolResult` 后返回最终文本。断言：

- 调用名称和参数经过 `DecodeToolCall`；
- 每个工具调用追加一个匹配 ID 的结果；
- 失败工具设置 `IsError`；
- 循环在最终停止原因或轮数上限结束；
- 带副作用工具不会因应用级 retry 被重复执行。

工具协议与执行清单见[工具](tools.md)，完整应用循环见[工具循环场景](recipes/tool-loop.md)。

## 取消、并发与状态隔离

- 为每个可能提前返回的流消费者增加取消测试。取消后继续接收直到通道关闭，并断言 goroutine 退出。
- 修改 adapter 或 provider override 的测试使用显式 `Client`，避免污染并行测试共享的默认注册表。
- 复用同一个测试 `http.Client`；不要为每个断言创建新的 Transport。
- 测试完成后关闭 `httptest.Server`，应用拥有的 Transport 可调用 `CloseIdleConnections`。

显式 client 的构造与所有权见[Client 与注册表](clients-and-registries.md)。

## 运行

编译并运行 LLM 包、示例和相关测试：

```sh
go test ./llm/... ./example/llm/...
```

该命令中的现有测试使用本地数据或 mock server，不要求真实 provider 账号。
