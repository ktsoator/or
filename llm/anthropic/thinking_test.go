package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/ktsoator/or/llm"
)

// captureThinkingRequest streams a reasoning request and returns the decoded
// "thinking" object from the request body the adapter sent, so tests can assert
// on the display field that the SDK writes there.
func captureThinkingRequest(t *testing.T, adaptive bool, display llm.ThinkingDisplay) map[string]any {
	t.Helper()
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		anthropicSSE(w, "message_start", `{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","content":[],"model":"test-model","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":0}}}`)
		anthropicSSE(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		anthropicSSE(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`)
		anthropicSSE(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		anthropicSSE(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":1}}`)
		anthropicSSE(w, "message_stop", `{"type":"message_stop"}`)
	}))
	defer server.Close()

	model := llm.Model{
		ID:        "test-model",
		Protocol:  llm.ProtocolAnthropicMessages,
		Provider:  "test",
		BaseURL:   server.URL,
		Reasoning: true,
		Input:     []llm.ModelInput{llm.Text},
		MaxTokens: 4096,
	}
	if adaptive {
		yes := true
		model.Compatibility = &llm.AnthropicMessagesCompatibility{ForceAdaptiveThinking: &yes}
	}
	stream, err := NewAdapter(nil).Stream(context.Background(), model, anthropicTestContext(), llm.StreamOptions{
		APIKey:          "test",
		Reasoning:       llm.ModelThinkingHigh,
		ProtocolOptions: &llm.AnthropicStreamOptions{ThinkingDisplay: display},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	for range stream {
	}

	var decoded struct {
		Thinking map[string]any `json:"thinking"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode request body: %v\n%s", err, body)
	}
	if decoded.Thinking == nil {
		t.Fatalf("request body has no thinking object: %s", body)
	}
	return decoded.Thinking
}

// Omitted display reaches the wire for adaptive models, alongside thinking:adaptive.
func TestApplyThinkingOmittedAdaptive(t *testing.T) {
	thinking := captureThinkingRequest(t, true, llm.ThinkingDisplayOmitted)
	if thinking["type"] != "adaptive" {
		t.Fatalf("thinking.type = %v, want adaptive", thinking["type"])
	}
	if thinking["display"] != "omitted" {
		t.Fatalf("thinking.display = %v, want omitted", thinking["display"])
	}
}

// Omitted display also reaches budget-based (non-adaptive) thinking; the budget
// still travels.
func TestApplyThinkingOmittedBudget(t *testing.T) {
	thinking := captureThinkingRequest(t, false, llm.ThinkingDisplayOmitted)
	if thinking["type"] != "enabled" {
		t.Fatalf("thinking.type = %v, want enabled", thinking["type"])
	}
	if thinking["display"] != "omitted" {
		t.Fatalf("thinking.display = %v, want omitted", thinking["display"])
	}
	if _, ok := thinking["budget_tokens"]; !ok {
		t.Fatalf("budget thinking lost budget_tokens: %#v", thinking)
	}
}

// An unset display defaults to summarized, matching prior behavior and the API
// default, on both thinking forms.
func TestApplyThinkingDefaultsToSummarized(t *testing.T) {
	for _, adaptive := range []bool{true, false} {
		thinking := captureThinkingRequest(t, adaptive, "")
		if thinking["display"] != "summarized" {
			t.Fatalf("adaptive=%t thinking.display = %v, want summarized", adaptive, thinking["display"])
		}
	}
}

func TestAdaptiveMaxThinkingPayload(t *testing.T) {
	model, ok := llm.LookupModel("anthropic", "claude-sonnet-5")
	if !ok {
		t.Fatal("anthropic/claude-sonnet-5 is missing from the catalog")
	}
	compatibility := resolveCompat(model)
	if !compatibility.forceAdaptiveThinking {
		t.Fatal("claude-sonnet-5 is not marked adaptive")
	}

	params := sdk.MessageNewParams{Model: model.ID, MaxTokens: model.MaxTokens}
	applyThinking(&params, model, compatibility, llm.ModelThinkingMax, "")
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var payload struct {
		Thinking struct {
			Type string `json:"type"`
		} `json:"thinking"`
		OutputConfig struct {
			Effort string `json:"effort"`
		} `json:"output_config"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if payload.Thinking.Type != "adaptive" {
		t.Fatalf("thinking.type = %q, want adaptive", payload.Thinking.Type)
	}
	if payload.OutputConfig.Effort != "max" {
		t.Fatalf("output_config.effort = %q, want max", payload.OutputConfig.Effort)
	}
}

type thinkingRequestPayload struct {
	Thinking *struct {
		Type         string `json:"type"`
		BudgetTokens *int64 `json:"budget_tokens"`
		Display      string `json:"display"`
	} `json:"thinking"`
	OutputConfig *struct {
		Effort string `json:"effort"`
	} `json:"output_config"`
}

func marshalThinkingRequest(
	t *testing.T,
	model llm.Model,
	adaptive bool,
	reasoning llm.ModelThinkingLevel,
) thinkingRequestPayload {
	t.Helper()
	params := sdk.MessageNewParams{Model: model.ID, MaxTokens: model.MaxTokens}
	applyThinking(&params, model, compat{forceAdaptiveThinking: adaptive}, reasoning, "")
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var payload thinkingRequestPayload
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode request: %v\n%s", err, encoded)
	}
	return payload
}

func TestThinkingRequestPayloadMatrix(t *testing.T) {
	max := "max"
	model := llm.Model{
		ID:               "test-model",
		Protocol:         llm.ProtocolAnthropicMessages,
		Reasoning:        true,
		MaxTokens:        32768,
		ThinkingLevelMap: map[llm.ModelThinkingLevel]*string{llm.ModelThinkingMax: &max},
	}
	tests := []struct {
		name       string
		adaptive   bool
		reasoning  llm.ModelThinkingLevel
		wantType   string
		wantBudget *int64
		wantEffort string
	}{
		{name: "budget unset"},
		{name: "budget off", reasoning: llm.ModelThinkingOff, wantType: "disabled"},
		{name: "budget high", reasoning: llm.ModelThinkingHigh, wantType: "enabled", wantBudget: int64Pointer(16384)},
		{name: "budget max", reasoning: llm.ModelThinkingMax, wantType: "enabled", wantBudget: int64Pointer(16384)},
		{name: "adaptive unset", adaptive: true},
		{name: "adaptive off", adaptive: true, reasoning: llm.ModelThinkingOff, wantType: "disabled"},
		{name: "adaptive high", adaptive: true, reasoning: llm.ModelThinkingHigh, wantType: "adaptive", wantEffort: "high"},
		{name: "adaptive max", adaptive: true, reasoning: llm.ModelThinkingMax, wantType: "adaptive", wantEffort: "max"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := marshalThinkingRequest(t, model, test.adaptive, test.reasoning)
			if test.wantType == "" {
				if payload.Thinking != nil || payload.OutputConfig != nil {
					t.Fatalf("unset thinking payload = %#v, %#v; want both absent", payload.Thinking, payload.OutputConfig)
				}
				return
			}
			if payload.Thinking == nil || payload.Thinking.Type != test.wantType {
				t.Fatalf("thinking = %#v, want type %q", payload.Thinking, test.wantType)
			}
			if !reflect.DeepEqual(payload.Thinking.BudgetTokens, test.wantBudget) {
				t.Errorf("budget_tokens = %v, want %v", payload.Thinking.BudgetTokens, test.wantBudget)
			}
			if test.wantType == "enabled" || test.wantType == "adaptive" {
				if payload.Thinking.Display != "summarized" {
					t.Errorf("thinking.display = %q, want summarized", payload.Thinking.Display)
				}
			} else if payload.Thinking.Display != "" {
				t.Errorf("disabled thinking.display = %q, want absent", payload.Thinking.Display)
			}
			if test.wantEffort == "" {
				if payload.OutputConfig != nil {
					t.Fatalf("output_config = %#v, want absent", payload.OutputConfig)
				}
			} else if payload.OutputConfig == nil || payload.OutputConfig.Effort != test.wantEffort {
				t.Fatalf("output_config = %#v, want effort %q", payload.OutputConfig, test.wantEffort)
			}
		})
	}
}

