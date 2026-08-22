package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/imageprep"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/llm"
)

type viewImageArgs struct {
	Path string `json:"path" jsonschema:"description=Path to the image file to inspect, absolute or relative to the workspace root,minLength=1"`
}

// ImageViewResult is the provider-independent metadata for a viewed image.
// The image bytes themselves live in ToolResult.Content and are deliberately
// omitted here so logs and product metadata do not duplicate the base64 data.
type ImageViewResult struct {
	Path           string
	MIMEType       string
	Bytes          int
	OriginalWidth  int
	OriginalHeight int
	OutputWidth    int
	OutputHeight   int
	Resized        bool
}

func viewImageTool(root string) Tool {
	def := llm.MustTool[viewImageArgs]("view_image", viewImageText.description)
	return Tool{
		AgentTool: agent.AgentTool{
			Definition: def,
			Label:      "View image",
			Execute: func(ctx context.Context, _ string, raw json.RawMessage, _ func(agent.ToolProgress)) (agent.ToolResult, error) {
				var in viewImageArgs
				if err := json.Unmarshal(raw, &in); err != nil {
					return agent.ToolResult{}, err
				}
				if strings.TrimSpace(in.Path) == "" {
					return viewImageFailure(in.Path, "image_path_required", errors.New("path is required")), nil
				}

				data, err := readViewImageFile(ctx, resolve(root, in.Path))
				if err != nil {
					return viewImageFailure(in.Path, viewImageErrorCode(err), err), nil
				}
				prepared, err := imageprep.Prepare(ctx, imageprep.Input{Data: data}, imageprep.DefaultPolicy())
				if err != nil {
					return viewImageFailure(in.Path, viewImageErrorCode(err), err), nil
				}

				metadata := ImageViewResult{
					Path:           in.Path,
					MIMEType:       prepared.Content.MIMEType,
					Bytes:          prepared.Bytes,
					OriginalWidth:  prepared.OriginalWidth,
					OriginalHeight: prepared.OriginalHeight,
					OutputWidth:    prepared.OutputWidth,
					OutputHeight:   prepared.OutputHeight,
					Resized:        prepared.Resized,
				}
				return agent.ToolResult{
					Content: []llm.ToolResultContent{
						&llm.TextContent{Text: formatViewImageResult(metadata)},
						&prepared.Content,
					},
					Outcome: agent.ToolOutcome{Status: agent.ToolOutcomeSuccess, Data: metadata},
				}, nil
			},
		},
		AccessFor:      pathAccess(permission.Read),
		RequiredInputs: []llm.ModelInput{llm.ModelInputImage},
	}
}

var (
	errViewImageTooLarge   = errors.New("image exceeds the 10 MiB size limit")
	errViewImageNotRegular = errors.New("path is not a regular file")
)

func readViewImageFile(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	before, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, errViewImageNotRegular
	}
	if before.Size() == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	if before.Size() > imageprep.DefaultMaxInputBytes {
		return nil, errViewImageTooLarge
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, readErr := readViewImageBytes(ctx, file)
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	after, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !sameFileVersion(before, after) {
		return nil, fmt.Errorf("%w while it was being read", ErrFileChanged)
	}
	return data, nil
}

func readViewImageBytes(ctx context.Context, reader io.Reader) ([]byte, error) {
	var data bytes.Buffer
	buffer := make([]byte, 32<<10)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		remaining := imageprep.DefaultMaxInputBytes + 1 - int64(data.Len())
		if remaining <= 0 {
			return nil, errViewImageTooLarge
		}
		chunk := buffer
		if int64(len(chunk)) > remaining {
			chunk = chunk[:remaining]
		}
		n, err := reader.Read(chunk)
		if n > 0 {
			_, _ = data.Write(chunk[:n])
			if int64(data.Len()) > imageprep.DefaultMaxInputBytes {
				return nil, errViewImageTooLarge
			}
		}
		switch err {
		case nil:
			continue
		case io.EOF:
			if data.Len() == 0 {
				return nil, io.ErrUnexpectedEOF
			}
			return data.Bytes(), nil
		default:
			return nil, err
		}
	}
}

func formatViewImageResult(result ImageViewResult) string {
	if result.Resized {
		return fmt.Sprintf(
			"Viewed image %s (%s, %dx%d; resized to %dx%d).",
			result.Path, result.MIMEType, result.OriginalWidth, result.OriginalHeight,
			result.OutputWidth, result.OutputHeight,
		)
	}
	return fmt.Sprintf("Viewed image %s (%s, %dx%d).", result.Path, result.MIMEType, result.OutputWidth, result.OutputHeight)
}

func viewImageFailure(path, code string, err error) agent.ToolResult {
	detail := fmt.Sprintf("Could not view image %s: %v", path, err)
	return failedResult(code, detail, map[string]any{"path": path})
}

func viewImageErrorCode(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "image_read_cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "image_read_timeout"
	case errors.Is(err, os.ErrNotExist):
		return "image_not_found"
	case errors.Is(err, errViewImageNotRegular):
		return "image_not_file"
	case errors.Is(err, errViewImageTooLarge):
		return "image_too_large"
	case imageprep.CodeOf(err) == imageprep.ErrorTooLarge:
		return "image_too_large"
	case imageprep.CodeOf(err) == imageprep.ErrorTooManyPixels:
		return "image_too_many_pixels"
	case imageprep.CodeOf(err) == imageprep.ErrorUnsupported:
		return "image_unsupported"
	case imageprep.CodeOf(err) == imageprep.ErrorAnimated:
		return "image_animated"
	case errors.Is(err, ErrFileChanged):
		return "image_changed"
	default:
		return "image_invalid"
	}
}
