// Package imageprep validates and normalizes raster images before they enter
// provider-independent model content.
package imageprep

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"strings"

	"github.com/ktsoator/or/llm"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	DefaultMaxInputBytes  int64 = 10 << 20
	DefaultMaxPixels      int64 = 40_000_000
	DefaultMaxDimension         = 2048
	DefaultMaxOutputBytes int64 = 10 << 20
)

// Policy bounds source decoding and normalized model content.
type Policy struct {
	MaxInputBytes  int64
	MaxPixels      int64
	MaxDimension   int
	MaxOutputBytes int64
}

// DefaultPolicy returns the common policy for current coding image sources.
func DefaultPolicy() Policy {
	return Policy{
		MaxInputBytes:  DefaultMaxInputBytes,
		MaxPixels:      DefaultMaxPixels,
		MaxDimension:   DefaultMaxDimension,
		MaxOutputBytes: DefaultMaxOutputBytes,
	}
}

// Input is one complete encoded raster and optional caller-declared media type.
type Input struct {
	Data         []byte
	DeclaredMIME string
}

// Prepared contains canonical model content and inspectable transformation facts.
// Raw bytes are represented only by Content.Data so metadata and logs do not
// accidentally duplicate the image payload.
type Prepared struct {
	Content        llm.ImageContent
	Bytes          int
	OriginalWidth  int
	OriginalHeight int
	OutputWidth    int
	OutputHeight   int
	Resized        bool
	Reencoded      bool
}

// ErrorCode is a stable machine-readable preparation failure category.
type ErrorCode string

const (
	ErrorInvalid       ErrorCode = "image_invalid"
	ErrorUnsupported   ErrorCode = "image_unsupported"
	ErrorTypeMismatch  ErrorCode = "image_type_mismatch"
	ErrorTooLarge      ErrorCode = "image_too_large"
	ErrorTooManyPixels ErrorCode = "image_too_many_pixels"
	ErrorAnimated      ErrorCode = "image_animated"
	ErrorCancelled     ErrorCode = "image_cancelled"
)

// Error is a preparation failure with a safe public message and stable code.
type Error struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *Error) Unwrap() error { return e.Cause }

// CodeOf returns the structured code carried by err, if any.
func CodeOf(err error) ErrorCode {
	var preparationError *Error
	if errors.As(err, &preparationError) {
		return preparationError.Code
	}
	return ""
}

func preparationError(code ErrorCode, message string, cause error) error {
	return &Error{Code: code, Message: message, Cause: cause}
}

