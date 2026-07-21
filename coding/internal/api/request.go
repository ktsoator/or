package api

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ktsoator/or/llm"
)

// Decoding for prompt request bodies, including the base64 image attachments
// the composer sends inline.

type promptImage struct {
	Data     string `json:"data"`
	MIMEType string `json:"mimeType"`
}

type messageRequest struct {
	ID     string        `json:"id"`
	Text   string        `json:"text"`
	Images []promptImage `json:"images"`
}

func bindMessageRequest(c *gin.Context) (messageRequest, []llm.ImageContent, bool) {
	var body messageRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPromptRequestBytes)
	if err := c.ShouldBindJSON(&body); err != nil || (strings.TrimSpace(body.Text) == "" && len(body.Images) == 0) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message must include text or an image"})
		return messageRequest{}, nil, false
	}
	body.Text = strings.TrimSpace(body.Text)
	images, err := decodePromptImages(body.Images)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return messageRequest{}, nil, false
	}
	return body, images, true
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
