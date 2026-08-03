package transport

import (
	"bytes"
	"io"
	"maps"
	"net/http"

	"github.com/ktsoator/or/llm"
	oai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// BuildClient creates one SDK client for the target model endpoint. Model
// headers provide defaults and request headers override values with the same
// name.
func BuildClient(httpClient *http.Client, model llm.Model, options llm.StreamOptions) oai.Client {
	clientOptions := []option.RequestOption{
		option.WithAPIKey(options.APIKey),
		option.WithHTTPClient(httpClient),
		// Drop non-compliant SSE keep-alive heartbeats so providers like Xiaomi
		// MiMo do not break the SDK decoder while the model is thinking.
		option.WithMiddleware(sseHeartbeatFilter),
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
	if options.RewriteRequest != nil {
		clientOptions = append(clientOptions, option.WithMiddleware(rewriteRequestMiddleware(options.RewriteRequest)))
	}
	if options.OnResponse != nil {
		clientOptions = append(clientOptions, option.WithMiddleware(onResponseMiddleware(options.OnResponse)))
	}
	for name, value := range mergedHeaders(model, options) {
		clientOptions = append(clientOptions, option.WithHeader(name, value))
	}
	return oai.NewClient(clientOptions...)
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

// rewriteRequestMiddleware lets hook replace the serialized request body before
// it is sent. Returning nil leaves the body unchanged. The original body is read
// each attempt, so a retried request is rewritten consistently; ContentLength is
// updated to match the rewritten body.
func rewriteRequestMiddleware(hook func(string, string, []byte) []byte) option.Middleware {
	return func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		if req.Body == nil {
			return next(req)
		}
		body, _ := io.ReadAll(req.Body)
		rewritten := hook(req.Method, req.URL.String(), body)
		if rewritten == nil {
			rewritten = body
		}
		req.Body = io.NopCloser(bytes.NewReader(rewritten))
		req.ContentLength = int64(len(rewritten))
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
	maps.Copy(merged, model.Headers)
	maps.Copy(merged, options.Headers)
	return merged
}
