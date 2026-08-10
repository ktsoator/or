package main

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/ktsoator/or/coding/internal/mcpclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestLocalMCPServer(t *testing.T) {
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := newTestServer().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	listed, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantTools := map[string]bool{
		"echo": false, "get-image": false, "structured-result": false, "fail": false, "slow": false,
	}
	for _, tool := range listed.Tools {
		if _, ok := wantTools[tool.Name]; ok {
			wantTools[tool.Name] = true
		}
	}
	for name, found := range wantTools {
		if !found {
			t.Errorf("tool %q was not discovered", name)
		}
	}

	echoResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "echo", Arguments: map[string]any{"text": "hello from Or"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := echoResult.Content[0].(*mcp.TextContent).Text; got != "hello from Or" {
		t.Fatalf("echo text = %q", got)
	}

	imageResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "get-image"})
	if err != nil {
		t.Fatal(err)
	}
	if len(imageResult.Content) != 2 {
		t.Fatalf("image content = %#v", imageResult.Content)
	}
	imageContent, ok := imageResult.Content[1].(*mcp.ImageContent)
	if !ok || imageContent.MIMEType != "image/png" {
		t.Fatalf("image content = %#v", imageResult.Content[1])
	}
	decoded, err := png.DecodeConfig(bytes.NewReader(imageContent.Data))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Width != 320 || decoded.Height != 180 {
		t.Fatalf("image dimensions = %dx%d", decoded.Width, decoded.Height)
	}

	structured, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "structured-result", Arguments: map[string]any{"label": "local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, ok := structured.StructuredContent.(map[string]any)
	if !ok || data["label"] != "local" || data["ok"] != true {
		t.Fatalf("structured content = %#v", structured.StructuredContent)
	}

	failed, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "fail"})
	if err != nil {
		t.Fatal(err)
	}
	if !failed.IsError {
		t.Fatalf("failure result = %#v", failed)
	}

	cancelCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	_, err = clientSession.CallTool(cancelCtx, &mcp.CallToolParams{
		Name: "slow", Arguments: map[string]any{"delayMs": 500},
	})
	if err == nil {
		t.Fatal("slow tool ignored cancellation")
	}
}

func TestLocalMCPServerThroughOrProbe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := mcpclient.Probe(ctx, "local-test", mcpclient.ServerConfig{
		Command:        os.Args[0],
		Args:           []string{"-test.run=^TestMCPStdioHelper$"},
		Env:            map[string]string{"OR_MCP_TEST_HELPER": "1"},
		Cwd:            t.TempDir(),
		TimeoutSeconds: 5,
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if result.Transport != "stdio" || len(result.Tools) != 5 {
		t.Fatalf("probe result = %#v", result)
	}
	foundImage := false
	for _, tool := range result.Tools {
		if tool.Original == "get-image" {
			foundImage = true
			if tool.Name != "mcp__local-test__get-image" || tool.Title != "Get test image" {
				t.Fatalf("image tool = %#v", tool)
			}
		}
	}
	if !foundImage {
		t.Fatal("get-image was not discovered through Or")
	}
}

func TestMCPStdioHelper(t *testing.T) {
	if os.Getenv("OR_MCP_TEST_HELPER") != "1" {
		return
	}
	if err := newTestServer().Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
