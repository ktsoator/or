# 模型切换

规范化的对话历史与厂商无关，但并不是每个模型都能原样接受其中的每一种 block。
`TransformMessages` 会在适配器转换线缆格式之前，把已存储的历史投影成目标模型可安全接收
的请求形式。

关键点是：转换发生在每次请求前，不会修改已存储的历史。因此，切到纯文本模型不会永久丢掉
图像；从 reasoning 模型切走，也不会抹掉原历史里的签名。

## 转换顺序

`TransformMessages` 做三轮处理：

1. `downgradeUnsupportedImages`：当目标模型的 `Model.Input` 不包含 `Image` 时，把图像
   block 替换成占位文本。
2. `reconcileAssistantHistory`：按目标模型重写历史 assistant turn，包括 reasoning block
   和工具调用 ID。
3. `synthesizeOrphanedToolResults`：修复存在 assistant 工具调用但缺少对应工具结果的转录。

## 推理与签名

reasoning 是模型相关的。推理签名或 redacted payload 只有在 provider、protocol 和 model
ID 都匹配目标模型时才适合重放。同一模型下，带签名的推理会被保留；跨模型时，可读 thinking
会降级成普通文本，空 thinking 会被丢弃，redacted thinking 会被移除，因为只有原模型能理解
它。

文本签名和工具调用签名遵循同样原则：不透明的 provider 元数据只保留给产出它的模型。

## 工具调用 ID

不同协议接受的工具调用 ID 形状不同。当 assistant turn 跨模型重放时，适配器会传入
`normalizeToolCallID` 函数。如果 ID 被改写，同一轮遍历中后续匹配的
`ToolResultMessage.ToolCallID` 也会同步重映射，让转录保持一致。

最后一轮会维护工具协议的不变量：一个 assistant 工具调用批次在进入下一条 user 或 assistant
消息前，必须每个调用都有一个结果。缺失的结果会被合成为错误 tool result，内容为
`"No result provided"`。

## 上下文溢出检测

`IsContextOverflow` 独立于转换逻辑。它检查一次完成或失败的 `AssistantMessage`，识别不同
provider 的上下文溢出信号：已知错误文本模式、成功响应但 usage 超过 context window、以及
输入几乎填满窗口且没有输出的 length stop。

源码：[`llm/transform.go`](https://github.com/ktsoator/or/blob/main/llm/transform.go)、
[`llm/overflow.go`](https://github.com/ktsoator/or/blob/main/llm/overflow.go)。
