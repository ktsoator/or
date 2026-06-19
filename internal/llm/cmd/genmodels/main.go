// Command genmodels builds llm's checked-in model catalog from the same public
// catalogs used by pi-ai. The generated catalog intentionally includes only
// protocols implemented by this Go package.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	modelsDevURL  = "https://models.dev/api.json"
	openRouterURL = "https://openrouter.ai/api/v1/models"
	vercelURL     = "https://ai-gateway.vercel.sh/v1/models"
)

type sourceModel struct {
	Name      string `json:"name"`
	ToolCall  bool   `json:"tool_call"`
	Reasoning bool   `json:"reasoning"`
	Status    string `json:"status"`
	Limit     struct {
		Context int64 `json:"context"`
		Output  int64 `json:"output"`
	} `json:"limit"`
	Cost struct {
		Input      float64 `json:"input"`
		Output     float64 `json:"output"`
		CacheRead  float64 `json:"cache_read"`
		CacheWrite float64 `json:"cache_write"`
	} `json:"cost"`
	Modalities struct {
		Input  []string `json:"input"`
		Output []string `json:"output"`
	} `json:"modalities"`
	Provider struct {
		NPM string `json:"npm"`
	} `json:"provider"`
}

type sourceProvider struct {
	Models map[string]sourceModel `json:"models"`
}

type model struct {
	ID               string
	Name             string
	Protocol         string
	Provider         string
	BaseURL          string
	Reasoning        bool
	Input            []string
	InputCost        float64
	OutputCost       float64
	CacheReadCost    float64
	CacheWriteCost   float64
	ContextWindow    int64
	MaxTokens        int64
	Headers          map[string]string
	ThinkingLevelMap map[string]*string
	Compat           compatibility
}

type compatibility struct {
	Kind                                        string
	SupportsStore                               *bool
	SupportsDeveloperRole                       *bool
	SupportsReasoningEffort                     *bool
	MaxTokensField                              string
	SupportsStrictMode                          *bool
	RequiresReasoningContentOnAssistantMessages *bool
	ThinkingFormat                              string
	ZAIToolStream                               *bool
	SupportsTemperature                         *bool
	SupportsCacheControl                        *bool
	SupportsCacheControlTools                   *bool
	ForceAdaptiveThinking                       *bool
	AllowEmptySignature                         *bool
}

type providerRule struct {
	Source   string
	Provider string
	Protocol string
	BaseURL  string
	Compat   compatibility
	Headers  map[string]string
}

func boolp(value bool) *bool       { return &value }
func stringp(value string) *string { return &value }

