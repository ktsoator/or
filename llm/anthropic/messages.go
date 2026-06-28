package anthropic

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/ktsoator/or/llm"
)

// compat holds the resolved Anthropic-compatible quirks the adapter consumes.
type compat struct {
	supportsTemperature       bool
	supportsCacheControl      bool
	supportsCacheControlTools bool
	allowEmptySignature       bool
	forceAdaptiveThinking     bool
}

// resolveCompat reads the model's optional Anthropic compatibility overrides.
// Anthropic-compatible vendors usually need none; sensible defaults apply.
//
// Prompt caching defaults on only for real Anthropic; other Anthropic-compatible
// vendors must opt in explicitly via compatibility, because some reject
// cache_control. Explicit overrides always win.
func resolveCompat(model llm.Model) compat {
	isAnthropic := model.Provider == "anthropic" || strings.Contains(model.BaseURL, "api.anthropic.com")
	resolved := compat{
		supportsTemperature:       true,
		supportsCacheControl:      isAnthropic,
		supportsCacheControlTools: isAnthropic,
	}

	override, ok := model.Compatibility.(*llm.AnthropicMessagesCompatibility)
	if !ok || override == nil {
		return resolved
	}
	if override.SupportsTemperature != nil {
		resolved.supportsTemperature = *override.SupportsTemperature
	}
	if override.SupportsCacheControl != nil {
		resolved.supportsCacheControl = *override.SupportsCacheControl
	}
	if override.SupportsCacheControlTools != nil {
		resolved.supportsCacheControlTools = *override.SupportsCacheControlTools
	}
	if override.AllowEmptySignature != nil {
		resolved.allowEmptySignature = *override.AllowEmptySignature
	}
	if override.ForceAdaptiveThinking != nil {
		resolved.forceAdaptiveThinking = *override.ForceAdaptiveThinking
	}
	return resolved
}

// applyCacheControl marks ephemeral prompt-cache breakpoints, mirroring pi: the
// system prompt, the final tool, and the last message's final content block.
// Anthropic caches the prefix up to each breakpoint, so later turns reuse the
// cached system, tools, and conversation history.
func applyCacheControl(params *sdk.MessageNewParams, compat compat) {
	if !compat.supportsCacheControl {
		return
	}
	ephemeral := sdk.NewCacheControlEphemeralParam()

	if n := len(params.System); n > 0 {
		params.System[n-1].CacheControl = ephemeral
	}
	if n := len(params.Messages); n > 0 {
		blocks := params.Messages[n-1].Content
		if b := len(blocks); b > 0 {
			if cacheControl := blocks[b-1].GetCacheControl(); cacheControl != nil {
				*cacheControl = ephemeral
			}
		}
	}
	if compat.supportsCacheControlTools {
		if n := len(params.Tools); n > 0 {
			if cacheControl := params.Tools[n-1].GetCacheControl(); cacheControl != nil {
				*cacheControl = ephemeral
			}
		}
	}
}

// convertMessages prepares the transcript for the target model and translates it
// into Anthropic message params. Consecutive tool results are merged into one
// user message, as the Messages API expects.
func convertMessages(input llm.Context, model llm.Model, compat compat) ([]sdk.MessageParam, error) {
	transformed := llm.TransformMessages(input.Messages, model, normalizeToolCallID)
	messages := make([]sdk.MessageParam, 0, len(transformed))

	for i := 0; i < len(transformed); i++ {
		switch message := transformed[i].(type) {
		case *llm.UserMessage:
			blocks, err := userBlocks(message)
			if err != nil {
				return nil, err
			}
			if len(blocks) > 0 {
				messages = append(messages, sdk.NewUserMessage(blocks...))
			}
		case *llm.AssistantMessage:
			blocks, err := assistantBlocks(message, compat)
			if err != nil {
				return nil, err
			}
			if len(blocks) > 0 {
				messages = append(messages, sdk.NewAssistantMessage(blocks...))
			}
		case *llm.ToolResultMessage:
			var results []sdk.ContentBlockParamUnion
			for ; i < len(transformed); i++ {
				toolResult, ok := transformed[i].(*llm.ToolResultMessage)
				if !ok {
					break
				}
				block, err := toolResultBlock(toolResult)
				if err != nil {
					return nil, err
				}
				results = append(results, block)
			}
			i--
			if len(results) > 0 {
				messages = append(messages, sdk.NewUserMessage(results...))
			}
		default:
			return nil, fmt.Errorf("unsupported message type %T", transformed[i])
		}
	}

	return messages, nil
}

func userBlocks(message *llm.UserMessage) ([]sdk.ContentBlockParamUnion, error) {
	blocks := make([]sdk.ContentBlockParamUnion, 0, len(message.Content))
	for _, rawContent := range message.Content {
		switch content := rawContent.(type) {
		case *llm.TextContent:
			if content == nil || strings.TrimSpace(content.Text) == "" {
				continue
			}
			blocks = append(blocks, sdk.NewTextBlock(content.Text))
		case *llm.ImageContent:
			if content == nil {
				return nil, errors.New("user image content is nil")
			}
			if content.MIMEType == "" || content.Data == "" {
				return nil, errors.New("user image content is missing MIME type or data")
			}
			blocks = append(blocks, sdk.NewImageBlockBase64(content.MIMEType, content.Data))
		default:
			return nil, fmt.Errorf("unsupported user content type %T", rawContent)
		}
	}
	return blocks, nil
}

