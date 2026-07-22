package skills

import (
	"fmt"
	"regexp"
	"strings"
)

var positionalArg = regexp.MustCompile(`\$(\d+)`)

// skillDirPlaceholder expands to a skill's absolute directory, so a skill body
// can reference bundled files (scripts/, references/) with an absolute path that
// the coding tools — which resolve relative paths against the workspace root —
// can open.
const skillDirPlaceholder = "${OR_SKILL_DIR}"

// Expand substitutes placeholders in a skill body:
//
//   - $ARGUMENTS and $@ expand to the full argument string.
//   - $1..$N expand to whitespace-separated fields of the argument string.
//   - ${OR_SKILL_DIR} expands to skillDir.
//
// A positional placeholder with no matching field expands to the empty string.
func Expand(content, skillDir, arguments string) string {
	fields := strings.Fields(arguments)
	content = positionalArg.ReplaceAllStringFunc(content, func(match string) string {
		var index int
		fmt.Sscanf(match, "$%d", &index)
		if index >= 1 && index <= len(fields) {
			return fields[index-1]
		}
		return ""
	})
	content = strings.ReplaceAll(content, "$ARGUMENTS", arguments)
	content = strings.ReplaceAll(content, "$@", arguments)
	content = strings.ReplaceAll(content, skillDirPlaceholder, skillDir)
	return content
}
