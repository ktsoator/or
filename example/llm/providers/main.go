// Command providers inspects and configures providers at runtime.
//
// It shows the provider registry that sits beside the model catalog: query
// whether a provider has a usable credential with AuthStatus, redirect a
// provider's traffic with SetOverride, and register a custom endpoint with
// NewSpecProvider. The package-level Complete resolves every request through
// the default registry, so an override applied here reaches the request below.
//
// The API key is read from the provider's environment variable when
// StreamOptions.APIKey is empty:
//
//	DEEPSEEK_API_KEY=sk-... go run ./example/llm/providers
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai" // register the OpenAI-compatible protocol (DeepSeek speaks it)
)

func main() {
	registry := llm.DefaultProviderRegistry()

	// 1. Check whether a provider is configured, without sending a request.
	status, ok := registry.AuthStatus("deepseek", nil)
	if !ok {
		log.Fatal("deepseek is not a known provider")
	}
	if status.Configured {
		fmt.Printf("deepseek configured via %s\n", status.Source)
	} else {
		fmt.Printf("deepseek not configured; set one of %v\n", status.Missing)
	}

	// 2. Register a custom provider. It then appears in the registry and
	// resolves its key from its own environment variable.
	if err := registry.Register(llm.NewSpecProvider(llm.ProviderSpec{
		ID:      "local",
		Name:    "Local LLM",
		EnvKeys: []string{"LOCAL_API_KEY"},
		Models: []llm.Model{{
			ID:       "qwen2.5-coder:7b",
			Provider: "local",
			Protocol: llm.ProtocolOpenAICompletions,
			BaseURL:  "http://localhost:11434/v1",
			Input:    []llm.ModelInput{llm.Text},
		}},
	})); err != nil {
		log.Fatal(err)
	}
	localStatus, _ := registry.AuthStatus("local", llm.ProviderEnv{"LOCAL_API_KEY": "ollama"})
	fmt.Printf("local configured via %s\n", localStatus.Source)

	// 3. Overrides apply to every request routed through the provider. Uncomment
	// to send DeepSeek traffic through a proxy without editing any Model:
	//
	//	proxy := "https://proxy.example.com/deepseek/v1"
	//	registry.SetOverride("deepseek", llm.ProviderOverride{BaseURL: &proxy})

	// The request below still resolves its key and base URL through the registry.
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	msg, err := llm.Complete(context.Background(), model,
		llm.Prompt("Name one benefit of a provider registry, in one sentence."),
		llm.StreamOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(msg.Text())
}
