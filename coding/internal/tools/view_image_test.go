package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

func TestViewImageSupportsDeclaredFormatsAndActualMIME(t *testing.T) {
	webpData, err := base64.StdEncoding.DecodeString(testWebPBase64)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		fileName string
		data     []byte
		mimeType string
		absolute bool
	}{
		{name: "PNG", fileName: "sample.png", data: encodeTestImage(t, "png", 3, 2), mimeType: "image/png"},
		{name: "JPEG ignores misleading extension", fileName: "actually-jpeg.png", data: encodeTestImage(t, "jpeg", 4, 3), mimeType: "image/jpeg", absolute: true},
		{name: "WebP", fileName: "sample.webp", data: webpData, mimeType: "image/webp"},
		{name: "static GIF", fileName: "sample.gif", data: encodeTestImage(t, "gif", 5, 4), mimeType: "image/gif"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			absolutePath := filepath.Join(root, test.fileName)
			if err := os.WriteFile(absolutePath, test.data, 0o600); err != nil {
				t.Fatal(err)
			}
			argumentPath := test.fileName
			if test.absolute {
				argumentPath = absolutePath
			}

			result := executeViewImage(t, root, argumentPath)
			if result.Outcome.Failed() {
				t.Fatalf("view_image failed: %+v", result.Outcome)
			}
			metadata, ok := result.Outcome.Data.(ImageViewResult)
			if !ok {
				t.Fatalf("Outcome.Data = %T, want ImageViewResult", result.Outcome.Data)
			}
			if metadata.Path != argumentPath || metadata.MIMEType != test.mimeType || metadata.Resized {
				t.Fatalf("metadata = %+v", metadata)
			}
			if metadata.OriginalWidth <= 0 || metadata.OriginalHeight <= 0 || metadata.Bytes != len(test.data) {
				t.Fatalf("metadata dimensions/bytes = %+v", metadata)
			}
			if len(result.Content) != 2 {
				t.Fatalf("content blocks = %d, want 2", len(result.Content))
			}
			text, ok := result.Content[0].(*llm.TextContent)
			if !ok || !strings.Contains(text.Text, "Viewed image") {
				t.Fatalf("text content = %#v", result.Content[0])
			}
			viewed, ok := result.Content[1].(*llm.ImageContent)
			if !ok || viewed.MIMEType != test.mimeType {
				t.Fatalf("image content = %#v", result.Content[1])
			}
			decoded, err := base64.StdEncoding.DecodeString(viewed.Data)
			if err != nil || !bytes.Equal(decoded, test.data) {
				t.Fatalf("returned image differs from input: decode error %v", err)
			}
		})
	}
}

func TestViewImageResizesLargeImages(t *testing.T) {
	tests := []struct {
		name       string
		format     string
		width      int
		height     int
		wantWidth  int
		wantHeight int
		wantMIME   string
	}{
		{name: "PNG becomes bounded PNG", format: "png", width: 3000, height: 1500, wantWidth: 2048, wantHeight: 1024, wantMIME: "image/png"},
		{name: "JPEG remains JPEG", format: "jpeg", width: 3000, height: 2, wantWidth: 2048, wantHeight: 1, wantMIME: "image/jpeg"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prepared, err := prepareViewImage(encodeTestImage(t, test.format, test.width, test.height))
			if err != nil {
				t.Fatal(err)
			}
			if !prepared.resized || prepared.outputWidth != test.wantWidth || prepared.outputHeight != test.wantHeight || prepared.mimeType != test.wantMIME {
				t.Fatalf("prepared image = %+v", prepared)
			}
			config, _, err := image.DecodeConfig(bytes.NewReader(prepared.data))
			if err != nil {
				t.Fatal(err)
			}
			if config.Width != test.wantWidth || config.Height != test.wantHeight {
				t.Fatalf("encoded dimensions = %dx%d", config.Width, config.Height)
			}
		})
	}
}

func TestViewImageRejectsInvalidInputs(t *testing.T) {
	root := t.TempDir()
	write := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("empty.png", nil)
	write("corrupt.png", []byte("\x89PNG\r\n\x1a\n"))
	write("unsupported.txt", []byte("not an image"))
	write("animated.gif", encodeAnimatedGIF(t))
	largePath := filepath.Join(root, "too-large.png")
	large, err := os.Create(largePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := large.Truncate(maxViewImageBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := large.Close(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		code string
	}{
		{name: "missing", path: "missing.png", code: "image_not_found"},
		{name: "directory", path: ".", code: "image_not_file"},
		{name: "empty", path: "empty.png", code: "image_invalid"},
		{name: "corrupt", path: "corrupt.png", code: "image_invalid"},
		{name: "unsupported", path: "unsupported.txt", code: "image_unsupported"},
		{name: "animated GIF", path: "animated.gif", code: "image_animated"},
		{name: "byte limit", path: "too-large.png", code: "image_too_large"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := executeViewImage(t, root, test.path)
			if !result.Outcome.Failed() || result.Outcome.ErrorCode != test.code {
				t.Fatalf("outcome = %+v, want failed code %q", result.Outcome, test.code)
			}
			if len(result.Content) != 1 {
				t.Fatalf("failure content blocks = %d, want 1", len(result.Content))
			}
		})
	}
}

func TestViewImagePixelLimitAndCancellation(t *testing.T) {
	if err := validateViewImageDimensions(40_000_001, 1); err != errViewImageTooManyPx {
		t.Fatalf("pixel limit error = %v", err)
	}
	if err := validateViewImageDimensions(10_000, 4_000); err != nil {
		t.Fatalf("boundary dimensions rejected: %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "image.png"), encodeTestImage(t, "png", 2, 2), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	raw, _ := json.Marshal(viewImageArgs{Path: "image.png"})
	result, err := viewImageTool(root).Execute(ctx, "call-1", raw, func(agent.ToolProgress) {})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome.ErrorCode != "image_read_cancelled" {
		t.Fatalf("cancelled outcome = %+v", result.Outcome)
	}
}

func executeViewImage(t *testing.T, root, path string) agent.ToolResult {
	t.Helper()
	raw, err := json.Marshal(viewImageArgs{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	result, err := viewImageTool(root).Execute(t.Context(), "call-1", raw, func(agent.ToolProgress) {})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return result
}

func encodeTestImage(t *testing.T, format string, width, height int) []byte {
	t.Helper()
	var output bytes.Buffer
	switch format {
	case "png":
		picture := image.NewNRGBA(image.Rect(0, 0, width, height))
		for y := range height {
			for x := range width {
				picture.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 120, A: 255})
			}
		}
		if err := png.Encode(&output, picture); err != nil {
			t.Fatal(err)
		}
	case "jpeg":
		picture := image.NewRGBA(image.Rect(0, 0, width, height))
		if err := jpeg.Encode(&output, picture, &jpeg.Options{Quality: 85}); err != nil {
			t.Fatal(err)
		}
	case "gif":
		palette := color.Palette{color.Black, color.White}
		picture := image.NewPaletted(image.Rect(0, 0, width, height), palette)
		if err := gif.Encode(&output, picture, nil); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unsupported test format %q", format)
	}
	return output.Bytes()
}

func encodeAnimatedGIF(t *testing.T) []byte {
	t.Helper()
	palette := color.Palette{color.Black, color.White}
	first := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	second := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	second.SetColorIndex(0, 0, 1)
	var output bytes.Buffer
	if err := gif.EncodeAll(&output, &gif.GIF{
		Image: []*image.Paletted{first, second},
		Delay: []int{0, 0},
	}); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

const testWebPBase64 = "UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA=="