var providerRules = []providerRule{
	{Source: "anthropic", Provider: "anthropic", Protocol: "anthropic-messages", BaseURL: "https://api.anthropic.com"},
	{Source: "deepseek", Provider: "deepseek", Protocol: "openai-completions", BaseURL: "https://api.deepseek.com"},
	{Source: "groq", Provider: "groq", Protocol: "openai-completions", BaseURL: "https://api.groq.com/openai/v1"},
	{Source: "cerebras", Provider: "cerebras", Protocol: "openai-completions", BaseURL: "https://api.cerebras.ai/v1"},
	{Source: "xai", Provider: "xai", Protocol: "openai-completions", BaseURL: "https://api.x.ai/v1"},
	{Source: "huggingface", Provider: "huggingface", Protocol: "openai-completions", BaseURL: "https://router.huggingface.co/v1", Compat: openAICompat(withDeveloperRole(false))},
	{Source: "fireworks-ai", Provider: "fireworks", Protocol: "anthropic-messages", BaseURL: "https://api.fireworks.ai/inference"},
	{Source: "minimax", Provider: "minimax", Protocol: "anthropic-messages", BaseURL: "https://api.minimax.io/anthropic"},
	{Source: "minimax-cn", Provider: "minimax-cn", Protocol: "anthropic-messages", BaseURL: "https://api.minimaxi.com/anthropic"},
	{Source: "moonshotai", Provider: "moonshotai", Protocol: "openai-completions", BaseURL: "https://api.moonshot.ai/v1", Compat: moonshotCompat()},
	{Source: "moonshotai-cn", Provider: "moonshotai-cn", Protocol: "openai-completions", BaseURL: "https://api.moonshot.cn/v1", Compat: moonshotCompat()},
	{Source: "xiaomi", Provider: "xiaomi", Protocol: "openai-completions", BaseURL: "https://api.xiaomimimo.com/v1", Compat: xiaomiCompat()},
	{Source: "xiaomi", Provider: "xiaomi-token-plan-cn", Protocol: "openai-completions", BaseURL: "https://token-plan-cn.xiaomimimo.com/v1", Compat: xiaomiCompat()},
	{Source: "xiaomi", Provider: "xiaomi-token-plan-ams", Protocol: "openai-completions", BaseURL: "https://token-plan-ams.xiaomimimo.com/v1", Compat: xiaomiCompat()},
	{Source: "xiaomi", Provider: "xiaomi-token-plan-sgp", Protocol: "openai-completions", BaseURL: "https://token-plan-sgp.xiaomimimo.com/v1", Compat: xiaomiCompat()},
	{Source: "zai-coding-plan", Provider: "zai", Protocol: "openai-completions", BaseURL: "https://api.z.ai/api/coding/paas/v4", Compat: zaiCompat()},
	{Source: "zai-coding-plan", Provider: "zai-coding-cn", Protocol: "openai-completions", BaseURL: "https://open.bigmodel.cn/api/coding/paas/v4", Compat: zaiCompat()},
	{Source: "kimi-for-coding", Provider: "kimi-coding", Protocol: "anthropic-messages", BaseURL: "https://api.kimi.com/coding", Headers: map[string]string{"User-Agent": "KimiCLI/1.5"}},
}

type compatOption func(*compatibility)

func openAICompat(options ...compatOption) compatibility {
	c := compatibility{Kind: "openai"}
	for _, option := range options {
		option(&c)
	}
	return c
}

func withDeveloperRole(value bool) compatOption {
	return func(c *compatibility) { c.SupportsDeveloperRole = boolp(value) }
}

func moonshotCompat() compatibility {
	return compatibility{
		Kind:                    "openai",
		SupportsStore:           boolp(false),
		SupportsDeveloperRole:   boolp(false),
		SupportsReasoningEffort: boolp(false),
		MaxTokensField:          "max_tokens",
		SupportsStrictMode:      boolp(false),
		ThinkingFormat:          "deepseek",
	}
}

func xiaomiCompat() compatibility {
	return compatibility{
		Kind: "openai",
		RequiresReasoningContentOnAssistantMessages: boolp(true),
		ThinkingFormat: "deepseek",
	}
}

func zaiCompat() compatibility {
	return compatibility{
		Kind:                    "openai",
		SupportsDeveloperRole:   boolp(false),
		SupportsReasoningEffort: boolp(false),
		ThinkingFormat:          "zai",
		ZAIToolStream:           boolp(true),
	}
}

func main() {
	output := flag.String("output", "catalog.go", "generated Go file")
	timeout := flag.Duration("timeout", 45*time.Second, "HTTP timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	client := &http.Client{Timeout: *timeout}

	models, err := collect(ctx, client)
	if err != nil {
		fatal(err)
	}
	generated, err := render(models)
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*output, generated, 0o644); err != nil {
		fatal(fmt.Errorf("write %s: %w", *output, err))
	}
	fmt.Printf("generated %s with %d models\n", *output, len(models))
}

