package conversation

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/titlegen"
)

// Session titles have three sources, in priority order: a name the user typed,
// a name the utility model generated, and a truncation of the first prompt.

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
	if s.record.AITitle != "" {
		return s.record.AITitle
	}
	return s.record.Title
}

func (s *sessionRuntime) titleChanged() TitleChanged {
	return TitleChanged{
		Title:       s.displayTitle(),
		AITitle:     s.record.AITitle,
		CustomTitle: s.record.CustomTitle,
	}
}

func (s *sessionRuntime) titleGenerationChanged() TitleGenerationChanged {
	return TitleGenerationChanged{Generation: s.titleGeneration}
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
	needsTitle := !m.closed && runtime.record.CustomTitle == "" && runtime.record.AITitle == ""
	if !needsTitle || !runtime.titleGenerating.CompareAndSwap(false, true) {
		m.mu.Unlock()
		return
	}
	sessionID := runtime.record.ID
	generator := m.generateTitle
	runtime.titleGeneration = TitleGeneration{
		Status:      TitleGenerationGenerating,
		AttemptedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	stateEvent := runtime.titleGenerationChanged()
	m.tasks.Add(1)
	m.mu.Unlock()
	runtime.emit(stateEvent)

	go func() {
		defer m.tasks.Done()
		defer runtime.titleGenerating.Store(false)
		if err := m.generateSessionTitle(m.ctx, runtime, firstPrompt, generator); err != nil &&
			!errors.Is(err, context.Canceled) {
			log.Printf("coding: generate title for session %s: %v", sessionID, err)
		}
	}()
}

func (m *Manager) generateSessionTitle(
	ctx context.Context,
	runtime *sessionRuntime,
	firstPrompt string,
	generator titlegen.Generator,
) error {
	if generator == nil {
		err := errors.New("utility title generator is not configured")
		m.recordTitleFailure(runtime, titlegen.Result{}, titlegen.CodeUnavailable)
		return err
	}
	result, err := generator.Generate(ctx, firstPrompt)
	if err != nil {
		m.recordTitleFailure(runtime, result, titlegen.ErrorCode(err))
		return err
	}
	title := clampTitle(result.Title)
	if title == "" {
		err = errors.New("model returned an invalid title")
		m.recordTitleFailure(runtime, result, titlegen.CodeInvalidOutput)
		return err
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
		runtime.titleGeneration = TitleGeneration{
			Status:      TitleGenerationFailed,
			Provider:    result.Provider,
			Model:       result.Model,
			ErrorCode:   "title_persist_failed",
			Error:       "The generated title could not be saved.",
			AttemptedAt: runtime.titleGeneration.AttemptedAt,
		}
		stateEvent := runtime.titleGenerationChanged()
		m.mu.Unlock()
		runtime.emit(stateEvent)
		return err
	}
	runtime.titleGeneration = TitleGeneration{
		Status:      TitleGenerationSucceeded,
		Provider:    result.Provider,
		Model:       result.Model,
		AttemptedAt: runtime.titleGeneration.AttemptedAt,
	}
	titleEvent := runtime.titleChanged()
	stateEvent := runtime.titleGenerationChanged()
	m.mu.Unlock()
	runtime.emit(titleEvent)
	runtime.emit(stateEvent)
	return nil
}

func (m *Manager) recordTitleFailure(runtime *sessionRuntime, result titlegen.Result, code string) {
	m.mu.Lock()
	current, exists := m.sessions[runtime.record.ID]
	if m.closed || !exists || current != runtime || runtime.record.CustomTitle != "" {
		m.mu.Unlock()
		return
	}
	status := TitleGenerationFailed
	if code == titlegen.CodeUnavailable {
		status = TitleGenerationUnavailable
	}
	runtime.titleGeneration = TitleGeneration{
		Status:      status,
		Provider:    result.Provider,
		Model:       result.Model,
		ErrorCode:   code,
		Error:       titlegen.PublicError(code),
		AttemptedAt: runtime.titleGeneration.AttemptedAt,
	}
	event := runtime.titleGenerationChanged()
	m.mu.Unlock()
	runtime.emit(event)
}
