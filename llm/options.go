package llm

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

// ProviderEnv contains request-scoped environment overrides. Non-empty values
// take precedence over process environment variables during credential lookup.
type ProviderEnv map[string]string

// ProtocolStreamOptions is the extension point for settings whose semantics
// cannot be shared across protocols. Custom protocol adapters may provide their
// own implementation and validate it before the stream starts.
type ProtocolStreamOptions interface {
	Protocol() Protocol
	Validate(tools []ToolDefinition) error
}

// AnthropicToolChoice is the native Anthropic tool_choice union. Use one of the
// AnthropicToolChoice* mode constants or AnthropicToolChoiceTool.
type AnthropicToolChoice interface {
	isAnthropicToolChoice()
}

// AnthropicToolChoiceMode is one of Anthropic's string tool-choice modes.
type AnthropicToolChoiceMode string

const (
	AnthropicToolChoiceAuto AnthropicToolChoiceMode = "auto"
	AnthropicToolChoiceAny  AnthropicToolChoiceMode = "any"
	AnthropicToolChoiceNone AnthropicToolChoiceMode = "none"
)

func (AnthropicToolChoiceMode) isAnthropicToolChoice() {}

// AnthropicToolChoiceTool forces the model to call Name.
type AnthropicToolChoiceTool struct {
	Name string
}

func (AnthropicToolChoiceTool) isAnthropicToolChoice() {}

// AnthropicStreamOptions contains settings understood only by the Anthropic
// Messages protocol. Keeping them nested prevents provider-specific knobs from
// flattening the shared StreamOptions namespace.
type AnthropicStreamOptions struct {
	// ThinkingDisplay controls how a reasoning model returns its thinking. Empty
	// defaults to summarized.
	ThinkingDisplay ThinkingDisplay
	// ToolChoice controls Anthropic's native tool selection behavior.
	ToolChoice AnthropicToolChoice
}

// Protocol identifies the protocol that accepts these options.
func (*AnthropicStreamOptions) Protocol() Protocol {
	return ProtocolAnthropicMessages
}

// Validate checks the Anthropic-specific settings against the available tools.
func (options *AnthropicStreamOptions) Validate(tools []ToolDefinition) error {
	if options == nil {
		return errors.New("Anthropic stream options are nil")
	}
	switch options.ThinkingDisplay {
	case "", ThinkingDisplaySummarized, ThinkingDisplayOmitted:
	default:
		return fmt.Errorf("unsupported Anthropic thinking display %q", options.ThinkingDisplay)
	}
	return validateAnthropicToolChoice(options.ToolChoice, tools)
}

// OpenAIToolChoice is the native OpenAI Chat Completions tool_choice union. Use
// one of the OpenAIToolChoice* mode constants or OpenAIToolChoiceFunction.
type OpenAIToolChoice interface {
	isOpenAIToolChoice()
}

// OpenAIToolChoiceMode is one of OpenAI's string tool-choice modes.
type OpenAIToolChoiceMode string

const (
	OpenAIToolChoiceAuto     OpenAIToolChoiceMode = "auto"
	OpenAIToolChoiceNone     OpenAIToolChoiceMode = "none"
	OpenAIToolChoiceRequired OpenAIToolChoiceMode = "required"
)

func (OpenAIToolChoiceMode) isOpenAIToolChoice() {}

// OpenAIToolChoiceFunction forces the model to call the named function tool.
type OpenAIToolChoiceFunction struct {
	Name string
}

func (OpenAIToolChoiceFunction) isOpenAIToolChoice() {}

// OpenAICompletionsStreamOptions contains settings understood only by the
// OpenAI-compatible Chat Completions protocol.
type OpenAICompletionsStreamOptions struct {
	ToolChoice OpenAIToolChoice
}

// Protocol identifies the protocol that accepts these options.
func (*OpenAICompletionsStreamOptions) Protocol() Protocol {
	return ProtocolOpenAICompletions
}

// Validate checks the OpenAI-specific settings against the available tools.
func (options *OpenAICompletionsStreamOptions) Validate(tools []ToolDefinition) error {
	if options == nil {
		return errors.New("OpenAI Completions stream options are nil")
	}
	return validateOpenAIToolChoice(options.ToolChoice, tools)
}

