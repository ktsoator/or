# 模型与协议

[`model.go`](https://github.com/ktsoator/or/blob/main/llm/model.go)
定义了描述一个模型所需的类型：协议、调用方可设置的中立选项，以及把端点同其能力与价格绑定在一起的 `Model`。模型本身存放于何处、又如何注册与取用，则由 [`model_registry.go`](https://github.com/ktsoator/or/blob/main/llm/model_registry.go) 与 [`catalog.go`](https://github.com/ktsoator/or/blob/main/llm/catalog.go) 实现。本页先讲单个模型如何定义、如何随协议解码、能力如何查询，再讲这些模型如何集中存放与取用。

## 中立类型

若干设置都是小型字符串类型，各自构成一组固定的常量。将它们定义为具名类型——而非裸字符串——既能让编译器捕捉拼写错误，也使公开 API 自带文档。

```go
type Protocol string           // "openai-completions"、"anthropic-messages"
type ModelInput string         // "text"、"image"
type ModelThinkingLevel string // off、minimal、low、medium、high、xhigh
type ThinkingDisplay string    // summarized、omitted
```

`Protocol` 命名一种通信协议，并经由它确定负责对接的适配器。`ModelInput` 命名一种模态，模型在 `Model.Input` 中列出自己接受的模态；发往纯文本模型的图像会被降级，而非直接拒绝。

`ModelThinkingLevel` 与厂商无关，模型通过 `Model.ThinkingLevelMap` 声明各级别如何映射到自家的方言，适配器在构建请求时据此查表。`ThinkingDisplay` 的作用更窄：它不改变模型是否推理、也不改变计费，只决定返回什么——`summarized` 返回可读的思考文本，`omitted` 保留签名但去掉文本。目前仅 Anthropic 协议遵从该设置。

## 定价

`ModelCost` 以「每百万 token 的美元价」存储价格，并按计费方式拆分：

```go
type ModelCost struct {
	Input      float64 // 新输入 token
	Output     float64 // 生成的 token
	CacheRead  float64 // 从提示缓存命中的 token
	CacheWrite float64 // 写入提示缓存的 token
}
```

这四类与响应上的 `Usage` 计数一一对应。`CalculateCost` 据此逐类计算——价格以每百万 token 计，故每类的花费即「单价 ÷ 1,000,000 × 该类 token 数」，四类相加得总额。缓存读写之所以与新输入分开计价，是因为厂商对它们的收费各不相同。

## Model

`Model` 按四类关注点分组，源码中的注释标出了边界：

```go
type Model struct {
	// 身份
	ID, Name, Provider string

	// 路由
	Protocol Protocol
	BaseURL  string
	Headers  map[string]string

	// 能力
	Reasoning        bool
	ThinkingLevelMap map[ModelThinkingLevel]*string
	Input            []ModelInput
	ContextWindow    int64
	MaxTokens        int64

	// 定价与各厂商差异
	Cost          ModelCost
	Compatibility ModelCompatibility
}
```

`Protocol` 是路由的判别器：`Client.Stream` 据此选取适配器。`BaseURL` 与 `Headers` 则使兼容厂商得以复用某种协议——将基址指向该厂商的端点，补上所需的请求头，同一个适配器即可对接。`ContextWindow` 是 token 总预算，`MaxTokens` 是生成上限；二者既参与请求构建，也参与[溢出检测](transform.md)。

`ThinkingLevelMap` 刻意采用指针值。`nil` 标记该级别不受支持；键缺失则回退到厂商默认值。这是两种不同的情形，而指针正是用以区分二者的手段——普通 `string` 无法区分「明确关闭」与「未配置」。

## 厂商兼容性

实现同一协议的厂商之间，仍存在细微差异，承载这些差异的是按协议划分的兼容性结构体（`Model.Compatibility`）。它是可选的覆盖项：留空时，适配器一律按参考实现的默认行为处理；仅当某厂商确有偏离时，才填上对应字段。

Anthropic 一侧较短，因为多数 Anthropic 兼容厂商无需任何覆盖：

```go
type AnthropicMessagesCompatibility struct {
	SupportsTemperature       *bool
	SupportsCacheControl      *bool
	SupportsCacheControlTools *bool
	ForceAdaptiveThinking     *bool
	AllowEmptySignature       *bool
}
```

OpenAI 一侧承载得更多，因为「OpenAI 兼容」涵盖的端点范围很广：

```go
type OpenAICompletionsCompatibility struct {
	SupportsStore           *bool
	SupportsDeveloperRole   *bool
	SupportsReasoningEffort *bool
	MaxTokensField          string // "max_tokens" 还是 "max_completion_tokens"
	SupportsStrictMode      *bool
	RequiresThinkingAsText  *bool  // 将思考作为前置文本块发送
	ThinkingFormat          string
	// …… 以及若干其他字段
}
```

其中的布尔字段为指针自有缘由。普通 `bool` 只有两种状态，无法区分「该厂商明确不支持此项」与「未指定，采用默认」。`*bool` 则有三种：`true`、`false` 与 `nil`，其中 `nil` 即默认路径。字符串字段（`MaxTokensField`、`ThinkingFormat`）直接指名某种变体，空串表示「采用参考实现的行为」。

## 按协议解码

两个兼容性结构体都满足同一个接口，其方法报告该配置描述的是哪种协议、并给出一份独立副本：

```go
type ModelCompatibility interface {
	Protocol() Protocol
	clone() ModelCompatibility
}
```

`Protocol()` 报告该兼容性属于哪种协议；`clone()` 返回一份完全独立的深拷贝。把 `clone()` 放在每个具体类型上，意味着 `cloneModel` 不必做类型 switch，新增字段也只会改动它所属的那个类型。这使 `Model` 不依赖于任何单一协议。代价在于 `compat` 字段是接口类型，而 JSON 并不携带「它装的是哪个具体结构体」的标签——解码时须自行选定。`Model.UnmarshalJSON` 正是以 `Protocol` 作为判别器来完成这一选择：

```go linenums="1" hl_lines="3 12 16"
func (model *Model) UnmarshalJSON(data []byte) error {
	// 解码除 compat 外的每个字段，并将 compat 暂存为原始字节。
	type modelAlias Model // (1)!
	wire := struct {
		*modelAlias
		Compatibility json.RawMessage `json:"compat"`
	}{modelAlias: (*modelAlias)(model)}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}

	if len(wire.Compatibility) == 0 || isJSONNull(wire.Compatibility) {
		model.Compatibility = nil // 无覆盖
		return nil
	}
	switch model.Protocol { // (2)!
	case ProtocolOpenAICompletions:
		var c OpenAICompletionsCompatibility
		// 将 wire.Compatibility 反序列化进 c，赋为 &c
	case ProtocolAnthropicMessages:
		var c AnthropicMessagesCompatibility
		// ...
	default:
		return fmt.Errorf("unsupported compatibility protocol %q", model.Protocol)
	}
}
```

1.  `modelAlias` 这一别名类型丢弃了 `UnmarshalJSON` 方法，因此向它反序列化不会递归回到本函数。`compat` 以 `json.RawMessage` 暂留，待第二遍再解码。
2.  `Protocol` 已在第一遍中解码完毕，因此此处可用它选定具体的兼容性类型。

请求时驱动路由的字段，与解码时选定类型的字段是同一个。模型得以序列化为 JSON 再还原，而无需额外的类型标签，因为它的协议本身已携带这一信息。在具备相应特性的语言中，这相当于一个在编译期就随协议而变的条件类型，此处则在运行期实现了同样的效果。

## 思考级别的适配

调用方使用的是中立的 `ModelThinkingLevel`，而各模型支持的级别并不一致。两个函数负责把一个请求级别落到目标模型实际接受的级别上。

`SupportedThinkingLevels` 给出某模型接受的级别集合：非推理模型仅支持 `off`；推理模型则依 `off → minimal → low → medium → high → xhigh` 的次序枚举，其中在 `ThinkingLevelMap` 中显式映射为 `nil` 的级别视为不支持，而 `xhigh` 须经显式映射方才纳入——即默认不开放最高档，除非模型明确声明。

`ClampThinkingLevel` 将任意请求级别收敛至最接近的受支持级别，规则依次为：命中则直接采用；否则沿次序向上抬升至首个受支持级别；仍无则转而向下回落；最终退至最低的受支持级别（或 `off`）。如此，调用方可请求任一级别，而总能得到目标模型可处理的结果，无需自行比对每个模型的能力表。

## 注册表

`ModelRegistry` 是模型的存储与检索中枢——内置模型在启动时注册于此，运行期的每次查询也都经由它。它只有两个字段：一把读写锁，和一张「厂商 → 模型 ID → 模型」的两层 map：

```go
type ModelRegistry struct {
	mu     sync.RWMutex
	models map[string]map[string]Model // 厂商 → 模型 ID → Model
}
```

外层 key 是厂商，内层 key 是该厂商下的模型 ID。厂商的分组独立于协议——同一厂商名下的模型可以分属不同协议，也照样并列嵌套其内：

```text
models ┌─ "anthropic" ─┬─ "claude-opus-4-8"   → Model{protocol: anthropic-messages}
       │               └─ "claude-sonnet-4-6" → Model{protocol: anthropic-messages}
       │
       └─ "deepseek" ──┬─ "deepseek-v4-flash" → Model{protocol: openai-completions}
                       └─ "deepseek-v4-pro"   → Model{protocol: openai-completions}
```

`mu` 这把 `sync.RWMutex` 保护对 `models` 的全部读写：查询取读锁、注册取写锁，因此注册表可在多个 goroutine 间共享而无需外部加锁。

### 注册与校验

`Register` 写入一个模型，但在落表之前要走完一套固定流程：

1. **非空校验**——provider、ID、protocol 三者必须齐备，任一为空即返回错误，注册中止。
2. **兼容性校验**——若模型携带兼容性配置（`Compatibility`），交由 `validateModelCompatibility` 进一步检查：其具体类型须为已知的协议兼容性结构体之一，且其声明的协议须与模型自身的 `Protocol` 一致。也就是说，一个标注 `anthropic-messages` 的模型不能挂上 OpenAI 的兼容性配置——此类不一致会在注册期被拦下，而非延后到请求时才暴露。
3. **加写锁**——经由 `mu.Lock` 取得写锁，保证并发注册之间互斥。
4. **惰性建表**——若该 provider 的内层 map 尚不存在，先行创建。
5. **存入深拷贝**——写入的是 `cloneModel(model)`，而非调用方传入的原件；同一 provider 下若以相同 ID 再次注册，则覆盖原有条目。

校验全部前置于加锁与写入：一个非法模型在改动注册表之前就被挡下，不会留下半成品状态。

### 获取模型

检索分两个层级。注册表实例上的方法面向任意注册表，共三个：

```go
func (r *ModelRegistry) Get(provider, modelID string) (Model, bool)
func (r *ModelRegistry) Providers() []string
func (r *ModelRegistry) Models(provider string) []Model
```

- `Get` 按 provider 与模型 ID 取单个模型：命中返回该模型与 `true`，缺失返回零值与 `false`。
- `Providers` 列出注册表中全部厂商 ID，按字典序排列。
- `Models` 列出某一厂商名下的全部模型，按模型 ID 排序。

三者的返回均经排序，因此遍历顺序稳定、可复现。

包级函数则是这三个方法在内置注册表 `builtInModelRegistry` 上的封装，多数调用方直接使用它们即可——免去自行持有注册表实例：

```go
func LookupModel(provider, modelID string) (Model, bool) // 对应 Get，缺失返回 false
func GetModel(provider, modelID string) Model            // 对应 Get，缺失则 panic
func GetProviders() []string                             // 对应 Providers
func GetModels(provider string) []Model                  // 对应 Models
```

取单个模型有 `LookupModel` 与 `GetModel` 两个版本，仅在缺失时的处理上有别：`GetModel` 适用于标识符在代码中写死、理应存在的场合，缺失即属程序错误；`LookupModel` 适用于标识符来自配置或外部输入、需由调用方自行应对缺失的场合。`GetProviders` 与 `GetModels` 则分别直通 `Providers` 与 `Models`，语义不变。

无论经由哪个入口，取回的都是深拷贝。`Model` 内含切片、map 与指针字段，若直接交出表中原件，调用方的改动便会波及其他持有者；返回独立副本即可杜绝这种隐性耦合。

## 内置模型

上述注册表中的内置模型并非在运行时拉取，而是随二进制一同发布。[`catalog.generated.json`](https://github.com/ktsoator/or/blob/main/llm/catalog.generated.json) 由 `go generate`（`internal/genmodels`）从上游目录数据生成——以 [Models.dev](https://models.dev) 为主，辅以 OpenRouter 与 Vercel AI Gateway 的实时目录与定价，且只输出本包已实现协议（`openai-completions` 与 `anthropic-messages`）的模型。生成结果连同源码一并提交，再经 `//go:embed` 编入二进制：

```go
//go:embed catalog.generated.json
var generatedCatalogJSON []byte
```

由此，构建与启动均不依赖网络或工作目录。填充注册表的动作发生在程序启动阶段：`builtInModelRegistry` 是一个包级变量，其初始化早于 `main`——过程是先解码这份内嵌 JSON，再将其中每个模型逐一 `Register` 进表：

```go
var builtInModelRegistry = newBuiltInModelRegistry()

func builtInModels() []Model {
	var models []Model
	if err := json.Unmarshal(generatedCatalogJSON, &models); err != nil {
		panic(...) // 目录损坏即无从启动
	}
	return models
}
```

此处对解码与注册失败一律 `panic`，而非返回错误。内嵌目录是编译期产物：到了运行时，它要么完好，要么意味着构建本身有误，二者之间并无可供降级运行的中间状态，因此让程序尽早终止。

源码：[`model.go`](https://github.com/ktsoator/or/blob/main/llm/model.go)、[`model_registry.go`](https://github.com/ktsoator/or/blob/main/llm/model_registry.go) 与 [`catalog.go`](https://github.com/ktsoator/or/blob/main/llm/catalog.go)。
