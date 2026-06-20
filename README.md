# or
Choose the path from intent to action.

## Setup

Create a Go application and install the library plus the `.env` loader used by
the examples:

```sh
mkdir myapp
cd myapp
go mod init myapp
go get github.com/ktsoator/or/llm
go get github.com/joho/godotenv
```

Create a `.env` file in the application directory. Keep the keys for the
providers you use and remove the rest:

```dotenv
# China and global providers
DEEPSEEK_API_KEY=your-deepseek-api-key
MINIMAX_API_KEY=your-minimax-global-api-key
MINIMAX_CN_API_KEY=your-minimax-cn-api-key
XIAOMI_API_KEY=your-xiaomi-mimo-api-key
ZAI_API_KEY=your-zai-global-api-key
ZAI_CODING_CN_API_KEY=your-zhipu-coding-cn-api-key
MOONSHOT_API_KEY=your-moonshot-api-key
KIMI_API_KEY=your-kimi-coding-api-key

# Additional catalog providers (not individually verified)
ANTHROPIC_API_KEY=your-anthropic-api-key
GROQ_API_KEY=your-groq-api-key
XAI_API_KEY=your-xai-api-key
OPENROUTER_API_KEY=your-openrouter-api-key
CEREBRAS_API_KEY=your-cerebras-api-key
FIREWORKS_API_KEY=your-fireworks-api-key
```

Xiaomi also accepts `MIMO_API_KEY` as an alternative to `XIAOMI_API_KEY`.
Only the key for the provider selected by `llm.GetModel` is read.

Add `.env` to the application's `.gitignore` so the key is never committed:

```gitignore
.env
```

Copy one of the complete examples below into `main.go`, then run:

```sh
go mod tidy
go run .
```

In production, inject the selected provider's API key as a process environment
variable instead of using a `.env` file.

## Providers and models

The library currently implements two protocol adapters:

- `openai-completions`
- `anthropic-messages`

The catalog and compatibility layer explicitly configure these providers:

| Provider | Provider ID | Protocol | Environment variable |
|---|---|---|---|
| DeepSeek | `deepseek` | `openai-completions` | `DEEPSEEK_API_KEY` |
| MiniMax Global | `minimax` | `anthropic-messages` | `MINIMAX_API_KEY` |
| MiniMax China | `minimax-cn` | `anthropic-messages` | `MINIMAX_CN_API_KEY` |
| Xiaomi MiMo | `xiaomi` | `openai-completions` | `XIAOMI_API_KEY` or `MIMO_API_KEY` |
| Z.AI Global | `zai` | `openai-completions` | `ZAI_API_KEY` |
| Zhipu Coding Plan China | `zai-coding-cn` | `openai-completions` | `ZAI_CODING_CN_API_KEY` |
| Moonshot AI Global | `moonshotai` | `openai-completions` | `MOONSHOT_API_KEY` |
| Moonshot AI China | `moonshotai-cn` | `openai-completions` | `MOONSHOT_API_KEY` |
| Kimi Coding | `kimi-coding` | `anthropic-messages` | `KIMI_API_KEY` |

The catalog also contains metadata for additional compatible providers and
models. Those entries can be queried and may work through one of the two
protocol adapters, but they have not all been verified against live provider
APIs and are not a support guarantee.

Automated tests exercise both protocol adapters with local mock servers. They do
not currently run live integration tests against every provider listed above.

Query the catalog instead of hard-coding model IDs supplied by users:

<details>
<summary>Complete model discovery example</summary>

```go
package main

import (
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
)

func main() {
	for _, provider := range llm.GetProviders() {
		fmt.Println(provider)
		for _, model := range llm.GetModels(provider) {
			fmt.Printf("  %s: %s\n", model.ID, model.Name)
		}
	}

	model, ok := llm.LookupModel("xiaomi", "mimo-v2-flash")
	if !ok {
		log.Fatal("model not found")
	}
	fmt.Printf("selected %s/%s via %s\n", model.Provider, model.ID, model.Protocol)
}
```

