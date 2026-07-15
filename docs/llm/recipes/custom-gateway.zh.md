# 接入自定义模型服务

本页用于将模型调用发送到代理、私有部署或兼容服务。模型服务地址是其公开的 HTTP API 地址，例如 `https://gateway.example.com/v1`。

## 选择接入方式

| 情况 | 做法 |
|---|---|
| 已收录提供方的所有请求都经过同一服务 | 使用 `ProviderOverride` |
| 只调用一个兼容服务，或服务不在内置模型清单中 | 直接构造 `Model` |
| 服务使用框架不支持的请求与响应格式 | 实现 `ProtocolAdapter` |

前两种方式复用现有协议适配器，不需要实现新的适配器。

## 覆盖已有提供方的配置

服务地址和 API 密钥从应用配置读取，不写入源码：

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

## 覆盖的生效范围

`ProviderOverride` 可覆盖 API 密钥、服务地址、请求头和环境值。它与请求选项、模型字段及进程环境的完整优先级只在[请求选项](../configuration.md#按请求提供凭证)和[模型与提供方](../providers.md#为-provider-的请求改道)中维护。

`SetOverride` 保存输入快照，之后修改原 map 不会改变注册内容。已经开始解析配置的请求不受后续更新影响。

## 接入单个兼容服务

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

模型服务必须实现所选协议的请求、流式响应和错误行为；只返回相似 JSON 不足以保证兼容。已知字段差异使用 `Model.Compatibility` 配置。只有请求与响应格式不属于现有协议时，才实现 `ProtocolAdapter`。

## 兼容性与安全边界

- `DefaultProviderRegistry` 是进程全局状态，不要在共享默认注册表上设置租户专用覆盖。
- 接受用户提供的服务地址前必须做 SSRF 控制和网络允许列表校验。
- 不要关闭 TLS 校验；自定义证书应配置在显式 `http.Transport` 上。
- 针对真实网关测试工具、推理、token 用量、重试和错误流。
- 测试后清理 override，或使用隔离 client，避免测试间泄漏。
