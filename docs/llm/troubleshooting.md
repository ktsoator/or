# Troubleshooting

This page maps common symptoms to diagnostics and fixes. For the failure model
and credential-resolution rules, see [Failure signals](errors.md).

## `no adapter registered for protocol "..."`

`Stream` or `Complete` returns this error immediately.

- **Cause:** the provider package for the model's protocol was never imported, so
  no adapter registered itself.
- **Fix:** add the blank import for the protocol you use — or `llm/all` for all
  built-ins. Imports are by protocol, not by vendor: DeepSeek needs
  `llm/openai`, MiniMax needs `llm/anthropic`.

```go
import (
	_ "github.com/ktsoator/or/llm/openai"    // openai-completions providers
	_ "github.com/ktsoator/or/llm/anthropic" // anthropic-messages providers
)
```

If the protocol is marked catalog-only in
[Protocol and provider status](support-matrix.md#protocol-status), importing `llm/all` does
not add an implementation. Choose a currently runnable model or implement an
adapter.

## A stream never closes after cancellation

The consumer stops receiving events, but the goroutine waiting on the stream
does not finish.

- **Cause:** `Stream` returns an unbuffered channel. Cancelling the context asks
  the producer to stop, but the consumer must continue reading until the channel
  closes. Returning from the receive loop early can leave the producer blocked
  while publishing its final event.
- **Fix:** record the cancellation or error, keep draining the channel, and only
  then return. There is no stream `Close` or `Abort` method.

```go
for event := range events {
	if event.Type == llm.EventError {
		streamErr = event.Err
	}
}
return streamErr
```

## `panic: llm: unknown model "..." for provider "..."`

`GetModel` panics when the provider/model pair is not in the built-in catalog.

- **Cause:** a typo, the wrong provider ID, or a model that is not cataloged.
- **Fix:** use `LookupModel`, which returns `(Model, false)` instead of
  panicking, and browse the catalog with `GetProviders` / `GetModels`.

```go
model, ok := llm.LookupModel("deepseek", "deepseek-v4-flash")
if !ok {
	log.Fatalf("not in catalog; providers: %v", llm.GetProviders())
}
```

For an endpoint that is not in the catalog, construct an `llm.Model` by hand and
set its `BaseURL`, `Protocol`, and `Provider`.

## `API key is empty for provider "..."`

The request never reaches the provider.

- **Cause:** the expected environment variable is unset — or you set the wrong
  one. Variables are provider-specific, and regional variants differ: MiniMax
  global reads `MINIMAX_API_KEY`, but MiniMax CN reads `MINIMAX_CN_API_KEY`. The
  error message lists the exact variables that were checked.
- **Fix:** set the named variable, pass `StreamOptions.APIKey` directly, or
  supply `StreamOptions.Env`. Confirm which variables a provider expects with
  `APIKeyEnvVars`, and which are actually set with `FindEnvAPIKeys`.

```go
fmt.Println(llm.APIKeyEnvVars("minimax-cn")) // [MINIMAX_CN_API_KEY]
fmt.Println(llm.FindEnvAPIKeys("minimax-cn")) // [] means none is set
```

## The response failed (`StopReasonError`)

`Complete` returns a non-nil error, or a stream ends with `EventError`.

- **Cause:** a provider or runtime failure mid-request (auth rejected, rate
  limit, an unavailable model).
- **Fix:** read `response.ErrorMessage` for the provider's detail. Do **not**
  execute any tool calls from a failed response. Transient failures are retried
  by the SDK; tune with `StreamOptions.MaxRetries` and `Timeout`.

## The answer is cut off (`StopReasonLength`)

The text ends mid-sentence.

- **Cause:** generation hit the `MaxTokens` cap.
- **Fix:** raise `MaxTokens`, or continue the turn by appending the partial
  assistant message and sending again.

## Silent truncation or context errors

The model ignores the start of a long conversation, or rejects it outright.

- **Cause:** the request exceeded the model's context window. Some providers
  error; others silently drop the overflow.
- **Fix:** call `IsContextOverflow(response, model.ContextWindow)` and, when it
  is true, compact or summarize old messages before retrying.

## Tool arguments are wrong or incomplete

`DecodeToolCall` returns an error, or `AssistantMessage.Diagnostics` reports
recovered arguments.

- **Cause:** the model produced malformed JSON, or the stream was truncated. The
  library recovers arguments best-effort and records how in `Diagnostics` rather
  than failing the whole response.
- **Fix:** before running a tool with side effects, check `Diagnostics` and
  decline `partial` or `invalid` arguments. On a decode error, return a tool
  error (`result.IsError = true`) so the model can correct the call. See the
  [Executing tool calls](recipes/tool-loop.md).