func collect(ctx context.Context, client *http.Client) ([]model, error) {
	var catalog map[string]sourceProvider
	if err := getJSON(ctx, client, modelsDevURL, &catalog); err != nil {
		return nil, fmt.Errorf("models.dev: %w", err)
	}

	models := fromModelsDev(catalog)
	if openRouter, err := fromOpenRouter(ctx, client); err != nil {
		fmt.Fprintf(os.Stderr, "warning: OpenRouter catalog unavailable: %v\n", err)
	} else {
		models = append(models, openRouter...)
	}
	if vercel, err := fromVercel(ctx, client); err != nil {
		fmt.Fprintf(os.Stderr, "warning: Vercel AI Gateway catalog unavailable: %v\n", err)
	} else {
		models = append(models, vercel...)
	}

	applyOverrides(models)
	return deduplicate(models), nil
}

func fromModelsDev(catalog map[string]sourceProvider) []model {
	var models []model
	for _, rule := range providerRules {
		for id, source := range catalog[rule.Source].Models {
			if !source.ToolCall || source.Status == "deprecated" {
				continue
			}
			if strings.HasPrefix(rule.Provider, "xiaomi-token-plan-") && id == "mimo-v2-flash" {
				continue
			}
			models = append(models, normalize(id, source, rule))
		}
	}
	models = append(models, fromOpenCode(catalog)...)
	models = append(models, fromCopilot(catalog)...)
	return models
}

func normalize(id string, source sourceModel, rule providerRule) model {
	name := source.Name
	if name == "" {
		name = id
	}
	return model{
		ID:             id,
		Name:           name,
		Protocol:       rule.Protocol,
		Provider:       rule.Provider,
		BaseURL:        rule.BaseURL,
		Reasoning:      source.Reasoning,
		Input:          inputModalities(source.Modalities.Input),
		InputCost:      source.Cost.Input,
		OutputCost:     source.Cost.Output,
		CacheReadCost:  source.Cost.CacheRead,
		CacheWriteCost: source.Cost.CacheWrite,
		ContextWindow:  defaultInt(source.Limit.Context, 4096),
		MaxTokens:      defaultInt(source.Limit.Output, 4096),
		Headers:        cloneMap(rule.Headers),
		Compat:         rule.Compat,
	}
}

func fromOpenCode(catalog map[string]sourceProvider) []model {
	variants := []struct{ source, provider, base string }{
		{"opencode", "opencode", "https://opencode.ai/zen"},
		{"opencode-go", "opencode-go", "https://opencode.ai/zen/go"},
	}
	var models []model
	for _, variant := range variants {
		for id, source := range catalog[variant.source].Models {
			if !source.ToolCall || source.Status == "deprecated" {
				continue
			}
			protocol := "openai-completions"
			baseURL := variant.base + "/v1"
			compat := compatibility{Kind: "openai", MaxTokensField: "max_tokens"}
			switch source.Provider.NPM {
			case "@ai-sdk/anthropic":
				protocol = "anthropic-messages"
				baseURL = variant.base
				compat = compatibility{}
			case "@ai-sdk/openai", "@ai-sdk/google":
				// These require protocols that the Go package does not implement yet.
				continue
			}
			// These models are mislabeled upstream and use the OpenAI-compatible path.
			if variant.provider == "opencode-go" && (id == "minimax-m2.7" || id == "qwen3.5-plus" || id == "qwen3.6-plus") {
				protocol = "openai-completions"
				baseURL = variant.base + "/v1"
				compat = compatibility{Kind: "openai", MaxTokensField: "max_tokens"}
				if strings.HasPrefix(id, "qwen") {
					compat.ThinkingFormat = "qwen"
				}
			}
			if protocol != "openai-completions" && protocol != "anthropic-messages" {
				continue
			}
			models = append(models, normalize(id, source, providerRule{
				Provider: variant.provider, Protocol: protocol, BaseURL: baseURL, Compat: compat,
			}))
		}
	}
	return models
}

