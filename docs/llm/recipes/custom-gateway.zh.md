# 自定义网关

## 选择接入方式

现有 provider 的全部请求都要使用同一个网关时，使用 provider override。只接入一个兼容 endpoint，或 provider 不在目录中时，直接构造 `Model`。两种方式都不会增加新线协议。

## 完整 Provider Override 程序

网关 URL 和 key 从应用配置读取，不写在源码中：

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func main() {
	baseURL := os.Getenv("GATEWAY_BASE_URL")
	apiKey := os.Getenv("GATEWAY_API_KEY")
	if baseURL == "" || apiKey == "" {
		log.Fatal("set GATEWAY_BASE_URL and GATEWAY_API_KEY")
	}

	registry := llm.DefaultProviderRegistry()
	registry.SetOverride("deepseek", llm.ProviderOverride{
		BaseURL: &baseURL,
		APIKey:  &apiKey,
		Headers: map[string]string{"X-Tenant": "team-a"},
	})
	defer registry.ClearOverride("deepseek")

	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	response, err := llm.Complete(context.Background(), model,
		llm.Prompt("Reply with OK."), llm.StreamOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.Text())
}
```

## 优先级

| 设置 | 从高到低 |
|---|---|
| API key | 请求 `APIKey` → override `APIKey` → 请求 `Env` → override `Env` → 进程环境 |
| Base URL | provider override → `Model.BaseURL` |
| 同名 Header | 请求 → override → provider spec → model |

`SetOverride` 保存输入快照，之后修改原 map 不会改变注册内容。已经解析完配置的请求不受后续更新影响。

## 单个兼容 Endpoint

```go
model := llm.Model{
	ID: "local-model", Name: "Local Model", Provider: "local",
	Protocol: llm.ProtocolOpenAICompletions,
	BaseURL: "http://localhost:8080/v1",
	Input: []llm.ModelInput{llm.Text}, MaxTokens: 4096,
}
response, err := llm.Complete(ctx, model, llm.Prompt("hello"),
	llm.StreamOptions{APIKey: "local-key"})
```

Endpoint 必须实现所选协议的流式和错误行为，只返回相似 JSON 并不足够。已知方言差异用 `Model.Compatibility` 配置；只有真正不同的线协议才实现 `ProtocolAdapter`。

## 运维约束

- `DefaultProviderRegistry` 是进程全局状态，不要在共享默认注册表上设置租户专用 override。
- 接受用户提供的 base URL 前必须做 SSRF 控制和网络 allowlist。
- 不要关闭 TLS 校验；自定义证书应配置在显式 `http.Transport` 上。
- 针对真实网关测试工具、推理、usage、重试和错误流。
- 测试后清理 override，或使用隔离 client，避免测试间泄漏。
