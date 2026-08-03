package openai

import (
	"testing"

	"github.com/ktsoator/or/llm"
)

func TestPublicAdaptersExposeBothOpenAIProtocols(t *testing.T) {
	tests := []struct {
		name    string
		adapter llm.ProtocolAdapter
		want    llm.Protocol
	}{
		{name: "chat completions", adapter: NewAdapter(nil), want: llm.ProtocolOpenAICompletions},
		{name: "responses", adapter: NewResponsesAdapter(nil), want: llm.ProtocolOpenAIResponses},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.adapter.Protocol(); got != test.want {
				t.Fatalf("Protocol() = %q, want %q", got, test.want)
			}
		})
	}
}
