# Error handling

The library reports failures through two distinct surfaces, and which one you
get tells you where the failure happened:

- A **returned `error`** from `Stream` or `Complete` means the request was
  rejected before anything reached the provider — bad configuration, a missing
  API key, or an unregistered protocol. No tokens were spent.
- A **failed response** means the request reached the provider but generation
  did not complete normally. `Complete` returns the partial `AssistantMessage`
  together with the error; a stream ends with an `EventError`. The message's
  `StopReason` is `StopReasonError` (or `StopReasonAborted` for cancellation)
  and `ErrorMessage` holds the detail.

```go
response, err := llm.Complete(ctx, model, input, opts)
if err != nil {
	// Request never left the process, or the provider stream failed.
	// response may still carry a partial message and StopReason.
	log.Fatalf("request failed: %v", err)
}
```

## Setup errors returned before sending

`Stream` and `Complete` validate the request and resolve credentials before
dispatching to an adapter. They return an error, without contacting the
provider, when:

- **The API key is empty.** If request options, provider overrides, and the
  environment cannot resolve a credential, the adapter returns a
  provider-aware error. See below.
- **No adapter is registered for the model's protocol.** You forgot the blank
  import (`_ "github.com/ktsoator/or/llm/openai"` or `.../llm/anthropic`, or
  `llm/all`). The error is `no adapter registered for protocol "..."`.
- **The options fail validation.** `StreamOptions.Validate` runs first — most
  commonly this rejects `ProtocolOptions` that don't match the target protocol
  (for example passing `AnthropicStreamOptions` to an OpenAI-compatible model).

## Missing API key

When no key is found, the error names the provider and every environment
variable that was checked, in precedence order:

```
API key is empty for provider "anthropic" (set ANTHROPIC_OAUTH_TOKEN or ANTHROPIC_API_KEY or pass StreamOptions.APIKey)
```

Credentials may come from `StreamOptions`, a provider override, or the process
environment. The complete precedence is maintained only in
[Configuration § per-request credentials](configuration.md#supply-credentials-per-request).

To inspect key resolution yourself — for example to fail fast at startup or to
show a setup hint — use the key helpers:

```go
if len(llm.FindEnvAPIKeys(model.Provider)) == 0 {
	log.Printf("no key configured; expected one of %v",
		llm.APIKeyEnvVars(model.Provider))
}
```

`APIKeyEnvVars` returns the variables a provider checks, `FindEnvAPIKeys`
returns the ones actually set, and `MissingAPIKeyError` builds the same message
the library uses. `AuthStatus` can also report an override or environment
source, but it does not verify that the credential is still valid.

## Failed and cancelled responses

Once the request reaches the provider, branch on `StopReason` rather than
treating every non-nil error as fatal. See
[Reading responses](results.md#stop-reasons) for the full table; the two
error-related reasons are:

- `StopReasonError` — a provider or runtime failure mid-stream. Read
  `ErrorMessage`; do **not** execute any tool calls on the message.
- `StopReasonAborted` — the `context` was cancelled. Stop cleanly; this is
  expected when you cancel a request.

```go
response, err := llm.Complete(ctx, model, input, opts)
switch response.StopReason {
case llm.StopReasonError:
	log.Printf("provider error: %s", response.ErrorMessage)
case llm.StopReasonAborted:
	log.Print("cancelled")
default:
	fmt.Println(response.Text())
}
_ = err
```

When streaming, the same failure arrives as a terminal event:

```go
for event := range events {
	if event.Type == llm.EventError {
		log.Printf("stream failed: %v", event.Err)
		break
	}
}
```

See [Streaming § cancellation](streaming.md#cancellation) for cancelling an
in-flight stream.

## Context overflow

A request that exceeds the model's context window may fail explicitly or be
silently truncated by the provider. `IsContextOverflow` detects both, so you can
compact history and retry instead of surfacing a raw error:

```go
if llm.IsContextOverflow(response, model.ContextWindow) {
	// Drop or summarize old messages, then retry the turn.
}
```

See [Reading responses § detect context overflow](results.md#detect-context-overflow).

## Retries and timeouts

Transient provider failures are retried by the underlying SDK. Tune this per
request with `StreamOptions.MaxRetries` (set `0` to disable) and `Timeout` (caps
each attempt independently of the `context` deadline). See
[Configuration](configuration.md) for the full option set.

## Recovered, non-fatal issues

Not every problem is an error. Malformed or truncated tool-call arguments are
recovered best-effort and recorded in `AssistantMessage.Diagnostics` rather than
failing the response — always inspect diagnostics before executing a tool with
side effects. See
[Reading responses § diagnostics](results.md#diagnostics).
