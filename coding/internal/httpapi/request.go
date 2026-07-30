package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/llm"
)

// Decoding for prompt request bodies, including the base64 image attachments
// the composer sends inline.

type promptImage struct {
	Data     string `json:"data"`
	MIMEType string `json:"mimeType"`
}

type promptFile struct {
	Name     string `json:"name"`
	MIMEType string `json:"mimeType"`
	Content  string `json:"content"`
}

type messageRequest struct {
	ID     string        `json:"id"`
	Text   string        `json:"text"`
	Images []promptImage `json:"images"`
	Files  []promptFile  `json:"files"`
}

func bindMessageRequest(
	c *gin.Context,
) (messageRequest, []llm.ImageContent, []engine.AttachedFile, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPromptRequestBytes)
	body, files, err := bindMessagePayload(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return messageRequest{}, nil, nil, false
	}
	if strings.TrimSpace(body.Text) == "" && len(body.Images) == 0 && len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message must include text or an attachment"})
		return messageRequest{}, nil, nil, false
	}
	body.Text = strings.TrimSpace(body.Text)
	images, err := decodePromptImages(body.Images)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return messageRequest{}, nil, nil, false
	}
	return body, images, files, true
}

func bindMessagePayload(c *gin.Context) (messageRequest, []engine.AttachedFile, error) {
	if !strings.HasPrefix(c.ContentType(), "multipart/form-data") {
		var body messageRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			return messageRequest{}, nil, err
		}
		files, err := decodePromptFiles(body.Files)
		return body, files, err
	}

	form, err := c.MultipartForm()
	if err != nil {
		return messageRequest{}, nil, err
	}
	defer form.RemoveAll()
	var body messageRequest
	if values := form.Value["payload"]; len(values) != 1 ||
		json.Unmarshal([]byte(values[0]), &body) != nil {
		return messageRequest{}, nil, errors.New("multipart prompt payload is invalid")
	}
	files, err := decodeUploadedPromptFiles(form.File["files"])
	return body, files, err
}

func decodePromptImages(input []promptImage) ([]llm.ImageContent, error) {
	if len(input) > maxPromptImages {
		return nil, fmt.Errorf("a prompt can include at most %d images", maxPromptImages)
	}
	allowed := map[string]bool{
		"image/gif":  true,
		"image/jpeg": true,
		"image/png":  true,
		"image/webp": true,
	}
	images := make([]llm.ImageContent, 0, len(input))
	total := 0
	for _, image := range input {
		mimeType := strings.ToLower(strings.TrimSpace(image.MIMEType))
		if !allowed[mimeType] {
			return nil, fmt.Errorf("unsupported image type %q", image.MIMEType)
		}
		decoded, err := base64.StdEncoding.DecodeString(image.Data)
		if err != nil || len(decoded) == 0 {
			return nil, errors.New("image data is not valid base64")
		}
		if len(decoded) > maxPromptImageBytes {
			return nil, fmt.Errorf("each image must be %d MB or smaller", maxPromptImageBytes>>20)
		}
		total += len(decoded)
		if total > maxPromptImagesBytes {
			return nil, fmt.Errorf("images must total %d MB or less", maxPromptImagesBytes>>20)
		}
		images = append(images, llm.ImageContent{Data: image.Data, MIMEType: mimeType})
	}
	return images, nil
}

func decodePromptFiles(input []promptFile) ([]engine.AttachedFile, error) {
	if len(input) > maxPromptFiles {
		return nil, fmt.Errorf("a prompt can include at most %d files", maxPromptFiles)
	}
	files := make([]engine.AttachedFile, 0, len(input))
	total := 0
	for _, inputFile := range input {
		name := strings.TrimSpace(inputFile.Name)
		if name == "" || len(name) > 255 || strings.ContainsAny(name, `/\`) {
			return nil, errors.New("attached file name is invalid")
		}
		mimeType := strings.ToLower(strings.TrimSpace(inputFile.MIMEType))
		if !supportedTextFile(name, mimeType) {
			return nil, fmt.Errorf("unsupported text file %q", name)
		}
		if !utf8.ValidString(inputFile.Content) || strings.IndexByte(inputFile.Content, 0) >= 0 {
			return nil, fmt.Errorf("attached file %q is not valid UTF-8 text", name)
		}
		size := len([]byte(inputFile.Content))
		if size > maxPromptFileBytes {
			return nil, fmt.Errorf("each file must be %d KB or smaller", maxPromptFileBytes>>10)
		}
		total += size
		if total > maxPromptFilesBytes {
			return nil, fmt.Errorf("files must total %d KB or less", maxPromptFilesBytes>>10)
		}
		if mimeType == "" || mimeType == "application/octet-stream" {
			mimeType = "text/plain"
		}
		files = append(files, engine.AttachedFile{
			File: engine.File{
				Name:     name,
				MIMEType: mimeType,
				Size:     size,
			},
			Content: inputFile.Content,
		})
	}
	return files, nil
}

func decodeUploadedPromptFiles(input []*multipart.FileHeader) ([]engine.AttachedFile, error) {
	if len(input) > maxPromptFiles {
		return nil, fmt.Errorf("a prompt can include at most %d files", maxPromptFiles)
	}
	files := make([]promptFile, 0, len(input))
	for _, header := range input {
		file, err := header.Open()
		if err != nil {
			return nil, fmt.Errorf("open attached file %q: %w", header.Filename, err)
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxPromptFileBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read attached file %q: %w", header.Filename, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close attached file %q: %w", header.Filename, closeErr)
		}
		files = append(files, promptFile{
			Name:     header.Filename,
			MIMEType: header.Header.Get("Content-Type"),
			Content:  string(data),
		})
	}
	return decodePromptFiles(files)
}

func supportedTextFile(name, mimeType string) bool {
	if strings.HasPrefix(mimeType, "text/") {
		return true
	}
	switch mimeType {
	case "application/json",
		"application/ld+json",
		"application/toml",
		"application/x-httpd-php",
		"application/x-javascript",
		"application/x-sh",
		"application/xhtml+xml",
		"application/xml":
		return true
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".adoc", ".bash", ".c", ".cc", ".cfg", ".cjs", ".conf", ".cpp", ".cs",
		".css", ".dart", ".env", ".erl", ".ex", ".exs", ".fish", ".go", ".gql",
		".graphql", ".h", ".hcl", ".hpp", ".hrl", ".htm", ".html", ".ini", ".java",
		".js", ".json", ".jsonc", ".jsx", ".kt", ".kts", ".less", ".lock", ".lua",
		".md", ".mdx", ".mjs", ".php", ".proto", ".py", ".r", ".rb", ".rs", ".rst",
		".scss", ".sh", ".sql", ".svelte", ".swift", ".tf", ".toml", ".ts", ".tsx",
		".txt", ".vue", ".xml", ".yaml", ".yml", ".zsh":
		return true
	}
	switch strings.ToLower(name) {
	case "dockerfile", "gemfile", "license", "makefile", "procfile", "readme":
		return true
	}
	return false
}