func fromCopilot(catalog map[string]sourceProvider) []model {
	var models []model
	for id, source := range catalog["github-copilot"].Models {
		if !source.ToolCall || source.Status == "deprecated" || strings.HasPrefix(id, "gpt-5") || strings.HasPrefix(id, "oswe") {
			continue
		}
		protocol := "openai-completions"
		compat := compatibility{
			Kind: "openai", SupportsStore: boolp(false), SupportsDeveloperRole: boolp(false), SupportsReasoningEffort: boolp(false),
		}
		if isCopilotClaude4(id) {
			protocol = "anthropic-messages"
			compat = compatibility{Kind: "anthropic"}
		}
		models = append(models, normalize(id, source, providerRule{
			Provider: "github-copilot", Protocol: protocol, BaseURL: "https://api.individual.githubcopilot.com",
			Compat: compat,
			Headers: map[string]string{
				"User-Agent": "GitHubCopilotChat/0.35.0", "Editor-Version": "vscode/1.107.0",
				"Editor-Plugin-Version": "copilot-chat/0.35.0", "Copilot-Integration-Id": "vscode-chat",
			},
		}))
	}
	return models
}

func isCopilotClaude4(id string) bool {
	return strings.HasPrefix(id, "claude-haiku-4") || strings.HasPrefix(id, "claude-sonnet-4") || strings.HasPrefix(id, "claude-opus-4")
}

type openRouterResponse struct {
	Data []struct {
		ID                  string   `json:"id"`
		Name                string   `json:"name"`
		ContextLength       int64    `json:"context_length"`
		SupportedParameters []string `json:"supported_parameters"`
		Architecture        struct {
			Modality string `json:"modality"`
		} `json:"architecture"`
		Pricing struct {
			Prompt          string `json:"prompt"`
			Completion      string `json:"completion"`
			InputCacheRead  string `json:"input_cache_read"`
			InputCacheWrite string `json:"input_cache_write"`
		} `json:"pricing"`
		TopProvider struct {
			MaxCompletionTokens int64 `json:"max_completion_tokens"`
		} `json:"top_provider"`
	} `json:"data"`
}

func fromOpenRouter(ctx context.Context, client *http.Client) ([]model, error) {
	var response openRouterResponse
	if err := getJSON(ctx, client, openRouterURL, &response); err != nil {
		return nil, err
	}
	var models []model
	for _, source := range response.Data {
		if !contains(source.SupportedParameters, "tools") {
			continue
		}
		models = append(models, model{
			ID: source.ID, Name: defaultString(source.Name, source.ID), Protocol: "openai-completions",
			Provider: "openrouter", BaseURL: "https://openrouter.ai/api/v1",
			Reasoning: contains(source.SupportedParameters, "reasoning"),
			Input:     inputModalities([]string{source.Architecture.Modality}),
			InputCost: perMillion(source.Pricing.Prompt), OutputCost: perMillion(source.Pricing.Completion),
			CacheReadCost: perMillion(source.Pricing.InputCacheRead), CacheWriteCost: perMillion(source.Pricing.InputCacheWrite),
			ContextWindow: defaultInt(source.ContextLength, 4096), MaxTokens: defaultInt(source.TopProvider.MaxCompletionTokens, 4096),
		})
	}
	return models, nil
}

type vercelResponse struct {
	Data []struct {
		ID, Name                 string
		ContextWindow, MaxTokens int64
		Tags                     []string
		Pricing                  struct{ Input, Output, InputCacheRead, InputCacheWrite json.Number }
	}
}

