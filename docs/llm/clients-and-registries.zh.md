# Client 与注册表

包级 `llm.Stream` 和 `llm.Complete` 使用一个全局默认 `Client`。默认 client 由一个 adapter 注册表和一个内置 provider 注册表组成。多数应用只需要空导入 adapter，然后调用包级入口。

显式 `Client` 用于以下场景：

- 为不同子系统或租户隔离 provider override；
- 注入自定义 `http.Client`、Transport、代理或 TLS 配置；
- 测试时避免修改包级全局状态；
- 只注册允许使用的协议；
- 接入自定义 `ProtocolAdapter`。

## 三类注册表

| 类型 | 存储内容 | 主要键 | 是否参与请求 |
|---|---|---|---|
| `AdapterRegistry` | `ProtocolAdapter` | `Protocol` | 根据 `Model.Protocol` 选择请求与响应协议实现 |
| `ProviderRegistry` | Provider 配置和 override | `Model.Provider` | 解析 key、URL 和 headers |
| `ModelRegistry` | `Model` 元数据 | provider + model ID | 用于模型发现；`Client` 不直接依赖它 |

三个注册表互不替代。将模型加入 `ModelRegistry` 不会注册 adapter，也不会自动增加 provider 凭证配置。

## 默认 Client

`llm/default.go` 在包初始化时创建：

```text
defaultRegistry         = NewAdapterRegistry()
defaultProviderRegistry = NewBuiltInProviderRegistry()
defaultClient           = NewClient(defaultRegistry, defaultProviderRegistry)
```

`llm/openai` 和 `llm/anthropic` 在各自的 `init` 中向默认 adapter 注册表注册。只有被导入的协议才会出现在 `SupportsProtocol` 中。

```go
import (
	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

response, err := llm.Complete(ctx, model, input, options)
```

`DefaultProviderRegistry` 返回默认 client 使用的 provider 注册表。对它调用 `SetOverride` 会影响进程中之后通过包级入口发送的对应 provider 请求。

## 显式 Client

显式 client 由应用持有的 adapter registry 和 provider registry 组成：

```go
adapters := llm.NewAdapterRegistry()
if err := adapters.Register(openai.NewAdapter(httpClient)); err != nil {
	log.Fatal(err)
}
client := llm.NewClient(adapters, llm.NewBuiltInProviderRegistry())
```

`NewClient(adapters, nil)` 跳过 provider 注册表，但仍按 `Model.Provider` 查找旧版环境变量映射。自定义 provider 不在该映射中时，应显式传入 `StreamOptions.APIKey`。

完整 import、Transport 配置和请求程序见[显式 Client 场景](recipes/custom-client.md)。

## 自定义 HTTP Client

两个内置 adapter 接受 `*http.Client`。以下片段只展示连接池设置；完整 client 构造不在本页重复：

```go
transport := http.DefaultTransport.(*http.Transport).Clone()
transport.MaxIdleConns = 100
transport.MaxIdleConnsPerHost = 20

httpClient := &http.Client{Transport: transport}

adapters := llm.NewAdapterRegistry()
if err := adapters.Register(openai.NewAdapter(httpClient)); err != nil {
	log.Fatal(err)
}

client := llm.NewClient(adapters, llm.NewBuiltInProviderRegistry())
```

`openai.NewAdapter(nil)` 使用 `http.DefaultClient`。`anthropic.NewAdapter(nil)` 让 Anthropic SDK 使用它的默认 client。请求级 `StreamOptions.Timeout` 仍会应用在 SDK 请求上。

共享同一个 `http.Client` 可复用连接池。不要为每次模型调用创建新的 Transport。

## ProviderRegistry

### 构造

```go
empty := llm.NewProviderRegistry()
builtIn := llm.NewBuiltInProviderRegistry()
defaultRegistry := llm.DefaultProviderRegistry()
```

- `NewProviderRegistry` 返回空注册表；
- `NewBuiltInProviderRegistry` 从嵌入目录构造 provider 配置；
- `DefaultProviderRegistry` 返回包级 client 正在使用的实例。

### 查询与修改

| 方法 | 作用 |
|---|---|
| `Register(provider)` | 新增或替换 provider |
| `Get(providerID)` | 查询 provider |
| `Providers()` | 按 ID 排序返回 provider |
| `SetOverride(providerID, override)` | 设置 URL、key、headers 或环境覆盖 |
| `ClearOverride(providerID)` | 删除覆盖 |
| `AuthStatus(providerID, env)` | 检查凭证是否可解析 |
| `ResolveRequest(model, options)` | 计算 adapter 最终收到的模型和选项 |

`ProviderRegistry` 使用读写锁保护内部 map。注册和 override 会复制输入值。已经解析完成的在途请求不受之后修改影响。

## ModelRegistry

`ModelRegistry` 是可选的应用级模型目录：

```go
models := llm.NewModelRegistry()
if err := models.Register(llm.Model{
	ID:       "local-model",
	Name:     "Local Model",
	Provider: "local",
	Protocol: llm.ProtocolOpenAICompletions,
	BaseURL:  "http://localhost:8080/v1",
	Input:    []llm.ModelInput{llm.Text},
}); err != nil {
	log.Fatal(err)
}

model, ok := models.Get("local", "local-model")
```

| 方法 | 作用 |
|---|---|
| `Register(model)` | 校验并新增或替换模型 |
| `Get(provider, modelID)` | 返回模型的防御性拷贝 |
| `Providers()` | 返回按字典序排序的 provider ID |
| `Models(provider)` | 返回按模型 ID 排序的模型 |

包级 `LookupModel` 和 `GetModel` 始终查询内置目录，不查询应用创建的 `ModelRegistry`。从自建注册表取出的 `Model` 可直接传给显式或包级 client。

## 并发与生命周期

- `AdapterRegistry`、`ProviderRegistry` 和 `ModelRegistry` 可并发读取与修改。
- adapter 保存的 `http.Client` 应在 client 生命周期内复用。
- `Client` 没有 `Close` 方法。每个请求流由 adapter 在消费 goroutine 退出时关闭。
- 使用 `Stream` 时必须继续读取事件通道直到关闭。若业务不再处理增量，仍应 drain 通道。
- provider override 是进程内配置，不会写入磁盘。
- `DefaultProviderRegistry` 是全局共享状态。测试若修改它，应在结束时调用 `ClearOverride`，或改用显式 client。

## 选择方式

| 场景 | 推荐方式 |
|---|---|
| 单一应用、固定 provider | 包级 `Stream`/`Complete` |
| 多租户且每次请求有独立 key | 包级或显式 client + `StreamOptions.APIKey` |
| 每个租户有独立 URL/headers | 每租户独立 `ProviderRegistry` 和 `Client` |
| 自定义代理、TLS 或 Transport | 显式 adapter + 自定义 `http.Client` |
| 单元测试或 mock server | 显式 client，避免修改默认注册表 |
| 框架尚未支持的请求与响应协议 | 自定义 `ProtocolAdapter` |