</details>

Use `LookupModel` for dynamic input. `GetModel` is convenient for known catalog
entries and panics when the provider or model ID does not exist.

## Quick start

The `llm` package includes the built-in OpenAI-compatible and Anthropic protocol
adapters and can call `Complete` directly:

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/joho/godotenv"
	"github.com/ktsoator/or/llm"
)

func main() {
	_ = godotenv.Load()

	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	response, err := llm.Complete(
		context.Background(),
		model,
		llm.Context{Messages: []llm.Message{
			&llm.UserMessage{Content: []llm.UserContent{
				&llm.TextContent{Text: "Explain Go channels briefly."},
			}},
		}},
		llm.StreamOptions{},
	)
	if err != nil {
		log.Fatal(err)
	}

	for _, block := range response.Content {
		if text, ok := block.(*llm.TextContent); ok {
			fmt.Println(text.Text)
		}
	}
}
```

Set the provider API key in the environment, for example
`DEEPSEEK_API_KEY`. Use `llm.NewClient` when an isolated built-in client is
needed.

## Streaming

Use `Stream` to process text as it is generated:

<details>
<summary>Complete streaming example</summary>

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/joho/godotenv"
	"github.com/ktsoator/or/llm"
)

func main() {
	_ = godotenv.Load()

	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	events, err := llm.Stream(
		context.Background(),
		model,
		llm.Context{Messages: []llm.Message{
			&llm.UserMessage{Content: []llm.UserContent{
				&llm.TextContent{Text: "Explain Go channels briefly."},
			}},
		}},
		llm.StreamOptions{Reasoning: llm.ModelThinkingHigh},
	)
	if err != nil {
		log.Fatal(err)
	}

	thinkingStarted := false
	answerStarted := false
	var finalMessage *llm.AssistantMessage
	for event := range events {
		switch event.Type {
		case llm.EventThinkingDelta:
			if !thinkingStarted {
				fmt.Println("[thinking]")
				thinkingStarted = true
			}
			fmt.Print(event.Delta)
		case llm.EventTextDelta:
			if !answerStarted {
				if thinkingStarted {
					fmt.Print("\n\n")
				}
				fmt.Println("[answer]")
				answerStarted = true
			}
			fmt.Print(event.Delta)
		case llm.EventDone:
			finalMessage = event.Message
		case llm.EventError:
			log.Fatal(event.Err)
		}
	}
	if finalMessage == nil {
		log.Fatal("stream closed without a final message")
	}
	fmt.Printf(
		"\nstop=%s tokens=%d cost=$%.6f\n",
		finalMessage.StopReason,
		finalMessage.Usage.TotalTokens,
		finalMessage.Usage.Cost.Total,
	)
}
```

</details>

Thinking events are emitted only when the selected model and provider expose
reasoning content.

### Stream event reference

| Event | Meaning | Main fields |
|---|---|---|
| `EventStart` | The provider stream started | `Partial` |
| `EventTextStart` | A text block started | `ContentIndex`, `Partial` |
| `EventTextDelta` | A text fragment arrived | `ContentIndex`, `Delta`, `Partial` |
| `EventTextEnd` | A text block completed | `ContentIndex`, `Content`, `Partial` |
| `EventThinkingStart` | A reasoning block started | `ContentIndex`, `Partial` |
| `EventThinkingDelta` | A reasoning fragment arrived | `ContentIndex`, `Delta`, `Partial` |
| `EventThinkingEnd` | A reasoning block completed | `ContentIndex`, `Content`, `Partial` |
| `EventToolCallStart` | A tool call block started | `ContentIndex`, `ToolCall`, `Partial` |
| `EventToolCallDelta` | A raw tool-argument JSON fragment arrived | `ContentIndex`, `Delta`, `ToolCall`, `Partial` |
| `EventToolCallEnd` | A tool call finished streaming, arguments parsed best-effort | `ContentIndex`, `ToolCall`, `Partial` |
| `EventDone` | The request completed successfully | `Message` |
| `EventError` | The request failed or was cancelled | `Err`, `Message` |

