package llm

import "testing"

func TestParseToolArguments(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: ""},
		{name: "valid", raw: `{"city":"Paris"}`, want: "Paris"},
		{name: "repairable control character", raw: "{\"city\":\"Par\nis\"}", want: "Par\nis"},
		{name: "truncated value salvaged", raw: `{"city":"Par`, want: "Par"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			arguments := ParseToolArguments(test.raw)
			if arguments == nil {
				t.Fatal("ParseToolArguments() = nil, want non-nil map")
			}
			if test.want != "" && arguments["city"] != test.want {
				t.Fatalf("city = %#v, want %q", arguments["city"], test.want)
			}
		})
	}
}

// Malformed input that cannot be salvaged degrades to an empty object rather
// than failing, so a recoverable tool call never aborts the stream.
func TestParseToolArgumentsDegradesToEmptyObject(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "key without value", raw: `{"city":`},
		{name: "not an object", raw: `not json at all`},
		{name: "bare array", raw: `[1,2,3]`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			arguments := ParseToolArguments(test.raw)
			if arguments == nil {
				t.Fatal("ParseToolArguments() = nil, want non-nil map")
			}
			if len(arguments) != 0 {
				t.Fatalf("ParseToolArguments() = %#v, want empty object", arguments)
			}
		})
	}
}

func TestParseToolArgumentsMode(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want ArgumentsMode
	}{
		{name: "empty is strict", raw: "", want: ArgumentsStrict},
		{name: "valid is strict", raw: `{"city":"Paris"}`, want: ArgumentsStrict},
		{name: "control char is repaired", raw: "{\"city\":\"Par\nis\"}", want: ArgumentsRepaired},
		{name: "truncated is partial", raw: `{"city":"Par`, want: ArgumentsPartial},
		{name: "garbage is invalid", raw: `not json at all`, want: ArgumentsInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, mode := ParseToolArgumentsMode(test.raw); mode != test.want {
				t.Fatalf("mode = %q, want %q", mode, test.want)
			}
		})
	}
}

func TestToolArgumentsDiagnostic(t *testing.T) {
	if _, ok := ToolArgumentsDiagnostic("id", "name", ArgumentsStrict); ok {
		t.Fatal("strict parse should not produce a diagnostic")
	}
	diagnostic, ok := ToolArgumentsDiagnostic("toolu_1", "weather", ArgumentsPartial)
	if !ok {
		t.Fatal("partial parse should produce a diagnostic")
	}
	if diagnostic.Type != DiagnosticToolArgumentsRecovered {
		t.Fatalf("diagnostic type = %q", diagnostic.Type)
	}
	if diagnostic.Details["mode"] != "partial" || diagnostic.Details["toolCallId"] != "toolu_1" {
		t.Fatalf("diagnostic details = %#v", diagnostic.Details)
	}
}

func TestCompleteJSON(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "open object", raw: `{`, want: `{}`},
		{name: "dangling colon", raw: `{"a":`, want: `{}`},
		{name: "complete pair then comma", raw: `{"a":1,`, want: `{"a":1}`},
		{name: "truncated string value", raw: `{"a":"hel`, want: `{"a":"hel"}`},
		{name: "nested array", raw: `{"a":[1,2`, want: `{"a":[1,2]}`},
		{name: "complete number kept", raw: `{"a":12`, want: `{"a":12}`},
		{name: "truncated number drops to safe point", raw: `{"a":1.`, want: `{}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := completeJSON(test.raw)
			if !ok {
				t.Fatalf("completeJSON(%q) ok = false", test.raw)
			}
			if got != test.want {
				t.Fatalf("completeJSON(%q) = %q, want %q", test.raw, got, test.want)
			}
		})
	}
}
