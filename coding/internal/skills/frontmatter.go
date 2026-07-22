package skills

import (
	"fmt"
	"strings"

	"github.com/goccy/go-yaml"
)

// frontmatter is the subset of SKILL.md YAML frontmatter the first version
// understands. Additional fields (allowed-tools, hooks, ...) are intentionally
// ignored for now.
type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// parseSKILL splits a SKILL.md file into its frontmatter and body. It requires a
// leading YAML frontmatter block delimited by lines containing only "---".
func parseSKILL(raw string) (frontmatter, string, error) {
	// Tolerate a leading UTF-8 BOM and a CRLF opening fence.
	raw = strings.TrimPrefix(raw, "\ufeff")
	rest, ok := strings.CutPrefix(raw, "---\n")
	if !ok {
		rest, ok = strings.CutPrefix(raw, "---\r\n")
	}
	if !ok {
		return frontmatter{}, "", fmt.Errorf("missing YAML frontmatter (expected a leading '---' line)")
	}

	end := findClosingFence(rest)
	if end.start < 0 {
		return frontmatter{}, "", fmt.Errorf("unterminated YAML frontmatter (missing closing '---' line)")
	}
	block := rest[:end.start]
	body := rest[end.next:]

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(block), &fm); err != nil {
		return frontmatter{}, "", fmt.Errorf("parse frontmatter: %w", err)
	}
	return fm, strings.TrimLeft(body, "\n"), nil
}

type fence struct {
	start int // index of the closing fence line within rest
	next  int // index just past the fence line (start of body)
}

// findClosingFence locates the first line that is exactly "---" (ignoring a
// trailing CR), returning where it starts and where the following body begins.
func findClosingFence(rest string) fence {
	for offset := 0; offset < len(rest); {
		nl := strings.IndexByte(rest[offset:], '\n')
		var line string
		var lineEnd int
		if nl < 0 {
			line = rest[offset:]
			lineEnd = len(rest)
		} else {
			line = rest[offset : offset+nl]
			lineEnd = offset + nl + 1
		}
		if strings.TrimRight(line, "\r") == "---" {
			return fence{start: offset, next: lineEnd}
		}
		if nl < 0 {
			break
		}
		offset = lineEnd
	}
	return fence{start: -1, next: -1}
}
