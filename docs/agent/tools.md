# Tools

A tool is something the model can call during a run. The agent advertises each
tool's schema to the model, validates the arguments the model produces, runs the
tool, and feeds the result back into the loop. The package bundles no concrete
tools — you define them.

## Anatomy of a tool

An `agent.AgentTool` pairs a schema with an implementation:

```go
type AgentTool struct {
	Definition       llm.ToolDefinition
	Label            string
	PrepareArguments func(arguments map[string]any) map[string]any
	Execute          func(ctx context.Context, callID string, args json.RawMessage, onProgress func(ToolProgress)) (ToolResult, error)
	ExecutionMode    ExecutionMode
}
```

- `Definition` is the schema and description shown to the model. Derive it from a
  Go struct with `llm.MustTool` or `llm.NewTool`, so the parameter schema and the
  type you decode into never drift apart.
- `Execute` runs the tool. It receives the call id and the validated arguments as
  raw JSON, and returns a `ToolResult` or an error.
- `Label` is optional UI metadata; it does not affect execution.
- `PrepareArguments` and `ExecutionMode` are covered below.

```go
type searchArgs struct {
	Query string `json:"query" jsonschema:"description=Search query,minLength=1"`
	Limit int    `json:"limit,omitempty" jsonschema:"description=Max results,minimum=1,maximum=50"`
}

search := agent.AgentTool{
	Definition: llm.MustTool[searchArgs]("search_docs", "Search the documentation"),
	Execute: func(ctx context.Context, callID string, args json.RawMessage, onProgress func(agent.ToolProgress)) (agent.ToolResult, error) {
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

## Arguments are validated before Execute

The engine validates the model's arguments against `Definition`'s schema before
calling `Execute`, coercing common model mistakes (a `"3"` string for a number)
toward the schema. By the time `Execute` runs, `args` is the validated,
re-serialized object, so a plain `json.Unmarshal` into your struct is safe.

If validation fails, the tool never runs: the engine produces an error result
explaining which fields were wrong and continues the loop.

`PrepareArguments` runs *before* validation and rewrites the raw argument map. Use
it to tolerate a provider quirk or fill a default the model omitted:

```go
PrepareArguments: func(arguments map[string]any) map[string]any {
	if _, ok := arguments["limit"]; !ok {
		arguments["limit"] = 10
	}
	return arguments
},
```

## The result

`Execute` returns a `ToolResult`:

```go
type ToolResult struct {
	Content   []llm.ToolResultContent // what the model sees
	Outcome   ToolOutcome             // terminal status and machine-readable data
	Terminate bool                    // hint to stop the run after this batch
}

type ToolOutcome struct {
	Status    ToolOutcomeStatus // success, failed, cancelled, or timeout
	ErrorCode string
	ExitCode  *int
	Data      any
}
```

- `Content` is the answer the model reads on the next turn — text and, for
  vision models, images.
- `Outcome` is the only terminal state. Its `Data` is arbitrary structured data
  attached to `ToolEnd` for logging or UI rendering; the model never sees it.
  An empty status is normalized to `success` for simple tools that return only
  content.
- `Terminate` requests an early stop. A tool batch stops the run only when *every*
  result in it sets `Terminate`, so one tool cannot unilaterally end a run that
  also called other tools.

## Failure never aborts the run

A tool reports failure with a failed outcome or by returning an error; the
engine normalizes it into an error result (`IsError` is derived from the
outcome) and continues, so one failing tool does not end the run.
A tool that panics is recovered the same way — into an error result — rather than
crashing the process. This holds even for tools running concurrently, which run
on their own goroutines.

```go
Execute: func(ctx context.Context, callID string, args json.RawMessage, onProgress func(agent.ToolProgress)) (agent.ToolResult, error) {
	if err := doWork(ctx); err != nil {
		return agent.ToolResult{}, err // becomes an error result; the loop continues
	}
	return agent.ToolResult{Content: ok}, nil
},
```

The same recovery covers a tool that cannot run at all: an unknown tool name, a
tool with no `Execute`, or a call blocked by `BeforeToolCall` all become error
results.

## Streaming progress

The `onProgress` callback emits a non-terminal `ToolProgress` as a `ToolUpdate`
event while the tool runs — for a progress bar, a spinner, or streamed output.
It cannot carry an outcome or terminate the run. The callback is valid only for
the duration of `Execute`; late updates after `Execute` returns are ignored.

```go
Execute: func(ctx context.Context, callID string, args json.RawMessage, onProgress func(agent.ToolProgress)) (agent.ToolResult, error) {
	onProgress(agent.ToolProgress{Data: "connecting"})
	rows := query(ctx)
	onProgress(agent.ToolProgress{Data: fmt.Sprintf("%d rows", len(rows))})
	return agent.ToolResult{Content: format(rows)}, nil
},
```

Subscribe to `ToolUpdate` and read `event.Progress` to render progress. The
returned `ToolResult.Outcome` appears only on `ToolEnd` — see
[Events and state](events.md).

## Execution order

A batch of tool calls runs **concurrently by default**. You can force sequential
execution for the whole loop, or for one tool:

```go
// Whole loop runs tools one at a time.
agent.New(agent.Options{ToolExecution: agent.ExecutionSequential /* ... */})

// This tool forces any batch it appears in to run sequentially.
search.ExecutionMode = agent.ExecutionSequential
```

Within a concurrent batch, only the tools' `Execute` functions run in parallel.
The lifecycle around them is deterministic and never concurrent:

- `ToolStart` events and `BeforeToolCall` run in source order, before execution.
- `AfterToolCall`, `ToolEnd`, and the result messages run in source order, after
  the whole batch finishes.

So your hooks are never called concurrently, and the results land in the
transcript in the order the model requested them, regardless of which tool
finished first.

## Intercepting calls

`BeforeToolCall` and `AfterToolCall` let you block a call, rewrite its result, or
stop the run. They are covered in [Lifecycle hooks](hooks.md).
