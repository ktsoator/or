package skills

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-yaml"
)

const (
	maxSkillNameChars          = 64
	maxSkillDescriptionChars   = 1024
	maxSkillCompatibilityChars = 500
)

var validSkillName = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

var standardFrontmatterFields = map[string]struct{}{
	"name":          {},
	"description":   {},
	"license":       {},
	"compatibility": {},
	"metadata":      {},
	"allowed-tools": {},
}

// frontmatter is the complete Agent Skills frontmatter defined by
// https://agentskills.io/specification.
type frontmatter struct {
	Name          string
	Description   string
	License       string
	Compatibility string
	Metadata      map[string]string
	AllowedTools  string
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

	var fields map[string]any
	if err := yaml.Unmarshal([]byte(block), &fields); err != nil {
		return frontmatter{}, "", fmt.Errorf("parse frontmatter: %w", err)
	}
	fm, err := decodeFrontmatter(fields)
	if err != nil {
		return frontmatter{}, "", err
	}
	return fm, body, nil
}

func decodeFrontmatter(fields map[string]any) (frontmatter, error) {
	unknown := make([]string, 0)
	for name := range fields {
		if _, ok := standardFrontmatterFields[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return frontmatter{}, fmt.Errorf("frontmatter contains unsupported field: %s", unknown[0])
	}

	name, err := requiredString(fields, "name")
	if err != nil {
		return frontmatter{}, err
	}
	if count := utf8.RuneCountInString(name); count > maxSkillNameChars {
		return frontmatter{}, fmt.Errorf("frontmatter field name must be at most %d characters", maxSkillNameChars)
	}
	if !validSkillName.MatchString(name) {
		return frontmatter{}, fmt.Errorf("frontmatter field name must contain only lowercase letters, numbers, and single hyphens, and must not start or end with a hyphen")
	}

	description, err := requiredString(fields, "description")
	if err != nil {
		return frontmatter{}, err
	}
	if count := utf8.RuneCountInString(description); count > maxSkillDescriptionChars {
		return frontmatter{}, fmt.Errorf("frontmatter field description must be at most %d characters", maxSkillDescriptionChars)
	}

	license, err := optionalString(fields, "license")
	if err != nil {
		return frontmatter{}, err
	}
	compatibility, err := optionalString(fields, "compatibility")
	if err != nil {
		return frontmatter{}, err
	}
	if _, present := fields["compatibility"]; present {
		if strings.TrimSpace(compatibility) == "" {
			return frontmatter{}, fmt.Errorf("frontmatter field compatibility must not be empty")
		}
		if utf8.RuneCountInString(compatibility) > maxSkillCompatibilityChars {
			return frontmatter{}, fmt.Errorf("frontmatter field compatibility must be at most %d characters", maxSkillCompatibilityChars)
		}
	}
	allowedTools, err := optionalString(fields, "allowed-tools")
	if err != nil {
		return frontmatter{}, err
	}
	metadata, err := metadataStrings(fields)
	if err != nil {
		return frontmatter{}, err
	}

	return frontmatter{
		Name:          name,
		Description:   description,
		License:       license,
		Compatibility: compatibility,
		Metadata:      metadata,
		AllowedTools:  allowedTools,
	}, nil
}

func requiredString(fields map[string]any, name string) (string, error) {
	value, ok := fields[name]
	if !ok {
		return "", fmt.Errorf("frontmatter is missing required field: %s", name)
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("frontmatter field %s must be a string", name)
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("frontmatter field %s must not be empty", name)
	}
	return text, nil
}

func optionalString(fields map[string]any, name string) (string, error) {
	value, ok := fields[name]
	if !ok {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("frontmatter field %s must be a string", name)
	}
	return text, nil
}

func metadataStrings(fields map[string]any) (map[string]string, error) {
	value, present := fields["metadata"]
	if !present {
		return nil, nil
	}
	entries, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("frontmatter field metadata must be a mapping of string keys to string values")
	}
	metadata := make(map[string]string, len(entries))
	for key, value := range entries {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("frontmatter metadata value for %q must be a string", key)
		}
		metadata[key] = text
	}
	return metadata, nil
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
