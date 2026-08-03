package openai

import "github.com/ktsoator/or/llm"

// init registers both OpenAI adapters into the llm package default registry.
func init() {
	if err := llm.Register(NewAdapter(nil)); err != nil {
		panic(err)
	}
	if err := llm.Register(NewResponsesAdapter(nil)); err != nil {
		panic(err)
	}
}
