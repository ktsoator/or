package compaction

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

const systemPrompt = `You create context checkpoint summaries for a coding agent. Preserve concrete implementation state, user requirements, exact paths, symbols, commands, test results, errors, and unresolved work. Do not continue the task and do not call tools.`

const summaryTemplate = `Create a structured checkpoint summary of the conversation above for another coding agent to continue from.

Use this exact format:

## Goal
## User Requirements
## Constraints
## Completed Work
## Current Work
## Key Decisions
## Files Read
## Files Modified
## Commands and Tests
## Errors and Blockers
## Pending Tasks
## Next Step

Keep it concise, but preserve exact file paths, function names, commands, error messages, and user constraints.`

const updateTemplate = `Update the previous checkpoint summary with the new conversation above. Preserve still-relevant facts from the previous summary, incorporate new progress, and remove only information that is conclusively obsolete.

Use the same section structure already present in the previous summary. Preserve exact file paths, function names, commands, error messages, and user constraints.`

const maxSerializedBlock = 12_000

func buildPrompt(request Request) string {
	var builder strings.Builder
	builder.WriteString("<conversation>\n")
	serializeMessages(&builder, request.Messages)
	builder.WriteString("</conversation>\n\n")
	if request.PreviousSummary != "" {
		builder.WriteString("<previous-summary>\n")
		builder.WriteString(request.PreviousSummary)
		builder.WriteString("\n</previous-summary>\n\n")
		builder.WriteString(updateTemplate)
	} else {
		builder.WriteString(summaryTemplate)
	}
	if strings.TrimSpace(request.Instructions) != "" {
		builder.WriteString("\n\nAdditional focus from the user:\n")
		builder.WriteString(strings.TrimSpace(request.Instructions))
	}
	return builder.String()
}

func serializeMessages(builder *strings.Builder, messages []agent.AgentMessage) {
	for _, wrapped := range messages {
		message, ok := agent.ToLLM(wrapped)
		if !ok {
			continue
		}
		switch typed := message.(type) {
		case *llm.UserMessage:
			builder.WriteString("[user]\n")
			for _, content := range typed.Content {
				switch block := content.(type) {
				case *llm.TextContent:
					if block != nil {
						writeTruncated(builder, block.Text)
					}
				case *llm.ImageContent:
					builder.WriteString("[image]\n")
				}
			}
		case *llm.AssistantMessage:
			builder.WriteString("[assistant]\n")
			for _, content := range typed.Content {
				switch block := content.(type) {
				case *llm.TextContent:
					if block != nil {
						writeTruncated(builder, block.Text)
					}
				case *llm.ThinkingContent:
					if block != nil && block.Thinking != "" {
						builder.WriteString("[reasoning]\n")
						writeTruncated(builder, block.Thinking)
					}
				case *llm.ToolCall:
					if block != nil {
						arguments, _ := json.Marshal(block.Arguments)
						writeTruncated(builder, fmt.Sprintf("[tool call %s id=%s] %s", block.Name, block.ID, arguments))
					}
				}
			}
		case *llm.ToolResultMessage:
			builder.WriteString(fmt.Sprintf("[tool result %s id=%s error=%t]\n", typed.ToolName, typed.ToolCallID, typed.IsError))
			for _, content := range typed.Content {
				switch block := content.(type) {
				case *llm.TextContent:
					if block != nil {
						writeTruncated(builder, block.Text)
					}
				case *llm.ImageContent:
					builder.WriteString("[image]\n")
				}
			}
		}
		builder.WriteByte('\n')
	}
}

func writeTruncated(builder *strings.Builder, value string) {
	if len(value) <= maxSerializedBlock {
		builder.WriteString(value)
		builder.WriteByte('\n')
		return
	}
	const tail = 2_000
	builder.WriteString(value[:maxSerializedBlock-tail])
	builder.WriteString("\n... [truncated for compaction] ...\n")
	builder.WriteString(value[len(value)-tail:])
	builder.WriteByte('\n')
}
