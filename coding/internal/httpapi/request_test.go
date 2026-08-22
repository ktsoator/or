package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ktsoator/or/coding/internal/imageprep"
)

func TestDecodePromptImagesPreparesValidatedContent(t *testing.T) {
	data := encodePromptTestImage(t, "png", 3000, 1500)
	images, err := decodePromptImages(t.Context(), []promptImage{{
		Data:     base64.StdEncoding.EncodeToString(data),
		MIMEType: " IMAGE/PNG ",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 || images[0].MIMEType != "image/png" {
		t.Fatalf("images = %#v", images)
	}
	prepared, err := base64.StdEncoding.DecodeString(images[0].Data)
	if err != nil {
		t.Fatal(err)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(prepared))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != imageprep.DefaultMaxDimension || config.Height != imageprep.DefaultMaxDimension/2 {
		t.Fatalf("prepared dimensions = %dx%d", config.Width, config.Height)
	}
}

func TestDecodePromptImagesRejectsInvalidContent(t *testing.T) {
	tests := []struct {
		name  string
		image promptImage
		code  imageprep.ErrorCode
	}{
		{
			name:  "declared MIME mismatch",
			image: promptImage{Data: base64.StdEncoding.EncodeToString(encodePromptTestImage(t, "jpeg", 2, 2)), MIMEType: "image/png"},
			code:  imageprep.ErrorTypeMismatch,
		},
		{
			name:  "invalid image",
			image: promptImage{Data: base64.StdEncoding.EncodeToString([]byte("not an image")), MIMEType: "image/png"},
			code:  imageprep.ErrorUnsupported,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodePromptImages(t.Context(), []promptImage{test.image})
			if imageprep.CodeOf(err) != test.code {
				t.Fatalf("error = %v, code = %q, want %q", err, imageprep.CodeOf(err), test.code)
			}
		})
	}

	if _, err := decodePromptImages(t.Context(), []promptImage{{Data: "not base64", MIMEType: "image/png"}}); err == nil {
		t.Fatal("invalid base64 unexpectedly succeeded")
	}
}

func TestDecodePromptFiles(t *testing.T) {
	for _, mimeType := range []string{"", "application/octet-stream"} {
		t.Run(mimeType, func(t *testing.T) {
			files, err := decodePromptFiles([]promptFile{{
				Name:     "main.go",
				MIMEType: mimeType,
				Content:  "package main\n",
			}})
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != 1 || files[0].Name != "main.go" ||
				files[0].MIMEType != "text/plain" || files[0].Size != 13 {
				t.Fatalf("files = %#v", files)
			}
		})
	}
}

func TestDecodePromptFilesRejectsUnsafeInput(t *testing.T) {
	tests := []struct {
		name  string
		files []promptFile
	}{
		{
			name:  "path instead of name",
			files: []promptFile{{Name: "../secret.txt", Content: "secret"}},
		},
		{
			name:  "unsupported extension",
			files: []promptFile{{Name: "archive.zip", Content: "not really a zip"}},
		},
		{
			name:  "binary content",
			files: []promptFile{{Name: "data.txt", Content: "a\x00b"}},
		},
		{
			name: "single file too large",
			files: []promptFile{{
				Name:    "large.txt",
				Content: strings.Repeat("x", maxPromptFileBytes+1),
			}},
		},
		{
			name: "too many files",
			files: func() []promptFile {
				result := make([]promptFile, maxPromptFiles+1)
				for index := range result {
					result[index] = promptFile{Name: "file.txt", Content: "x"}
				}
				return result
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := decodePromptFiles(test.files); err == nil {
				t.Fatal("decodePromptFiles unexpectedly succeeded")
			}
		})
	}
}

func TestBindMultipartMessagePayloadReadsFilesOnServer(t *testing.T) {
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	payload, err := json.Marshal(messageRequest{Text: "review"})
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("payload", string(payload)); err != nil {
		t.Fatal(err)
	}
	file, err := writer.CreateFormFile("files", "main.go")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("package main\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("POST", "/prompt", &requestBody)
	context.Request.Header.Set("Content-Type", writer.FormDataContentType())

	body, files, err := bindMessagePayload(context)
	if err != nil {
		t.Fatal(err)
	}
	if body.Text != "review" || len(files) != 1 ||
		files[0].Name != "main.go" || files[0].MIMEType != "text/plain" ||
		files[0].Content != "package main\n" {
		t.Fatalf("payload/files = %#v/%#v", body, files)
	}
}

func encodePromptTestImage(t *testing.T, format string, width, height int) []byte {
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
		if err := jpeg.Encode(&output, image.NewRGBA(image.Rect(0, 0, width, height)), nil); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unsupported test format %q", format)
	}
	return output.Bytes()
}
