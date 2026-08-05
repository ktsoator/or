package prompttemplate

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ktsoator/or/coding/internal/invocation"
)

const (
	explicitInvocationPrefix       = "[[OR_PROMPT_TEMPLATE_INVOCATION_V1]]\n"
	legacyExplicitInvocationPrefix = "<or-prompt-template-invocation "
)

var placeholderPattern = regexp.MustCompile(`\$\{(\d+|ARGUMENTS|@):-([^}]*)\}|\$\{@:(\d+)(?::(\d+))?\}|\$(ARGUMENTS|@|\d+)`)

type Registry struct {
	order  []string
	byName map[string]Template
}

func NewRegistry(templates []Template) *Registry {
	byName := make(map[string]Template, len(templates))
	for _, template := range templates {
		byName[template.Name] = template
	}
	return newRegistry(byName)
}

func newRegistry(byName map[string]Template) *Registry {
	order := make([]string, 0, len(byName))
	for name := range byName {
		order = append(order, name)
	}
	sort.Strings(order)
	return &Registry{order: order, byName: byName}
}

func (r *Registry) List() []Template {
	if r == nil {
		return nil
	}
	result := make([]Template, len(r.order))
	for index, name := range r.order {
		result[index] = r.byName[name]
	}
	return result
}

func (r *Registry) Lookup(name string) (Template, bool) {
	if r == nil {
		return Template{}, false
	}
	template, ok := r.byName[name]
	return template, ok
}

// ExpandExplicitInvocation recognizes /name followed by optional arguments.
// Unknown slash commands remain ordinary user text.
func (r *Registry) ExpandExplicitInvocation(text string) (string, bool) {
	name, rawArguments, matched := parseInvocation(text)
	if !matched {
		return "", false
	}
	template, ok := r.Lookup(name)
	if !ok {
		return "", false
	}
	expanded := Substitute(template.Content, ParseArguments(rawArguments))
	metadata, _ := json.Marshal(invocation.Record{
		Kind:   invocation.PromptTemplate,
		Name:   template.Name,
		Source: string(template.Source),
		Path:   template.Path,
	})
	return fmt.Sprintf(
		"%s%s\n"+
			"The user explicitly invoked this prompt template. Follow the expanded prompt below.\n\n"+
			"%s",
		explicitInvocationPrefix,
		metadata,
		expanded,
	), true
}

func IsExplicitInvocationText(text string) bool {
	return strings.HasPrefix(text, explicitInvocationPrefix) ||
		strings.HasPrefix(text, legacyExplicitInvocationPrefix)
}

// ParseExplicitInvocationText returns metadata only from the hidden block the
// backend created after resolving and expanding a template. User-authored
// slash text lives in the first content block and is never parsed here.
func ParseExplicitInvocationText(text string) (invocation.Record, bool) {
	if !strings.HasPrefix(text, explicitInvocationPrefix) {
		return invocation.Record{}, false
	}
	rest := strings.TrimPrefix(text, explicitInvocationPrefix)
	line, _, found := strings.Cut(rest, "\n")
	if !found {
		return invocation.Record{}, false
	}
	var record invocation.Record
	if err := json.Unmarshal([]byte(line), &record); err != nil || record.Validate() != nil {
		return invocation.Record{}, false
	}
	return record, true
}

func parseInvocation(text string) (name, arguments string, matched bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "/") {
		return "", "", false
	}
	rest := strings.TrimPrefix(trimmed, "/")
	if split := strings.IndexFunc(rest, func(char rune) bool {
		return char == ' ' || char == '\t' || char == '\r' || char == '\n'
	}); split >= 0 {
		return rest[:split], strings.TrimSpace(rest[split:]), true
	}
	return rest, "", rest != ""
}

// ParseArguments splits a command tail while preserving whitespace inside
// matching single or double quotes, matching Pi's prompt-template behavior.
func ParseArguments(value string) []string {
	var args []string
	var current strings.Builder
	var quote rune
	for _, char := range value {
		switch {
		case quote != 0 && char == quote:
			quote = 0
		case quote != 0:
			current.WriteRune(char)
		case char == '\'' || char == '"':
			quote = char
		case char == ' ' || char == '\t' || char == '\r' || char == '\n':
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(char)
		}
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}

// Substitute expands Pi-compatible positional, all-argument, default, and
// slicing placeholders in one pass so values are never recursively expanded.
func Substitute(content string, args []string) string {
	allArguments := strings.Join(args, " ")
	return placeholderPattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := placeholderPattern.FindStringSubmatch(match)
		if target := parts[1]; target != "" {
			value := allArguments
			if target != "@" && target != "ARGUMENTS" {
				value = positional(args, target)
			}
			if value == "" {
				return parts[2]
			}
			return value
		}
		if startText := parts[3]; startText != "" {
			start, _ := strconv.Atoi(startText)
			start--
			if start < 0 {
				start = 0
			}
			if start >= len(args) {
				return ""
			}
			end := len(args)
			if lengthText := parts[4]; lengthText != "" {
				length, _ := strconv.Atoi(lengthText)
				end = min(start+length, len(args))
			}
			return strings.Join(args[start:end], " ")
		}
		simple := parts[5]
		if simple == "@" || simple == "ARGUMENTS" {
			return allArguments
		}
		return positional(args, simple)
	})
}

func positional(args []string, rawIndex string) string {
	index, err := strconv.Atoi(rawIndex)
	if err != nil || index < 1 || index > len(args) {
		return ""
	}
	return args[index-1]
}
