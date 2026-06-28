# Configuration

An agent has two layers of settings: agent-level options that describe the run
(model, system prompt, tools, reasoning), and a base `llm.StreamOptions` passed to
the model on every turn for per-request knobs.

## Reasoning and per-request options

`ThinkingLevel` sets the reasoning effort, clamped to what the model supports.
`StreamOptions` carries the shared request settings — temperature, output cap,
headers, and the HTTP observation hooks.

```go
temperature := 0.2
assistant := agent.New(agent.Options{
	Model:         model,
	ThinkingLevel: llm.ModelThinkingHigh,
	StreamOptions: llm.StreamOptions{
		Temperature: &temperature,
		MaxTokens:   4096,
		Headers:     map[string]string{"X-Trace": traceID},
		OnRequest:   func(method, url string, body []byte) { log.Println(method, url) },
		RewriteRequest: func(method, url string, body []byte) []byte {
			return patchVendorField(body) // for fields the typed API does not expose
		},
	},
})
```

The agent fills in `StreamOptions.Reasoning` from `ThinkingLevel` and
`StreamOptions.APIKey` from `GetAPIKey`, so any values you put in those two fields
are ignored. Everything else in `StreamOptions` — including the `OnRequest`,
`OnResponse`, and `RewriteRequest` hooks — applies to every turn. See the llm
[Configuration](../llm/configuration.md) guide for what each option does.

## Dynamic API keys

`GetAPIKey` resolves the provider key before each turn, for short-lived tokens
that may expire during a long run. A non-empty return overrides the key; an empty
return leaves it to the environment.

```go
GetAPIKey: func(provider string) string {
	return currentOAuthToken(provider) // refreshed out of band
},
```

## A custom transport

`StreamFn` reaches a model for one turn and defaults to `llm.Stream`. It exists
mainly as a seam for tests and custom transports — a recorded fixture, a proxy, or
a fake that returns canned turns.

```go
StreamFn: func(ctx context.Context, model llm.Model, input llm.Context, opts llm.StreamOptions) (<-chan llm.Event, error) {
	return myRecordingClient.Stream(ctx, model, input, opts)
},
```

## Reconfiguring between runs

The setters change configuration for the next run; they do not disturb a run
already in progress, which captured its configuration when it started. All are
safe to call concurrently.

```go
assistant.SetModel(llm.GetModel("minimax-cn", "MiniMax-M3"))
assistant.SetSystemPrompt("Answer in one sentence.")
assistant.SetThinkingLevel(llm.ModelThinkingHigh)
assistant.SetTools([]agent.AgentTool{weatherTool}) // the slice is copied
assistant.SetToolExecution(agent.ExecutionSequential)
```

To switch the model *within* a single run instead, use `PrepareNextTurn` — see
[Lifecycle hooks](hooks.md#switching-models-between-turns).

`Reset` clears the transcript, the last error, and both queues while keeping all
of this configuration, so the next run starts a fresh conversation with the same
setup.
