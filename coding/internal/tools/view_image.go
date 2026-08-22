package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/llm"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	maxViewImageBytes     int64 = 10 << 20
	maxViewImagePixels    int64 = 40_000_000
	maxViewImageDimension       = 2048
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
				prepared, err := prepareViewImage(data)
				if err != nil {
					return viewImageFailure(in.Path, viewImageErrorCode(err), err), nil
				}

				metadata := ImageViewResult{
					Path:           in.Path,
					MIMEType:       prepared.mimeType,
					Bytes:          len(prepared.data),
					OriginalWidth:  prepared.originalWidth,
					OriginalHeight: prepared.originalHeight,
					OutputWidth:    prepared.outputWidth,
					OutputHeight:   prepared.outputHeight,
					Resized:        prepared.resized,
				}
				return agent.ToolResult{
					Content: []llm.ToolResultContent{
						&llm.TextContent{Text: formatViewImageResult(metadata)},
						&llm.ImageContent{
							Data:     base64.StdEncoding.EncodeToString(prepared.data),
							MIMEType: prepared.mimeType,
						},
					},
					Outcome: agent.ToolOutcome{Status: agent.ToolOutcomeSuccess, Data: metadata},
				}, nil
			},
		},
		AccessFor:      pathAccess(permission.Read),
		RequiredInputs: []llm.ModelInput{llm.ModelInputImage},
	}
}

type preparedImage struct {
	data           []byte
	mimeType       string
	originalWidth  int
	originalHeight int
	outputWidth    int
	outputHeight   int
	resized        bool
}

