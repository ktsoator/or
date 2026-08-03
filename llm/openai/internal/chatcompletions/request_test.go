package chatcompletions

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ktsoator/or/llm"
	oai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

func TestMergeExtraFieldsPreservesExisting(t *testing.T) {
	params := oai.ChatCompletionNewParams{}
	params.SetExtraFields(map[string]any{"keep": 1, "shared": "old"})

	mergeExtraFields(&params, map[string]any{"shared": "new", "added": true})

	got := params.ExtraFields()
	if got["keep"] != 1 {
		t.Errorf("keep field lost: %#v", got)
	}
	if got["shared"] != "new" {
		t.Errorf("shared field not overridden: %#v", got["shared"])
	}
	if got["added"] != true {
		t.Errorf("added field missing: %#v", got)
	}
}

func TestMergeExtraFieldsAcceptsNilStartingMap(t *testing.T) {
	// SetExtraFields was never called, so the SDK starts with no map at all.
	params := oai.ChatCompletionNewParams{}
	mergeExtraFields(&params, map[string]any{"a": 1})

	if got := params.ExtraFields()["a"]; got != 1 {
		t.Fatalf("a = %#v, want 1", got)
	}
}

func TestBuildParamsMaxTokensRoutesByCompat(t *testing.T) {
	model := llm.Model{ID: "x", Protocol: llm.ProtocolOpenAICompletions}
	opts := llm.StreamOptions{MaxTokens: 128}

	std := buildParams(model, nil, nil, opts, resolvedCompat{maxTokensField: "max_completion_tokens"})
	rawStd, _ := json.Marshal(std)
	wireStd := string(rawStd)
	if !strings.Contains(wireStd, `"max_completion_tokens":128`) {
		t.Errorf("wanted max_completion_tokens=128 in %s", wireStd)
	}
	if strings.Contains(wireStd, `"max_tokens":128`) {
		t.Errorf("must not emit max_tokens for standard compat: %s", wireStd)
	}

	legacy := buildParams(model, nil, nil, opts, resolvedCompat{maxTokensField: "max_tokens"})
	rawLegacy, _ := json.Marshal(legacy)
	wireLegacy := string(rawLegacy)
	if !strings.Contains(wireLegacy, `"max_tokens":128`) {
		t.Errorf("wanted max_tokens=128 in %s", wireLegacy)
	}
}

func TestBuildParamsSetsStoreForOpenAIDefault(t *testing.T) {
	// supportsStore is the default for native OpenAI: an explicit store=false
	// keeps the conversation transient.
	params := buildParams(
		llm.Model{ID: "x", Protocol: llm.ProtocolOpenAICompletions},
		nil, nil, llm.StreamOptions{},
		resolvedCompat{supportsStore: true},
	)
	raw, _ := json.Marshal(params)
	if !strings.Contains(string(raw), `"store":false`) {
		t.Fatalf("wanted store=false in %s", raw)
	}
}

func TestBuildParamsOmitsStoreForNonStandard(t *testing.T) {
	params := buildParams(
		llm.Model{ID: "x", Protocol: llm.ProtocolOpenAICompletions},
		nil, nil, llm.StreamOptions{},
		resolvedCompat{supportsStore: false},
	)
	raw, _ := json.Marshal(params)
	if strings.Contains(string(raw), `"store"`) {
		t.Fatalf("store must be omitted for non-standard: %s", raw)
	}
}

func TestBuildParamsTemperatureAndProtocolOptions(t *testing.T) {
	temp := 0.42
	opts := llm.StreamOptions{
		Temperature:     &temp,
		ProtocolOptions: &llm.OpenAICompletionsStreamOptions{ToolChoice: llm.OpenAIToolChoiceRequired},
	}
	params := buildParams(
		llm.Model{ID: "x", Protocol: llm.ProtocolOpenAICompletions},
		nil, nil, opts,
		resolvedCompat{},
	)
	raw, _ := json.Marshal(params)
	wire := string(raw)
	if !strings.Contains(wire, `"temperature":0.42`) {
		t.Errorf("wanted temperature: %s", wire)
	}
	if !strings.Contains(wire, `"tool_choice":"required"`) {
		t.Errorf("wanted tool_choice=required: %s", wire)
	}
}

func TestBuildParamsAddsZAIToolStreamWhenToolsAndCompatSet(t *testing.T) {
	tools := []oai.ChatCompletionToolUnionParam{oai.ChatCompletionFunctionTool(
		shared.FunctionDefinitionParam{Name: "noop"},
	)}
	params := buildParams(
		llm.Model{ID: "x", Protocol: llm.ProtocolOpenAICompletions},
		nil, tools, llm.StreamOptions{},
		resolvedCompat{zaiToolStream: true},
	)
	if got := params.ExtraFields()["tool_stream"]; got != true {
		t.Fatalf("tool_stream = %#v, want true", got)
	}
}

func TestBuildParamsSkipsZAIToolStreamWithoutTools(t *testing.T) {
	params := buildParams(
		llm.Model{ID: "x", Protocol: llm.ProtocolOpenAICompletions},
		nil, nil, llm.StreamOptions{},
		resolvedCompat{zaiToolStream: true},
	)
	if _, present := params.ExtraFields()["tool_stream"]; present {
		t.Fatalf("tool_stream must not be set without tools")
	}
}

func TestBuildParamsAlwaysIncludesUsageInStream(t *testing.T) {
	params := buildParams(
		llm.Model{ID: "x", Protocol: llm.ProtocolOpenAICompletions},
		nil, nil, llm.StreamOptions{},
		resolvedCompat{},
	)
	raw, _ := json.Marshal(params)
	if !strings.Contains(string(raw), `"include_usage":true`) {
		t.Fatalf("stream_options.include_usage must be true: %s", raw)
	}
}