// assistantBlocks replays a prior assistant turn. Reasoning is preserved with its
// signature; redacted reasoning is passed back as its opaque payload; unsigned
// reasoning is sent as plain text unless the provider accepts empty signatures.
func assistantBlocks(message *llm.AssistantMessage, compat compat) ([]sdk.ContentBlockParamUnion, error) {
	blocks := make([]sdk.ContentBlockParamUnion, 0, len(message.Content))
	for _, rawContent := range message.Content {
		switch content := rawContent.(type) {
		case *llm.TextContent:
			if content == nil || strings.TrimSpace(content.Text) == "" {
				continue
			}
			blocks = append(blocks, sdk.NewTextBlock(content.Text))
		case *llm.ThinkingContent:
			if content == nil {
				return nil, errors.New("assistant thinking content is nil")
			}
			if content.Redacted {
				blocks = append(blocks, sdk.NewRedactedThinkingBlock(content.ThinkingSignature))
				continue
			}
			if strings.TrimSpace(content.Thinking) == "" {
				continue
			}
			if content.ThinkingSignature == "" {
				if compat.allowEmptySignature {
					blocks = append(blocks, sdk.NewThinkingBlock("", content.Thinking))
				} else {
					blocks = append(blocks, sdk.NewTextBlock(content.Thinking))
				}
				continue
			}
			blocks = append(blocks, sdk.NewThinkingBlock(content.ThinkingSignature, content.Thinking))
		case *llm.ToolCall:
			if content == nil {
				return nil, errors.New("assistant tool call content is nil")
			}
			input := any(content.Arguments)
			if content.Arguments == nil {
				input = map[string]any{}
			}
			blocks = append(blocks, sdk.NewToolUseBlock(content.ID, input, content.Name))
		default:
			return nil, fmt.Errorf("unsupported assistant content type %T", rawContent)
		}
	}
	return blocks, nil
}

// toolResultBlock converts a tool result, forwarding both text and image parts.
func toolResultBlock(message *llm.ToolResultMessage) (sdk.ContentBlockParamUnion, error) {
	var content []sdk.ToolResultBlockParamContentUnion
	for _, rawContent := range message.Content {
		switch item := rawContent.(type) {
		case *llm.TextContent:
			if item == nil {
				continue
			}
			content = append(content, sdk.ToolResultBlockParamContentUnion{
				OfText: &sdk.TextBlockParam{Text: item.Text},
			})
		case *llm.ImageContent:
			if item == nil {
				continue
			}
			if item.MIMEType == "" || item.Data == "" {
				return sdk.ContentBlockParamUnion{}, errors.New("tool result image content is missing MIME type or data")
			}
			content = append(content, sdk.ToolResultBlockParamContentUnion{
				OfImage: &sdk.ImageBlockParam{
					Source: sdk.ImageBlockParamSourceUnion{
						OfBase64: &sdk.Base64ImageSourceParam{
							Data:      item.Data,
							MediaType: sdk.Base64ImageSourceMediaType(item.MIMEType),
						},
					},
				},
			})
		default:
			return sdk.ContentBlockParamUnion{}, fmt.Errorf("unsupported tool result content type %T", rawContent)
		}
	}

	block := sdk.ToolResultBlockParam{
		ToolUseID: message.ToolCallID,
		Content:   content,
		IsError:   sdk.Bool(message.IsError),
	}
	return sdk.ContentBlockParamUnion{OfToolResult: &block}, nil
}

// convertTools maps tool definitions to Anthropic tool params.
func convertTools(tools []llm.ToolDefinition) ([]sdk.ToolUnionParam, error) {
	if len(tools) == 0 {
		return nil, nil
	}

	converted := make([]sdk.ToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		if tool.Name == "" {
			return nil, errors.New("tool definition is missing a name")
		}

		schema := sdk.ToolInputSchemaParam{}
		if len(tool.Parameters) > 0 {
			var parsed struct {
				Properties any      `json:"properties"`
				Required   []string `json:"required"`
			}
			if err := json.Unmarshal(tool.Parameters, &parsed); err != nil {
				return nil, fmt.Errorf("decode parameters for tool %q: %w", tool.Name, err)
			}
			schema.Properties = parsed.Properties
			schema.Required = parsed.Required
		}

		toolParam := sdk.ToolParam{Name: tool.Name, InputSchema: schema}
		if tool.Description != "" {
			toolParam.Description = sdk.String(tool.Description)
		}
		converted = append(converted, sdk.ToolUnionParam{OfTool: &toolParam})
	}
	return converted, nil
}

// normalizeToolCallID rewrites a tool-call ID to Anthropic's accepted shape when
// an assistant turn from another model is replayed: characters outside
// [a-zA-Z0-9_-] become underscores and the result is capped at 64 bytes.
func normalizeToolCallID(id string) string {
	sanitized := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			return r
		default:
			return '_'
		}
	}, id)
	if len(sanitized) > 64 {
		return sanitized[:64]
	}
	return sanitized
}
