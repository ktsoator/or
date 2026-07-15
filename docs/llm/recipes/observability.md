# Recording and rewriting requests

`StreamOptions` provides three request callbacks: `OnRequest` inspects the outgoing request, `RewriteRequest` changes the serialized request body, and `OnResponse` inspects the HTTP status and headers.

The example records method, URL, body size, response status, request ID, and elapsed time without logging prompt content or credentials. Each SDK attempt invokes the callbacks, so retries produce additional records.

## Before running the example

```sh
go get github.com/ktsoator/or/llm@latest
export DEEPSEEK_API_KEY=your-key
```

## Complete program

```go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func main() {
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	started := time.Now()
	options := llm.StreamOptions{
		Headers: map[string]string{"X-Request-ID": "example-123"},
		OnRequest: func(method, url string, body []byte) {
			log.Printf("llm request method=%s url=%s bytes=%d",
				method, url, len(body))
		},
		OnResponse: func(status int, headers http.Header) {
			log.Printf("llm response status=%d request_id=%q elapsed=%s",
				status, headers.Get("X-Request-ID"), time.Since(started))
		},
	}
	response, err := llm.Complete(context.Background(), model,
		llm.Prompt("Reply with OK."), options)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.Text())
}
```

## Callback timing

The example uses `OnRequest` to observe the serialized request,
`RewriteRequest` to return a replacement body, and `OnResponse` to read status
and headers. All three run for every SDK attempt. Their exact signatures,
return behavior, and call constraints are maintained in
[Request options](../configuration.md#observe-http-requests-and-responses).

Use `RewriteRequest` for model-service fields not yet represented by typed options. Prefer `StreamOptions`, protocol options, `Model.Compatibility`, headers, or provider overrides because they are easier to validate and maintain.

## Logging and performance boundaries

- Request bodies contain prompts, images, tools, and tool arguments. Log size or a redacted structure rather than raw bytes.
- URLs and headers can contain tenant IDs or credentials. Apply an allowlist before exporting attributes.
- Move slow telemetry delivery to a buffered application queue; hook callbacks should do bounded work.
- One logical request can produce multiple hook calls because of SDK retries. Attach an attempt number in application state if needed.
- The package provides no logger, metrics exporter, or trace backend. Hooks must integrate with the application's telemetry stack.
