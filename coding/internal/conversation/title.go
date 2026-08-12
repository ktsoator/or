package conversation

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// A custom title takes priority over the automatic title. The automatic title
// starts as a prompt-derived fallback and is replaced when AI generation succeeds.

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

func clampTitle(title string) string {
	title = strings.TrimSpace(title)
	if utf8.RuneCountInString(title) <= MaxTitleRunes {
		return title
	}
	return strings.TrimSpace(string([]rune(title)[:MaxTitleRunes]))
}

func (s *sessionRuntime) displayTitle() string {
	if s.record.CustomTitle != "" {
		return s.record.CustomTitle
	}
	return s.record.Title
}

func (s *sessionRuntime) titleChanged() TitleChanged {
	return TitleChanged{Title: s.displayTitle()}
}

// nextForkTitleLocked returns a durable display title that distinguishes a
// new branch from every existing session. The caller must hold m.mu.
func (m *Manager) nextForkTitleLocked(sourceTitle string) string {
	used := make(map[string]struct{}, len(m.sessions))
	for _, runtime := range m.sessions {
		used[runtime.displayTitle()] = struct{}{}
	}

	sourceTitle = strings.TrimSpace(sourceTitle)
	if sourceTitle == "" {
		sourceTitle = defaultTitle
	}
	for number := 1; ; number++ {
		suffix := " (branch)"
		if number > 1 {
			suffix = fmt.Sprintf(" (branch %d)", number)
		}
		candidate := titleWithSuffix(sourceTitle, suffix)
		if _, exists := used[candidate]; !exists {
			return candidate
		}
	}
}

func titleWithSuffix(title, suffix string) string {
	maxTitleRunes := MaxTitleRunes - utf8.RuneCountInString(suffix)
	runes := []rune(strings.TrimSpace(title))
	if len(runes) > maxTitleRunes {
		runes = runes[:maxTitleRunes]
	}
	return strings.TrimSpace(string(runes)) + suffix
}
