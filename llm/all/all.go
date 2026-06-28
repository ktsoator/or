// Package all registers every built-in protocol adapter into the llm package
// default registry. Import it for side effects when an application wants both
// the OpenAI-compatible and Anthropic protocols available to llm.Stream and
// llm.Complete without choosing providers individually:
//
//	import (
//		"github.com/ktsoator/or/llm"
//		_ "github.com/ktsoator/or/llm/all"
//	)
//
// To link only the providers an application uses — and avoid pulling in the
// other vendor SDK — import the specific provider packages instead.
package all

import (
	_ "github.com/ktsoator/or/llm/anthropic"
	_ "github.com/ktsoator/or/llm/openai"
)
