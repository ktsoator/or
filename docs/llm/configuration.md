# Request configuration

`StreamOptions` contains settings shared by all protocols. Settings whose
semantics differ between protocols are nested under `ProtocolOptions`.

```go
temperature := 0.2
retries := 2
options := llm.StreamOptions{
	Temperature: &temperature,
	MaxTokens:   2048,
	MaxRetries:  &retries,
	Timeout:     30 * time.Second,
	Headers: map[string]string{
		"X-Request-ID": requestID,
	},
}
```

The shared options are:

| Option | Purpose |
|---|---|
| `APIKey` | Override credential lookup for this request |
| `Env` | Override named environment values without mutating the process environment |
| `Temperature` | Override the model sampling temperature |
| `MaxTokens` | Cap output tokens; zero leaves the model default |
| `Headers` | Merge custom HTTP headers over model defaults |
| `Reasoning` | Request a provider-neutral reasoning effort level |
| `ProtocolOptions` | Carry settings for exactly one wire protocol |
| `MaxRetries` | Override SDK retries; `0` disables them |
| `Timeout` | Cap each HTTP attempt independently of context cancellation |
| `OnRequest` | Observe every serialized HTTP request attempt |
| `RewriteRequest` | Replace the serialized request body before it is sent |
| `OnResponse` | Observe every HTTP response attempt |

## Observe HTTP requests and responses

The hooks are useful for logging, tracing, and debugging. Both fire once per
attempt, so retries remain visible. `OnRequest` receives the exact request body
serialized for the provider, including protocol-specific fields.

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

Hooks may expose prompts, tool arguments, credentials in URLs or headers, and
provider response metadata. Redact sensitive data before sending it to logs or
telemetry systems.

## Rewrite the request body

`RewriteRequest` transforms the serialized request body before it is sent, an
escape hatch for provider-specific fields the typed API does not expose. It
receives the same method, URL, and body as `OnRequest` and returns the body to
send; returning `nil` leaves it unchanged. Like the observers it fires once per
attempt and always rewrites the original body, so retries stay consistent.

```go
options := llm.StreamOptions{
	RewriteRequest: func(method, url string, body []byte) []byte {
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil // leave the body unchanged on error
		}
		payload["custom_provider_field"] = true
		patched, err := json.Marshal(payload)
		if err != nil {
			return nil
		}
		return patched
	},
}
```

See [Reasoning](reasoning.md) and [Tools](tools.md) for the protocol-specific
option types currently included with the package.
