# 创建自定义 Client

包级 `Complete` 和 `Stream` 使用默认 Client 和默认注册表。需要自行控制网络连接、提供方配置或可用协议时，可以创建独立的 `llm.Client`。

这种方式适用于自定义 TLS 或代理、隔离不同租户、避免测试共享全局状态，以及限制应用能使用的协议。

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
	"github.com/ktsoator/or/llm/openai"
)

func main() {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 20
	transport.IdleConnTimeout = 90 * time.Second
	httpClient := &http.Client{Transport: transport}

	adapters := llm.NewAdapterRegistry()
	if err := adapters.Register(openai.NewAdapter(httpClient)); err != nil {
		log.Fatal(err)
	}
	providers := llm.NewBuiltInProviderRegistry()
	client := llm.NewClient(adapters, providers)

	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	response, err := client.Complete(context.Background(), model,
		llm.Prompt("Reply with OK."), llm.StreamOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.Text())
}
```

这里使用普通 import，因为程序要调用 `openai.NewAdapter` 并自行注册协议适配器。包级默认 Client 不会使用这个注册表。

## 专用 Client 的组成

示例显式创建 `AdapterRegistry`、`ProviderRegistry` 和 `http.Client`，再交给 `NewClient`。三类注册表各自保存什么、是否参与请求以及并发语义统一见[Client 与注册表](../clients-and-registries.md#三类注册表)。

`ModelRegistry` 只负责模型查找，不是 `NewClient` 的参数。`NewClient(adapters, nil)` 不使用提供方注册表，但已知 `Model.Provider` 仍会按旧环境变量映射查找 API 密钥。协议适配器注册表为 nil 时，请求会失败。

## 隔离范围与生命周期

- 注册表支持并发读取和修改。
- 应复用 `http.Client` 和 `http.Transport`。每次请求重新创建会失去连接池复用。
- `llm.Client` 没有 `Close` 方法。应用拥有传入的 HTTP transport；若需要，在应用退出时调用 `CloseIdleConnections`。
- 服务地址、请求头或网络策略不同的租户可各用一个 Client；仅 API 密钥不同可通过请求选项传入，无需拆分 Client。
- 需要限制二进制依赖或模型服务访问策略时，只注册允许的协议。

自定义提供方可在隔离注册表上注册 `NewSpecProvider`。接入框架尚未支持的请求与响应协议，参见[自定义协议](../extending.md)。