`EventDone.Message` is the final assistant message and contains content, usage,
cost, and stop reason. `EventError.Message` may contain partial content and usage.
The channel emits exactly one terminal event and then closes.

Events from different content blocks may be interleaved. Use `ContentIndex` to
associate deltas with their block. `EventToolCallDelta.Delta` is raw partial
JSON. `EventToolCallEnd` carries the call with its arguments parsed best-effort:
malformed or truncated JSON degrades to the fields received so far, or to an
empty object, so validate arguments before use. Collect tool calls while
streaming and execute them only after `EventDone`. On `EventError`, treat
`EventError.Message` as partial content for display, logging, or retry only; do
not execute any tool calls from that response.

When a tool call's arguments could not be parsed strictly, the response records
a `tool_arguments_recovered` entry in `Message.Diagnostics` with the recovery
`mode` (`repaired`, `partial`, or `invalid`). Inspect `Diagnostics` before
executing a tool with side effects, and decline `partial` or `invalid`
arguments — return them to the model as a tool error so it can retry.

## Typed tools

Generate a provider-compatible JSON Schema from a Go struct instead of writing
tool parameters by hand. The same type is used to validate, coerce, and decode
the tool call returned by the model:

<details>
<summary>Complete typed tool example</summary>

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/joho/godotenv"
	"github.com/ktsoator/or/llm"
)

type WeatherArgs struct {
	City  string `json:"city" jsonschema:"description=City name,minLength=1"`
	Units string `json:"units,omitempty" jsonschema:"enum=celsius,enum=fahrenheit"`
	Days  int    `json:"days" jsonschema:"minimum=1,maximum=10"`
}

func main() {
	_ = godotenv.Load()

	ctx := context.Background()
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	weatherTool := llm.MustTool[WeatherArgs](
		"get_weather",
		"Get a weather forecast",
	)

	messages := []llm.Message{
		&llm.UserMessage{Content: []llm.UserContent{
			&llm.TextContent{Text: "What's the weather in Shanghai for the next 3 days?"},
		}},
	}
	input := llm.Context{
		Messages: messages,
		Tools:    []llm.ToolDefinition{weatherTool},
	}

	response, err := llm.Complete(ctx, model, input, llm.StreamOptions{})
	if err != nil {
		log.Fatal(err)
	}
	messages = append(messages, &response)

	toolUsed := false
	for _, content := range response.Content {
		toolCall, ok := content.(*llm.ToolCall)
		if !ok || toolCall.Name != weatherTool.Name {
			continue
		}

		arguments, err := llm.DecodeToolCall[WeatherArgs](weatherTool, *toolCall)
		if err != nil {
			log.Fatal(err)
		}
		result := getWeather(arguments)
		messages = append(messages, &llm.ToolResultMessage{
			ToolCallID: toolCall.ID,
			ToolName:   toolCall.Name,
			Content: []llm.ToolResultContent{
				&llm.TextContent{Text: result},
			},
		})
		toolUsed = true
	}
	if !toolUsed {
		log.Fatal("model returned no weather tool call")
	}

	response, err = llm.Complete(ctx, model, llm.Context{
		Messages: messages,
		Tools:    []llm.ToolDefinition{weatherTool},
	}, llm.StreamOptions{})
	if err != nil {
		log.Fatal(err)
	}
	for _, content := range response.Content {
		if text, ok := content.(*llm.TextContent); ok {
			fmt.Println(text.Text)
		}
	}
}

