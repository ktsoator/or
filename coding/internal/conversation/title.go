package conversation

import (
	"context"
	"encoding/json"
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
func (s *Runtime) displayTitle() string {
	if s.record.CustomTitle != "" {
		return s.record.CustomTitle
	}
	if s.record.AITitle != "" {
		return s.record.AITitle
	}
	return s.record.Title
}

// broadcastTitle sends the current title to connected clients. Callers must hold
// Manager.mu; emit never blocks, so holding it is cheap.
func (s *Runtime) broadcastTitle() {
	s.emit(TitleChanged{
		Title:       s.displayTitle(),
		AITitle:     s.record.AITitle,
		CustomTitle: s.record.CustomTitle,
	})
}

// maybeGenerateTitle starts background AI title generation after a session
// finishes a response, unless the user has already named it or a title was
// generated earlier. The flag only guards against two generations running at
// once: a failed attempt clears it so the next completed response retries,
// because a model error or an unparseable reply should not cost the session its
// title for the lifetime of the process. Runs on the session's event goroutine,
// so it must not block on the model call.
func (m *Manager) maybeGenerateTitle(runtime *Runtime) {
	m.mu.Lock()
	needsTitle := runtime.record.CustomTitle == "" && runtime.record.AITitle == ""
	provider, model := runtime.record.Provider, runtime.record.Model
	m.mu.Unlock()

	if !needsTitle || !runtime.titleGenerating.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer runtime.titleGenerating.Store(false)
		m.generateSessionTitle(m.ctx, runtime, provider, model)
	}()
}

// generateSessionTitle asks the model for a concise session title derived from
// the first user message and stores it as the session's AI title. Failures are
// silent: the session keeps its prompt-derived title.
func (m *Manager) generateSessionTitle(ctx context.Context, runtime *Runtime, provider, modelID string) {
	// Find the first user message with text content.
	history := runtime.session.History()
	var firstPrompt string
	for _, item := range history {
		if item.Type == engine.HistoryUser && strings.TrimSpace(item.Text) != "" {
			firstPrompt = strings.TrimSpace(item.Text)
			break
		}
	}
	if firstPrompt == "" {
		return
	}

	model, ok := llm.LookupModel(provider, modelID)
	if !ok {
		return
	}

	systemPrompt := `Generate a concise, sentence-case title (3-7 words) that captures the main topic of the user's first message. Use sentence case: capitalize only the first word and proper nouns. Return JSON with a single "title" field.

Good examples:
{"title": "Fix login button on mobile"}
{"title": "Add OAuth authentication"}
{"title": "Debug failing CI tests"}

Bad (too vague): {"title": "Code changes"}
Bad (too long): {"title": "Investigate and fix the issue with the login flow"}`

	titleCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Thinking is off: a title needs none, and reasoning tokens would consume
	// the output budget before any JSON is emitted.
	result, err := llm.Complete(titleCtx, model, llm.PromptWithSystem(systemPrompt, firstPrompt), llm.StreamOptions{
		MaxTokens: 128,
		Reasoning: llm.ModelThinkingOff,
	})
	if err != nil {
		return
	}

	title := clampTitle(parseTitleJSON(result.Text()))
	if title == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// Re-check under the lock: the user may have renamed while we generated.
	if runtime.record.CustomTitle != "" {
		return
	}
	runtime.record.AITitle = title
	if err := m.saveLocked(); err != nil {
		runtime.record.AITitle = ""
		return
	}
	runtime.broadcastTitle()
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
