package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// ToolName is the advertised name of the skill-loading tool.
const ToolName = "skill"

const toolDescription = "Load a skill's full instructions on demand. The system prompt lists the " +
	"available skills by name and description; when the current task matches one, call this tool " +
	"with that skill's name BEFORE acting, then follow the instructions it returns. Only names from " +
	"the listing are valid — never guess. Pass any extra task detail via the optional arguments field."

type skillCallArgs struct {
	Name      string `json:"name" jsonschema:"description=Name of the skill to load, exactly as shown in the available skills listing,minLength=1"`
	Arguments string `json:"arguments,omitempty" jsonschema:"description=Optional free-form detail for the skill, substituted into its $ARGUMENTS placeholder"`
}

// Tool returns the agent tool that loads a skill's body on demand. The returned
// tool only reads registered skills; it makes no workspace changes, so callers
// should advertise it as read-only. On an unknown name it returns an error
// naming the valid skills, so the model corrects rather than guesses.
func (r *Registry) Tool() agent.AgentTool {
	return agent.AgentTool{
		Definition: llm.MustTool[skillCallArgs](ToolName, toolDescription),
		Label:      "Skill",
		Execute: func(_ context.Context, _ string, raw json.RawMessage, _ func(agent.ToolResult)) (agent.ToolResult, error) {
			var in skillCallArgs
			if err := json.Unmarshal(raw, &in); err != nil {
				return agent.ToolResult{}, err
			}
			name := strings.TrimSpace(in.Name)
			s, ok := r.Lookup(name)
			if !ok {
				msg := unknownSkillMessage(name, r.names())
				return textResult(msg), fmt.Errorf("%s", msg)
			}
			return textResult(formatLoadedSkill(s, in.Arguments)), nil
		},
	}
}

// formatLoadedSkill renders a skill's expanded body as the tool result the model
// reads, wrapped so the boundary of the injected instructions is explicit.
func formatLoadedSkill(s Skill, arguments string) string {
	body := Expand(s.Content, s.Dir, arguments)
	return fmt.Sprintf("<loaded_skill name=%q root=%q>\n%s\n</loaded_skill>\n\nFollow the loaded skill instructions for the current task.",
		s.Name, s.Dir, body)
}

// unknownSkillMessage explains an unknown skill name and lists the valid ones.
func unknownSkillMessage(name string, valid []string) string {
	if len(valid) == 0 {
		return fmt.Sprintf("Unknown skill %q: no skills are available.", name)
	}
	return fmt.Sprintf("Unknown skill %q. Available skills: %s.", name, strings.Join(valid, ", "))
}

// textResult builds a ToolResult carrying a single text block.
func textResult(text string) agent.ToolResult {
	return agent.ToolResult{
		Content: []llm.ToolResultContent{&llm.TextContent{Text: text}},
	}
}
