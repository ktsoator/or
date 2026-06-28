# 引导与追加

`Prompt` 会阻塞到一次运行完成，而一个 agent 同一时间只跑一个 prompt——在运行进行时再次
调用 `Prompt` 会返回错误。要在运行进行中影响它，请从另一个 goroutine 调用 `Steer`、
`FollowUp` 或 `Abort`。

```go
go func() {
	_ = assistant.Prompt(ctx, "Summarize the repository")
}()

assistant.Steer(agent.FromLLM(llm.UserText("Focus on the agent package.")))
```

agent 的所有方法都可安全并发使用。

## 引导：运行中注入

`Steer` 把一条消息排队，在运行的下一轮之前注入。循环在每一轮工具调用结束后排空引导队列，
所以一条引导消息会在紧接的下一轮被模型看到——用于在不重启的情况下重新引导一个长任务。

```go
assistant.Steer(agent.FromLLM(llm.UserText("Stop and show me what you have so far.")))
```

## 追加：越过停止点继续

`FollowUp` 把一条消息排队，在 agent 本应停止时处理。当一次运行到达结束点时，循环会排空
追加队列；若发现内容，就再跑一轮而不是结束。

```go
assistant.FollowUp(agent.FromLLM(llm.UserText("Now write the tests.")))
```

区别在时机：引导在回合之间打断一次进行中的运行；追加则延长一次本将结束的运行。

## 单次排空的数量

`SteeringMode` 和 `FollowUpMode` 控制单次排空返回多少条排队消息：

- `QueueOneAtATime`（默认）只注入最旧的一条，其余留给后续的排空点。
- `QueueAll` 一次注入全部排队消息。

```go
assistant := agent.New(agent.Options{
	SteeringMode: agent.QueueAll,        // 下一轮注入全部待处理引导
	FollowUpMode: agent.QueueOneAtATime, // 追加每次运行处理一条
	/* ... */
})
```

## 继续运行

`Continue` 在不添加新消息的情况下，从当前 transcript 恢复——用于重试，或在带外追加消息
之后。

provider 需要最新一轮是用户或工具结果。当 transcript 以 assistant 消息结尾时，`Continue`
会回退到队列：先排空引导队列，再排空追加队列，并运行找到的内容。只有当最后一条是 assistant
消息且两个队列都为空时，它才返回错误。

```go
if err := assistant.Continue(ctx); err != nil {
	log.Fatal(err) // 例如 "cannot continue from an assistant message"
}
```

## 中止

`Abort` 取消当前运行（如果有）。在途的一轮以中止停止原因结束，仍在等待的工具会收到一个
中止结果，从而每个工具调用都仍被回应、transcript 对后续请求仍然有效。

```go
assistant.Abort()
```

## 检查与清空队列

```go
assistant.HasQueuedMessages()   // 有任何引导或追加消息排队吗？
assistant.ClearSteeringQueue()  // 丢弃排队的引导消息
assistant.ClearFollowUpQueue()  // 丢弃排队的追加消息
assistant.ClearQueues()         // 两个都丢弃
```

`Reset` 清空 transcript、最近错误和两个队列，同时保留配置——在 agent 空闲时调用它，以
开始一次全新的对话。
