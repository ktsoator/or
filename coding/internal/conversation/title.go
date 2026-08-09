package conversation

import (
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
