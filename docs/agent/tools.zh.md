# 工具

工具是模型在一次运行中可以调用的东西。agent 把每个工具的 schema 告知模型、校验模型产生的
参数、运行工具，再把结果喂回循环。本包不内置任何具体工具——由你来定义。

## 工具的构成

一个 `agent.AgentTool` 把 schema 和实现配在一起：

```go
type AgentTool struct {
	Definition       llm.ToolDefinition
	Label            string
	PrepareArguments func(arguments map[string]any) map[string]any
	Execute          func(ctx context.Context, callID string, args json.RawMessage, onUpdate func(ToolResult)) (ToolResult, error)
	ExecutionMode    ExecutionMode
}
```

- `Definition` 是展示给模型的 schema 和描述。用 `llm.MustTool` 或 `llm.NewTool` 从一个
  Go 结构体派生它，这样参数 schema 和你解码进的类型不会各走各的。
- `Execute` 运行工具。它拿到调用 id 和校验后的原始 JSON 参数，返回一个 `ToolResult`
  或一个 error。
- `Label` 是可选的 UI 元数据，不影响执行。
- `PrepareArguments` 和 `ExecutionMode` 见下文。

```go
type searchArgs struct {
	Query string `json:"query" jsonschema:"description=Search query,minLength=1"`
	Limit int    `json:"limit,omitempty" jsonschema:"description=Max results,minimum=1,maximum=50"`
}

search := agent.AgentTool{
	Definition: llm.MustTool[searchArgs]("search_docs", "Search the documentation"),
	Execute: func(ctx context.Context, callID string, args json.RawMessage, onUpdate func(agent.ToolResult)) (agent.ToolResult, error) {
		var in searchArgs
		if err := json.Unmarshal(args, &in); err != nil {
			return agent.ToolResult{}, err
		}
		hits := runSearch(ctx, in.Query, in.Limit)
		return agent.ToolResult{
			Content: []llm.ToolResultContent{&llm.TextContent{Text: hits}},
		}, nil
	},
}
```

## 执行前的参数校验

引擎在调用 `Execute` 之前，会用 `Definition` 的 schema 校验模型的参数，并对常见的模型
错误做强制转换（数字位置上的 `"3"` 字符串）。等到 `Execute` 运行时，`args` 已是校验过、
重新序列化的对象，所以直接 `json.Unmarshal` 进你的结构体是安全的。

如果校验失败，工具根本不会运行：引擎会生成一个说明哪些字段有误的错误结果，并继续循环。

`PrepareArguments` 在校验**之前**运行，用来改写原始参数 map。用它来容忍某个 provider 的
怪癖，或补上模型漏掉的默认值：

```go
PrepareArguments: func(arguments map[string]any) map[string]any {
	if _, ok := arguments["limit"]; !ok {
		arguments["limit"] = 10
	}
	return arguments
},
```

## 工具结果

`Execute` 返回一个 `ToolResult`：

```go
type ToolResult struct {
	Content   []llm.ToolResultContent // 模型看到的内容
	Details   any                     // 给日志或 UI 的结构化数据；不发给模型
	Terminate bool                    // 提示本批结束后停止运行
}
```

- `Content` 是模型下一轮读到的答案——文本，以及对视觉模型而言的图片。
- `Details` 是挂在 `ToolEnd` 事件上、供日志或 UI 渲染的任意结构化数据。模型看不到它。
- `Terminate` 请求提前停止。只有当一批里**每个**结果都设了 `Terminate`，这批才会停止
  运行，所以单个工具无法单方面结束一次还调用了其它工具的运行。

## 失败不中断运行

工具通过返回 error 来报告失败；引擎把它转成一个错误结果（设上 `IsError`）并继续，所以
单个失败的工具不会结束运行。会 panic 的工具也以同样方式被恢复成错误结果，而不是让进程
崩溃。这一点对并发运行的工具同样成立——它们各自跑在独立的 goroutine 上。

```go
Execute: func(ctx context.Context, callID string, args json.RawMessage, onUpdate func(agent.ToolResult)) (agent.ToolResult, error) {
	if err := doWork(ctx); err != nil {
		return agent.ToolResult{}, err // 变成错误结果；循环继续
	}
	return agent.ToolResult{Content: ok}, nil
},
```

同一套恢复机制也覆盖根本无法运行的工具：未知工具名、没有 `Execute` 的工具、被
`BeforeToolCall` 拦下的调用，都会变成错误结果。

## 流式进度

`onUpdate` 回调在工具运行时以 `ToolUpdate` 事件发出一个部分 `ToolResult`——用于进度条、
转圈或流式输出。它仅在该次调用期间有效。

```go
Execute: func(ctx context.Context, callID string, args json.RawMessage, onUpdate func(agent.ToolResult)) (agent.ToolResult, error) {
	onUpdate(agent.ToolResult{Details: "connecting"})
	rows := query(ctx)
	onUpdate(agent.ToolResult{Details: fmt.Sprintf("%d rows", len(rows))})
	return agent.ToolResult{Content: format(rows)}, nil
},
```

订阅 `ToolUpdate` 来渲染进度——见[事件与状态](events.md)。

## 执行顺序

一批工具调用**默认并发**运行。你可以为整个循环、或为某个工具强制顺序执行：

```go
// 整个循环逐个运行工具。
agent.New(agent.Options{ToolExecution: agent.ExecutionSequential /* ... */})

// 这个工具会让它所在的任何一批都变成顺序执行。
search.ExecutionMode = agent.ExecutionSequential
```

在并发的一批里，只有工具的 `Execute` 函数并行运行。围绕它们的生命周期是确定的，绝不并发：

- `ToolStart` 事件和 `BeforeToolCall` 在执行前、按源序运行。
- `AfterToolCall`、`ToolEnd` 和结果消息在整批结束后、按源序运行。

所以你的钩子绝不会被并发调用，结果也会按模型请求的顺序落进 transcript，无论哪个工具先
跑完。

## 拦截调用

`BeforeToolCall` 和 `AfterToolCall` 让你拦下一次调用、改写其结果，或停止运行。详见
[生命周期钩子](hooks.md)。
