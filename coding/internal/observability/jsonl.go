package observability

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	DefaultMaxBytes int64 = 10 << 20
	DefaultMaxFiles       = 5
)

// FileOptions configures bounded local JSONL storage. MaxFiles includes the
// active file. Zero values use the product defaults.
type FileOptions struct {
	MaxBytes int64
	MaxFiles int
}

// JSONLRecorder writes one JSON object per line to a bounded set of private
// local files.
type JSONLRecorder struct {
	handler slog.Handler
	writer  *rotatingWriter
}

// NewJSONL opens a JSONL recorder rooted at path.
func NewJSONL(path string, options FileOptions) (*JSONLRecorder, error) {
	if options.MaxBytes <= 0 {
		options.MaxBytes = DefaultMaxBytes
	}
	if options.MaxFiles <= 0 {
		options.MaxFiles = DefaultMaxFiles
	}
	writer, err := newRotatingWriter(path, options.MaxBytes, options.MaxFiles)
	if err != nil {
		return nil, err
	}
	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.MessageKey {
				attr.Key = "event"
			}
			return attr
		},
	})
	return &JSONLRecorder{handler: handler, writer: writer}, nil
}

func (r *JSONLRecorder) Record(event Event) {
	if r == nil {
		return
	}
	recordWithHandler(r.handler, event)
}

func (r *JSONLRecorder) Close() error {
	if r == nil || r.writer == nil {
		return nil
	}
	return r.writer.Close()
}

// DeleteSession removes matching records from the active log and every
// rotation while keeping the recorder open for subsequent events.
func (r *JSONLRecorder) DeleteSession(sessionID string) error {
	if r == nil || r.writer == nil {
		return nil
	}
	if strings.TrimSpace(sessionID) == "" {
		return errors.New("observability: session ID is empty")
	}
	return r.writer.deleteSession(sessionID)
}

type rotatingWriter struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	maxFiles int
	file     *os.File
	size     int64
	closed   bool
}

func newRotatingWriter(path string, maxBytes int64, maxFiles int) (*rotatingWriter, error) {
	if path == "" {
		return nil, errors.New("observability: log path is empty")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	writer := &rotatingWriter{path: path, maxBytes: maxBytes, maxFiles: maxFiles}
	if err := writer.open(); err != nil {
		return nil, err
	}
	return writer, nil
}

func (w *rotatingWriter) open() error {
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	w.file = file
	w.size = info.Size()
	return nil
}

func (w *rotatingWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, os.ErrClosed
	}
	if w.file == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	if w.size > 0 && w.size+int64(len(data)) > w.maxBytes {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	written, err := w.file.Write(data)
	w.size += int64(written)
	return written, err
}

func (w *rotatingWriter) rotate() error {
	if w.file == nil {
		return w.open()
	}
	if err := w.file.Close(); err != nil {
		return err
	}
	w.file = nil
	w.size = 0
	if err := w.rotateFiles(); err != nil {
		// Keep the writer usable after a filesystem-level rotation failure. The
		// triggering record may be lost, but diagnostics must never panic or make
		// the coding run fail.
		_ = w.open()
		return err
	}
	return w.open()
}

func (w *rotatingWriter) rotateFiles() error {
	if w.maxFiles > 1 {
		oldest := backupPath(w.path, w.maxFiles-1)
		if err := os.Remove(oldest); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		for index := w.maxFiles - 2; index >= 1; index-- {
			from := backupPath(w.path, index)
			to := backupPath(w.path, index+1)
			if err := os.Rename(from, to); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		if err := os.Rename(w.path, backupPath(w.path, 1)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else if err := os.Remove(w.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (w *rotatingWriter) deleteSession(sessionID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return os.ErrClosed
	}
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return err
		}
		w.file = nil
	}
	w.size = 0
	var rewriteErr error
	for _, path := range diagnosticLogPaths(w.path) {
		if err := rewriteJSONLWithoutSession(path, sessionID); err != nil {
			rewriteErr = err
			break
		}
	}
	return errors.Join(rewriteErr, w.open())
}

func rewriteJSONLWithoutSession(path, sessionID string) error {
	source, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".observability-cleanup-*.tmp")
	if err != nil {
		_ = source.Close()
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = source.Close()
		_ = temporary.Close()
		return err
	}

	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		var identity struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(line, &identity); err == nil && identity.SessionID == sessionID {
			continue
		}
		if _, err := temporary.Write(line); err != nil {
			_ = source.Close()
			_ = temporary.Close()
			return err
		}
		if _, err := temporary.Write([]byte{'\n'}); err != nil {
			_ = source.Close()
			_ = temporary.Close()
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		_ = source.Close()
		_ = temporary.Close()
		return fmt.Errorf("scan observability log: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = source.Close()
		_ = temporary.Close()
		return err
	}
	if err := source.Close(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if w.file == nil {
		return nil
	}
	return w.file.Close()
}

func backupPath(path string, index int) string {
	return path + "." + strconv.Itoa(index)
}

var _ io.Writer = (*rotatingWriter)(nil)
var _ SessionCleaner = (*JSONLRecorder)(nil)
