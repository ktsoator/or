// Command model_switch continues one conversation across two protocols.
//
// Turn 1 goes to DeepSeek, which speaks OpenAI-compatible Chat Completions.
// Turn 2 sends the same history — unchanged — to MiniMax on its China endpoint,
// which speaks Anthropic-compatible Messages. The caller does not rebuild the
// conversation: llm re-adapts the stored history for the target protocol on
// each request (downgrading images, reconciling tool-call IDs, and so on).
//
// Because the two turns use different protocols, both provider packages must be
// registered. Each needs its own key:
//
//	DEEPSEEK_API_KEY=sk-...   (DeepSeek, OpenAI-compatible)
//	MINIMAX_CN_API_KEY=...    (MiniMax CN, Anthropic-compatible)
//
//	DEEPSEEK_API_KEY=... MINIMAX_CN_API_KEY=... go run ./example/llm/model_switch
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/anthropic" // MiniMax CN speaks Anthropic-compatible Messages
	_ "github.com/ktsoator/or/llm/openai"    // DeepSeek speaks OpenAI-compatible Chat Completions
)

func main() {
	ctx := context.Background()

	deepseek := llm.GetModel("deepseek", "deepseek-v4-flash")
	minimax := llm.GetModel("minimax-cn", "MiniMax-M2.7")

	history := []llm.Message{
		llm.UserText("Suggest a name for a Go library that unifies LLM providers."),
	}

	// Turn 1 — DeepSeek (OpenAI-compatible).
	first := complete(ctx, deepseek, history)
	fmt.Printf("[%s] %s\n", deepseek.Provider, first.Text())

	// Carry the reply forward and ask a follow-up.
	history = append(history, &first)
	history = append(history, llm.UserText("Now critique that name in one sentence."))

	// Turn 2 — MiniMax CN (Anthropic-compatible). Same history slice, different
	// protocol; no manual conversion needed.
	second := complete(ctx, minimax, history)
	fmt.Printf("[%s] %s\n", minimax.Provider, second.Text())
}

func complete(ctx context.Context, model llm.Model, history []llm.Message) llm.AssistantMessage {
	msg, err := llm.Complete(ctx, model, llm.NewContext(history...), llm.StreamOptions{MaxTokens: 500})
	if err != nil {
		log.Fatal(err)
	}
	return msg
}
