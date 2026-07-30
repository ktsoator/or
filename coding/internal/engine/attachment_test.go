package engine

import (
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

func TestAttachedFilesReachModelAndStayOutOfVisibleText(t *testing.T) {
	files := []AttachedFile{{
		File:    File{Name: "main.go", MIMEType: "text/plain", Size: 27},
		Content: "package main\n\nfunc main() {}",
	}}
	message := userMessage(
		"review this",
		files,
		[]llm.ImageContent{{Data: "AAAA", MIMEType: "image/png"}},
	)

	projected, ok := agent.ToLLM(message)
	if !ok {
		t.Fatal("message has no LLM projection")
	}
	user, ok := projected.(*llm.UserMessage)
	if !ok {
		t.Fatalf("projected message = %T, want *llm.UserMessage", projected)
	}
	if len(user.Content) != 3 {
		t.Fatalf("content blocks = %d, want text, files, and image", len(user.Content))
	}
	context, ok := user.Content[1].(*llm.TextContent)
	if !ok ||
		!strings.HasPrefix(context.Text, attachedFilesContextPrefix) ||
		!strings.Contains(context.Text, `"name":"main.go"`) ||
		!strings.Contains(context.Text, files[0].Content) {
		t.Fatalf("attachment context = %#v", user.Content[1])
	}

	text, images, metadata := userMessageContent(user)
	if text != "review this" {
		t.Fatalf("visible text = %q", text)
	}
	if len(images) != 1 || images[0].MIMEType != "image/png" {
		t.Fatalf("images = %#v", images)
	}
	if len(metadata) != 1 || metadata[0] != files[0].File {
		t.Fatalf("file metadata = %#v", metadata)
	}
}

func TestMalformedAttachedFileContextRemainsHidden(t *testing.T) {
	text, _, files := userMessageContent(&llm.UserMessage{
		Content: []llm.UserContent{
			&llm.TextContent{Text: "visible"},
			&llm.TextContent{Text: attachedFilesContextPrefix + "invalid"},
		},
	})
	if text != "visible" || len(files) != 0 {
		t.Fatalf("visible text/files = %q/%#v", text, files)
	}
}

func TestUserAuthoredAttachedFilePrefixRemainsVisible(t *testing.T) {
	authored := attachedFilesContextPrefix + "user text"
	text, _, files := userMessageContent(&llm.UserMessage{
		Content: []llm.UserContent{&llm.TextContent{Text: authored}},
	})
	if text != authored || len(files) != 0 {
		t.Fatalf("visible text/files = %q/%#v", text, files)
	}
}