func fromVercel(ctx context.Context, client *http.Client) ([]model, error) {
	var raw struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	if err := getJSON(ctx, client, vercelURL, &raw); err != nil {
		return nil, err
	}
	var models []model
	for _, item := range raw.Data {
		var id, name string
		var contextWindow, maxTokens int64
		var tags []string
		var pricing map[string]any
		_ = json.Unmarshal(item["id"], &id)
		_ = json.Unmarshal(item["name"], &name)
		_ = json.Unmarshal(item["context_window"], &contextWindow)
		_ = json.Unmarshal(item["max_tokens"], &maxTokens)
		_ = json.Unmarshal(item["tags"], &tags)
		_ = json.Unmarshal(item["pricing"], &pricing)
		if id == "" || !contains(tags, "tool-use") {
			continue
		}
		models = append(models, model{
			ID: id, Name: defaultString(name, id), Protocol: "anthropic-messages", Provider: "vercel-ai-gateway",
			BaseURL: "https://ai-gateway.vercel.sh", Reasoning: contains(tags, "reasoning"),
			Input: inputModalities(tags), InputCost: anyPerMillion(pricing["input"]), OutputCost: anyPerMillion(pricing["output"]),
			CacheReadCost: anyPerMillion(pricing["input_cache_read"]), CacheWriteCost: anyPerMillion(pricing["input_cache_write"]),
			ContextWindow: defaultInt(contextWindow, 4096), MaxTokens: defaultInt(maxTokens, 4096),
		})
	}
	return models, nil
}

func applyOverrides(models []model) {
	for i := range models {
		m := &models[i]
		if m.Protocol == "anthropic-messages" && isAdaptiveAnthropic(m.ID) {
			m.Compat.Kind = "anthropic"
			m.Compat.ForceAdaptiveThinking = boolp(true)
		}
		id := strings.ToLower(m.ID)
		if m.Protocol == "anthropic-messages" && (strings.Contains(id, "opus-4-7") || strings.Contains(id, "opus-4.7") || strings.Contains(id, "opus-4-8") || strings.Contains(id, "opus-4.8")) {
			m.Compat.Kind = "anthropic"
			m.Compat.SupportsTemperature = boolp(false)
		}
		if strings.Contains(m.ID, "deepseek-v4") {
			high, max := "high", "max"
			m.ThinkingLevelMap = map[string]*string{"minimal": nil, "low": nil, "medium": nil, "high": &high, "xhigh": &max}
		}
		if m.Provider == "zai" || m.Provider == "zai-coding-cn" {
			if m.ID == "glm-5.2" {
				high, max := "high", "max"
				m.ThinkingLevelMap = map[string]*string{"minimal": nil, "low": &high, "medium": &high, "high": &high, "xhigh": &max}
				m.Compat.SupportsReasoningEffort = boolp(true)
			}
		}
	}
}

func isAdaptiveAnthropic(id string) bool {
	id = strings.ToLower(id)
	for _, marker := range []string{"opus-4-6", "opus-4.6", "opus-4-7", "opus-4.7", "opus-4-8", "opus-4.8", "sonnet-4-6", "sonnet-4.6", "fable-5"} {
		if strings.Contains(id, marker) {
			return true
		}
	}
	return false
}

