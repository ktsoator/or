# 示例

可运行程序位于 [`example/llm`](https://github.com/ktsoator/or/tree/main/example/llm)。每个程序都从内置目录解析模型，并从环境读取其 API key，因此每条运行命令都带上了所需的 key。除 `model_switch` 外，都只用一个 DeepSeek key。

每个示例的代码在下面默认折叠——点击对应块即可展开。建议从上往下读：先是请求基础，再是流式与推理，然后是工具、对话与更底层的控制。

## 最小请求（basic）

端到端最小的程序。`GetModel` 从内置目录解析一个条目（一个 provider 加一个模型 id）；`Prompt` 把单个字符串包成只含一条消息的 `Context`；`Complete` 执行整个请求，阻塞到模型完成，返回一个 `AssistantMessage`。传空的 `StreamOptions{}` 时，API key 从该 provider 的环境变量读取。先从这里跑通，确认 key 与网络无误，再叠加其它功能。

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

## 系统提示与选项（options）

比 `basic` 进一步。`PromptWithSystem` 在前面加一条系统消息，用来设定语气与角色；`Temperature` 与 `MaxTokens` 约束生成方式；返回的消息也不只有文本——`Usage`（输入与输出 token）、`Usage.Cost.Total`（按目录计价）与 `StopReason`（生成为何结束）。生产环境应在每个响应上都读取这些，而不只在调试时。

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

## 流式输出（streaming）

与 `basic` 是同一个请求，但增量消费。`Stream` 返回一个 `Event` 通道；每个 `EventTextDelta` 是一段可边到达边打印的文本，流以恰好一个终止事件 `EventDone`（其 `Message` 为拼装好的最终消息）或 `EventError` 结束。这就是所有流式 UI 背后的形态——`Complete` 就是把这个循环收拢成只返回最终消息。若需更细的结构，文本、推理与工具调用块各自还会发出 start 与 end 事件。

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

## 推理过程（reasoning）

用协议无关的 `Reasoning` 强度（`off` 到 `xhigh`）开启思考。模型的思考以 `EventThinking*` 事件流出，与 `EventText*` 答案分开，因此可以展示"思考中…"面板、记录它，或完全丢弃。各适配器会把强度映射到该 provider 的原生推理形式，并钳制到模型支持的范围；不具备推理的模型会直接忽略该设置，所以同一份代码在不同模型间都安全。

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

## 工具循环（tools）

最值得研究的一个——手写的工具循环，把 `or/agent` 替调用方自动化的东西摊开来看。模型可以调用类型化工具、读取结果，再继续，直到给出最终文本答案。每一轮：

- `MustTool[T]` 预先从 Go 结构体派生一次 JSON Schema。
- `Stream` 实时呈现模型的思考与答案。
- 在 `EventDone` 时，**先追加 assistant 消息**，再检查 `ToolCalls()`。
- `DecodeToolCall[T]` 校验每个调用；失败时把错误作为 `ToolResult` 回传，让模型自我纠正参数。
- 否则执行工具并追加其 `ToolResult`，然后继续循环。
- 没有工具调用，说明流式输出的答案即为最终结果——停止。

给这个循环套上运行状态、引导与持久化，便得到一个 agent——这正是为什么这段循环属于库的基础层，而不是被藏在它背后。

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

## 多轮对话（conversation）

多轮交流，也是"本库无状态"最清晰的演示。历史由调用方自己持有 `[]llm.Message`；每次回复后，追加 assistant 这一轮（用指针，以保留重放所需的类型）与下一条用户消息，再重新发送整个切片。库不在服务端保存任何东西——后续追问（"那个模式"）能被解析，只因为前几轮随请求一起被带上。设在 `Context` 上的 `SystemPrompt` 每轮生效，且不属于历史。

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

## 跨协议切换模型（model_switch）

同一段对话横跨两种不同的线协议，库的核心价值浓缩在这一个文件里。第 1 轮发给 DeepSeek（OpenAI 兼容的 Chat Completions）；第 2 轮把*同一段、未改动*的历史发给 MiniMax CN（Anthropic 兼容的 Messages）。因为涉及两种协议，两个 provider 包都必须注册（空导入），且各需自己的 key。每次请求前，`llm` 会为目标协议重新适配已存历史（降级图像、协调工具调用 ID、处理推理签名），所以从不需要手动重建对话。

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

## 底层控制（advanced）

在一个原本普通的请求上叠加两项更底层的控制。`OnRequest` 在请求发出前把实际序列化的请求体交回调用方（每次尝试一次，含重试）——便于调试、日志，或在测试里对线格式做断言。通过 `ProtocolOptions` 携带的协议特定 `ToolChoice` 强制模型本轮调用工具；它在发送前会针对目标协议校验，因此不匹配的选项会尽早失败，而不会到达 provider。若端点不在目录中，可手动构造 `llm.Model` 并把其 `BaseURL` 指向它。

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

## 提供方配置（providers）

与模型目录并列的 provider 注册表。`DefaultProviderRegistry` 返回包级 `Complete` 背后的注册表。`AuthStatus` 无需请求即可报告 provider 的 key 是否解析得到及其来源。`NewSpecProvider` 注册一个自定义端点。`SetOverride`（示例中以注释给出）把 provider 的流量导向代理，而不改动任何 `Model`。适合设置界面或网关场景。

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

## 查看已配置的提供方（whoami）

一个只读工具，不调用模型，只检查本地配置。它遍历 provider 注册表，用 `AuthStatus` 询问每家 key 是否能从环境解析出来，然后打印已配置的提供方及其 key 来源和模型数量。`--models` 展开每家的模型；`--all` 会连未配置的提供方及其期望的环境变量一起列出。适合在跑别的东西之前确认哪些 key 已就位。协议没有 adapter 的提供方会被标记为 `catalog-only`。

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
