// Command image sends a local image to a multimodal model.
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ktsoator/or/llm"

	_ "github.com/ktsoator/or/llm/openai" // registers the OpenAI-compatible protocol
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: go run ./example/llm/image <image-path>\n")
		os.Exit(2)
	}

	image, err := loadImage(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	model := llm.GetModel("xiaomi", "mimo-v2.5")
	response, err := llm.Complete(
		context.Background(),
		model,
		llm.NewContext(&llm.UserMessage{Content: []llm.UserContent{
			&llm.TextContent{Text: "Describe this image in one paragraph."},
			&image,
		}}),
		llm.StreamOptions{},
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(response.Text())
}

func loadImage(path string) (llm.ImageContent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return llm.ImageContent{}, fmt.Errorf("read image: %w", err)
	}

	mimeType := mime.TypeByExtension(filepath.Ext(path))
	if mimeType == "" {
		mimeType = http.DetectContentType(raw)
	}
	return llm.ImageContent{
		Data:     base64.StdEncoding.EncodeToString(raw),
		MIMEType: mimeType,
	}, nil
}
