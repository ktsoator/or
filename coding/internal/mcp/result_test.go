package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/ktsoator/or/coding/internal/imageprep"
	"github.com/ktsoator/or/llm"
	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestProjectResultPreparesImageContent(t *testing.T) {
	result, err := projectResult(t.Context(), "server", "image", &protocol.CallToolResult{
		Content: []protocol.Content{&protocol.ImageContent{
			Data:     encodeMCPTestPNG(t, 3000, 1500),
			MIMEType: " IMAGE/PNG ",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %#v", result.Content)
	}
	prepared, ok := result.Content[0].(*llm.ImageContent)
	if !ok || prepared.MIMEType != "image/png" {
		t.Fatalf("prepared content = %#v", result.Content[0])
	}
	data, err := base64.StdEncoding.DecodeString(prepared.Data)
	if err != nil {
		t.Fatal(err)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != imageprep.DefaultMaxDimension || config.Height != imageprep.DefaultMaxDimension/2 {
		t.Fatalf("prepared dimensions = %dx%d", config.Width, config.Height)
	}
}

func TestProjectResultOmitsInvalidImageAndPreservesText(t *testing.T) {
	result, err := projectResult(t.Context(), "server", "image", &protocol.CallToolResult{
		Content: []protocol.Content{
			&protocol.TextContent{Text: "before"},
			&protocol.ImageContent{Data: []byte("not an image"), MIMEType: "image/png"},
			&protocol.TextContent{Text: "after"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 3 {
		t.Fatalf("content = %#v", result.Content)
	}
	omitted, ok := result.Content[1].(*llm.TextContent)
	if !ok || !strings.Contains(omitted.Text, "[image omitted:") {
		t.Fatalf("omission = %#v", result.Content[1])
	}
	if result.Content[0].(*llm.TextContent).Text != "before" || result.Content[2].(*llm.TextContent).Text != "after" {
		t.Fatalf("surrounding content = %#v", result.Content)
	}
}

func TestProjectResultPropagatesImagePreparationCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := projectResult(ctx, "server", "image", &protocol.CallToolResult{
		Content: []protocol.Content{&protocol.ImageContent{
			Data:     encodeMCPTestPNG(t, 2, 2),
			MIMEType: "image/png",
		}},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
}

func encodeMCPTestPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	picture := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			picture.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 120, A: 255})
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, picture); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