func getWeather(arguments WeatherArgs) string {
	units := arguments.Units
	if units == "" {
		units = "celsius"
	}
	return fmt.Sprintf(
		"%s will be sunny for the next %d days, around 24 degrees %s.",
		arguments.City,
		arguments.Days,
		units,
	)
}
```

</details>

Fields without `omitempty` are required. The generated schema is fully inline
and omits document metadata such as `$schema`, `$id`, `$ref`, and `$defs`.

### Protocol-specific tool choice

Tool choice keeps each protocol's native vocabulary, matching the underlying
APIs. Supply it through `ProtocolOptions`; the client validates that the option
type matches the selected model protocol and that named tools exist in the
request context.

OpenAI-compatible Chat Completions uses `required` and function choices:

```go
options := llm.StreamOptions{
	ProtocolOptions: &llm.OpenAICompletionsStreamOptions{
		ToolChoice: llm.OpenAIToolChoiceRequired,
		// To force one function instead:
		// ToolChoice: llm.OpenAIToolChoiceFunction{Name: "get_weather"},
	},
}
```

Anthropic Messages uses `any` and tool choices:

```go
options := llm.StreamOptions{
	ProtocolOptions: &llm.AnthropicStreamOptions{
		ToolChoice: llm.AnthropicToolChoiceAny,
		// To force one tool instead:
		// ToolChoice: llm.AnthropicToolChoiceTool{Name: "get_weather"},
	},
}
```

Both protocols also expose `Auto` and `None` mode constants. An explicit tool
choice requires at least one tool in `Context.Tools`.

## Reasoning and thinking

`Reasoning` is a provider-neutral effort level. Each adapter maps it to the
target provider's native form — Anthropic adaptive or budget thinking, the
OpenAI-compatible reasoning fields — and clamps it to the levels the model
supports. Non-reasoning models ignore it.

```go
options := llm.StreamOptions{Reasoning: llm.ModelThinkingHigh}
```

The accepted levels are `ModelThinkingOff`, `ModelThinkingMinimal`,
`ModelThinkingLow`, `ModelThinkingMedium`, `ModelThinkingHigh`, and
`ModelThinkingXHigh`. `SupportedThinkingLevels` reports the levels a model
accepts and `ClampThinkingLevel` adjusts a requested level to the nearest
supported one.

On the Anthropic protocol, `ThinkingDisplay` controls how the reasoning is
returned without changing whether the model reasons. `ThinkingDisplayOmitted`
withholds the thinking text while still returning the signature required for
multi-turn tool use, which suits backends that never surface reasoning:

```go
options := llm.StreamOptions{
	Reasoning: llm.ModelThinkingHigh,
	ProtocolOptions: &llm.AnthropicStreamOptions{
		ThinkingDisplay: llm.ThinkingDisplayOmitted,
	},
}
```

While streaming, reasoning arrives as `EventThinkingDelta` events before the
answer text.

## Image input

Multimodal models accept images alongside text in a user message. Provide the
raw bytes as base64 with their MIME type:

```go
raw, err := os.ReadFile("screenshot.png")
if err != nil {
	log.Fatal(err)
}
input := llm.Context{Messages: []llm.Message{
	&llm.UserMessage{Content: []llm.UserContent{
		&llm.TextContent{Text: "Describe the problem shown in this screenshot."},
		&llm.ImageContent{
			MIMEType: "image/png",
			Data:     base64.StdEncoding.EncodeToString(raw),
		},
	}},
}}
```

A model declares image support through `Model.Input`. When a conversation that
contains images is sent to a text-only model, the images are replaced with a
short placeholder automatically, so the same history remains valid across models
of differing capabilities.

## Switching models between turns

The conversation history is provider-neutral, so the target model may change
from one turn to the next — for example, drafting with an inexpensive model and
reviewing with a stronger one. Before each request the library adapts the stored
history for the target model: it downgrades images for text-only models,
preserves reasoning signatures for the same model while downgrading or dropping
them across models, and normalizes tool-call identifiers. No history rebuilding
is required.

<details>
<summary>Complete model-switching example</summary>

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/joho/godotenv"
	"github.com/ktsoator/or/llm"
)

func main() {
	_ = godotenv.Load()

	ctx := context.Background()
	draft := llm.GetModel("deepseek", "deepseek-v4-flash")
	review := llm.GetModel("anthropic", "claude-opus-4-8")

	messages := []llm.Message{
		&llm.UserMessage{Content: []llm.UserContent{
			&llm.TextContent{Text: "Compute 25 * 18 and explain the steps."},
		}},
	}

	first, err := llm.Complete(ctx, draft, llm.Context{Messages: messages}, llm.StreamOptions{})
	if err != nil {
		log.Fatal(err)
	}
	messages = append(messages, &first)
	messages = append(messages, &llm.UserMessage{Content: []llm.UserContent{
		&llm.TextContent{Text: "Check the calculation above for mistakes."},
	}})

	second, err := llm.Complete(ctx, review, llm.Context{Messages: messages}, llm.StreamOptions{})
	if err != nil {
		log.Fatal(err)
	}
	for _, content := range second.Content {
		if text, ok := content.(*llm.TextContent); ok {
			fmt.Println(text.Text)
		}
	}
}
```