func deduplicate(models []model) []model {
	seen := make(map[string]model, len(models))
	for _, m := range models {
		if m.ID == "" || m.Provider == "" || m.Protocol == "" {
			continue
		}
		key := m.Provider + "\x00" + m.ID
		if _, exists := seen[key]; !exists {
			seen[key] = m
		}
	}
	result := make([]model, 0, len(seen))
	for _, m := range seen {
		result = append(result, m)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Provider != result[j].Provider {
			return result[i].Provider < result[j].Provider
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func render(models []model) ([]byte, error) {
	var out bytes.Buffer
	providers := modelsByProvider(models)
	providerIDs := sortedKeys(providers)

	out.WriteString("// Code generated by go generate; DO NOT EDIT.\n\n")
	out.WriteString("package llm\n\n")
	out.WriteString("//go:generate go run ./cmd/genmodels -output catalog.go\n\n")
	out.WriteString("var builtInModelRegistry = newBuiltInModelRegistry()\n\n")
	out.WriteString("func newBuiltInModelRegistry() *ModelRegistry {\n\tregistry := NewModelRegistry()\n\tfor _, model := range builtInModels() {\n\t\tif err := registry.Register(model); err != nil { panic(err) }\n\t}\n\treturn registry\n}\n\n")
	fmt.Fprintf(&out, "func builtInModels() []Model {\n\tmodels := make([]Model, 0, %d)\n", len(models))
	for _, providerID := range providerIDs {
		fmt.Fprintf(&out, "\tmodels = append(models, %s()...)\n", providerFunctionName(providerID))
	}
	out.WriteString("\treturn models\n}\n\n")

	for _, providerID := range providerIDs {
		fmt.Fprintf(&out, "// %s contains the generated %s model catalog.\n", providerFunctionName(providerID), providerID)
		fmt.Fprintf(&out, "func %s() []Model {\n\treturn []Model{\n", providerFunctionName(providerID))
		for _, m := range providers[providerID] {
			writeModel(&out, m)
		}
		out.WriteString("\t}\n}\n\n")
	}
	out.WriteString("func generatedString(value string) *string { return &value }\n")
	out.WriteString("func generatedBool(value bool) *bool { return &value }\n")
	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format generated source: %w", err)
	}
	return formatted, nil
}

func modelsByProvider(models []model) map[string][]model {
	providers := make(map[string][]model)
	for _, model := range models {
		providers[model.Provider] = append(providers[model.Provider], model)
	}
	return providers
}

func providerFunctionName(provider string) string {
	parts := strings.FieldsFunc(provider, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9')
	})
	if len(parts) == 0 {
		return "unknownProviderModels"
	}
	name := strings.ToLower(parts[0])
	for _, part := range parts[1:] {
		name += title(strings.ToLower(part))
	}
	return name + "Models"
}

func writeModel(out *bytes.Buffer, m model) {
	fmt.Fprintln(out, "\t\t{")
	field(out, "ID", m.ID)
	field(out, "Name", m.Name)
	fmt.Fprintf(out, "\t\t\tProtocol: %s,\n", protocolConstant(m.Protocol))
	field(out, "Provider", m.Provider)
	field(out, "BaseURL", m.BaseURL)
	fmt.Fprintf(out, "\t\t\tReasoning: %t,\n", m.Reasoning)
	if len(m.ThinkingLevelMap) > 0 {
		fmt.Fprintln(out, "\t\t\tThinkingLevelMap: map[ModelThinkingLevel]*string{")
		levels := []string{"off", "minimal", "low", "medium", "high", "xhigh"}
		for _, level := range levels {
			value, ok := m.ThinkingLevelMap[level]
			if !ok {
				continue
			}
			if value == nil {
				fmt.Fprintf(out, "\t\t\t\tModelThinking%s: nil,\n", title(level))
			} else {
				fmt.Fprintf(out, "\t\t\t\tModelThinking%s: generatedString(%q),\n", title(level), *value)
			}
		}
		fmt.Fprintln(out, "\t\t\t},")
	}
	fmt.Fprint(out, "\t\t\tInput: []ModelInput{")
	for i, input := range m.Input {
		if i > 0 {
			fmt.Fprint(out, ", ")
		}
		fmt.Fprint(out, title(input))
	}
	fmt.Fprintln(out, "},")
	fmt.Fprintf(out, "\t\t\tCost: ModelCost{Input: %s, Output: %s, CacheRead: %s, CacheWrite: %s},\n", number(m.InputCost), number(m.OutputCost), number(m.CacheReadCost), number(m.CacheWriteCost))
	fmt.Fprintf(out, "\t\t\tContextWindow: %d,\n\t\t\tMaxTokens: %d,\n", m.ContextWindow, m.MaxTokens)
	if len(m.Headers) > 0 {
		fmt.Fprintln(out, "\t\t\tHeaders: map[string]string{")
		keys := sortedKeys(m.Headers)
		for _, key := range keys {
			fmt.Fprintf(out, "\t\t\t\t%q: %q,\n", key, m.Headers[key])
		}
		fmt.Fprintln(out, "\t\t\t},")
	}
	writeCompatibility(out, m.Compat)
	fmt.Fprintln(out, "\t\t},")
}

func writeCompatibility(out *bytes.Buffer, c compatibility) {
	if c.Kind == "" {
		return
	}
	name := "OpenAICompletionsCompatibility"
	if c.Kind == "anthropic" {
		name = "AnthropicMessagesCompatibility"
	}
	fmt.Fprintf(out, "\t\t\tCompatibility: &%s{\n", name)
	pointerBoolField(out, "SupportsStore", c.SupportsStore)
	pointerBoolField(out, "SupportsDeveloperRole", c.SupportsDeveloperRole)
	pointerBoolField(out, "SupportsReasoningEffort", c.SupportsReasoningEffort)
	stringField(out, "MaxTokensField", c.MaxTokensField)
	pointerBoolField(out, "SupportsStrictMode", c.SupportsStrictMode)
	pointerBoolField(out, "RequiresReasoningContentOnAssistantMessages", c.RequiresReasoningContentOnAssistantMessages)
	stringField(out, "ThinkingFormat", c.ThinkingFormat)
	pointerBoolField(out, "ZAIToolStream", c.ZAIToolStream)
	pointerBoolField(out, "SupportsTemperature", c.SupportsTemperature)
	pointerBoolField(out, "SupportsCacheControl", c.SupportsCacheControl)
	pointerBoolField(out, "SupportsCacheControlTools", c.SupportsCacheControlTools)
	pointerBoolField(out, "ForceAdaptiveThinking", c.ForceAdaptiveThinking)
	pointerBoolField(out, "AllowEmptySignature", c.AllowEmptySignature)
	fmt.Fprintln(out, "\t\t\t},")
}

func getJSON(ctx context.Context, client *http.Client, url string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "or-genmodels/1")
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return fmt.Errorf("%s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	decoder := json.NewDecoder(response.Body)
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func inputModalities(values []string) []string {
	result := []string{"text"}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), "image") || strings.EqualFold(value, "vision") {
			return append(result, "image")
		}
	}
	return result
}
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func defaultInt(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}
func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
func cloneMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	result := make(map[string]string, len(source))
	for k, v := range source {
		result[k] = v
	}
	return result
}
func perMillion(value string) float64 {
	parsed, _ := strconv.ParseFloat(value, 64)
	return parsed * 1_000_000
}
func anyPerMillion(value any) float64 {
	switch v := value.(type) {
	case json.Number:
		parsed, _ := v.Float64()
		return parsed * 1_000_000
	case string:
		return perMillion(v)
	case float64:
		return v * 1_000_000
	}
	return 0
}
func protocolConstant(protocol string) string {
	if protocol == "anthropic-messages" {
		return "ProtocolAnthropicMessages"
	}
	return "ProtocolOpenAICompletions"
}
func title(value string) string {
	if value == "xhigh" {
		return "XHigh"
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
func number(value float64) string                 { return strconv.FormatFloat(value, 'f', -1, 64) }
func field(out *bytes.Buffer, name, value string) { fmt.Fprintf(out, "\t\t\t%s: %q,\n", name, value) }
func stringField(out *bytes.Buffer, name, value string) {
	if value != "" {
		fmt.Fprintf(out, "\t\t\t\t%s: %q,\n", name, value)
	}
}
func pointerBoolField(out *bytes.Buffer, name string, value *bool) {
	if value != nil {
		fmt.Fprintf(out, "\t\t\t\t%s: generatedBool(%t),\n", name, *value)
	}
}
func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
func fatal(err error) {
	if err == nil {
		err = errors.New("unknown error")
	}
	fmt.Fprintln(os.Stderr, "genmodels:", err)
	os.Exit(1)
}
