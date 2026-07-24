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

// Session titles have three sources, in priority order: a name the user typed,
// a name a model generated from the first prompt, and a truncation of that
// prompt. This file owns all three plus the generation that produces the
// middle one.

type titleGenerator func(context.Context, llm.Model, string) (string, error)

func titleFromPrompt(prompt string) string {
	title := strings.Join(strings.Fields(prompt), " ")
	if title == "" {
		return defaultTitle
	}
	const maxRunes = 42
	if utf8.RuneCountInString(title) <= maxRunes {
		return title
	}
	runes := []rune(title)
	return strings.TrimSpace(string(runes[:maxRunes])) + "…"
}

// clampTitle trims a title and caps it at MaxTitleRunes so a long model or
// client-supplied string cannot bloat the session transcript.
func clampTitle(title string) string {
	title = strings.TrimSpace(title)
	if utf8.RuneCountInString(title) <= MaxTitleRunes {
		return title
	}
	runes := []rune(title)
	return strings.TrimSpace(string(runes[:MaxTitleRunes]))
}

// displayTitle returns the best available title for this session. Callers must
// hold Manager.mu.
func (s *sessionRuntime) displayTitle() string {
	if s.record.CustomTitle != "" {
		return s.record.CustomTitle
	}
	if s.record.AITitle != "" {
		return s.record.AITitle
	}
	return s.record.Title
}

// titleChanged snapshots the current title event. Callers hold Manager.mu so
// the record cannot change while the payload is built.
func (s *sessionRuntime) titleChanged() TitleChanged {
	return TitleChanged{
		Title:       s.displayTitle(),
		AITitle:     s.record.AITitle,
		CustomTitle: s.record.CustomTitle,
	}
}

// maybeGenerateTitle starts background AI title generation as soon as a user
// message enters the session, unless the user has already named it or a title
// was generated earlier. The flag only guards against two generations running
// at once: a failed attempt clears it so the next user message retries,
// because a model error or an unparseable reply should not cost the session its
// title for the lifetime of the process. Runs on the session's event goroutine,
// so it must not block on the model call.
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
	needsTitle := !m.closed && runtime.record.CustomTitle == "" && runtime.record.AITitle == ""
	sessionID := runtime.record.ID
	provider, model := runtime.record.Provider, runtime.record.Model
	generate := m.generateTitle
	if !needsTitle || !runtime.titleGenerating.CompareAndSwap(false, true) {
		m.mu.Unlock()
		return
	}
	m.tasks.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.tasks.Done()
		defer runtime.titleGenerating.Store(false)
		if err := m.generateSessionTitle(m.ctx, runtime, provider, model, firstPrompt, generate); err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Printf("coding: generate title for session %s: %v", sessionID, err)
			}
		}
	}()
}

// generateSessionTitle asks the model for a concise session title derived from
// the first user message and stores it as the session's AI title. Failures are
// reported while the session keeps its prompt-derived title.
func (m *Manager) generateSessionTitle(
	ctx context.Context,
	runtime *sessionRuntime,
	provider, modelID, firstPrompt string,
	generate titleGenerator,
) error {
	model, ok := llm.LookupModel(provider, modelID)
	if !ok {
		return errors.New("selected model is no longer available")
	}
	titleCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	title, err := generate(titleCtx, model, firstPrompt)
	if err != nil {
		return err
	}
	title = clampTitle(title)
	if title == "" {
		return errors.New("model returned an invalid title")
	}

	m.mu.Lock()
	current, exists := m.sessions[runtime.record.ID]
	if m.closed || !exists || current != runtime || runtime.record.CustomTitle != "" {
		m.mu.Unlock()
		return nil
	}
	runtime.record.AITitle = title
	if err := m.saveLocked(); err != nil {
		runtime.record.AITitle = ""
		m.mu.Unlock()
		return err
	}
	event := runtime.titleChanged()
	m.mu.Unlock()
	runtime.emit(event)
	return nil
}

func defaultTitleGenerator(ctx context.Context, model llm.Model, firstPrompt string) (string, error) {
	systemPrompt := `Generate a concise, sentence-case title (3-7 words) that captures the main topic of the user's first message. Use sentence case: capitalize only the first word and proper nouns. Return JSON with a single "title" field.

Good examples:
{"title": "Fix login button on mobile"}
{"title": "Add OAuth authentication"}
{"title": "Debug failing CI tests"}

Bad (too vague): {"title": "Code changes"}
Bad (too long): {"title": "Investigate and fix the issue with the login flow"}`

	// Thinking is off: a title needs none, and reasoning tokens would consume
	// the output budget before any JSON is emitted.
	result, err := llm.Complete(ctx, model, llm.PromptWithSystem(systemPrompt, firstPrompt), llm.StreamOptions{
		MaxTokens: 128,
		Reasoning: llm.ModelThinkingOff,
	})
	if err != nil {
		return "", err
	}
	return parseTitleJSON(result.Text()), nil
}

// parseTitleJSON pulls the title out of a model response. It accepts the JSON
// object the prompt asks for, the same object wrapped in prose or a code fence,
// and — because smaller models often ignore the format — a bare one-line title.
func parseTitleJSON(text string) string {
	var parsed struct {
		Title string `json:"title"`
	}
	if json.Unmarshal([]byte(text), &parsed) == nil {
		return parsed.Title
	}
	if start, end := strings.Index(text, "{"), strings.LastIndex(text, "}"); start >= 0 && end > start {
		if json.Unmarshal([]byte(text[start:end+1]), &parsed) == nil {
			return parsed.Title
		}
	}
	// Bare title: a single short line with no JSON in sight. Anything longer or
	// multi-line is prose, not a title, so it is discarded.
	line := strings.TrimSpace(text)
	if strings.ContainsAny(line, "{}\n\r") || utf8.RuneCountInString(line) > MaxTitleRunes {
		return ""
	}
	return strings.Trim(line, `"'`)
}
