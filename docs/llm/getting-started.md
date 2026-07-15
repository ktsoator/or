# Getting started

## Install

Create a Go application and install the package:

```sh
mkdir myapp
cd myapp
go mod init myapp
go get github.com/ktsoator/or/llm@latest
```

The package reads the API key for the selected provider from the process
environment. For example:

```sh
export DEEPSEEK_API_KEY=your-deepseek-api-key
```

For local development, a `.env` loader such as
[`godotenv`](https://github.com/joho/godotenv) can load credentials before the
first request. Keep `.env` in `.gitignore`; production applications should
inject credentials through their deployment environment.

## Complete a request

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai" // registers the OpenAI-compatible protocol (DeepSeek, Groq, xAI, ...)
)

func main() {
	model, ok := llm.LookupModel("deepseek", "deepseek-v4-flash")
	if !ok || !llm.SupportsProtocol(model.Protocol) {
		log.Fatal("model is not runnable")
	}
	response, err := llm.Complete(
		context.Background(),
		model,
		llm.Prompt("Explain Go channels briefly."),
		llm.StreamOptions{},
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(response.Text())
}
```

Run the program:

```sh
go run .
```

`llm.Complete` collects the stream into one `AssistantMessage`. Use
[`llm.Stream`](streaming.md) when the application needs deltas as they arrive.
The package-level functions dispatch through a default registry; the blank
`llm/openai` import above registers the OpenAI-compatible protocol into it. Import
the provider package for each protocol you use — or `llm/all` for every built-in —
so only the SDKs you need are linked into your binary.

`LookupModel` checks catalog membership, while `SupportsProtocol` checks adapter
registration in this process. Build model pickers from `GetRunnableModels`.
`GetModels` also returns catalog-only protocol entries and is not a runnable list.

## Customize the request

The first example sends an empty `StreamOptions{}`. Add a system prompt with
`PromptWithSystem`, and set common options such as temperature and an output
cap. Options apply to any model regardless of protocol.

```go
temperature := 0.2
response, err := llm.Complete(
	context.Background(),
	model,
	llm.PromptWithSystem("You are a concise Go tutor.", "Explain Go channels."),
	llm.StreamOptions{
		Temperature: &temperature,
		MaxTokens:   512,
	},
)
```

See [Request configuration](configuration.md) for the full option set.

## Inspect usage and cost

Every response reports the tokens it consumed and their cost:

```go
fmt.Printf("tokens=%d cost=$%.6f\n",
	response.Usage.TotalTokens, response.Usage.Cost.Total)
```

[Reading responses](results.md) covers stop reasons, usage, and diagnostics.

## Next steps

- Continue the exchange over several turns with [conversations](conversations.md).
- Choose a model from the [provider catalog](providers.md).
- Confirm protocol and provider status in the [support matrix](support-matrix.md).
- Render responses incrementally with [streaming events](streaming.md).
- Give the model structured capabilities with [typed tools](tools.md).
- Browse runnable programs on the [examples](examples.md) page.
- Find minimal task-oriented code in [recipes](recipes/README.md).