var (
	errViewImageTooLarge    = errors.New("image exceeds the 10 MiB size limit")
	errViewImageTooManyPx   = errors.New("image exceeds the 40 million pixel limit")
	errViewImageUnsupported = errors.New("unsupported image format")
	errViewImageAnimated    = errors.New("animated GIF images are not supported")
	errViewImageNotRegular  = errors.New("path is not a regular file")
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
	if before.Size() > maxViewImageBytes {
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
		remaining := maxViewImageBytes + 1 - int64(data.Len())
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
			if int64(data.Len()) > maxViewImageBytes {
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

func prepareViewImage(data []byte) (preparedImage, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return preparedImage{}, fmt.Errorf("decode image metadata: %w", err)
	}
	mimeType, ok := viewImageMIMEType(format)
	if !ok {
		return preparedImage{}, fmt.Errorf("%w: %s", errViewImageUnsupported, format)
	}
	if err := validateViewImageDimensions(config.Width, config.Height); err != nil {
		return preparedImage{}, err
	}
	if format == "gif" {
		frames, err := gifFrameCount(data)
		if err != nil {
			return preparedImage{}, fmt.Errorf("decode GIF: %w", err)
		}
		if frames != 1 {
			return preparedImage{}, errViewImageAnimated
		}
	}

	decoded, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return preparedImage{}, fmt.Errorf("decode image: %w", err)
	}
	if decodedFormat != format {
		return preparedImage{}, errors.New("image format changed while decoding")
	}
	bounds := decoded.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width != config.Width || height != config.Height {
		return preparedImage{}, errors.New("decoded image dimensions do not match its metadata")
	}

	result := preparedImage{
		data:           data,
		mimeType:       mimeType,
		originalWidth:  width,
		originalHeight: height,
		outputWidth:    width,
		outputHeight:   height,
	}
	if width <= maxViewImageDimension && height <= maxViewImageDimension {
		return result, nil
	}

	outputWidth, outputHeight := resizedViewImageDimensions(width, height)
	destination := image.NewNRGBA(image.Rect(0, 0, outputWidth, outputHeight))
	xdraw.CatmullRom.Scale(destination, destination.Bounds(), decoded, bounds, xdraw.Src, nil)

	var encoded bytes.Buffer
	if format == "jpeg" {
		err = jpeg.Encode(&encoded, destination, &jpeg.Options{Quality: 90})
		result.mimeType = "image/jpeg"
	} else {
		err = png.Encode(&encoded, destination)
		result.mimeType = "image/png"
	}
	if err != nil {
		return preparedImage{}, fmt.Errorf("encode resized image: %w", err)
	}
	if int64(encoded.Len()) > maxViewImageBytes {
		return preparedImage{}, fmt.Errorf("resized output: %w", errViewImageTooLarge)
	}
	result.data = encoded.Bytes()
	result.outputWidth = outputWidth
	result.outputHeight = outputHeight
	result.resized = true
	return result, nil
}

func validateViewImageDimensions(width, height int) error {
	if width <= 0 || height <= 0 {
		return errors.New("image dimensions must be positive")
	}
	if int64(width) > maxViewImagePixels/int64(height) {
		return errViewImageTooManyPx
	}
	return nil
}

func resizedViewImageDimensions(width, height int) (int, int) {
	if width >= height {
		return maxViewImageDimension, max(1, int((int64(height)*maxViewImageDimension+int64(width)/2)/int64(width)))
	}
	return max(1, int((int64(width)*maxViewImageDimension+int64(height)/2)/int64(height))), maxViewImageDimension
}

func viewImageMIMEType(format string) (string, bool) {
	switch format {
	case "png":
		return "image/png", true
	case "jpeg":
		return "image/jpeg", true
	case "webp":
		return "image/webp", true
	case "gif":
		return "image/gif", true
	default:
		return "", false
	}
}

// gifFrameCount walks GIF container blocks without decoding every frame into
// memory. This lets the tool reject animation before a small compressed file
// can expand into an unbounded frame set.
func gifFrameCount(data []byte) (int, error) {
	if len(data) < 13 || (string(data[:6]) != "GIF87a" && string(data[:6]) != "GIF89a") {
		return 0, errors.New("invalid GIF header")
	}
	offset := 13
	if data[10]&0x80 != 0 {
		colorTableBytes := 3 * (1 << (uint(data[10]&0x07) + 1))
		if offset+colorTableBytes > len(data) {
			return 0, io.ErrUnexpectedEOF
		}
		offset += colorTableBytes
	}

	frames := 0
	for {
		if offset >= len(data) {
			return 0, io.ErrUnexpectedEOF
		}
		blockType := data[offset]
		offset++
		switch blockType {
		case 0x3b:
			return frames, nil
		case 0x21:
			if offset >= len(data) {
				return 0, io.ErrUnexpectedEOF
			}
			offset++ // Extension label.
			var err error
			offset, err = skipGIFSubBlocks(data, offset)
			if err != nil {
				return 0, err
			}
		case 0x2c:
			frames++
			if frames > 1 {
				return frames, nil
			}
			if offset+9 > len(data) {
				return 0, io.ErrUnexpectedEOF
			}
			packed := data[offset+8]
			offset += 9
			if packed&0x80 != 0 {
				colorTableBytes := 3 * (1 << (uint(packed&0x07) + 1))
				if offset+colorTableBytes > len(data) {
					return 0, io.ErrUnexpectedEOF
				}
				offset += colorTableBytes
			}
			if offset >= len(data) {
				return 0, io.ErrUnexpectedEOF
			}
			offset++ // LZW minimum code size.
			var err error
			offset, err = skipGIFSubBlocks(data, offset)
			if err != nil {
				return 0, err
			}
		default:
			return 0, fmt.Errorf("invalid GIF block type 0x%02x", blockType)
		}
	}
}

func skipGIFSubBlocks(data []byte, offset int) (int, error) {
	for {
		if offset >= len(data) {
			return 0, io.ErrUnexpectedEOF
		}
		size := int(data[offset])
		offset++
		if size == 0 {
			return offset, nil
		}
		if offset+size > len(data) {
			return 0, io.ErrUnexpectedEOF
		}
		offset += size
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
	case errors.Is(err, errViewImageTooManyPx):
		return "image_too_many_pixels"
	case errors.Is(err, image.ErrFormat):
		return "image_unsupported"
	case errors.Is(err, errViewImageUnsupported):
		return "image_unsupported"
	case errors.Is(err, errViewImageAnimated):
		return "image_animated"
	case errors.Is(err, ErrFileChanged):
		return "image_changed"
	default:
		return "image_invalid"
	}
}
