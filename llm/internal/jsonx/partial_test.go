package jsonx

import "testing"

func TestComplete(t *testing.T) {
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
			got, ok := Complete(test.raw)
			if !ok {
				t.Fatalf("Complete(%q) ok = false", test.raw)
			}
			if got != test.want {
				t.Fatalf("Complete(%q) = %q, want %q", test.raw, got, test.want)
			}
		})
	}
}
