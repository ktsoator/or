# 模型与协议

[`types.go`](https://github.com/ktsoator/or/blob/main/internal/llm/types.go)
定义了包内其余部分所使用的词汇：各种协议、调用方设置的中立旋钮，以及把端点同其能力与
价格绑定在一起的 `Model`。

## 中立词汇

若干设置都是小型字符串类型，各自构成一组固定的常量。将它们定义为具名类型——而非裸字符串
——既能让编译器捕捉拼写错误，也使公开 API 自带文档。

```go
type Protocol string           // "openai-completions"、"anthropic-messages"
type ModelInput string         // "text"、"image"
type ModelThinkingLevel string // off、minimal、low、medium、high、xhigh
type ThinkingDisplay string    // summarized、omitted
```

`Protocol` 命名一种通信协议，并经由它确定负责对接的适配器。`ModelInput` 命名一种模态，
模型在 `Model.Input` 中列出自己接受的模态；发往纯文本模型的图像会被降级，而非直接拒绝。

`ModelThinkingLevel` 与厂商无关，模型通过 `Model.ThinkingLevelMap` 声明各级别如何映射
到自家的方言，适配器在构建请求时据此查表。`ThinkingDisplay` 的作用更窄：它不改变模型是否
推理、也不改变计费，只决定返回什么——`summarized` 返回可读的思考文本，`omitted` 保留
签名但去掉文本。目前仅 Anthropic 协议遵从该设置。

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

这四类与响应上的 `Usage` 计数一一对应，因此 `CalculateCost` 不过是逐类相乘。缓存读写
之所以与新输入分开计价，是因为厂商对它们的收费各不相同。

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

`Protocol` 是路由的判别器：`Client.Stream` 据此选取适配器。`BaseURL` 与 `Headers`
则使兼容厂商得以复用某种协议——将基址指向该厂商的端点，补上所需的请求头，同一个适配器
即可对接。`ContextWindow` 是 token 总预算，`MaxTokens` 是生成上限；二者既参与请求构建，
也参与[溢出检测](transform.md)。

`ThinkingLevelMap` 刻意采用指针值。`nil` 标记该级别不受支持；键缺失则回退到厂商默认值。
这是两种不同的情形，而指针正是用以区分二者的手段——普通 `string` 无法区分「明确关闭」
与「未配置」。

## 三态兼容性

实现同一协议的厂商之间，仍存在细微差异。这些差异承载在按协议划分的兼容性结构体上。
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

其中的布尔字段为指针自有缘由。普通 `bool` 只有两种状态，无法区分「该厂商明确不支持此项」
与「未指定，采用默认」。`*bool` 则有三种：`true`、`false` 与 `nil`，其中 `nil` 即默认
路径。字符串字段（`MaxTokensField`、`ThinkingFormat`）直接指名某种变体，空串表示「采用
参考实现的行为」。

## 协议作为解码期判别器

两个兼容性结构体都满足同一个接口，其唯一的方法报告该配置描述的是哪种协议：

```go
type ModelCompatibility interface {
	Protocol() Protocol
}
```

这使 `Model` 不依赖于任何单一协议。代价在于 `compat` 字段是接口类型，而 JSON 并不携带
「它装的是哪个具体结构体」的标签——解码时须自行选定。`Model.UnmarshalJSON` 正是以
`Protocol` 作为判别器来完成这一选择：

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

1.  `modelAlias` 这一别名类型丢弃了 `UnmarshalJSON` 方法，因此向它反序列化不会递归回到
    本函数。`compat` 以 `json.RawMessage` 暂留，待第二遍再解码。
2.  `Protocol` 已在第一遍中解码完毕，因此此处可用它选定具体的兼容性类型。

请求时驱动路由的字段，与解码时选定类型的字段是同一个。模型得以序列化为 JSON 再还原，
而无需额外的类型标签，因为它的协议本身已携带这一信息。在具备相应特性的语言中，这相当于
一个在编译期就随协议而变的条件类型，此处则在运行期实现了同样的效果。

源码：[`internal/llm/types.go`](https://github.com/ktsoator/or/blob/main/internal/llm/types.go)。
这些模型从中加载的目录，见
[`models.go`](https://github.com/ktsoator/or/blob/main/internal/llm/models.go)。
