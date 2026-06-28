package llm

import "os"

var providerAPIKeyEnvVars = map[string][]string{
	"github-copilot":         {"COPILOT_GITHUB_TOKEN"},
	"anthropic":              {"ANTHROPIC_OAUTH_TOKEN", "ANTHROPIC_API_KEY"},
	"ant-ling":               {"ANT_LING_API_KEY"},
	"openai":                 {"OPENAI_API_KEY"},
	"azure-openai-responses": {"AZURE_OPENAI_API_KEY"},
	"nvidia":                 {"NVIDIA_API_KEY"},
	"deepseek":               {"DEEPSEEK_API_KEY"},
	"google":                 {"GEMINI_API_KEY"},
	"google-vertex":          {"GOOGLE_CLOUD_API_KEY"},
	"groq":                   {"GROQ_API_KEY"},
	"cerebras":               {"CEREBRAS_API_KEY"},
	"xai":                    {"XAI_API_KEY"},
	"openrouter":             {"OPENROUTER_API_KEY"},
	"vercel-ai-gateway":      {"AI_GATEWAY_API_KEY"},
	"zai":                    {"ZAI_API_KEY"},
	"zai-coding-cn":          {"ZAI_CODING_CN_API_KEY"},
	"mistral":                {"MISTRAL_API_KEY"},
	"minimax":                {"MINIMAX_API_KEY"},
	"minimax-cn":             {"MINIMAX_CN_API_KEY"},
	"moonshotai":             {"MOONSHOT_API_KEY"},
	"moonshotai-cn":          {"MOONSHOT_API_KEY"},
	"huggingface":            {"HF_TOKEN"},
	"fireworks":              {"FIREWORKS_API_KEY"},
	"together":               {"TOGETHER_API_KEY"},
	"opencode":               {"OPENCODE_API_KEY"},
	"opencode-go":            {"OPENCODE_API_KEY"},
	"kimi-coding":            {"KIMI_API_KEY"},
	"cloudflare-workers-ai":  {"CLOUDFLARE_API_KEY"},
	"cloudflare-ai-gateway":  {"CLOUDFLARE_API_KEY"},
	"xiaomi":                 {"XIAOMI_API_KEY", "MIMO_API_KEY"},
	"xiaomi-token-plan-cn":   {"XIAOMI_TOKEN_PLAN_CN_API_KEY"},
	"xiaomi-token-plan-ams":  {"XIAOMI_TOKEN_PLAN_AMS_API_KEY"},
	"xiaomi-token-plan-sgp":  {"XIAOMI_TOKEN_PLAN_SGP_API_KEY"},
}

// APIKeyEnvVars returns the environment variables checked for provider in
// precedence order. The returned slice is safe for the caller to modify.
func APIKeyEnvVars(provider string) []string {
	return append([]string(nil), providerAPIKeyEnvVars[provider]...)
}

// FindEnvAPIKeys returns the names of configured API key environment variables
// for provider, in lookup order.
func FindEnvAPIKeys(provider string) []string {
	return findEnvAPIKeys(provider, nil)
}

// FindEnvAPIKeysWithEnv is FindEnvAPIKeys with request-scoped overrides.
func FindEnvAPIKeysWithEnv(provider string, env ProviderEnv) []string {
	return findEnvAPIKeys(provider, env)
}

// GetEnvAPIKey returns the first configured API key for provider.
func GetEnvAPIKey(provider string) string {
	return GetEnvAPIKeyWithEnv(provider, nil)
}

// GetEnvAPIKeyWithEnv returns the first configured API key for provider,
// preferring non-empty request-scoped values over process environment values.
func GetEnvAPIKeyWithEnv(provider string, env ProviderEnv) string {
	for _, name := range providerAPIKeyEnvVars[provider] {
		if value := providerEnvValue(name, env); value != "" {
			return value
		}
	}
	return ""
}

func findEnvAPIKeys(provider string, env ProviderEnv) []string {
	var found []string
	for _, name := range providerAPIKeyEnvVars[provider] {
		if providerEnvValue(name, env) != "" {
			found = append(found, name)
		}
	}
	return found
}

func providerEnvValue(name string, env ProviderEnv) string {
	if value := env[name]; value != "" {
		return value
	}
	return os.Getenv(name)
}
