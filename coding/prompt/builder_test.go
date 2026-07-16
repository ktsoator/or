package prompt

import (
	"strings"
	"testing"
)

func TestBuildAlwaysDisablesEmoji(t *testing.T) {
	for _, instructions := range []string{"", "Custom instructions."} {
		got := Build(Options{Instructions: instructions})
		if !strings.Contains(got, "Never use emojis") {
			t.Fatalf("Build(%q) omitted response style: %q", instructions, got)
		}
	}
}
