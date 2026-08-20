package transcript

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

func TestReconstructProviderRequestFromCommittedTranscript(t *testing.T) {
	temperature := 0.25
	maxRetries := 3
	tool := llm.ToolDefinition{
		Name: "read", Description: "read a file",
		Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
	}
	streamOptions := llm.StreamOptions{
		APIKey:      "private-api-key",
		BaseURL:     "https://private-route.example/v1",
		Env:         llm.ProviderEnv{"PRIVATE_API_KEY": "private-env-key"},
		Temperature: &temperature,
		MaxTokens:   512,
		Headers:     map[string]string{"Authorization": "Bearer private-token"},
		ProtocolOptions: &llm.OpenAIResponsesStreamOptions{
			ThinkingDisplay: llm.ThinkingDisplayOmitted,
			ToolChoice:      llm.OpenAIToolChoiceRequired,
		},
		MaxRetries: &maxRetries,
		Timeout:    2 * time.Second,
	}
	captured, err := CaptureRequestOptions(
		llm.ProtocolOpenAIResponses,
		[]llm.ToolDefinition{tool},
		streamOptions,
	)
	if err != nil {
		t.Fatal(err)
	}

	contextEntry := NewContext(ContextAttachment{
		AttachmentID: "base:1:revision", Epoch: 1, Kind: "base",
		Placement: "prefix", Revision: "revision", Rendered: "hidden context",
	})
	headerEntry := NewRequestHeader(RequestHeader{
		ProviderRequestID: "request-1",
		RunID:             "run-1", TurnID: "turn-1", StepID: "step-1",
		Provider: "openai", Model: "gpt-test", Protocol: llm.ProtocolOpenAIResponses,
		ThinkingLevel: llm.ModelThinkingHigh,
		SystemPrompt:  "system prompt",
		Tools:         []llm.ToolDefinition{tool},
		Options:       captured,
		Attachments: []RequestAttachment{{
			AttachmentID: contextEntry.Context.AttachmentID,
			MessageIndex: 0,
		}},
	})
	entries := sequencedForTest(
		NewRunStart("run-1"),
		NewTurnStart("run-1", "turn-1"),
		NewMessage(agent.UserMessage("question")),
		NewStepStart("run-1", "turn-1", "step-1"),
		contextEntry,
		headerEntry,
	)

	reconstructed, err := ReconstructProviderRequest(entries, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	sessionProjection, err := ProjectSession(entries)
	if err != nil {
		t.Fatal(err)
	}
	modelProjection, err := ProjectModelContext(entries)
	if err != nil {
		t.Fatal(err)
	}
	online, err := ReconstructCommittedProviderRequest(
		sessionProjection,
		modelProjection,
		"request-1",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(online.Input, reconstructed.Input) ||
		!reflect.DeepEqual(online.Header, reconstructed.Header) {
		t.Fatalf("online reconstruction differs from replay: %#v != %#v", online, reconstructed)
	}
	if reconstructed.HeaderEntrySeq != 5 || reconstructed.Header.InputSeq != 4 {
		t.Fatalf(
			"request boundary = header %d input %d, want 5/4",
			reconstructed.HeaderEntrySeq,
			reconstructed.Header.InputSeq,
		)
	}
	if reconstructed.Header.Provider != "openai" ||
		reconstructed.Header.Model != "gpt-test" ||
		reconstructed.Header.ThinkingLevel != llm.ModelThinkingHigh {
		t.Fatalf("request identity = %#v", reconstructed.Header)
	}
	if reconstructed.Input.SystemPrompt != "system prompt" ||
		!reflect.DeepEqual(reconstructed.Input.Tools, []llm.ToolDefinition{tool}) {
		t.Fatalf("reconstructed input metadata = %#v", reconstructed.Input)
	}
	if len(reconstructed.Input.Messages) != 2 {
		t.Fatalf("reconstructed messages = %#v", reconstructed.Input.Messages)
	}
	assertUserText(t, reconstructed.Input.Messages[0], "hidden context")
	assertUserText(t, reconstructed.Input.Messages[1], "question")
	if reconstructed.Options.APIKey != "" || reconstructed.Options.BaseURL != "" ||
		len(reconstructed.Options.Env) != 0 || len(reconstructed.Options.Headers) != 0 ||
		reconstructed.Options.MaxRetries != nil || reconstructed.Options.Timeout != 0 ||
		reconstructed.Options.OnRequest != nil || reconstructed.Options.OnResponse != nil ||
		reconstructed.Options.RewriteRequest != nil {
		t.Fatalf("reconstructed options contain transport state: %#v", reconstructed.Options)
	}
	if reconstructed.Options.Temperature == nil ||
		*reconstructed.Options.Temperature != temperature ||
		reconstructed.Options.MaxTokens != 512 ||
		reconstructed.Options.Reasoning != llm.ModelThinkingHigh {
		t.Fatalf("reconstructed semantic options = %#v", reconstructed.Options)
	}
	protocolOptions, ok := reconstructed.Options.ProtocolOptions.(*llm.OpenAIResponsesStreamOptions)
	if !ok || protocolOptions.ThinkingDisplay != llm.ThinkingDisplayOmitted ||
		protocolOptions.ToolChoice != llm.OpenAIToolChoiceRequired {
		t.Fatalf("reconstructed protocol options = %#v", reconstructed.Options.ProtocolOptions)
	}

	encoded, err := json.Marshal(entries[len(entries)-1])
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{
		"private-api-key", "private-route.example", "private-env-key", "private-token",
	} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("request header contains transport secret %q: %s", secret, encoded)
		}
	}
}

