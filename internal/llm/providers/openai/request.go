package openai

import (
	"maps"
	"net/http"

	"github.com/ktsoator/or/internal/llm"
	oai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// buildClient creates one SDK client for the target model endpoint. Model
// headers provide defaults and request headers override values with the same
// name.
func buildClient(httpClient *http.Client, model llm.Model, options llm.StreamOptions) oai.Client {
	clientOptions := []option.RequestOption{
		option.WithAPIKey(options.APIKey),
		option.WithHTTPClient(httpClient),
	}
	if model.BaseURL != "" {
		clientOptions = append(clientOptions, option.WithBaseURL(model.BaseURL))
	}
	for name, value := range mergedHeaders(model, options) {
		clientOptions = append(clientOptions, option.WithHeader(name, value))
	}
	return oai.NewClient(clientOptions...)
}

// buildParams translates provider-independent request options into OpenAI Chat
// Completions parameters using the model's resolved compatibility dialect.
func buildParams(
	model llm.Model,
	messages []oai.ChatCompletionMessageParamUnion,
	tools []oai.ChatCompletionToolUnionParam,
	options llm.StreamOptions,
	compat resolvedCompat,
) oai.ChatCompletionNewParams {
	params := oai.ChatCompletionNewParams{
		Model:    model.ID,
		Messages: messages,
		StreamOptions: oai.ChatCompletionStreamOptionsParam{
			IncludeUsage: oai.Bool(true),
		},
	}
	if len(tools) > 0 {
		params.Tools = tools
	}
	if options.MaxTokens > 0 {
		if compat.maxTokensField == "max_tokens" {
			params.MaxTokens = oai.Int(options.MaxTokens)
		} else {
			params.MaxCompletionTokens = oai.Int(options.MaxTokens)
		}
	}
	if options.Temperature != nil {
		params.Temperature = oai.Float(*options.Temperature)
	}
	if compat.supportsStore {
		params.Store = oai.Bool(false)
	}
	applyThinking(&params, model, compat, resolveEffort(model, options.Reasoning))
	return params
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
