package llm

// Convenience constructors for the common case of building a conversation from
// plain text. The underlying Context, Message, and Content types stay fully
// general; these helpers just remove the nesting boilerplate for the frequent
// "send some text" path.

// UserText builds a user message containing a single text block.
func UserText(text string) *UserMessage {
	return &UserMessage{Content: []UserContent{&TextContent{Text: text}}}
}

// AssistantText builds an assistant message containing a single text block. It
// is handy for seeding conversation history with a prior model reply.
func AssistantText(text string) *AssistantMessage {
	return &AssistantMessage{Content: []AssistantContent{&TextContent{Text: text}}}
}

// NewContext assembles a Context from the given messages.
func NewContext(messages ...Message) Context {
	return Context{Messages: messages}
}

// Prompt builds a Context holding a single user text message. It is the shortest
// way to start a one-shot completion.
func Prompt(text string) Context {
	return NewContext(UserText(text))
}

// PromptWithSystem builds a Context with a system prompt and a single user text
// message.
func PromptWithSystem(system, user string) Context {
	context := Prompt(user)
	context.SystemPrompt = system
	return context
}

// UserImage builds a user message containing a single base64-encoded image.
func UserImage(data, mimeType string) *UserMessage {
	return &UserMessage{Content: []UserContent{&ImageContent{Data: data, MIMEType: mimeType}}}
}

// ToolResult builds a tool result message answering the assistant tool call with
// the given ID. The text becomes a single text block in the result.
func ToolResult(callID, toolName, text string) *ToolResultMessage {
	return &ToolResultMessage{
		ToolCallID: callID,
		ToolName:   toolName,
		Content:    []ToolResultContent{&TextContent{Text: text}},
	}
}
