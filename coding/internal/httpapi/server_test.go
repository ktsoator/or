package httpapi

import "testing"

func TestMaskAPIKey(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: "", want: ""},
		{name: "short secret reveals nothing", value: "secret", want: "••••••••"},
		{name: "long secret keeps only edges", value: "sk-example-123456", want: "sk-••••3456"},
		{name: "surrounding whitespace is ignored", value: "  token-abcdef  ", want: "tok••••cdef"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := maskAPIKey(test.value); got != test.want {
				t.Fatalf("maskAPIKey(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}
