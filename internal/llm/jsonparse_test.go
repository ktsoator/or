package llm

import (
	"strings"
	"testing"
)

func TestParseToolArguments(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: ""},
		{name: "valid", raw: `{"city":"Paris"}`, want: "Paris"},
		{name: "repairable control character", raw: "{\"city\":\"Par\nis\"}", want: "Par\nis"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			arguments, err := ParseToolArguments(test.raw)
			if err != nil {
				t.Fatalf("ParseToolArguments() error = %v", err)
			}
			if test.want != "" && arguments["city"] != test.want {
				t.Fatalf("city = %#v, want %q", arguments["city"], test.want)
			}
		})
	}
}

func TestParseToolArgumentsRejectsMalformedJSON(t *testing.T) {
	arguments, err := ParseToolArguments(`{"city":`)
	if err == nil {
		t.Fatalf("ParseToolArguments() = %#v, want error", arguments)
	}
	if !strings.Contains(err.Error(), "parse tool arguments") {
		t.Fatalf("ParseToolArguments() error = %q", err)
	}
}