</details>

`TransformMessages` performs this adaptation and is exported for callers that
need to inspect the exact history a model would receive.

## Saving and restoring conversations

A `Context` serializes to self-describing JSON: messages carry a role and
content blocks carry a type, so the document round-trips back into concrete
message and content types without manual dispatch. This makes it suitable for
persisting chat history or passing a conversation between services.

```go
data, err := json.MarshalIndent(llm.Context{Messages: messages}, "", "  ")
if err != nil {
	log.Fatal(err)
}
if err := os.WriteFile("conversation.json", data, 0o644); err != nil {
	log.Fatal(err)
}

raw, err := os.ReadFile("conversation.json")
if err != nil {
	log.Fatal(err)
}
var restored llm.Context
if err := json.Unmarshal(raw, &restored); err != nil {
	log.Fatal(err)
}
// restored.Messages is ready to extend and replay against any model.
```

## Discovering providers and models

The built-in catalog is queryable, which is useful for model pickers or
capability filters:

```go
for _, provider := range llm.GetProviders() {
	for _, model := range llm.GetModels(provider) {
		fmt.Printf("%s/%s reasoning=%t vision=%t context=%d\n",
			model.Provider, model.ID, model.Reasoning,
			slices.Contains(model.Input, llm.Image), model.ContextWindow)
	}
}
```

`LookupModel` returns a model and a found flag; `GetModel` returns it directly
and panics when the model is unknown.

## Custom and OpenAI-compatible endpoints

Any endpoint that implements one of the two protocols can be used by
constructing a `Model` directly and pointing `BaseURL` at it. This covers local
servers such as Ollama, vLLM, and LM Studio, as well as private model gateways:

```go
model := llm.Model{
	ID:            "qwen2.5-coder:7b",
	Name:          "Qwen2.5 Coder 7B",
	Provider:      "ollama",
	Protocol:      llm.ProtocolOpenAICompletions,
	BaseURL:       "http://localhost:11434/v1",
	Input:         []llm.ModelInput{llm.Text},
	ContextWindow: 32768,
	MaxTokens:     4096,
}

events, err := llm.Stream(ctx, model, input, llm.StreamOptions{APIKey: "ollama"})
```

Endpoint-specific quirks — reasoning field names, cache-control support, and
similar differences — are configured through `Model.Compatibility` with
`OpenAICompletionsCompatibility` or `AnthropicMessagesCompatibility`.

## Cancelling a request

Cancelling the request context stops an in-flight request. The stream emits a
single `EventError` whose message reports `StopReasonAborted`, then closes.

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

