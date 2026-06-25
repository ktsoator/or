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
)

func main() {
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
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
The package-level functions use a client containing both built-in protocol
adapters; `llm.NewClient` creates an isolated client with the same adapters.

## Next steps

- Choose a model from the [provider catalog](providers.md).
- Render responses incrementally with [streaming events](streaming.md).
- Give the model structured capabilities with [typed tools](tools.md).
- Explore the runnable [`llm` examples](../../example/llm/README.md).