func TestThinkingRequestClampsUnsupportedLevels(t *testing.T) {
	high := "high"
	model := llm.Model{
		ID:        "always-thinking",
		Protocol:  llm.ProtocolAnthropicMessages,
		Reasoning: true,
		MaxTokens: 32768,
		ThinkingLevelMap: map[llm.ModelThinkingLevel]*string{
			llm.ModelThinkingOff:     nil,
			llm.ModelThinkingMinimal: nil,
			llm.ModelThinkingLow:     nil,
			llm.ModelThinkingMedium:  nil,
			llm.ModelThinkingHigh:    &high,
			llm.ModelThinkingXHigh:   nil,
			llm.ModelThinkingMax:     nil,
		},
	}

	for _, adaptive := range []bool{false, true} {
		name := "budget"
		if adaptive {
			name = "adaptive"
		}
		t.Run(name+" off clamps high", func(t *testing.T) {
			payload := marshalThinkingRequest(t, model, adaptive, llm.ModelThinkingOff)
			if payload.Thinking == nil {
				t.Fatal("thinking is absent")
			}
			wantType := "enabled"
			if adaptive {
				wantType = "adaptive"
			}
			if payload.Thinking.Type != wantType {
				t.Fatalf("thinking.type = %q, want %q", payload.Thinking.Type, wantType)
			}
			if adaptive && (payload.OutputConfig == nil || payload.OutputConfig.Effort != "high") {
				t.Fatalf("output_config = %#v, want effort high", payload.OutputConfig)
			}
			if !thinkingActive(model, llm.ModelThinkingOff) {
				t.Fatal("clamped unsupported off must be active")
			}
		})
	}

	payload := marshalThinkingRequest(t, model, true, llm.ModelThinkingMax)
	if payload.OutputConfig == nil || payload.OutputConfig.Effort != "high" {
		t.Fatalf("unsupported max output_config = %#v, want effort high", payload.OutputConfig)
	}
}

