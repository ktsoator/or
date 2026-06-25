# 请求配置

`StreamOptions` 包含所有协议共享的设置。语义因协议而异的设置则嵌套在
`ProtocolOptions` 之下。

```go
temperature := 0.2
retries := 2
options := llm.StreamOptions{
	Temperature: &temperature,
	MaxTokens:   2048,
	MaxRetries:  &retries,
	Timeout:     30 * time.Second,
	Headers: map[string]string{
		"X-Request-ID": requestID,
	},
}
```

共享选项有：

| 选项 | 用途 |
|---|---|
| `APIKey` | 为本次请求覆盖凭证查找 |
| `Env` | 覆盖具名的环境值，而不改动进程环境 |
| `Temperature` | 覆盖模型采样温度 |
| `MaxTokens` | 限制输出 token；为零则使用模型默认值 |
| `Headers` | 在模型默认值之上合并自定义 HTTP 请求头 |
| `Reasoning` | 请求一个与厂商无关的推理努力级别 |
| `ProtocolOptions` | 携带专属于某一种线缆协议的设置 |
| `MaxRetries` | 覆盖 SDK 重试次数；`0` 表示禁用 |
| `Timeout` | 独立于 context 取消之外，限制每次 HTTP 尝试 |
| `OnRequest` | 观察每一次序列化后的 HTTP 请求尝试 |
| `RewriteRequest` | 在发送前替换序列化的请求体 |
| `OnResponse` | 观察每一次 HTTP 响应尝试 |

## 观察 HTTP 请求与响应

这些钩子可用于日志、追踪和调试。两者都按每次尝试触发一次，因此重试始终可见。
`OnRequest` 收到的是为提供方序列化的确切请求体，包含协议特定字段。

```go
options := llm.StreamOptions{
	OnRequest: func(method, url string, body []byte) {
		log.Printf("→ %s %s\n%s", method, url, body)
	},
	OnResponse: func(status int, headers http.Header) {
		log.Printf("← %d", status)
	},
}
```

钩子可能暴露提示词、工具参数、URL 或请求头中的凭证，以及提供方响应元数据。在发送到
日志或遥测系统之前，请对敏感数据做脱敏处理。

## 重写请求体

`RewriteRequest` 在请求体发送前对其进行变换，是一个针对类型化 API 未暴露的提供方
特定字段的逃生口。它接收与 `OnRequest` 相同的方法、URL 和请求体，并返回要发送的
请求体；返回 `nil` 表示保持不变。与观察器一样，它按每次尝试触发一次，并且总是对原始
请求体进行重写，因此重试保持一致。

```go
options := llm.StreamOptions{
	RewriteRequest: func(method, url string, body []byte) []byte {
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil // 出错时保持请求体不变
		}
		payload["custom_provider_field"] = true
		patched, err := json.Marshal(payload)
		if err != nil {
			return nil
		}
		return patched
	},
}
```

本包当前附带的协议特定选项类型，参见[推理](reasoning.md)和[工具](tools.md)。
