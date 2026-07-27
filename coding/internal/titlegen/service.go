// Package titlegen generates short product-facing session titles through the
// independently configured utility-model route.
package titlegen

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ktsoator/or/coding/internal/provider"
	"github.com/ktsoator/or/llm"
)

const (
	CodeUnavailable   = "utility_model_unavailable"
	CodeTimeout       = "title_generation_timeout"
	CodeRequestFailed = "title_request_failed"
	CodeInvalidOutput = "invalid_title_output"

	generationTimeout = 15 * time.Second
	maxOutputTokens   = 128
	maxTitleRunes     = 120
)

type Result struct {
	Title    string
	Provider string
	Model    string
}

type Generator interface {
	Generate(context.Context, string) (Result, error)
}

type Service struct {
	providers *provider.Store
	complete  func(context.Context, llm.Model, llm.Context, llm.StreamOptions) (llm.AssistantMessage, error)
}

func New(providers *provider.Store) *Service {
	return &Service{providers: providers, complete: llm.Complete}
}

func (s *Service) Generate(ctx context.Context, prompt string) (Result, error) {
	if s == nil || s.providers == nil {
		return Result{}, failure(CodeUnavailable, provider.ErrUtilityModelUnavailable)
	}
	route, err := s.providers.ResolveUtilityModel()
	if err != nil {
		return Result{}, failure(CodeUnavailable, err)
	}
	result := Result{Provider: route.Route.Provider, Model: route.Route.Model}

	titleCtx, cancel := context.WithTimeout(ctx, generationTimeout)
	defer cancel()
	options := route.Options
	options.MaxTokens = maxOutputTokens
	options.Reasoning = llm.ModelThinkingOff
	complete := s.complete
	if complete == nil {
		complete = llm.Complete
	}
	message, err := complete(
		titleCtx,
		route.Model,
		llm.PromptWithSystem(titleSystemPrompt, strings.TrimSpace(prompt)),
		options,
	)
	if err != nil {
		if errors.Is(titleCtx.Err(), context.DeadlineExceeded) {
			return result, failure(CodeTimeout, titleCtx.Err())
		}
		return result, failure(CodeRequestFailed, err)
	}
	result.Title = parseTitleJSON(message.Text())
	if result.Title == "" {
		return result, failure(CodeInvalidOutput, errors.New("model returned no valid title"))
	}
	return result, nil
}

type generationError struct {
	code string
	err  error
}

func (e *generationError) Error() string { return e.err.Error() }
func (e *generationError) Unwrap() error { return e.err }

func failure(code string, err error) error {
	return &generationError{code: code, err: err}
}

func ErrorCode(err error) string {
	var target *generationError
	if errors.As(err, &target) {
		return target.code
	}
	return CodeRequestFailed
}

func PublicError(code string) string {
	switch code {
	case CodeUnavailable:
		return "No configured utility model is available."
	case CodeTimeout:
		return "The utility model timed out while generating a title."
	case CodeInvalidOutput:
		return "The utility model returned no usable title."
	default:
		return "The utility model could not generate a title."
	}
}

const titleSystemPrompt = `Generate a concise, sentence-case title (3-7 words) that captures the main topic or goal of this coding session. The title should be clear enough that the user recognizes the session in a list. Use sentence case: capitalize only the first word and proper nouns. Return JSON with a single "title" field.

Good examples:
{"title": "Fix login button on mobile"}
{"title": "Add OAuth authentication"}
{"title": "Debug failing CI tests"}
{"title": "Refactor API client error handling"}

Bad (too vague): {"title": "Code changes"}
Bad (too long): {"title": "Investigate and fix the issue where the login button does not respond on mobile devices"}`

func parseTitleJSON(text string) string {
	var parsed struct {
		Title string `json:"title"`
	}
	if json.Unmarshal([]byte(text), &parsed) == nil {
		return clampTitle(parsed.Title)
	}
	if start := strings.Index(text, "{"); start >= 0 {
		if end := strings.LastIndex(text, "}"); end > start && json.Unmarshal([]byte(text[start:end+1]), &parsed) == nil {
			return clampTitle(parsed.Title)
		}
	}
	line := strings.Trim(strings.TrimSpace(text), `"'`)
	if strings.ContainsAny(line, "{}\n\r") || utf8.RuneCountInString(line) > maxTitleRunes {
		return ""
	}
	return clampTitle(line)
}

func clampTitle(title string) string {
	title = strings.TrimSpace(title)
	if utf8.RuneCountInString(title) <= maxTitleRunes {
		return title
	}
	return strings.TrimSpace(string([]rune(title)[:maxTitleRunes]))
}
