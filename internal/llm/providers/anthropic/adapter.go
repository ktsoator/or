// Package anthropic implements the Anthropic Messages protocol on top of the
// official anthropic-sdk-go. The same adapter serves real Anthropic models and
// any Anthropic-compatible vendor (e.g. MiniMax) by pointing the base URL at the
// vendor's endpoint, mirroring how the openai package serves many vendors.
package anthropic

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/ktsoator/or/internal/llm"
)

// Adapter translates the Anthropic Messages protocol.
type Adapter struct {
	httpClient *http.Client
}

// NewAdapter creates an adapter that uses httpClient for requests. A nil client
// uses the SDK default.
func NewAdapter(httpClient *http.Client) *Adapter {
	return &Adapter{httpClient: httpClient}
}

// Protocol returns the registry key for the Anthropic Messages protocol.
func (a *Adapter) Protocol() llm.Protocol {
	return llm.ProtocolAnthropicMessages
}

// Stream starts a Messages request and translates SDK stream events into package
// events. It supports text, reasoning, and tool call content.
func (a *Adapter) Stream(
	ctx context.Context,
	model llm.Model,
	input llm.Context,
	options llm.StreamOptions,
) (<-chan llm.Event, error) {
	if model.Protocol != a.Protocol() {
		return nil, fmt.Errorf("model protocol %q does not match adapter protocol %q", model.Protocol, a.Protocol())
	}
	if model.ID == "" {
		return nil, errors.New("model ID is empty")
	}
	if options.APIKey == "" {
		return nil, errors.New("Anthropic API key is empty")
	}

	compat := resolveCompat(model)

	messages, err := convertMessages(input, model, compat)
	if err != nil {
		return nil, err
	}
	tools, err := convertTools(input.Tools)
	if err != nil {
		return nil, err
	}

	// The Messages API requires max_tokens. Fall back to the model's ceiling when
	// the caller does not request a smaller cap.
	maxTokens := options.MaxTokens
	if maxTokens <= 0 {
		maxTokens = model.MaxTokens
	}
	if maxTokens <= 0 {
		return nil, errors.New("max tokens is required and the model defines no default")
	}

	client := buildClient(a.httpClient, model, options)

	params := sdk.MessageNewParams{
		Model:     model.ID,
		MaxTokens: maxTokens,
		Messages:  messages,
	}
	if input.SystemPrompt != "" {
		params.System = []sdk.TextBlockParam{{Text: input.SystemPrompt}}
	}
	if len(tools) > 0 {
		params.Tools = tools
	}
	display := llm.ThinkingDisplay("")
	if anthropicOptions, ok := options.ProtocolOptions.(*llm.AnthropicStreamOptions); ok && anthropicOptions != nil {
		display = anthropicOptions.ThinkingDisplay
		applyToolChoice(&params, anthropicOptions.ToolChoice)
	}
	applyThinking(&params, model, compat, options.Reasoning, display)
	// Temperature is incompatible with thinking and rejected by some models.
	if options.Temperature != nil && compat.supportsTemperature && !thinkingActive(model, options.Reasoning) {
		params.Temperature = sdk.Float(*options.Temperature)
	}
	applyCacheControl(&params, compat)

	events := make(chan llm.Event)
	go consumeStream(ctx, client, params, model, events)
	return events, nil
}

// buildClient creates one SDK client for the target model endpoint. Model
// headers provide defaults and request headers override values with the same
// name.
func buildClient(httpClient *http.Client, model llm.Model, options llm.StreamOptions) sdk.Client {
	clientOptions := []option.RequestOption{
		option.WithAPIKey(options.APIKey),
	}
	if httpClient != nil {
		clientOptions = append(clientOptions, option.WithHTTPClient(httpClient))
	}
	if model.BaseURL != "" {
		clientOptions = append(clientOptions, option.WithBaseURL(model.BaseURL))
	}
	if options.MaxRetries != nil {
		clientOptions = append(clientOptions, option.WithMaxRetries(*options.MaxRetries))
	}
	if options.Timeout > 0 {
		clientOptions = append(clientOptions, option.WithRequestTimeout(options.Timeout))
	}
	if options.OnRequest != nil {
		clientOptions = append(clientOptions, option.WithMiddleware(onRequestMiddleware(options.OnRequest)))
	}
	if options.OnResponse != nil {
		clientOptions = append(clientOptions, option.WithMiddleware(onResponseMiddleware(options.OnResponse)))
	}
	for name, value := range mergedHeaders(model, options) {
		clientOptions = append(clientOptions, option.WithHeader(name, value))
	}
	return sdk.NewClient(clientOptions...)
}

// onRequestMiddleware reports each HTTP request's method, URL, and body to hook
// before it is sent. The body is read and restored so the request still streams
// to the provider. The SDK re-runs middleware on every retry, so the hook
// observes each attempt.
func onRequestMiddleware(hook func(string, string, []byte)) option.Middleware {
	return func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		var body []byte
		if req.Body != nil {
			body, _ = io.ReadAll(req.Body)
			req.Body = io.NopCloser(bytes.NewReader(body))
		}
		hook(req.Method, req.URL.String(), body)
		return next(req)
	}
}

// onResponseMiddleware reports each HTTP response's status and headers to hook
// before the body is consumed. The SDK re-runs middleware on every retry, so the
// hook observes each attempt.
func onResponseMiddleware(hook func(int, http.Header)) option.Middleware {
	return func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		resp, err := next(req)
		if resp != nil {
			hook(resp.StatusCode, resp.Header)
		}
		return resp, err
	}
}

func mergedHeaders(model llm.Model, options llm.StreamOptions) map[string]string {
	if len(model.Headers) == 0 && len(options.Headers) == 0 {
		return nil
	}
	merged := make(map[string]string, len(model.Headers)+len(options.Headers))
	for name, value := range model.Headers {
		merged[name] = value
	}
	for name, value := range options.Headers {
		merged[name] = value
	}
	return merged
}