func TestMiniMaxThinkingOffPayloads(t *testing.T) {
	m27, ok := llm.LookupModel("minimax-cn", "MiniMax-M2.7")
	if !ok {
		t.Fatal("minimax-cn/MiniMax-M2.7 is missing from the catalog")
	}
	m27Payload := marshalThinkingRequest(t, m27, false, llm.ModelThinkingOff)
	if m27Payload.Thinking == nil || m27Payload.Thinking.Type != "enabled" {
		t.Fatalf("M2.7 thinking = %#v, want enabled after unsupported off is clamped", m27Payload.Thinking)
	}
	if m27Payload.Thinking.BudgetTokens == nil {
		t.Fatal("M2.7 enabled thinking is missing budget_tokens")
	}

	m3, ok := llm.LookupModel("minimax-cn", "MiniMax-M3")
	if !ok {
		t.Fatal("minimax-cn/MiniMax-M3 is missing from the catalog")
	}
	m3Payload := marshalThinkingRequest(t, m3, false, llm.ModelThinkingOff)
	if m3Payload.Thinking == nil || m3Payload.Thinking.Type != "disabled" {
		t.Fatalf("M3 thinking = %#v, want disabled", m3Payload.Thinking)
	}
}

func TestKimiCodingAdaptiveThinkingPayloads(t *testing.T) {
	tests := []struct {
		modelID string
		levels  []llm.ModelThinkingLevel
	}{
		{modelID: "k3", levels: []llm.ModelThinkingLevel{llm.ModelThinkingHigh, llm.ModelThinkingMax}},
		{modelID: "k3-256k", levels: []llm.ModelThinkingLevel{llm.ModelThinkingHigh, llm.ModelThinkingMax}},
		{modelID: "kimi-for-coding", levels: []llm.ModelThinkingLevel{llm.ModelThinkingMedium}},
		{modelID: "kimi-for-coding-highspeed", levels: []llm.ModelThinkingLevel{llm.ModelThinkingMedium}},
	}
	for _, test := range tests {
		t.Run(test.modelID, func(t *testing.T) {
			modelID := test.modelID
			model, ok := llm.LookupModel("kimi-coding", modelID)
			if !ok {
				t.Fatalf("kimi-coding/%s is missing from the catalog", modelID)
			}
			compatibility := resolveCompat(model)
			if !compatibility.forceAdaptiveThinking {
				t.Fatal("model is not marked adaptive")
			}

			for _, level := range test.levels {
				t.Run(string(level), func(t *testing.T) {
					payload := marshalThinkingRequest(t, model, compatibility.forceAdaptiveThinking, level)
					if payload.Thinking == nil || payload.Thinking.Type != "adaptive" {
						t.Fatalf("thinking = %#v, want adaptive", payload.Thinking)
					}
					if payload.OutputConfig == nil || payload.OutputConfig.Effort != string(level) {
						t.Fatalf("output_config = %#v, want effort %s", payload.OutputConfig, level)
					}
				})
			}
		})
	}
}

func TestThinkingActiveUsesClampedLevel(t *testing.T) {
	high := "high"
	model := llm.Model{
		Reasoning: true,
		ThinkingLevelMap: map[llm.ModelThinkingLevel]*string{
			llm.ModelThinkingOff:     nil,
			llm.ModelThinkingMinimal: nil,
			llm.ModelThinkingLow:     nil,
			llm.ModelThinkingMedium:  nil,
			llm.ModelThinkingHigh:    &high,
		},
	}
	if thinkingActive(model, "") {
		t.Fatal("unset thinking must not be active")
	}
	if !thinkingActive(model, llm.ModelThinkingOff) {
		t.Fatal("unsupported off clamps to high and must be active")
	}
	model.ThinkingLevelMap = nil
	if thinkingActive(model, llm.ModelThinkingOff) {
		t.Fatal("supported off must not be active")
	}
}

func int64Pointer(value int64) *int64 { return &value }
