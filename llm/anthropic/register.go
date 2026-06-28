package anthropic

import "github.com/ktsoator/or/llm"

// init registers the Anthropic Messages adapter into the llm package default
// registry, so importing this package — typically for side effects — makes the
// protocol available to llm.Stream and llm.Complete.
func init() {
	if err := llm.Register(NewAdapter(nil)); err != nil {
		panic(err)
	}
}
