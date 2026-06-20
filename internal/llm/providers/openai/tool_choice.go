package openai

import (
	"github.com/ktsoator/or/internal/llm"
	oai "github.com/openai/openai-go/v3"
)

// applyToolChoice maps the public OpenAI-native union onto the SDK union.
func applyToolChoice(params *oai.ChatCompletionNewParams, choice llm.OpenAIToolChoice) {
	switch typed := choice.(type) {
	case llm.OpenAIToolChoiceMode:
		switch typed {
		case llm.OpenAIToolChoiceAuto, llm.OpenAIToolChoiceNone, llm.OpenAIToolChoiceRequired:
			params.ToolChoice = oai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: oai.String(string(typed))}
		}
	case llm.OpenAIToolChoiceFunction:
		params.ToolChoice = namedToolChoice(typed.Name)
	case *llm.OpenAIToolChoiceFunction:
		if typed != nil {
			params.ToolChoice = namedToolChoice(typed.Name)
		}
	}
}

func namedToolChoice(name string) oai.ChatCompletionToolChoiceOptionUnionParam {
	return oai.ToolChoiceOptionFunctionToolChoice(
		oai.ChatCompletionNamedToolChoiceFunctionParam{Name: name},
	)
}
