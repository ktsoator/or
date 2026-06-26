# Reading responses

`Complete` returns an `AssistantMessage`; streaming delivers the same value as
`EventDone.Message`. This page covers what to read back from it: content, why
generation stopped, token usage and cost, and non-fatal diagnostics.

## Content and metadata

The two accessors cover most reads:

```go
response.Text()      // all text blocks joined
response.ToolCalls() // every tool call, in order
```

The message also carries provider metadata: `Provider`, `Model`, the provider's
own `ResponseModel` and `ResponseID`, and a `Timestamp`. `ErrorMessage` holds
the provider or runtime error string on a failed response.

```go
fmt.Printf("model=%s response=%s id=%s\n",
	response.Model, response.ResponseModel, response.ResponseID)
if response.ErrorMessage != "" {
	log.Printf("provider error: %s", response.ErrorMessage)
}
```

To walk individual blocks instead of the joined text — for example to render
thinking and text differently — type-switch over `response.Content`:

```go
for _, block := range response.Content {
	switch b := block.(type) {
	case *llm.TextContent:
		fmt.Println("text:", b.Text)
	case *llm.ThinkingContent:
		fmt.Println("thinking:", b.Thinking)
	case *llm.ToolCall:
		fmt.Printf("tool call: %s(%v)\n", b.Name, b.Arguments)
	}
}
```

## Stop reasons

`StopReason` explains why generation stopped. Branch on it before using the
response — especially before executing tool calls.

| `StopReason` | Meaning | Typical handling |
|---|---|---|
| `StopReasonStop` | Normal completion | Use `response.Text()` |
| `StopReasonToolUse` | The model wants tool results | Run the [tool loop](tools.md#run-the-tool-loop) |
| `StopReasonLength` | Output hit the `MaxTokens` cap | Continue the turn or raise the cap |
| `StopReasonError` | Provider or runtime failure | Inspect `ErrorMessage`; do not execute tool calls |
| `StopReasonAborted` | Request was cancelled | Stop; the context was cancelled |

```go
switch response.StopReason {
case llm.StopReasonStop:
	fmt.Println(response.Text())
case llm.StopReasonToolUse:
	runTools(response.ToolCalls()) // see the tool loop
case llm.StopReasonLength:
	log.Println("truncated: raise MaxTokens or continue the turn")
case llm.StopReasonError, llm.StopReasonAborted:
	log.Printf("stopped early: %s %s", response.StopReason, response.ErrorMessage)
}
```

## Token usage and cost

`Usage` records token consumption for the response. Cached tokens are reported
separately so cache hits are visible:

| Field | Meaning |
|---|---|
| `Input` | Prompt tokens billed at the full input rate |
| `Output` | Generated tokens, including reasoning tokens |
| `CacheRead` | Input tokens served from the provider cache |
| `CacheWrite` | Input tokens written to the cache |
| `TotalTokens` | Sum reported for the response |

`Usage.Cost` is a `UsageCost` with the same breakdown in currency units
(`Input`, `Output`, `CacheRead`, `CacheWrite`, and `Total`), computed from the
model's pricing when the response is assembled.

```go
fmt.Printf("tokens=%d (cached %d) cost=$%.6f\n",
	response.Usage.TotalTokens,
	response.Usage.CacheRead,
	response.Usage.Cost.Total,
)
```

To price a usage record yourself — for example to re-cost stored history against
a different model — call `CalculateCost`:

```go
cost := llm.CalculateCost(model, response.Usage)
fmt.Printf("input=$%.6f output=$%.6f total=$%.6f\n",
	cost.Input, cost.Output, cost.Total)
```

To track spend across a multi-turn conversation, accumulate `Cost.Total` from
each response:

```go
var spent float64
for _, turn := range responses {
	spent += turn.Usage.Cost.Total
}
fmt.Printf("conversation cost: $%.4f\n", spent)
```

## Detect context overflow

`IsContextOverflow` reports whether a response exceeded the model's context
window. It recognises explicit provider errors as well as silent overflows where
the provider truncates input instead of failing. Use it to trigger history
compaction or summarization before the next turn.

```go
if llm.IsContextOverflow(response, model.ContextWindow) {
	// Drop or summarize old messages, then retry.
}
```

## Diagnostics

`Diagnostics` records non-fatal events that occurred while producing the
response, such as tool arguments recovered from malformed JSON. It is `nil` for
a clean response. Each `Diagnostic` carries a `Type`, a `Timestamp`, an optional
`Message`, and structured `Details`.

```go
for _, d := range response.Diagnostics {
	if d.Type == llm.DiagnosticToolArgumentsRecovered {
		log.Printf("recovered tool arguments: mode=%v call=%v",
			d.Details["mode"], d.Details["toolCallId"])
	}
}
```

Inspect diagnostics before executing a tool with side effects; see
[stream diagnostics](streaming.md#tool-call-deltas-and-diagnostics) for the
recovery modes.
