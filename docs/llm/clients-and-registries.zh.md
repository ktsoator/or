# `Client` 与注册表

包级 `llm.Stream` 和 `llm.Complete` 使用一个全局默认 `Client`。默认 `Client` 由协议适配器注册表和内置提供方注册表组成。多数应用只需以副作用方式导入相应的协议包，然后调用包级函数。

独立 `Client` 用于以下场景：

- 为不同子系统或租户隔离提供方覆盖配置；
- 注入自定义 `http.Client`，配置代理、TLS 或连接池；
- 测试时避免修改包级全局状态；
- 只注册允许使用的协议；
- 接入自定义 `ProtocolAdapter`。

## 三类注册表

| 类型 | 存储内容 | 主要键 | 是否参与请求 |
|---|---|---|---|
| `AdapterRegistry` | `ProtocolAdapter` | `Protocol` | 根据 `Model.Protocol` 选择请求与响应协议实现 |
| `ProviderRegistry` | 提供方配置和覆盖配置 | `Model.Provider` | 解析凭证、服务地址和请求头 |
| `ModelRegistry` | `Model` 元数据 | 提供方 ID + 模型 ID | 用于模型发现；`Client` 不直接依赖它 |

三个注册表互不替代。将模型加入 `ModelRegistry` 不会注册协议适配器，也不会自动增加提供方凭证配置。

## 默认 `Client`

`llm/default.go` 在包初始化时创建：

```text
defaultRegistry         = NewAdapterRegistry()
defaultProviderRegistry = NewBuiltInProviderRegistry()
defaultClient           = NewClient(defaultRegistry, defaultProviderRegistry)
```

`llm/openai` 和 `llm/anthropic` 在各自的 `init` 中向默认协议适配器注册表注册。只有被导入的协议才会出现在 `SupportsProtocol` 中。

```go
import (
	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

response, err := llm.Complete(ctx, model, input, options)
```

`DefaultProviderRegistry` 返回默认 `Client` 使用的提供方注册表。对它调用 `SetOverride` 会影响此后通过包级函数发送给相应提供方的请求。

## 独立 `Client`

独立 `Client` 由应用持有的协议适配器注册表和提供方注册表组成：

```go
adapters := llm.NewAdapterRegistry()
if err := adapters.Register(openai.NewAdapter(httpClient)); err != nil {
	log.Fatal(err)
}
client := llm.NewClient(adapters, llm.NewBuiltInProviderRegistry())
```

`NewClient(adapters, nil)` 不使用提供方注册表，但仍按 `Model.Provider` 查找兼容保留的环境变量映射。自定义提供方不在该映射中时，应直接传入 `StreamOptions.APIKey`。

完整导入、HTTP 传输配置和请求程序见[创建自定义 Client](recipes/custom-client.md)。

## 自定义 HTTP 客户端

两个内置协议适配器都接受 `*http.Client`。以下片段只展示连接池设置；完整的 `Client` 构造程序不在本页重复：

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

`openai.NewAdapter(nil)` 使用 `http.DefaultClient`。`anthropic.NewAdapter(nil)` 使用 Anthropic SDK 的默认 HTTP 客户端。请求级 `StreamOptions.Timeout` 仍会作用于底层 SDK 请求。

共享同一个 `http.Client` 可复用连接池。不要为每次模型调用创建新的 `http.Transport`。

## ProviderRegistry

### 构造

```go
empty := llm.NewProviderRegistry()
builtIn := llm.NewBuiltInProviderRegistry()
defaultRegistry := llm.DefaultProviderRegistry()
```

- `NewProviderRegistry` 返回空注册表；
- `NewBuiltInProviderRegistry` 从嵌入模型清单构造提供方配置；
- `DefaultProviderRegistry` 返回包级 `Client` 正在使用的实例。

### 查询与修改

| 方法 | 作用 |
|---|---|
| `Register(provider)` | 新增或替换提供方 |
| `Get(providerID)` | 查询提供方 |
| `Providers()` | 按 ID 排序返回提供方 |
| `SetOverride(providerID, override)` | 设置服务地址、凭证、请求头或环境变量覆盖 |
| `ClearOverride(providerID)` | 删除覆盖 |
| `AuthStatus(providerID, env)` | 检查凭证是否可解析 |
| `ResolveRequest(model, options)` | 计算协议适配器最终收到的模型和选项 |

`ProviderRegistry` 使用读写锁保护内部映射。注册提供方或设置覆盖配置时会复制输入值。已经完成配置解析的在途请求不受后续修改影响。

## ModelRegistry

`ModelRegistry` 是可选的应用级模型清单：

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
| `Providers()` | 返回按字典序排序的提供方 ID |
| `Models(provider)` | 返回按模型 ID 排序的模型 |

包级 `LookupModel` 和 `GetModel` 始终查询内置模型清单，不查询应用创建的 `ModelRegistry`。从自建注册表取出的 `Model` 可直接传给独立 `Client` 或包级函数。

## 并发与生命周期

- `AdapterRegistry`、`ProviderRegistry` 和 `ModelRegistry` 可并发读取与修改。
- 协议适配器持有的 `http.Client` 应在 `Client` 生命周期内复用。
- `Client` 没有 `Close` 方法。每个请求流由协议适配器在后台协程退出时关闭。
- 使用 `Stream` 时必须继续读取事件通道直到关闭。即使业务不再处理增量，也要读取并丢弃剩余事件。
- 提供方覆盖配置只存在于当前进程，不会写入磁盘。
- `DefaultProviderRegistry` 是全局共享状态。测试若修改它，应在结束时调用 `ClearOverride`，或改用独立 `Client`。

## 选择方式

| 场景 | 推荐方式 |
|---|---|
| 单一应用、固定提供方 | 包级 `Stream`/`Complete` |
| 多租户且每次请求使用独立凭证 | 包级函数或独立 `Client` + `StreamOptions.APIKey` |
| 每个租户使用独立服务地址或请求头 | 每个租户创建独立的 `ProviderRegistry` 和 `Client` |
| 自定义代理、TLS 或连接池 | 独立协议适配器 + 自定义 `http.Client` |
| 单元测试或本地模拟服务 | 使用独立 `Client`，避免修改默认注册表 |
| 框架尚未支持的请求与响应协议 | 自定义 `ProtocolAdapter` |
