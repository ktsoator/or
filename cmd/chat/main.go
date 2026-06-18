package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ktsoator/or/internal/llm"
)

func main() {
	modelID := os.Getenv("DEEPSEEK_MODEL")
	model := llm.Model{
		ID:       modelID,
		Name:     modelID,
		API:      llm.OpenAICompletionsAPI,
		Provider: "deepseek",
		BaseURL:  "https://api.deepseek.com",
	}
	apiKey := os.Getenv("DEEPSEEK_API_KEY")

	fmt.Print("Prompt: ")
	prompt, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	prompt = strings.TrimSpace(prompt)

	registry := llm.NewRegistry()
	if err := registry.Register(llm.NewOpenAIProvider(nil)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	events, err := llm.NewClient(registry).Stream(
		context.Background(),
		model,
		llm.Context{
			SystemPrompt: "You are a helpful assistant.",
			Messages: []llm.Message{{
				Role: llm.RoleUser,
				Content: []llm.Content{{
					Type: llm.ContentText,
					Text: prompt,
				}},
			}},
		},
		llm.StreamOptions{APIKey: apiKey},
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	thinkingStarted := false
	answerStarted := false
	for event := range events {
		switch event.Type {
		case llm.EventThinkingDelta:
			if !thinkingStarted {
				fmt.Print("[Thinking]\n")
				thinkingStarted = true
			}
			fmt.Print(event.Delta)
		case llm.EventTextDelta:
			if thinkingStarted && !answerStarted {
				fmt.Print("\n\n[Answer]\n")
			}
			answerStarted = true
			fmt.Print(event.Delta)
		case llm.EventDone:
			fmt.Println()
		case llm.EventError:
			fmt.Fprintln(os.Stderr, event.Err)
			os.Exit(1)
		}
	}
}
