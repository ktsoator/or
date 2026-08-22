package imageprep

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestPrepareSupportedFormats(t *testing.T) {
	webpData, err := base64.StdEncoding.DecodeString(testWebPBase64)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		data     []byte
		declared string
		wantMIME string
	}{
		{name: "PNG", data: encodeImage(t, "png", 3, 2), declared: " image/PNG ", wantMIME: "image/png"},
		{name: "JPEG", data: encodeImage(t, "jpeg", 4, 3), declared: "image/jpeg", wantMIME: "image/jpeg"},
		{name: "WebP", data: webpData, declared: "image/webp", wantMIME: "image/webp"},
		{name: "static GIF", data: encodeImage(t, "gif", 5, 4), declared: "image/gif", wantMIME: "image/gif"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prepared, err := Prepare(t.Context(), Input{Data: test.data, DeclaredMIME: test.declared}, DefaultPolicy())
			if err != nil {
				t.Fatal(err)
			}
			if prepared.Content.MIMEType != test.wantMIME || prepared.Resized || prepared.Reencoded {
				t.Fatalf("prepared = %+v", prepared)
			}
			decoded, err := base64.StdEncoding.DecodeString(prepared.Content.Data)
			if err != nil || !bytes.Equal(decoded, test.data) {
				t.Fatalf("content differs from input: decode error %v", err)
			}
		})
	}
}

func TestPrepareRejectsDeclaredTypeMismatch(t *testing.T) {
	_, err := Prepare(t.Context(), Input{
		Data:         encodeImage(t, "jpeg", 2, 2),
		DeclaredMIME: "image/png",
	}, DefaultPolicy())
	if CodeOf(err) != ErrorTypeMismatch {
		t.Fatalf("error = %v, code = %q", err, CodeOf(err))
	}
}

func TestPrepareResizesAndVerifiesOutput(t *testing.T) {
	tests := []struct {
		name       string
		format     string
		width      int
		height     int
		wantWidth  int
		wantHeight int
		wantMIME   string
	}{
		{name: "PNG", format: "png", width: 3000, height: 1500, wantWidth: 2048, wantHeight: 1024, wantMIME: "image/png"},
		{name: "JPEG", format: "jpeg", width: 3000, height: 2, wantWidth: 2048, wantHeight: 1, wantMIME: "image/jpeg"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prepared, err := Prepare(t.Context(), Input{Data: encodeImage(t, test.format, test.width, test.height)}, DefaultPolicy())
			if err != nil {
				t.Fatal(err)
			}
			if !prepared.Resized || !prepared.Reencoded || prepared.OutputWidth != test.wantWidth ||
				prepared.OutputHeight != test.wantHeight || prepared.Content.MIMEType != test.wantMIME {
				t.Fatalf("prepared = %+v", prepared)
			}
			data, err := base64.StdEncoding.DecodeString(prepared.Content.Data)
			if err != nil {
				t.Fatal(err)
			}
			config, _, err := image.DecodeConfig(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if config.Width != test.wantWidth || config.Height != test.wantHeight || len(data) != prepared.Bytes {
				t.Fatalf("encoded image = %dx%d/%d bytes, prepared = %+v", config.Width, config.Height, len(data), prepared)
			}
		})
	}
}

func TestPrepareRejectsInvalidAndUnsafeImages(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		mime string
		code ErrorCode
	}{
		{name: "empty", code: ErrorInvalid},
		{name: "unsupported", data: []byte("not an image"), code: ErrorUnsupported},
		{name: "corrupt", data: []byte("\x89PNG\r\n\x1a\n"), mime: "image/png", code: ErrorInvalid},
		{name: "unsupported declaration", data: encodeImage(t, "png", 2, 2), mime: "image/svg+xml", code: ErrorUnsupported},
		{name: "animated GIF", data: encodeAnimatedGIF(t), mime: "image/gif", code: ErrorAnimated},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Prepare(t.Context(), Input{Data: test.data, DeclaredMIME: test.mime}, DefaultPolicy())
			if CodeOf(err) != test.code {
				t.Fatalf("error = %v, code = %q, want %q", err, CodeOf(err), test.code)
			}
		})
	}
}

func TestPrepareEnforcesLimitsAndCancellation(t *testing.T) {
	data := encodeImage(t, "png", 10, 10)
	tests := []struct {
		name   string
		policy Policy
		code   ErrorCode
	}{
		{name: "input bytes", policy: Policy{MaxInputBytes: int64(len(data) - 1), MaxPixels: 100, MaxDimension: 10, MaxOutputBytes: 1 << 20}, code: ErrorTooLarge},
		{name: "pixels", policy: Policy{MaxInputBytes: 1 << 20, MaxPixels: 99, MaxDimension: 10, MaxOutputBytes: 1 << 20}, code: ErrorTooManyPixels},
		{name: "output bytes", policy: Policy{MaxInputBytes: 1 << 20, MaxPixels: 100, MaxDimension: 10, MaxOutputBytes: 1}, code: ErrorTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Prepare(t.Context(), Input{Data: data}, test.policy)
			if CodeOf(err) != test.code {
				t.Fatalf("error = %v, code = %q, want %q", err, CodeOf(err), test.code)
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Prepare(ctx, Input{Data: data}, DefaultPolicy())
	if CodeOf(err) != ErrorCancelled || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled error = %v, code = %q", err, CodeOf(err))
	}
}

func TestValidateDimensionsDefaultPixelBoundary(t *testing.T) {
	if err := validateDimensions(10_000, 4_000, DefaultMaxPixels); err != nil {
		t.Fatalf("boundary dimensions rejected: %v", err)
	}
	if code := CodeOf(validateDimensions(40_000_001, 1, DefaultMaxPixels)); code != ErrorTooManyPixels {
		t.Fatalf("over-limit code = %q, want %q", code, ErrorTooManyPixels)
	}
}

func encodeImage(t *testing.T, format string, width, height int) []byte {
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
