package harness

import (
	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// TurnInfo describes the run state a system-prompt builder sees when a turn is
// about to start. It lets the prompt depend on the live conversation — the
// current model, reasoning level, advertised tools, and transcript so far.
type TurnInfo struct {
	// Model is the model that will run the upcoming turn.
	Model llm.Model
	// ThinkingLevel is the reasoning effort for the upcoming turn.
	ThinkingLevel llm.ModelThinkingLevel
	// Tools are the tools advertised to the model.
	Tools []agent.AgentTool
	// Messages is the transcript as it stands before the upcoming turn. On the
	// first turn of a run it does not yet include the prompt being submitted; on
	// later turns it includes every message appended so far.
	Messages []agent.AgentMessage
	// Skills are the registered skills. Pass them to FormatSkillsForSystemPrompt
	// to advertise the model-invocable ones in the prompt.
	Skills []Skill
}

// SystemPromptFunc builds the system prompt for an upcoming turn. When set on
// Options it is called before every turn, so the prompt can reflect the current
// model or conversation state, and its result takes precedence over the static
// Options.SystemPrompt.
type SystemPromptFunc func(TurnInfo) string
