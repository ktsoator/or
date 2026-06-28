package anthropic

import (
	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/ktsoator/or/llm"
)

// applyToolChoice maps the public Anthropic-native union onto the SDK union.
func applyToolChoice(params *sdk.MessageNewParams, choice llm.AnthropicToolChoice) {
	switch typed := choice.(type) {
	case llm.AnthropicToolChoiceMode:
		switch typed {
		case llm.AnthropicToolChoiceAuto:
			params.ToolChoice = sdk.ToolChoiceUnionParam{OfAuto: &sdk.ToolChoiceAutoParam{}}
		case llm.AnthropicToolChoiceAny:
			params.ToolChoice = sdk.ToolChoiceUnionParam{OfAny: &sdk.ToolChoiceAnyParam{}}
		case llm.AnthropicToolChoiceNone:
			none := sdk.NewToolChoiceNoneParam()
			params.ToolChoice = sdk.ToolChoiceUnionParam{OfNone: &none}
		}
	case llm.AnthropicToolChoiceTool:
		params.ToolChoice = sdk.ToolChoiceParamOfTool(typed.Name)
	case *llm.AnthropicToolChoiceTool:
		if typed != nil {
			params.ToolChoice = sdk.ToolChoiceParamOfTool(typed.Name)
		}
	}
}
