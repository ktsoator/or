package chatcompletions

import (
	"maps"

	"github.com/ktsoator/or/llm"
	oai "github.com/openai/openai-go/v3"
)

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
	if openAIOptions, ok := options.ProtocolOptions.(*llm.OpenAICompletionsStreamOptions); ok && openAIOptions != nil {
		applyToolChoice(&params, openAIOptions.ToolChoice)
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
	applyThinking(&params, model, compat, resolveThinking(model, options.Reasoning))
	if len(tools) > 0 && compat.zaiToolStream {
		mergeExtraFields(&params, map[string]any{"tool_stream": true})
	}
	return params
}

// mergeExtraFields preserves provider-specific fields already installed by
// applyThinking. The SDK's SetExtraFields replaces rather than merges its map.
func mergeExtraFields(params *oai.ChatCompletionNewParams, fields map[string]any) {
	merged := maps.Clone(params.ExtraFields())
	if merged == nil {
		merged = make(map[string]any, len(fields))
	}
	maps.Copy(merged, fields)
	params.SetExtraFields(merged)
}
