// Command mcp-test-server runs a deterministic local MCP server for manually
// exercising Or's tool discovery and result handling.
package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	serverName    = "or-local-test"
	serverVersion = "0.1.0"
)

type echoInput struct {
	Text string `json:"text" jsonschema:"Text to return unchanged"`
}

type structuredInput struct {
	Label string `json:"label" jsonschema:"Label to include in the structured result"`
}

type structuredOutput struct {
	Label  string `json:"label" jsonschema:"The supplied label"`
	Length int    `json:"length" jsonschema:"UTF-8 byte length of the supplied label"`
	OK     bool   `json:"ok" jsonschema:"Whether the operation succeeded"`
}

type failInput struct {
	Message string `json:"message,omitempty" jsonschema:"Optional failure message"`
}

type slowInput struct {
	DelayMS int `json:"delayMs" jsonschema:"Delay in milliseconds, from 0 to 30000"`
}

func main() {
	if err := newTestServer().Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Printf("MCP test server failed: %v", err)
	}
}

func newTestServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Version: serverVersion,
	}, nil)

	mcp.AddTool(server, testTool("echo", "Echo text", "Returns the supplied text unchanged."), echo)
	mcp.AddTool(server, testTool("get-image", "Get test image", "Returns a deterministic PNG image for testing image results."), getImage)
	mcp.AddTool(server, testTool("structured-result", "Get structured result", "Returns text together with typed structured content."), structuredResult)
	mcp.AddTool(server, testTool("fail", "Return expected failure", "Returns an MCP tool error for testing failure presentation."), fail)
	mcp.AddTool(server, testTool("slow", "Wait before responding", "Waits for a requested duration and respects cancellation."), slow)

	return server
}

func testTool(name, title, description string) *mcp.Tool {
	return &mcp.Tool{
		Name:        name,
		Title:       title,
		Description: description,
		Annotations: &mcp.ToolAnnotations{Title: title},
	}
}

func echo(_ context.Context, _ *mcp.CallToolRequest, input echoInput) (*mcp.CallToolResult, any, error) {
	return textResult(input.Text), nil, nil
}

func getImage(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	data, err := testPNG()
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.TextContent{Text: "Generated a 320 x 180 local MCP test image."},
		&mcp.ImageContent{Data: data, MIMEType: "image/png"},
	}}, nil, nil
}

func structuredResult(
	_ context.Context,
	_ *mcp.CallToolRequest,
	input structuredInput,
) (*mcp.CallToolResult, structuredOutput, error) {
	output := structuredOutput{Label: input.Label, Length: len(input.Label), OK: true}
	return textResult(fmt.Sprintf("Structured result for %q.", input.Label)), output, nil
}

func fail(_ context.Context, _ *mcp.CallToolRequest, input failInput) (*mcp.CallToolResult, any, error) {
	message := input.Message
	if message == "" {
		message = "Intentional local MCP test failure."
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
		IsError: true,
	}, nil, nil
}

func slow(ctx context.Context, _ *mcp.CallToolRequest, input slowInput) (*mcp.CallToolResult, any, error) {
	if input.DelayMS < 0 || input.DelayMS > 30_000 {
		return nil, nil, fmt.Errorf("delayMs must be between 0 and 30000")
	}
	timer := time.NewTimer(time.Duration(input.DelayMS) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-timer.C:
		return textResult(fmt.Sprintf("Waited %d ms.", input.DelayMS)), nil, nil
	}
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

func testPNG() ([]byte, error) {
	canvas := image.NewRGBA(image.Rect(0, 0, 320, 180))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.RGBA{R: 250, G: 250, B: 249, A: 255}}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(0, 0, 320, 42), &image.Uniform{C: color.RGBA{R: 28, G: 25, B: 23, A: 255}}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(20, 66, 146, 154), &image.Uniform{C: color.RGBA{R: 5, G: 150, B: 105, A: 255}}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(174, 66, 300, 154), &image.Uniform{C: color.RGBA{R: 37, G: 99, B: 235, A: 255}}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(148, 84, 172, 136), &image.Uniform{C: color.RGBA{R: 28, G: 25, B: 23, A: 255}}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(134, 98, 186, 122), &image.Uniform{C: color.RGBA{R: 28, G: 25, B: 23, A: 255}}, image.Point{}, draw.Src)

	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		return nil, fmt.Errorf("encode test image: %w", err)
	}
	return output.Bytes(), nil
}
