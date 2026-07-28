package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ktsoator/or/llm"
)

const (
	connectionTestStandardTimeout  = 12 * time.Second
	connectionTestReasoningTimeout = 30 * time.Second
	connectionTestStandardTokens   = 64
	connectionTestPrompt           = "hi"
)

// ConnectionTestStatus is the user-actionable outcome of one provider probe.
type ConnectionTestStatus string

const (
	ConnectionTestSuccess              ConnectionTestStatus = "success"
	ConnectionTestAuthenticationFailed ConnectionTestStatus = "authentication_failed"
	ConnectionTestRateLimited          ConnectionTestStatus = "rate_limited"
	ConnectionTestTimeout              ConnectionTestStatus = "timeout"
	ConnectionTestUnreachable          ConnectionTestStatus = "unreachable"
	ConnectionTestNotFound             ConnectionTestStatus = "not_found"
	ConnectionTestProviderError        ConnectionTestStatus = "provider_error"
)

// ConnectionTestRequest describes an exact provider, endpoint, credential and
// model combination. APIKey and BaseURL may be unsaved values from the settings
// form. A blank APIKey resolves the saved key identified by ConnectionID/KeyID.
type ConnectionTestRequest struct {
	ProviderID    string
	ConnectionID  string
	KeyID         string
	BaseURL       string
	APIKey        string
	ModelID       string
	ThinkingLevel llm.ModelThinkingLevel
}

// ConnectionTestResult contains no secret or raw provider response. It exposes
// the fixed probe, model-visible thinking and assistant text separately so the
// user can verify the exchange shown in settings.
type ConnectionTestResult struct {
	Status         ConnectionTestStatus
	ModelID        string
	ModelName      string
	RequestText    string
	ThinkingLevel  llm.ModelThinkingLevel
	ThinkingText   string
	ResponseText   string
	StopReason     llm.StopReason
	InputTokens    int64
	OutputTokens   int64
	Latency        time.Duration
	ProviderStatus int
}

// CompleteFunc is the one-shot LLM boundary used by ConnectionTester.
type CompleteFunc func(context.Context, llm.Model, llm.Context, llm.StreamOptions) (llm.AssistantMessage, error)

// ConnectionTester probes draft or persisted connection settings without
// mutating the provider store or its shared registry overrides.
type ConnectionTester struct {
	store    *Store
	complete CompleteFunc
}

func NewConnectionTester(store *Store, complete CompleteFunc) *ConnectionTester {
	return &ConnectionTester{store: store, complete: complete}
}

func (t *ConnectionTester) Test(ctx context.Context, request ConnectionTestRequest) (ConnectionTestResult, error) {
	model, options, err := t.resolve(request)
	if err != nil {
		return ConnectionTestResult{}, err
	}
	if t.complete == nil {
		return ConnectionTestResult{}, errors.New("provider connection testing is unavailable")
	}

	thinkingLevel, err := resolveConnectionTestThinkingLevel(model, request.ThinkingLevel)
	if err != nil {
		return ConnectionTestResult{}, err
	}
	timeout, maxTokens := connectionTestLimits(thinkingLevel)
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var providerStatus atomic.Int64
	zeroRetries := 0
	options.MaxRetries = &zeroRetries
	options.MaxTokens = maxTokens
	if model.MaxTokens > 0 && model.MaxTokens < options.MaxTokens {
		options.MaxTokens = model.MaxTokens
	}
	options.Reasoning = thinkingLevel
	options.Timeout = timeout
	if model.Protocol == llm.ProtocolAnthropicMessages && thinkingLevel != llm.ModelThinkingOff {
		options.ProtocolOptions = &llm.AnthropicStreamOptions{
			ThinkingDisplay: llm.ThinkingDisplaySummarized,
		}
	}
	options.OnResponse = func(status int, _ http.Header) {
		providerStatus.Store(int64(status))
	}

	started := time.Now()
	message, testErr := t.complete(probeCtx, model, llm.Prompt(connectionTestPrompt), options)
	result := ConnectionTestResult{
		Status:         ConnectionTestSuccess,
		ModelID:        model.ID,
		ModelName:      model.Name,
		RequestText:    connectionTestPrompt,
		ThinkingLevel:  thinkingLevel,
		ThinkingText:   assistantThinkingText(message),
		ResponseText:   message.Text(),
		StopReason:     message.StopReason,
		InputTokens:    message.Usage.Input,
		OutputTokens:   message.Usage.Output,
		Latency:        time.Since(started),
		ProviderStatus: int(providerStatus.Load()),
	}
	if result.ModelName == "" {
		result.ModelName = model.ID
	}
	if testErr != nil {
		result.Status = classifyConnectionTestError(probeCtx, testErr, result.ProviderStatus)
	}
	return result, nil
}