events, err := llm.Stream(ctx, model, input, llm.StreamOptions{})
if err != nil {
	log.Fatal(err)
}
// Call cancel() from elsewhere, for example when the user presses Stop.
for event := range events {
	switch event.Type {
	case llm.EventTextDelta:
		fmt.Print(event.Delta)
	case llm.EventError:
		fmt.Printf("\nstopped: %s\n", event.Message.StopReason)
	}
}
```

## Observing requests and responses

Two optional hooks expose the raw HTTP exchange for logging or debugging. Both
fire once per attempt, so retries are observable. `OnRequest` receives the exact
request body serialized for the provider, including protocol-specific fields
that the neutral types do not show directly.

```go
options := llm.StreamOptions{
	OnRequest: func(method, url string, body []byte) {
		log.Printf("→ %s %s\n%s", method, url, body)
	},
	OnResponse: func(status int, headers http.Header) {
		log.Printf("← %d", status)
	},
}
```

## Custom protocol adapters

The two built-in protocols cover most needs, and any OpenAI- or
Anthropic-compatible endpoint is reachable by pointing a `Model` at its
`BaseURL`. To serve a genuinely different wire protocol, implement
`ProtocolAdapter` and register it alongside the built-ins.

An adapter implements two methods: `Protocol` returns the registry key, and
`Stream` translates the request and emits events. `NewStreamWriter` provides the
event-stream machinery the built-in adapters use — a single `EventStart`, a
`Partial` snapshot on every event, a single terminal event, and cancellation
reported as `StopReasonAborted` — so the adapter only builds the message and
emits deltas.

<details>
<summary>Minimal custom adapter</summary>

```go
type myAdapter struct{ http *http.Client }

func (myAdapter) Protocol() llm.Protocol { return "my-protocol" }

func (a myAdapter) Stream(
	ctx context.Context, model llm.Model, input llm.Context, options llm.StreamOptions,
) (<-chan llm.Event, error) {
	events := make(chan llm.Event)
	go func() {
		defer close(events)

		message := llm.AssistantMessage{
			Protocol: model.Protocol, Provider: model.Provider, Model: model.ID,
		}
		writer := llm.NewStreamWriter(ctx, events, &message)

		// Translate input into the wire request, call the endpoint, and stream the
		// response. On any failure, writer.Fail(err) emits the terminal error.
		reply, usage, err := callMyEndpoint(ctx, a.http, model, input, options)
		if err != nil {
			writer.Fail(err)
			return
		}

		text := &llm.TextContent{}
		message.Content = append(message.Content, text)
		writer.Emit(llm.Event{Type: llm.EventTextStart, ContentIndex: 0})
		for chunk := range reply {
			text.Text += chunk
			writer.Emit(llm.Event{Type: llm.EventTextDelta, ContentIndex: 0, Delta: chunk})
		}
		writer.Emit(llm.Event{Type: llm.EventTextEnd, ContentIndex: 0, Content: text.Text})

		message.Usage = usage
		message.StopReason = llm.StopReasonStop
		writer.Done()
	}()
	return events, nil
}
```

Register it on a client that keeps the built-in protocols:

```go
registry := llm.NewRegistry()
llm.RegisterBuiltins(registry)
if err := registry.Register(myAdapter{http: http.DefaultClient}); err != nil {
	log.Fatal(err)
}
client := llm.NewClientWithRegistry(registry)

model := llm.Model{ID: "x", Provider: "me", Protocol: "my-protocol", MaxTokens: 1024}
message, err := client.Complete(ctx, model, input, llm.StreamOptions{})
```

</details>

The adapter is responsible for the full translation in both directions —
building the wire request from `Context`, framing the response, and emitting
deltas. `CloneToolCall` deep-copies a tool call for an event's `ToolCall` field;
`ParseToolArgumentsMode` recovers truncated argument JSON the same way the
built-in adapters do.

## Acknowledgements

This project is inspired by and partially adapted from
[earendil-works/pi](https://github.com/earendil-works/pi),
created by Mario Zechner.
