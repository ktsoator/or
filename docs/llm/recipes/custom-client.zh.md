# 显式 Client

## 用途

注入自定义 HTTP Transport，并隔离 adapter 和 provider 配置。

## 核心代码

```go
transport := http.DefaultTransport.(*http.Transport).Clone()
transport.Proxy = http.ProxyFromEnvironment
transport.MaxIdleConnsPerHost = 20

httpClient := &http.Client{Transport: transport}

adapters := llm.NewAdapterRegistry()
if err := adapters.Register(openai.NewAdapter(httpClient)); err != nil {
	log.Fatal(err)
}

providers := llm.NewBuiltInProviderRegistry()
client := llm.NewClient(adapters, providers)

model := llm.GetModel("deepseek", "deepseek-v4-flash")
response, err := client.Complete(ctx, model,
	llm.Prompt("Reply with OK."), llm.StreamOptions{})
```

需要普通导入：

```go
import "github.com/ktsoator/or/llm/openai"
```

显式注册时不依赖 adapter 的副作用 import。复用 `http.Client` 和 Transport，不要每次请求重新创建连接池。