func resolveConnectionTestThinkingLevel(model llm.Model, requested llm.ModelThinkingLevel) (llm.ModelThinkingLevel, error) {
	available := llm.SupportedThinkingLevels(model)
	requested = llm.ModelThinkingLevel(strings.TrimSpace(string(requested)))
	if requested == "" {
		if slices.Contains(available, llm.ModelThinkingOff) {
			return llm.ModelThinkingOff, nil
		}
		if len(available) > 0 {
			return available[0], nil
		}
		return "", fmt.Errorf("model %q has no available thinking levels", model.ID)
	}
	if !slices.Contains(available, requested) {
		return "", fmt.Errorf(
			"thinking level %q is not available for model %q",
			requested,
			model.ID,
		)
	}
	return requested, nil
}

func connectionTestLimits(thinkingLevel llm.ModelThinkingLevel) (time.Duration, int64) {
	switch thinkingLevel {
	case llm.ModelThinkingMinimal:
		return connectionTestReasoningTimeout, 2_048
	case llm.ModelThinkingLow:
		return connectionTestReasoningTimeout, 4_096
	case llm.ModelThinkingMedium:
		return connectionTestReasoningTimeout, 10_240
	case llm.ModelThinkingHigh, llm.ModelThinkingXHigh, llm.ModelThinkingMax:
		return connectionTestReasoningTimeout, 20_480
	default:
		return connectionTestStandardTimeout, connectionTestStandardTokens
	}
}

func assistantThinkingText(message llm.AssistantMessage) string {
	var builder strings.Builder
	for _, rawContent := range message.Content {
		content, ok := rawContent.(*llm.ThinkingContent)
		if !ok || content == nil || content.Thinking == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(content.Thinking)
	}
	return builder.String()
}

func (t *ConnectionTester) resolve(request ConnectionTestRequest) (llm.Model, llm.StreamOptions, error) {
	if t == nil || t.store == nil || t.store.registry == nil {
		return llm.Model{}, llm.StreamOptions{}, errors.New("provider connection testing is unavailable")
	}
	request.ProviderID = strings.TrimSpace(request.ProviderID)
	request.ConnectionID = strings.TrimSpace(request.ConnectionID)
	request.KeyID = strings.TrimSpace(request.KeyID)
	request.BaseURL = strings.TrimSpace(request.BaseURL)
	request.APIKey = strings.TrimSpace(request.APIKey)
	request.ModelID = strings.TrimSpace(request.ModelID)
	if request.ProviderID == "" || request.ConnectionID == "" || request.KeyID == "" || request.ModelID == "" {
		return llm.Model{}, llm.StreamOptions{}, errors.New("provider, connection, key and model are required")
	}

	registered, ok := t.store.registry.Get(request.ProviderID)
	if !ok {
		return llm.Model{}, llm.StreamOptions{}, fmt.Errorf("unknown provider %q", request.ProviderID)
	}
	var model llm.Model
	for _, candidate := range registered.Models() {
		if candidate.ID == request.ModelID && llm.SupportsProtocol(candidate.Protocol) {
			model = candidate
			break
		}
	}
	if model.ID == "" {
		return llm.Model{}, llm.StreamOptions{}, fmt.Errorf("model %q is not available for provider %q", request.ModelID, request.ProviderID)
	}

	apiKey := request.APIKey
	if apiKey == "" {
		t.store.mu.Lock()
		profile, found := t.store.profiles[request.ProviderID]
		if found {
			profile = cloneProfile(profile)
		}
		t.store.mu.Unlock()
		if !found {
			return llm.Model{}, llm.StreamOptions{}, fmt.Errorf("provider %q is not configured", request.ProviderID)
		}
		connection := FindConnection(normalizeProfile(profile), request.ConnectionID)
		if connection == nil {
			return llm.Model{}, llm.StreamOptions{}, fmt.Errorf("connection %q was not found", request.ConnectionID)
		}
		key := FindKey(*connection, request.KeyID)
		if key == nil || strings.TrimSpace(key.APIKey) == "" {
			return llm.Model{}, llm.StreamOptions{}, fmt.Errorf("key %q was not found", request.KeyID)
		}
		apiKey = key.APIKey
	}

	baseURL := model.BaseURL
	if request.ConnectionID != OfficialConnectionID {
		baseURL = request.BaseURL
	}
	if err := validateConnectionTestBaseURL(baseURL); err != nil {
		return llm.Model{}, llm.StreamOptions{}, err
	}
	return model, llm.StreamOptions{APIKey: apiKey, BaseURL: baseURL}, nil
}

func validateConnectionTestBaseURL(value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("a valid HTTP(S) base URL is required")
	}
	return nil
}

func classifyConnectionTestError(ctx context.Context, err error, status int) ConnectionTestStatus {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ConnectionTestAuthenticationFailed
	case http.StatusTooManyRequests:
		return ConnectionTestRateLimited
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return ConnectionTestTimeout
	case http.StatusNotFound:
		return ConnectionTestNotFound
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return ConnectionTestTimeout
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return ConnectionTestUnreachable
	}
	return ConnectionTestProviderError
}
