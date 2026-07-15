# Observability hooks

## What this builds

A request records attempt duration, method, URL, serialized byte count, status, and provider request ID without logging prompt bodies or credentials.

Hooks run once per SDK attempt, so retries remain visible. They execute synchronously in the request path.

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

## Hook semantics

| Hook | Input | Failure behavior |
|---|---|---|
| `OnRequest` | Exact serialized method, URL, and body for each attempt | No error return; blocking delays the request |
| `RewriteRequest` | Same request data; returns replacement bytes | `nil` keeps original; invalid JSON can break the provider call |
| `OnResponse` | Status and headers for each HTTP response | Runs before response body consumption |

`RewriteRequest` is an escape hatch for a provider field not represented by typed options. Prefer `StreamOptions`, protocol options, model compatibility, headers, or provider overrides first.

## Security and operations

- Request bodies contain prompts, images, tools, and tool arguments. Log size or a redacted structure rather than raw bytes.
- URLs and headers can contain tenant IDs or credentials. Apply an allowlist before exporting attributes.
- Move slow telemetry delivery to a buffered application queue; hook callbacks should do bounded work.
- One logical request can produce multiple hook calls because of SDK retries. Attach an attempt number in application state if needed.
- The package provides no logger, metrics exporter, or trace backend. Hooks must integrate with the application's telemetry stack.