func TestCaptureRequestOptionsRejectsBodyRewrite(t *testing.T) {
	_, err := CaptureRequestOptions(
		llm.ProtocolOpenAIResponses,
		nil,
		llm.StreamOptions{RewriteRequest: func(string, string, []byte) []byte { return nil }},
	)
	if err == nil || !strings.Contains(err.Error(), "cannot be represented durably") {
		t.Fatalf("CaptureRequestOptions() error = %v", err)
	}
}

func TestSessionValidatorRejectsInvalidRequestHeaderOrdering(t *testing.T) {
	validHeader := RequestHeader{
		ProviderRequestID: "request-1",
		RunID:             "run-1", TurnID: "turn-1", StepID: "step-1",
		Provider: "openai", Model: "gpt-test", Protocol: llm.ProtocolOpenAIResponses,
	}
	tests := []struct {
		name    string
		entries []Entry
		want    string
	}{
		{
			name: "outside step",
			entries: []Entry{
				NewRunStart("run-1"),
				NewTurnStart("run-1", "turn-1"),
				NewRequestHeader(validHeader),
			},
			want: "has no open step",
		},
		{
			name: "mismatched scope",
			entries: []Entry{
				NewRunStart("run-1"),
				NewTurnStart("run-1", "turn-1"),
				NewStepStart("run-1", "turn-1", "step-1"),
				NewRequestHeader(func() RequestHeader {
					header := validHeader
					header.StepID = "step-other"
					return header
				}()),
			},
			want: "does not match open step",
		},
		{
			name: "unknown attachment",
			entries: []Entry{
				NewRunStart("run-1"),
				NewTurnStart("run-1", "turn-1"),
				NewStepStart("run-1", "turn-1", "step-1"),
				NewRequestHeader(func() RequestHeader {
					header := validHeader
					header.Attachments = []RequestAttachment{{
						AttachmentID: "missing", MessageIndex: 0,
					}}
					return header
				}()),
			},
			want: "unknown context attachment",
		},
		{
			name: "second request in step",
			entries: []Entry{
				NewRunStart("run-1"),
				NewTurnStart("run-1", "turn-1"),
				NewStepStart("run-1", "turn-1", "step-1"),
				NewRequestHeader(validHeader),
				NewRequestHeader(func() RequestHeader {
					header := validHeader
					header.ProviderRequestID = "request-2"
					return header
				}()),
			},
			want: "second provider request",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			entries := sequencedForTest(test.entries...)
			if _, err := ValidateSession(entries); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateSession() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRequestHeaderSequenceIsReassignedWithTranscript(t *testing.T) {
	header := NewRequestHeader(RequestHeader{
		ProviderRequestID: "request-1",
		RunID:             "run-1", TurnID: "turn-1", StepID: "step-1",
		Provider: "openai", Model: "gpt-test", Protocol: llm.ProtocolOpenAIResponses,
	})
	first, err := SequenceEntries([]Entry{header}, 5)
	if err != nil {
		t.Fatal(err)
	}
	second, err := SequenceEntries(first, 9)
	if err != nil {
		t.Fatal(err)
	}
	if header.RequestHeader.InputSeq != unassignedSequence ||
		first[0].RequestHeader.InputSeq != 4 || second[0].RequestHeader.InputSeq != 8 {
		t.Fatalf(
			"input sequences = original %d first %d second %d",
			header.RequestHeader.InputSeq,
			first[0].RequestHeader.InputSeq,
			second[0].RequestHeader.InputSeq,
		)
	}
}

func assertUserText(t *testing.T, message llm.Message, want string) {
	t.Helper()
	user, ok := message.(*llm.UserMessage)
	if !ok || len(user.Content) != 1 {
		t.Fatalf("message = %#v, want one user text block", message)
	}
	text, ok := user.Content[0].(*llm.TextContent)
	if !ok || text.Text != want {
		t.Fatalf("message content = %#v, want %q", user.Content, want)
	}
}
