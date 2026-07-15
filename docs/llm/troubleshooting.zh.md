# 排障

针对最常见问题、以症状为先的修复。底层模型（错误如何暴露、key 如何解析）见[错误处理](errors.md)。

## 协议没有注册适配器

`Stream` 或 `Complete` 立即返回 `no adapter registered for protocol "..."`。

- **原因：** 从未导入模型协议对应的 provider 包，因此没有适配器完成自注册。
- **修复：** 为所用协议加上空导入——或用 `llm/all` 引入全部内置。导入按协议而非厂商划分：DeepSeek 需要 `llm/openai`，MiniMax 需要 `llm/anthropic`。

```go
import (
	_ "github.com/ktsoator/or/llm/openai"    // openai-completions 提供方
	_ "github.com/ktsoator/or/llm/anthropic" // anthropic-messages 提供方
)
```

如果模型协议在[支持矩阵](support-matrix.md#协议状态)中标记为“仅目录”，导入
`llm/all` 也不会增加对应实现。应选择当前可运行模型，或自行实现 adapter。

## 取消后流一直不关闭

消费者不再收到业务事件，但等待流结束的 goroutine 没有退出。

- **原因：** `Stream` 返回无缓冲通道。取消 context 会请求 producer 停止，
  但 consumer 仍必须读到通道关闭。提前退出接收循环时，producer 可能阻塞
  在发送最终事件上。
- **修复：** 记录取消或流错误，继续排空通道，再返回。流没有 `Close` 或
  `Abort` 方法。

```go
for event := range events {
	if event.Type == llm.EventError {
		streamErr = event.Err
	}
}
return streamErr
```

## 模型不在目录中（panic）

当 provider/模型组合不在内置目录中时，`GetModel` 会 panic，消息为 `panic: llm: unknown model "..." for provider "..."`。

- **原因：** 拼写错误、provider ID 写错，或该模型未被收录。
- **修复：** 改用 `LookupModel`，它返回 `(Model, false)` 而不 panic；并用 `GetProviders` / `GetModels` 浏览目录。

```go
model, ok := llm.LookupModel("deepseek", "deepseek-v4-flash")
if !ok {
	log.Fatalf("不在目录中;可用 provider: %v", llm.GetProviders())
}
```

若端点不在目录中，可手动构造 `llm.Model` 并设置其 `BaseURL`、`Protocol` 与 `Provider`。

## 缺少 API key

请求根本没到达 provider，错误为 `API key is empty for provider "..."`。

- **原因：** 期望的环境变量未设置——或设错了变量。变量是按 provider 区分的，且地区变体不同：MiniMax 全球版读 `MINIMAX_API_KEY`，而 MiniMax CN 读 `MINIMAX_CN_API_KEY`。错误消息会列出实际检查过的变量。
- **修复：** 设置被点名的变量，或直接传 `StreamOptions.APIKey`，或提供 `StreamOptions.Env`。用 `APIKeyEnvVars` 确认某 provider 期望哪些变量，用 `FindEnvAPIKeys` 确认实际设置了哪些。

```go
fmt.Println(llm.APIKeyEnvVars("minimax-cn")) // [MINIMAX_CN_API_KEY]
fmt.Println(llm.FindEnvAPIKeys("minimax-cn")) // [] 表示一个都没设
```

## 响应失败（`StopReasonError`）

`Complete` 返回非空 error，或流以 `EventError` 结束。

- **原因：** 请求中途出现 provider 或运行时故障（鉴权被拒、限流、模型不可用）。
- **修复：** 读取 `response.ErrorMessage` 获取 provider 的详情。**不要**执行失败响应上的任何工具调用。暂时性故障会由 SDK 重试；用 `StreamOptions.MaxRetries` 与 `Timeout` 调节。

## 答案被截断（`StopReasonLength`）

文本在句子中途结束。

- **原因：** 生成触及 `MaxTokens` 上限。
- **修复：** 调高 `MaxTokens`，或追加这条部分 assistant 消息后再次发送以续写本轮。

## 静默截断或上下文错误

模型忽略了长对话的开头，或直接拒绝。

- **原因：** 请求超出模型的上下文窗口。有些 provider 报错，有些静默丢弃溢出部分。
- **修复：** 调用 `IsContextOverflow(response, model.ContextWindow)`，为真时在重试前压缩或摘要旧消息。

## 工具参数错误或不完整

`DecodeToolCall` 返回错误，或 `AssistantMessage.Diagnostics` 报告了被恢复的参数。

- **原因：** 模型产生了畸形 JSON，或流被截断。库会尽力恢复参数并在 `Diagnostics` 中记录方式，而不是让整个响应失败。
- **修复：** 在执行有副作用的工具前，检查 `Diagnostics` 并拒绝 `partial` 或 `invalid` 参数。遇到 decode 错误时，回传工具错误（`result.IsError = true`），让模型纠正调用。见[工具循环清单](tools.md#运行工具循环)。