// Prepare validates, fully decodes, bounds, and encodes one model image.
func Prepare(ctx context.Context, input Input, policy Policy) (Prepared, error) {
	if err := validatePolicy(policy); err != nil {
		return Prepared{}, err
	}
	if err := contextError(ctx); err != nil {
		return Prepared{}, err
	}
	if len(input.Data) == 0 {
		return Prepared{}, preparationError(ErrorInvalid, "image is empty", nil)
	}
	if int64(len(input.Data)) > policy.MaxInputBytes {
		return Prepared{}, preparationError(
			ErrorTooLarge,
			fmt.Sprintf("image exceeds the %d byte input limit", policy.MaxInputBytes),
			nil,
		)
	}

	config, format, err := image.DecodeConfig(bytes.NewReader(input.Data))
	if err != nil {
		code := ErrorInvalid
		if errors.Is(err, image.ErrFormat) {
			code = ErrorUnsupported
		}
		return Prepared{}, preparationError(code, "could not decode image metadata", err)
	}
	mimeType, ok := mimeTypeForFormat(format)
	if !ok {
		return Prepared{}, preparationError(ErrorUnsupported, "unsupported image format", nil)
	}
	declaredMIME := strings.ToLower(strings.TrimSpace(input.DeclaredMIME))
	if declaredMIME != "" {
		if !supportedMIMEType(declaredMIME) {
			return Prepared{}, preparationError(
				ErrorUnsupported,
				fmt.Sprintf("unsupported declared image type %q", input.DeclaredMIME),
				nil,
			)
		}
		if declaredMIME != mimeType {
			return Prepared{}, preparationError(
				ErrorTypeMismatch,
				fmt.Sprintf("declared image type %q does not match detected type %q", declaredMIME, mimeType),
				nil,
			)
		}
	}
	if err := validateDimensions(config.Width, config.Height, policy.MaxPixels); err != nil {
		return Prepared{}, err
	}
	if format == "gif" {
		frames, err := gifFrameCount(input.Data)
		if err != nil {
			return Prepared{}, preparationError(ErrorInvalid, "could not decode GIF container", err)
		}
		if frames != 1 {
			return Prepared{}, preparationError(ErrorAnimated, "animated GIF images are not supported", nil)
		}
	}
	if err := contextError(ctx); err != nil {
		return Prepared{}, err
	}

	decoded, decodedFormat, err := image.Decode(bytes.NewReader(input.Data))
	if err != nil {
		return Prepared{}, preparationError(ErrorInvalid, "could not decode image pixels", err)
	}
	if decodedFormat != format {
		return Prepared{}, preparationError(ErrorInvalid, "decoded image format does not match its metadata", nil)
	}
	bounds := decoded.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width != config.Width || height != config.Height {
		return Prepared{}, preparationError(ErrorInvalid, "decoded image dimensions do not match its metadata", nil)
	}
	if err := contextError(ctx); err != nil {
		return Prepared{}, err
	}

	data := input.Data
	outputWidth, outputHeight := width, height
	resized := width > policy.MaxDimension || height > policy.MaxDimension
	if resized {
		outputWidth, outputHeight = resizedDimensions(width, height, policy.MaxDimension)
		destination := image.NewNRGBA(image.Rect(0, 0, outputWidth, outputHeight))
		xdraw.CatmullRom.Scale(destination, destination.Bounds(), decoded, bounds, xdraw.Src, nil)
		if err := contextError(ctx); err != nil {
			return Prepared{}, err
		}

		var encoded bytes.Buffer
		if format == "jpeg" {
			err = jpeg.Encode(&encoded, destination, &jpeg.Options{Quality: 90})
			mimeType = "image/jpeg"
		} else {
			err = png.Encode(&encoded, destination)
			mimeType = "image/png"
		}
		if err != nil {
			return Prepared{}, preparationError(ErrorInvalid, "could not encode prepared image", err)
		}
		data = encoded.Bytes()
		if err := verifyOutput(data, mimeType, outputWidth, outputHeight); err != nil {
			return Prepared{}, err
		}
	}
	if int64(len(data)) > policy.MaxOutputBytes {
		return Prepared{}, preparationError(
			ErrorTooLarge,
			fmt.Sprintf("prepared image exceeds the %d byte output limit", policy.MaxOutputBytes),
			nil,
		)
	}
	if err := contextError(ctx); err != nil {
		return Prepared{}, err
	}

	return Prepared{
		Content: llm.ImageContent{
			Data:     base64.StdEncoding.EncodeToString(data),
			MIMEType: mimeType,
		},
		Bytes:          len(data),
		OriginalWidth:  width,
		OriginalHeight: height,
		OutputWidth:    outputWidth,
		OutputHeight:   outputHeight,
		Resized:        resized,
		Reencoded:      resized,
	}, nil
}

func validatePolicy(policy Policy) error {
	if policy.MaxInputBytes <= 0 || policy.MaxPixels <= 0 || policy.MaxDimension <= 0 || policy.MaxOutputBytes <= 0 {
		return preparationError(ErrorInvalid, "image preparation policy limits must be positive", nil)
	}
	return nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return preparationError(ErrorCancelled, "image preparation context is nil", nil)
	}
	if err := ctx.Err(); err != nil {
		return preparationError(ErrorCancelled, "image preparation was cancelled", err)
	}
	return nil
}

func validateDimensions(width, height int, maxPixels int64) error {
	if width <= 0 || height <= 0 {
		return preparationError(ErrorInvalid, "image dimensions must be positive", nil)
	}
	if int64(width) > maxPixels/int64(height) {
		return preparationError(
			ErrorTooManyPixels,
			fmt.Sprintf("image exceeds the %d pixel limit", maxPixels),
			nil,
		)
	}
	return nil
}

func resizedDimensions(width, height, maxDimension int) (int, int) {
	if width >= height {
		return maxDimension, max(1, int((int64(height)*int64(maxDimension)+int64(width)/2)/int64(width)))
	}
	return max(1, int((int64(width)*int64(maxDimension)+int64(height)/2)/int64(height))), maxDimension
}

func supportedMIMEType(mimeType string) bool {
	switch mimeType {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		return true
	default:
		return false
	}
}

func mimeTypeForFormat(format string) (string, bool) {
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

func verifyOutput(data []byte, mimeType string, width, height int) error {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return preparationError(ErrorInvalid, "could not verify prepared image", err)
	}
	detectedMIME, ok := mimeTypeForFormat(format)
	if !ok || detectedMIME != mimeType || config.Width != width || config.Height != height {
		return preparationError(ErrorInvalid, "prepared image does not match its expected metadata", nil)
	}
	return nil
}

// gifFrameCount walks GIF container blocks without decoding every frame.
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
			offset++
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
			offset++
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
