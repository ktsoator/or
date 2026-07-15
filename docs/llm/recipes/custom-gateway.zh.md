# 自定义网关

## Provider 级改道

将某个 provider 的全部请求发送到代理，不修改目录中的每个 `Model`：

```go
registry := llm.DefaultProviderRegistry()

baseURL := "https://gateway.example.com/deepseek/v1"
apiKey := "gateway-key"
registry.SetOverride("deepseek", llm.ProviderOverride{
	BaseURL: &baseURL,
	APIKey:  &apiKey,
	Headers: map[string]string{
		"X-Tenant": "tenant-a",
	},
})
defer registry.ClearOverride("deepseek")
```

`SetOverride` 影响使用该 registry 的后续请求。默认 registry 是进程共享状态；不同租户需要不同 URL 时应创建独立 `ProviderRegistry` 和 `Client`。

## 单个兼容模型

```go
model := llm.Model{
	ID:            "local-model",
	Name:          "Local Model",
	Provider:      "local",
	Protocol:      llm.ProtocolOpenAICompletions,
	BaseURL:       "http://localhost:8080/v1",
	Input:         []llm.ModelInput{llm.Text},
	ContextWindow: 32768,
	MaxTokens:     4096,
}

response, err := llm.Complete(ctx, model, llm.Prompt("hello"),
	llm.StreamOptions{APIKey: "local-key"})
```

endpoint 必须真实实现所选线协议。仅返回相似 JSON 不足以保证兼容。
