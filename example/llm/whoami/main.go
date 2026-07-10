// Command whoami reports which providers are configured and lists their models.
//
// It reads the provider registry — the same one backing the package-level
// Complete — and asks each provider whether a usable API key resolves from the
// environment via AuthStatus. Nothing is sent to any provider; this only
// inspects local configuration, so it is a good way to confirm which keys are
// in place before running anything else.
//
// Run it with whatever provider keys you have exported:
//
//	DEEPSEEK_API_KEY=sk-... go run ./example/llm/whoami
//	go run ./example/llm/whoami --models   # also list each configured model
//	go run ./example/llm/whoami --all      # also list unconfigured providers
package main

import (
	"flag"
	"fmt"
	"sort"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/all" // register the built-in protocol adapters
)

func main() {
	showModels := flag.Bool("models", false, "list each configured provider's models")
	all := flag.Bool("all", false, "also list unconfigured providers")
	flag.Parse()

	registry := llm.DefaultProviderRegistry()

	// The two protocols with a registered adapter. A provider whose models use
	// another protocol is listed in the catalog but cannot serve a request yet.
	hasAdapter := map[llm.Protocol]bool{
		llm.ProtocolOpenAICompletions: true,
		llm.ProtocolAnthropicMessages: true,
	}

	var configured, unconfigured []*llm.Provider
	for _, provider := range registry.Providers() {
		if status, _ := registry.AuthStatus(provider.ID(), nil); status.Configured {
			configured = append(configured, provider)
		} else {
			unconfigured = append(unconfigured, provider)
		}
	}

	fmt.Printf("Configured providers: %d / %d\n\n", len(configured), len(configured)+len(unconfigured))

	if len(configured) == 0 {
		fmt.Println("(none — export a provider key, e.g. DEEPSEEK_API_KEY, and re-run)")
	}
	for _, provider := range configured {
		status, _ := registry.AuthStatus(provider.ID(), nil)
		models := provider.Models()
		note := ""
		if !hasAdapter[protocolOf(models)] {
			note = "  [catalog-only: no adapter for this protocol yet]"
		}
		fmt.Printf("✓ %-22s %-24s %3d models%s\n", provider.ID(), status.Source, len(models), note)
		if *showModels {
			printModels(models)
		}
	}

	if *all && len(unconfigured) > 0 {
		fmt.Printf("\nUnconfigured (%d):\n", len(unconfigured))
		for _, provider := range unconfigured {
			status, _ := registry.AuthStatus(provider.ID(), nil)
			fmt.Printf("  %-22s set %v\n", provider.ID(), status.Missing)
		}
	}
}

// protocolOf returns the protocol shared by a provider's models. Models are
// grouped by provider, so the first model's protocol represents the set.
func protocolOf(models []llm.Model) llm.Protocol {
	if len(models) == 0 {
		return ""
	}
	return models[0].Protocol
}

func printModels(models []llm.Model) {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		fmt.Printf("      - %s\n", id)
	}
}
