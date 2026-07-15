# 显式 Client

## 本场景实现什么

应用自行创建 HTTP transport、adapter registry、provider registry 和 `Client`，避免使用包级 provider override，并显式管理网络依赖。

自定义 TLS 或代理、租户隔离、测试、协议 allowlist 和自定义 adapter 适合使用这种形式。

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

这里使用普通 import，不使用副作用 import，因为程序自行构造并注册 adapter。

## 注册表职责

| 组件 | 职责 |
|---|---|
| `AdapterRegistry` | 将 `Model.Protocol` 映射到请求与响应协议实现 |
| `ProviderRegistry` | 解析凭证、URL override 和 headers |
| `ModelRegistry` | 可选的应用模型发现；`Client` 不依赖它 |
| `http.Client` | 连接池、代理、TLS 和 transport 行为 |

`NewClient(adapters, nil)` 跳过 provider registry，但已知 `Model.Provider` 仍使用旧环境变量映射。Adapter registry 为 nil 时请求失败。

## 生命周期与并发

- 注册表支持并发读取与修改，并在文档说明的位置返回防御性拷贝。
- 复用 `http.Client` 和 transport。每次请求创建 transport 会丢失连接池。
- `llm.Client` 没有 `Close` 方法。应用拥有传入的 HTTP transport；若需要，在应用退出时调用 `CloseIdleConnections`。
- Provider URL 或 header 不同的租户可以各用一个 client；只有 request API key 不同时无需拆 client。
- 需要限制二进制依赖或模型服务访问策略时，只注册允许的协议。

自定义 provider 可在隔离 registry 上注册 `NewSpecProvider`。接入框架尚未支持的请求与响应协议，参见[自定义协议](../extending.md)。