// StreamOptions contains shared request settings plus optional protocol-specific
// extensions. A non-nil extension must match the target model protocol.
type StreamOptions struct {
	APIKey string
	Env    ProviderEnv
	// Temperature overrides the model's default sampling temperature when set.
	Temperature *float64
	// MaxTokens caps the output tokens for this request. Zero leaves it unset.
	MaxTokens int64
	// Headers are merged into the request, overriding model default headers.
	Headers map[string]string
	// Reasoning requests a thinking level. The provider clamps it to what the
	// model supports. Empty leaves the model's default; "off" disables thinking.
	Reasoning ModelThinkingLevel
	// ProtocolOptions carries settings specific to exactly one protocol.
	ProtocolOptions ProtocolStreamOptions
	// MaxRetries overrides the SDK client-side retry count for transient failures
	// (HTTP 429 and 5xx, connection errors). Nil leaves the SDK default; a value
	// of 0 disables retries so the caller can manage them.
	MaxRetries *int
	// Timeout caps the total duration of one HTTP attempt. Zero leaves the SDK
	// default. It is independent of the request context, which still cancels the
	// whole call.
	Timeout time.Duration
	// OnResponse, when set, is called with the status and headers of every HTTP
	// response before its body is consumed. It fires once per attempt, so a
	// retried request invokes it for each try, making retries observable.
	OnResponse func(status int, headers http.Header)
	// OnRequest, when set, is called with the method, URL, and full body of every
	// HTTP request before it is sent. The body is the exact JSON serialized for
	// the provider. It fires once per attempt, so a retried request invokes it
	// for each try, making retries observable.
	OnRequest func(method, url string, body []byte)
	// RewriteRequest, when set, transforms the serialized request body before it
	// is sent. It receives the method, URL, and body, and returns the body to
	// send; returning nil leaves the body unchanged. Use it to patch
	// provider-specific fields the typed API does not expose. It fires once per
	// attempt and always rewrites the original body, so a retried request is
	// rewritten consistently.
	RewriteRequest func(method, url string, body []byte) []byte
}

// Validate checks that explicitly supplied protocol extensions match the target
// protocol and contain supported values.
func (options StreamOptions) Validate(protocol Protocol, tools []ToolDefinition) error {
	if options.ProtocolOptions == nil {
		return nil
	}
	if optionProtocol := options.ProtocolOptions.Protocol(); optionProtocol != protocol {
		return fmt.Errorf(
			"stream options for protocol %q are unsupported by protocol %q",
			optionProtocol,
			protocol,
		)
	}
	return options.ProtocolOptions.Validate(tools)
}

func validateAnthropicToolChoice(choice AnthropicToolChoice, tools []ToolDefinition) error {
	if choice == nil {
		return nil
	}
	if len(tools) == 0 {
		return errors.New("Anthropic tool choice requires at least one tool")
	}

	switch typed := choice.(type) {
	case AnthropicToolChoiceMode:
		switch typed {
		case AnthropicToolChoiceAuto, AnthropicToolChoiceAny, AnthropicToolChoiceNone:
			return nil
		default:
			return fmt.Errorf("unsupported Anthropic tool choice %q", typed)
		}
	case AnthropicToolChoiceTool:
		return validateNamedToolChoice("Anthropic", typed.Name, tools)
	case *AnthropicToolChoiceTool:
		if typed == nil {
			return errors.New("Anthropic named tool choice is nil")
		}
		return validateNamedToolChoice("Anthropic", typed.Name, tools)
	default:
		return fmt.Errorf("unsupported Anthropic tool choice type %T", choice)
	}
}

func validateOpenAIToolChoice(choice OpenAIToolChoice, tools []ToolDefinition) error {
	if choice == nil {
		return nil
	}
	if len(tools) == 0 {
		return errors.New("OpenAI tool choice requires at least one tool")
	}

	switch typed := choice.(type) {
	case OpenAIToolChoiceMode:
		switch typed {
		case OpenAIToolChoiceAuto, OpenAIToolChoiceNone, OpenAIToolChoiceRequired:
			return nil
		default:
			return fmt.Errorf("unsupported OpenAI tool choice %q", typed)
		}
	case OpenAIToolChoiceFunction:
		return validateNamedToolChoice("OpenAI", typed.Name, tools)
	case *OpenAIToolChoiceFunction:
		if typed == nil {
			return errors.New("OpenAI named tool choice is nil")
		}
		return validateNamedToolChoice("OpenAI", typed.Name, tools)
	default:
		return fmt.Errorf("unsupported OpenAI tool choice type %T", choice)
	}
}

func validateNamedToolChoice(provider, name string, tools []ToolDefinition) error {
	if name == "" {
		return fmt.Errorf("%s named tool choice has an empty name", provider)
	}
	for _, tool := range tools {
		if tool.Name == name {
			return nil
		}
	}
	return fmt.Errorf("%s tool choice names unknown tool %q", provider, name)
}
