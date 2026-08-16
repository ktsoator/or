package snapshot

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"
	"unicode"

	"github.com/ktsoator/or/llm"
)

var secretAssignments = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*[:=]\s*(?:bearer\s+)?)[^\s,"'}]+`),
	regexp.MustCompile(`(?i)((?:api[_-]?key|access[_-]?token|auth[_-]?token|password|secret)\s*[:=]\s*)[^\s,"'}]+`),
	regexp.MustCompile(`\b(?:sk|sk-ant|ghp|github_pat|xox[baprs])[-_][A-Za-z0-9_-]{8,}\b`),
}

func sanitizeInput(input llm.Context) Input {
	result := Input{
		SystemPrompt: sanitizeText(input.SystemPrompt),
		Messages:     make([]Message, 0, len(input.Messages)),
		Tools:        make([]Tool, 0, len(input.Tools)),
	}
	for _, message := range input.Messages {
		result.Messages = append(result.Messages, sanitizeMessage(message))
	}
	for _, tool := range input.Tools {
		result.Tools = append(result.Tools, Tool{
			Name: tool.Name, Description: sanitizeText(tool.Description),
			Parameters: sanitizeRawJSON(tool.Parameters), Strict: tool.Strict,
		})
	}
	return result
}

func sanitizeMessage(message llm.Message) Message {
	switch typed := message.(type) {
	case *llm.UserMessage:
		result := Message{Role: "user", Content: make([]Content, 0, len(typed.Content))}
		for _, content := range typed.Content {
			result.Content = append(result.Content, sanitizeContent(content))
		}
		return result
	case *llm.AssistantMessage:
		result := Message{
			Role: "assistant", ProviderRequestID: typed.ProviderRequestID,
			Content: make([]Content, 0, len(typed.Content)),
		}
		for _, content := range typed.Content {
			result.Content = append(result.Content, sanitizeContent(content))
		}
		return result
	case *llm.ToolResultMessage:
		result := Message{
			Role: "toolResult", ToolCallID: typed.ToolCallID,
			ToolName: typed.ToolName, IsError: typed.IsError,
			Content: make([]Content, 0, len(typed.Content)),
		}
		for _, content := range typed.Content {
			result.Content = append(result.Content, sanitizeContent(content))
		}
		return result
	default:
		return Message{Role: "unknown", Content: []Content{}}
	}
}

func sanitizeContent(content any) Content {
	switch typed := content.(type) {
	case *llm.TextContent:
		return Content{Type: "text", Text: sanitizeText(typed.Text)}
	case *llm.ThinkingContent:
		if typed.Redacted {
			return Content{Type: "thinking", Thinking: "[redacted reasoning omitted]", Redacted: true}
		}
		return Content{Type: "thinking", Thinking: sanitizeText(typed.Thinking)}
	case *llm.ImageContent:
		return Content{Type: "image", Image: &Image{
			MIMEType: typed.MIMEType, EncodedBytes: decodedBase64Size(typed.Data),
		}}
	case *llm.ToolCall:
		return Content{
			Type: "toolCall", ToolCallID: typed.ID, ToolName: typed.Name,
			Arguments: sanitizeMap(typed.Arguments),
		}
	default:
		return Content{Type: "unknown"}
	}
}

func decodedBase64Size(data string) int {
	if data == "" {
		return 0
	}
	size := base64.StdEncoding.DecodedLen(len(data))
	padding := 0
	if strings.HasSuffix(data, "==") {
		padding = 2
	} else if strings.HasSuffix(data, "=") {
		padding = 1
	}
	return max(0, size-padding)
}

func sanitizeMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	result := make(map[string]any, len(input))
	for key, value := range input {
		if sensitiveKey(key) {
			result[key] = "[redacted]"
			continue
		}
		result[key] = sanitizeValue(value)
	}
	return result
}

func sanitizeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return sanitizeMap(typed)
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = sanitizeValue(item)
		}
		return result
	case string:
		return sanitizeText(typed)
	default:
		return value
	}
}

func sanitizeRawJSON(input json.RawMessage) json.RawMessage {
	if len(input) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(input, &value); err != nil {
		return json.RawMessage(sanitizeText(string(input)))
	}
	encoded, err := json.Marshal(sanitizeValue(value))
	if err != nil {
		return json.RawMessage(`{"error":"schema unavailable"}`)
	}
	return encoded
}

func sanitizeText(value string) string {
	result := value
	for _, pattern := range secretAssignments {
		if pattern.NumSubexp() > 0 {
			result = pattern.ReplaceAllString(result, `${1}[redacted]`)
		} else {
			result = pattern.ReplaceAllString(result, "[redacted]")
		}
	}
	return result
}

func sensitiveKey(key string) bool {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, key)
	for _, fragment := range []string{
		"apikey", "authorization", "credential", "password", "secret", "cookie",
	} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	if normalized == "token" {
		return true
	}
	for _, tokenKey := range []string{"accesstoken", "authtoken", "bearertoken", "idtoken", "refreshtoken", "sessiontoken"} {
		if strings.HasSuffix(normalized, tokenKey) {
			return true
		}
	}
	return false
}
