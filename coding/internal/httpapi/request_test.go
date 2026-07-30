package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

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
