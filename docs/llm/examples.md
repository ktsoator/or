# Examples

Runnable programs live under [`example/llm`](https://github.com/ktsoator/or/tree/main/example/llm). Each resolves a model from the built-in catalog and reads its API key from the environment, so every run command is prefixed with the key it needs. All but `model_switch` use a single DeepSeek key.

The code for each example is collapsed below — click a block to expand it. A good reading order is top to bottom: request basics, then streaming and reasoning, then tools, conversations, and the lower-level controls.

## basic

The smallest possible program, end to end. `GetModel` resolves an entry from the built-in catalog (a provider plus a model id); `Prompt` wraps a single string into a one-message `Context`; and `Complete` runs the whole request, blocking until the model finishes, and returns one `AssistantMessage`. With an empty `StreamOptions{}`, the API key is read from the provider's environment variable. Start here to confirm your key and network path work before adding anything else.

```sh
DEEPSEEK_API_KEY=… go run ./example/llm/basic
```

??? example "example/llm/basic/main.go"

    ```go
    // Command basic sends a single prompt to a model and prints the reply.
    //
    // It is the smallest possible use of the llm package: resolve a model from the
    // built-in catalog, build a one-message Context with Prompt, and let Complete
    // run the request and return the final assistant message.
    //
    // The API key is read from the provider's environment variable when
    // StreamOptions.APIKey is empty:
    //
    //	DEEPSEEK_API_KEY=sk-... go run ./example/llm/basic
    package main

    import (
    	"context"
    	"fmt"
    	"log"

    	"github.com/ktsoator/or/llm"
    	_ "github.com/ktsoator/or/llm/openai" // register the OpenAI-compatible protocol (DeepSeek speaks it)
    )

    func main() {
    	model := llm.GetModel("deepseek", "deepseek-v4-flash")

    	msg, err := llm.Complete(context.Background(), model,
    		llm.Prompt("Explain goroutines in one sentence."),
    		llm.StreamOptions{})
    	if err != nil {
    		log.Fatal(err)
    	}

    	fmt.Println(msg.Text())
    }
    ```

## options

One step past `basic`. `PromptWithSystem` prepends a system message that sets tone and role; `Temperature` and `MaxTokens` bound how the model generates; and the returned message carries more than text — `Usage` (input and output tokens), `Usage.Cost.Total` (priced from the catalog), and `StopReason` (why generation ended). In production, read these on every response, not just when debugging.

```sh
DEEPSEEK_API_KEY=… go run ./example/llm/options
```

??? example "example/llm/options/main.go"

    ```go
    // Command options sends a prompt with a system message and per-request options.
    //
    // It shows the next step after the basic example: shape the model's behavior
    // with a system prompt, control generation with StreamOptions, and inspect
    // response metadata such as token usage, cost, and stop reason.
    //
    // The API key is read from the provider's environment variable when
    // StreamOptions.APIKey is empty:
    //
    //	DEEPSEEK_API_KEY=sk-... go run ./example/llm/options
    package main

    import (
    	"context"
    	"fmt"
    	"log"

    	"github.com/ktsoator/or/llm"
    	_ "github.com/ktsoator/or/llm/openai" // register the OpenAI-compatible protocol (DeepSeek speaks it)
    )

    func ptr[T any](v T) *T {
    	return &v
    }

    func main() {
    	model := llm.GetModel("deepseek", "deepseek-v4-flash")

    	input := llm.PromptWithSystem(
    		"You are a concise Go expert.",
    		"How should I choose between channels and mutexes?",
    	)

    	msg, err := llm.Complete(context.Background(), model, input, llm.StreamOptions{
    		Temperature: ptr(0.2),
    		MaxTokens:   500,
    	})
    	if err != nil {
    		log.Fatal(err)
    	}

    	fmt.Println(msg.Text())
    	fmt.Printf(
    		"\ntokens in=%d out=%d cost=$%.5f stop=%s\n",
    		msg.Usage.Input,
    		msg.Usage.Output,
    		msg.Usage.Cost.Total,
    		msg.StopReason,
    	)
    }
    ```

## streaming

The same request as `basic`, consumed incrementally. `Stream` returns a channel of `Event`; each `EventTextDelta` is a chunk of text to print as it arrives, and the stream ends with exactly one terminal `EventDone` (whose `Message` is the assembled final message) or `EventError`. This is the shape behind every streaming UI — `Complete` is literally this loop collapsed to return only the final message. Text, reasoning, and tool-call blocks each also emit start and end events when you need finer structure.

```sh
DEEPSEEK_API_KEY=… go run ./example/llm/streaming
```

??? example "example/llm/streaming/main.go"

    ```go
    // Command streaming consumes a response as a live event stream instead of
    // waiting for the final message.
    //
    // It shows the difference from the basic example: Stream returns a channel of
    // Event values that report incremental progress. Here it prints each text delta
    // as it arrives, then stops on the single terminal EventDone (or EventError).
    // Complete is just this loop wrapped up when you only want the final message.
    //
    // The API key is read from the provider's environment variable when
    // StreamOptions.APIKey is empty:
    //
    //	DEEPSEEK_API_KEY=sk-... go run ./example/llm/streaming
    package main

    import (
    	"context"
    	"fmt"
    	"log"

    	"github.com/ktsoator/or/llm"
    	_ "github.com/ktsoator/or/llm/openai" // register the OpenAI-compatible protocol (DeepSeek speaks it)
    )

    func main() {
    	model := llm.GetModel("deepseek", "deepseek-v4-flash")

    	events, err := llm.Stream(context.Background(), model,
    		llm.Prompt("Write a short haiku about Go."),
    		llm.StreamOptions{})
    	if err != nil {
    		log.Fatal(err)
    	}

    	for event := range events {
    		switch event.Type {
    		case llm.EventTextDelta:
    			fmt.Print(event.Delta) // incremental text, printed as it streams
    		case llm.EventDone:
    			fmt.Println() // terminal event: event.Message holds the final message
    		case llm.EventError:
    			log.Fatal(event.Err)
    		}
    	}
    }
    ```

## reasoning

Turns on thinking with the provider-neutral `Reasoning` level (`off` through `xhigh`). The model's thinking streams as `EventThinking*` events, kept separate from the `EventText*` answer, so you can show a "thinking…" panel, log it, or drop it entirely. Each adapter maps the level to that provider's native reasoning form and clamps it to what the model supports; a model without reasoning simply ignores the setting, so the same code stays safe across models.

```sh
DEEPSEEK_API_KEY=… go run ./example/llm/reasoning
```

??? example "example/llm/reasoning/main.go"

    ```go
    // Command reasoning asks a reasoning-capable model to think before answering and
    // streams the reasoning and the final answer as separate phases.
    //
    // It shows the provider-neutral Reasoning knob: a ModelThinkingLevel that each
    // adapter maps to its own native form and clamps to what the model supports.
    // Thinking arrives as EventThinking* events, distinct from the EventText* answer,
    // so a caller can display or hide it. Non-reasoning models ignore the setting.
    //
    // The API key is read from the provider's environment variable when
    // StreamOptions.APIKey is empty:
    //
    //	DEEPSEEK_API_KEY=sk-... go run ./example/llm/reasoning
    package main

    import (
    	"context"
    	"fmt"
    	"log"

    	"github.com/ktsoator/or/llm"
    	_ "github.com/ktsoator/or/llm/openai" // register the OpenAI-compatible protocol (DeepSeek speaks it)
    )

    func main() {
    	model := llm.GetModel("deepseek", "deepseek-v4-flash")

    	events, err := llm.Stream(context.Background(), model,
    		llm.Prompt("A farmer must cross a river with a wolf, a goat, and a cabbage. "+
    			"The boat carries only one item at a time. How does he get all three across?"),
    		llm.StreamOptions{
    			Reasoning: llm.ModelThinkingHigh, // off, minimal, low, medium, high, xhigh
    		})
    	if err != nil {
    		log.Fatal(err)
    	}

    	for event := range events {
    		switch event.Type {
    		case llm.EventThinkingStart:
    			fmt.Println("--- thinking ---")
    		case llm.EventThinkingDelta:
    			fmt.Print(event.Delta)
    		case llm.EventTextStart:
    			fmt.Println("\n--- answer ---")
    		case llm.EventTextDelta:
    			fmt.Print(event.Delta)
    		case llm.EventDone:
    			fmt.Println()
    		case llm.EventError:
    			log.Fatal(event.Err)
    		}
    	}
    }
    ```

## tools

The example to study — a hand-written tool loop that lays bare what `or/agent` automates for you. The model can call a typed tool, read the result, and keep going until it produces a final text answer. Each turn:

- `MustTool[T]` derives a JSON Schema from a Go struct once, up front.
- `Stream` surfaces the model's thinking and answer live.
- On `EventDone`, **append the assistant message first**, then inspect `ToolCalls()`.
- `DecodeToolCall[T]` validates each call; on failure, feed the error back as a `ToolResult` so the model can self-correct its arguments.
- Otherwise run the tool and append its `ToolResult`, then loop.
- No tool calls means the streamed answer was final — stop.

Wrap this loop with run state, steering, and persistence and you have an agent, which is exactly why the loop lives in the library's foundations rather than hidden behind them.

```sh
DEEPSEEK_API_KEY=… go run ./example/llm/tools
```

??? example "example/llm/tools/main.go"

    ```go
    // Command tools runs a streaming tool loop with reasoning: the model thinks,
    // optionally calls a typed tool, sees the result, and continues until it gives a
    // final answer.
    //
    // It combines the pieces the agent package automates into one loop:
    //   - MustTool derives a JSON Schema from a Go struct.
    //   - Stream with Reasoning surfaces the model's thinking (EventThinking*) and
    //     answer (EventText*) live, turn by turn.
    //   - EventDone carries the final assistant message; DecodeToolCall validates and
    //     decodes any tool calls, and ToolResult feeds each outcome back.
    //
    // Wrap this loop with state, steering, and persistence and you have an agent.
    //
    // The API key is read from the provider's environment variable when
    // StreamOptions.APIKey is empty:
    //
    //	DEEPSEEK_API_KEY=sk-... go run ./example/llm/tools
    package main

    import (
    	"context"
    	"fmt"
    	"log"

    	"github.com/ktsoator/or/llm"
    	_ "github.com/ktsoator/or/llm/openai" // register the OpenAI-compatible protocol (DeepSeek speaks it)
    )

    // WeatherArgs is the tool's argument schema. A json field without omitempty is
    // required; jsonschema tags add descriptions and constraints to the schema.
    type WeatherArgs struct {
    	City string `json:"city" jsonschema:"description=City name,minLength=1"`
    	Days int    `json:"days" jsonschema:"minimum=1,maximum=10"`
    }

    func main() {
    	model := llm.GetModel("deepseek", "deepseek-v4-flash")

    	weather := llm.MustTool[WeatherArgs]("get_weather", "Get a weather forecast for a city")

    	input := llm.NewContext(llm.UserText("What should I pack for a trip to Beijing over the next 3 days?"))
    	input.Tools = []llm.ToolDefinition{weather}

    	for turn := 1; ; turn++ {
    		fmt.Printf("\n===== turn %d =====\n", turn)

    		events, err := llm.Stream(context.Background(), model, input, llm.StreamOptions{
    			Reasoning: llm.ModelThinkingHigh,
    		})
    		if err != nil {
    			log.Fatal(err)
    		}

    		var final llm.AssistantMessage
    		for event := range events {
    			switch event.Type {
    			case llm.EventThinkingStart:
    				fmt.Print("[thinking] ")
    			case llm.EventThinkingDelta:
    				fmt.Print(event.Delta)
    			case llm.EventTextStart:
    				fmt.Print("\n[answer] ")
    			case llm.EventTextDelta:
    				fmt.Print(event.Delta)
    			case llm.EventDone:
    				fmt.Println()
    				final = *event.Message // the assembled assistant turn
    			case llm.EventError:
    				log.Fatal(event.Err)
    			}
    		}
    		input.Messages = append(input.Messages, &final) // record the assistant turn

    		calls := final.ToolCalls()
    		if len(calls) == 0 {
    			return // no tool calls: the streamed answer above was final
    		}

    		for _, call := range calls {
    			args, err := llm.DecodeToolCall[WeatherArgs](weather, call)
    			if err != nil {
    				// Feed the error back so the model can correct its arguments.
    				input.Messages = append(input.Messages,
    					llm.ToolResult(call.ID, call.Name, "invalid arguments: "+err.Error()))
    				continue
    			}

    			// A real tool would do work here; this one returns a canned result.
    			result := fmt.Sprintf("%s: sunny, around 25C for the next %d days", args.City, args.Days)
    			fmt.Printf("[tool] get_weather(city=%q, days=%d) -> %s\n", args.City, args.Days, result)
    			input.Messages = append(input.Messages, llm.ToolResult(call.ID, call.Name, result))
    		}
    	}
    }
    ```

## conversation

A multi-turn exchange, and the clearest demonstration of the library being stateless. The history is a `[]llm.Message` you own; after each reply you append the assistant turn (as a pointer, so it keeps the type needed to replay it) and the next user message, then resend the whole slice. The library stores nothing server-side — the follow-up ("that pattern") only resolves because the earlier turns travel with the request. A `SystemPrompt` set on the `Context` applies every turn without being part of the history.

```sh
DEEPSEEK_API_KEY=… go run ./example/llm/conversation
```

??? example "example/llm/conversation/main.go"

    ```go
    // Command conversation carries history across multiple turns.
    //
    // It shows the step beyond a one-shot Complete: keep the messages in a slice,
    // append each reply and follow-up, and send the growing history back every
    // turn. The library is stateless, so retaining and resending the history is how
    // the model "remembers" earlier turns.
    //
    // The API key is read from the provider's environment variable when
    // StreamOptions.APIKey is empty:
    //
    //	DEEPSEEK_API_KEY=sk-... go run ./example/llm/conversation
    package main

    import (
    	"context"
    	"fmt"
    	"log"

    	"github.com/ktsoator/or/llm"
    	_ "github.com/ktsoator/or/llm/openai" // register the OpenAI-compatible protocol (DeepSeek speaks it)
    )

    func main() {
    	ctx := context.Background()
    	model := llm.GetModel("deepseek", "deepseek-v4-flash")

    	// The conversation is just a slice of messages the caller owns.
    	history := []llm.Message{
    		llm.UserText("Name one classic Go concurrency pattern."),
    	}

    	// Turn 1: ask the first question.
    	first := ask(ctx, model, history)
    	fmt.Println("A1:", first.Text())

    	// Append the reply, then a follow-up that relies on it ("that pattern").
    	// Resending the whole history is what lets the model resolve the reference.
    	history = append(history, &first)
    	history = append(history, llm.UserText("Show a minimal code sketch of that pattern."))

    	// Turn 2: the model answers with the earlier turn in context.
    	second := ask(ctx, model, history)
    	fmt.Println("\nA2:", second.Text())
    }

    // ask sends the current history with a shared system prompt and returns the
    // final assistant message.
    func ask(ctx context.Context, model llm.Model, history []llm.Message) llm.AssistantMessage {
    	input := llm.Context{
    		SystemPrompt: "You are a concise Go tutor. Keep answers short.",
    		Messages:     history,
    	}

    	msg, err := llm.Complete(ctx, model, input, llm.StreamOptions{MaxTokens: 500})
    	if err != nil {
    		log.Fatal(err)
    	}
    	return msg
    }
    ```

## model_switch

One conversation carried across two different wire protocols — the library's core value in a single file. Turn 1 goes to DeepSeek (OpenAI-compatible Chat Completions); turn 2 sends the *same, unchanged* history to MiniMax CN (Anthropic-compatible Messages). Because two protocols are in play, both provider packages must be registered (blank imports) and each needs its own key. Before each request `llm` re-adapts the stored history for the target protocol — downgrading images, reconciling tool-call IDs, handling reasoning signatures — so you never rebuild the conversation by hand.

```sh
DEEPSEEK_API_KEY=… MINIMAX_CN_API_KEY=… go run ./example/llm/model_switch
```

??? example "example/llm/model_switch/main.go"

    ```go
    // Command model_switch continues one conversation across two protocols.
    //
    // Turn 1 goes to DeepSeek, which speaks OpenAI-compatible Chat Completions.
    // Turn 2 sends the same history — unchanged — to MiniMax on its China endpoint,
    // which speaks Anthropic-compatible Messages. The caller does not rebuild the
    // conversation: llm re-adapts the stored history for the target protocol on
    // each request (downgrading images, reconciling tool-call IDs, and so on).
    //
    // Because the two turns use different protocols, both provider packages must be
    // registered. Each needs its own key:
    //
    //	DEEPSEEK_API_KEY=sk-...   (DeepSeek, OpenAI-compatible)
    //	MINIMAX_CN_API_KEY=...    (MiniMax CN, Anthropic-compatible)
    //
    //	DEEPSEEK_API_KEY=... MINIMAX_CN_API_KEY=... go run ./example/llm/model_switch
    package main

    import (
    	"context"
    	"fmt"
    	"log"

    	"github.com/ktsoator/or/llm"
    	_ "github.com/ktsoator/or/llm/anthropic" // MiniMax CN speaks Anthropic-compatible Messages
    	_ "github.com/ktsoator/or/llm/openai"    // DeepSeek speaks OpenAI-compatible Chat Completions
    )

    func main() {
    	ctx := context.Background()

    	deepseek := llm.GetModel("deepseek", "deepseek-v4-flash")
    	minimax := llm.GetModel("minimax-cn", "MiniMax-M2.7")

    	history := []llm.Message{
    		llm.UserText("Suggest a name for a Go library that unifies LLM providers."),
    	}

    	// Turn 1 — DeepSeek (OpenAI-compatible).
    	first := complete(ctx, deepseek, history)
    	fmt.Printf("[%s] %s\n", deepseek.Provider, first.Text())

    	// Carry the reply forward and ask a follow-up.
    	history = append(history, &first)
    	history = append(history, llm.UserText("Now critique that name in one sentence."))

    	// Turn 2 — MiniMax CN (Anthropic-compatible). Same history slice, different
    	// protocol; no manual conversion needed.
    	second := complete(ctx, minimax, history)
    	fmt.Printf("[%s] %s\n", minimax.Provider, second.Text())
    }

    func complete(ctx context.Context, model llm.Model, history []llm.Message) llm.AssistantMessage {
    	msg, err := llm.Complete(ctx, model, llm.NewContext(history...), llm.StreamOptions{MaxTokens: 500})
    	if err != nil {
    		log.Fatal(err)
    	}
    	return msg
    }
    ```

## advanced

Two lower-level controls layered onto an otherwise normal request. `OnRequest` hands you the exact serialized request body just before it goes out (once per attempt, retries included) — useful for debugging, logging, or asserting on the wire format in tests. A protocol-specific `ToolChoice`, carried on `ProtocolOptions`, forces the model to call a tool this turn; it is validated against the target protocol before sending, so a mismatched option fails fast instead of reaching the provider. For an endpoint not in the catalog, build an `llm.Model` by hand and point its `BaseURL` at it.

```sh
DEEPSEEK_API_KEY=… go run ./example/llm/advanced
```

??? example "example/llm/advanced/main.go"

    ```go
    // Command advanced shows two lower-level controls layered on a normal request:
    //
    //   - Observe the exact serialized HTTP request with the OnRequest hook. It fires
    //     once per attempt, including retries.
    //   - Force the model to call a tool with a protocol-specific ToolChoice, carried
    //     on ProtocolOptions and validated against the target protocol before sending.
    //
    // The model comes from the built-in catalog. To reach an OpenAI-compatible
    // endpoint that is not in the catalog, construct an llm.Model by hand instead and
    // point its BaseURL at that endpoint (Provider still drives the API key lookup).
    //
    // The API key is read from the provider's environment variable when
    // StreamOptions.APIKey is empty:
    //
    //	DEEPSEEK_API_KEY=sk-... go run ./example/llm/advanced
    package main

    import (
    	"context"
    	"fmt"
    	"log"

    	"github.com/ktsoator/or/llm"
    	_ "github.com/ktsoator/or/llm/openai" // register the OpenAI-compatible protocol (DeepSeek speaks it)
    )

    // WeatherArgs is the tool's argument schema.
    type WeatherArgs struct {
    	City string `json:"city" jsonschema:"description=City name,minLength=1"`
    	Days int    `json:"days" jsonschema:"minimum=1,maximum=10"`
    }

    func main() {
    	model := llm.GetModel("deepseek", "deepseek-v4-flash")

    	weather := llm.MustTool[WeatherArgs]("get_weather", "Get a weather forecast for a city")

    	input := llm.NewContext(llm.UserText("What's the weather in Beijing for the next 3 days?"))
    	input.Tools = []llm.ToolDefinition{weather}

    	msg, err := llm.Complete(context.Background(), model, input, llm.StreamOptions{
    		// Observe the exact request body sent to the provider.
    		OnRequest: func(method, url string, body []byte) {
    			fmt.Printf(">> %s %s\n%s\n\n", method, url, body)
    		},
    		// Protocol-specific option: force the model to call a tool this turn.
    		ProtocolOptions: &llm.OpenAICompletionsStreamOptions{
    			ToolChoice: llm.OpenAIToolChoiceRequired,
    		},
    	})
    	if err != nil {
    		log.Fatal(err)
    	}

    	for _, call := range msg.ToolCalls() {
    		args, err := llm.DecodeToolCall[WeatherArgs](weather, call)
    		if err != nil {
    			log.Fatal(err)
    		}
    		fmt.Printf("model chose: get_weather(city=%q, days=%d)\n", args.City, args.Days)
    	}
    }
    ```

## providers

The provider registry that sits beside the model catalog. `DefaultProviderRegistry` returns the registry behind the package-level `Complete`. `AuthStatus` reports whether a provider's key resolves and where it comes from, without a request. `NewSpecProvider` registers a custom endpoint. `SetOverride`, shown commented, sends a provider's traffic through a proxy without editing any `Model`. Useful for a settings screen or a gateway.

```sh
DEEPSEEK_API_KEY=… go run ./example/llm/providers
```

??? example "example/llm/providers/main.go"

    ```go
    // Command providers inspects and configures providers at runtime.
    //
    // It shows the provider registry that sits beside the model catalog: query
    // whether a provider has a usable credential with AuthStatus, redirect a
    // provider's traffic with SetOverride, and register a custom endpoint with
    // NewSpecProvider. The package-level Complete resolves every request through
    // the default registry, so an override applied here reaches the request below.
    //
    // The API key is read from the provider's environment variable when
    // StreamOptions.APIKey is empty:
    //
    //	DEEPSEEK_API_KEY=sk-... go run ./example/llm/providers
    package main

    import (
    	"context"
    	"fmt"
    	"log"

    	"github.com/ktsoator/or/llm"
    	_ "github.com/ktsoator/or/llm/openai" // register the OpenAI-compatible protocol (DeepSeek speaks it)
    )

    func main() {
    	registry := llm.DefaultProviderRegistry()

    	// 1. Check whether a provider is configured, without sending a request.
    	status, ok := registry.AuthStatus("deepseek", nil)
    	if !ok {
    		log.Fatal("deepseek is not a known provider")
    	}
    	if status.Configured {
    		fmt.Printf("deepseek configured via %s\n", status.Source)
    	} else {
    		fmt.Printf("deepseek not configured; set one of %v\n", status.Missing)
    	}

    	// 2. Register a custom provider. It then appears in the registry and
    	// resolves its key from its own environment variable.
    	if err := registry.Register(llm.NewSpecProvider(llm.ProviderSpec{
    		ID:      "local",
    		Name:    "Local LLM",
    		EnvKeys: []string{"LOCAL_API_KEY"},
    		Models: []llm.Model{{
    			ID:       "qwen2.5-coder:7b",
    			Provider: "local",
    			Protocol: llm.ProtocolOpenAICompletions,
    			BaseURL:  "http://localhost:11434/v1",
    			Input:    []llm.ModelInput{llm.Text},
    		}},
    	})); err != nil {
    		log.Fatal(err)
    	}
    	localStatus, _ := registry.AuthStatus("local", llm.ProviderEnv{"LOCAL_API_KEY": "ollama"})
    	fmt.Printf("local configured via %s\n", localStatus.Source)

    	// 3. Overrides apply to every request routed through the provider. Uncomment
    	// to send DeepSeek traffic through a proxy without editing any Model:
    	//
    	//	proxy := "https://proxy.example.com/deepseek/v1"
    	//	registry.SetOverride("deepseek", llm.ProviderOverride{BaseURL: &proxy})

    	// The request below still resolves its key and base URL through the registry.
    	model := llm.GetModel("deepseek", "deepseek-v4-flash")
    	msg, err := llm.Complete(context.Background(), model,
    		llm.Prompt("Name one benefit of a provider registry, in one sentence."),
    		llm.StreamOptions{})
    	if err != nil {
    		log.Fatal(err)
    	}
    	fmt.Println(msg.Text())
    }
    ```

## whoami

A read-only tool that inspects local configuration instead of calling a model. It walks the provider registry and asks `AuthStatus` whether each provider's key resolves from the environment, then prints the configured ones with their key source and model count. `--models` expands each provider's models; `--all` also lists the unconfigured providers and the env vars they expect. Use it to confirm which keys are in place before running anything else. Providers on a protocol without an adapter are flagged `catalog-only`.

```sh
DEEPSEEK_API_KEY=… go run ./example/llm/whoami --models
```

??? example "example/llm/whoami/main.go"

    ```go
    // Command whoami reports which providers are configured and lists their models.
    //
    // It reads the provider registry — the same one backing the package-level
    // Complete — and asks each provider whether a usable API key resolves from the
    // environment via AuthStatus. Nothing is sent to any provider; this only
    // inspects local configuration, so it is a good way to confirm which keys are
    // in place before running anything else.
    //
    // Run it with whatever provider keys you have exported:
    //
    //	DEEPSEEK_API_KEY=sk-... go run ./example/llm/whoami
    //	go run ./example/llm/whoami --models   # also list each configured model
    //	go run ./example/llm/whoami --all      # also list unconfigured providers
    package main

    import (
    	"flag"
    	"fmt"
    	"sort"

    	"github.com/ktsoator/or/llm"
    	_ "github.com/ktsoator/or/llm/all" // register the built-in protocol adapters
    )

    func main() {
    	showModels := flag.Bool("models", false, "list each configured provider's models")
    	all := flag.Bool("all", false, "also list unconfigured providers")
    	flag.Parse()

    	registry := llm.DefaultProviderRegistry()

    	// The two protocols with a registered adapter. A provider whose models use
    	// another protocol is listed in the catalog but cannot serve a request yet.
    	hasAdapter := map[llm.Protocol]bool{
    		llm.ProtocolOpenAICompletions: true,
    		llm.ProtocolAnthropicMessages: true,
    	}

    	var configured, unconfigured []*llm.Provider
    	for _, provider := range registry.Providers() {
    		if status, _ := registry.AuthStatus(provider.ID(), nil); status.Configured {
    			configured = append(configured, provider)
    		} else {
    			unconfigured = append(unconfigured, provider)
    		}
    	}

    	fmt.Printf("Configured providers: %d / %d\n\n", len(configured), len(configured)+len(unconfigured))

    	if len(configured) == 0 {
    		fmt.Println("(none — export a provider key, e.g. DEEPSEEK_API_KEY, and re-run)")
    	}
    	for _, provider := range configured {
    		status, _ := registry.AuthStatus(provider.ID(), nil)
    		models := provider.Models()
    		note := ""
    		if !hasAdapter[protocolOf(models)] {
    			note = "  [catalog-only: no adapter for this protocol yet]"
    		}
    		fmt.Printf("✓ %-22s %-24s %3d models%s\n", provider.ID(), status.Source, len(models), note)
    		if *showModels {
    			printModels(models)
    		}
    	}

    	if *all && len(unconfigured) > 0 {
    		fmt.Printf("\nUnconfigured (%d):\n", len(unconfigured))
    		for _, provider := range unconfigured {
    			status, _ := registry.AuthStatus(provider.ID(), nil)
    			fmt.Printf("  %-22s set %v\n", provider.ID(), status.Missing)
    		}
    	}
    }

    // protocolOf returns the protocol shared by a provider's models. Models are
    // grouped by provider, so the first model's protocol represents the set.
    func protocolOf(models []llm.Model) llm.Protocol {
    	if len(models) == 0 {
    		return ""
    	}
    	return models[0].Protocol
    }

    func printModels(models []llm.Model) {
    	ids := make([]string, 0, len(models))
    	for _, model := range models {
    		ids = append(ids, model.ID)
    	}
    	sort.Strings(ids)
    	for _, id := range ids {
    		fmt.Printf("      - %s\n", id)
    	}
    }
    ```
