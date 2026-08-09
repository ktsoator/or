package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/llm"
)

const (
	titleGenerationTimeout = 15 * time.Second
	titleMaxOutputTokens   = 128
)

type titleGenerator func(context.Context, llm.Model, string) (string, error)

const titleSystemPrompt = `Generate a concise, sentence-case title (3-7 words) that captures the main topic or goal of this coding session. The title should be clear enough that the user recognizes the session in a list. Use sentence case: capitalize only the first word and proper nouns. Return JSON with a single "title" field.

Good examples:
{"title": "Fix login button on mobile"}
{"title": "Add OAuth authentication"}
{"title": "Debug failing CI tests"}
{"title": "Refactor API client error handling"}

Bad (too vague): {"title": "Code changes"}
Bad (too long): {"title": "Investigate and fix the issue where the login button does not respond on mobile devices"}`

func generateAITitle(ctx context.Context, model llm.Model, prompt string) (string, error) {
	return generateAITitleWith(ctx, model, prompt, llm.Complete)
}

func generateAITitleWith(
	ctx context.Context,
	model llm.Model,
	prompt string,
	complete func(context.Context, llm.Model, llm.Context, llm.StreamOptions) (llm.AssistantMessage, error),
) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, titleGenerationTimeout)
	defer cancel()
	message, err := complete(
		ctx,
		model,
		llm.PromptWithSystem(titleSystemPrompt, strings.TrimSpace(prompt)),
		llm.StreamOptions{
			MaxTokens: titleMaxOutputTokens,
			Reasoning: llm.ModelThinkingOff,
		},
	)
	if err != nil {
		return "", err
	}
	title := parseAITitle(message.Text())
	if title == "" {
		return "", errors.New("model returned no valid title")
	}
	return title, nil
}

func parseAITitle(text string) string {
	var parsed struct {
		Title string `json:"title"`
	}
	if json.Unmarshal([]byte(text), &parsed) == nil {
		return clampTitle(parsed.Title)
	}
	if start := strings.Index(text, "{"); start >= 0 {
		if end := strings.LastIndex(text, "}"); end > start &&
			json.Unmarshal([]byte(text[start:end+1]), &parsed) == nil {
			return clampTitle(parsed.Title)
		}
	}
	line := strings.Trim(strings.TrimSpace(text), `"'`)
	if strings.ContainsAny(line, "{}\n\r") || utf8.RuneCountInString(line) > MaxTitleRunes {
		return ""
	}
	return clampTitle(line)
}

// maybeGenerateTitle starts background generation as soon as a real user
// message is durable. A failed attempt clears the in-flight flag so a later
// user message can retry without blocking the main assistant response.
func (m *Manager) maybeGenerateTitle(runtime *sessionRuntime, eventPrompt string) {
	firstPrompt := strings.TrimSpace(eventPrompt)
	for _, item := range runtime.session.History() {
		if item.Type == engine.HistoryUser && strings.TrimSpace(item.Text) != "" {
			firstPrompt = strings.TrimSpace(item.Text)
			break
		}
	}
	if firstPrompt == "" {
		return
	}

	m.mu.Lock()
	needsTitle := !m.closed && runtime.record.CustomTitle == "" && runtime.record.GenerateTitle
	model, modelFound := llm.LookupModel(runtime.record.Provider, runtime.record.Model)
	generator := m.generateTitle
	if !needsTitle || !modelFound || generator == nil ||
		!runtime.titleGenerating.CompareAndSwap(false, true) {
		m.mu.Unlock()
		return
	}
	sessionID := runtime.record.ID
	m.tasks.Add(1)
	m.mu.Unlock()

	go func() {
		defer func() {
			runtime.titleGenerating.Store(false)
			m.tasks.Done()
			m.ReleaseIfIdle(sessionID)
		}()
		if err := m.generateSessionTitle(m.ctx, runtime, model, firstPrompt, generator); err != nil &&
			!errors.Is(err, context.Canceled) {
			log.Printf("coding: generate title for session %s: %v", sessionID, err)
		}
	}()
}

func (m *Manager) generateSessionTitle(
	ctx context.Context,
	runtime *sessionRuntime,
	model llm.Model,
	firstPrompt string,
	generator titleGenerator,
) error {
	title, err := generator(ctx, model, firstPrompt)
	if err != nil {
		return err
	}
	title = clampTitle(title)
	if title == "" {
		return errors.New("model returned an invalid title")
	}

	m.mu.Lock()
	current, exists := m.sessions[runtime.record.ID]
	if m.closed || !exists || current != runtime || runtime.record.CustomTitle != "" ||
		!runtime.record.GenerateTitle {
		m.mu.Unlock()
		return nil
	}
	previousTitle := runtime.record.Title
	runtime.record.Title = title
	runtime.record.GenerateTitle = false
	if err := m.saveLocked(); err != nil {
		runtime.record.Title = previousTitle
		runtime.record.GenerateTitle = true
		m.mu.Unlock()
		return err
	}
	titleEvent := runtime.titleChanged()
	m.mu.Unlock()
	runtime.emit(titleEvent)
	return nil
}
