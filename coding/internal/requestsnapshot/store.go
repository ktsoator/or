// Package requestsnapshot stores inspectable, provider-neutral model exchanges.
// Snapshots are separate from the privacy-safe performance event log because
// they contain conversation content and are loaded only on explicit request.
package requestsnapshot

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ktsoator/or/llm"
)

const (
	CurrentVersion            = 3
	DefaultMaxSnapshots       = 200
	DefaultMaxBytes     int64 = 128 << 20
	DefaultMaxFileBytes       = 32 << 20
)

var (
	ErrNotFound       = errors.New("request snapshot not found")
	ErrInvalidID      = errors.New("request snapshot ID is invalid")
	requestIDPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	secretAssignments = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(authorization\s*[:=]\s*(?:bearer\s+)?)[^\s,"'}]+`),
		regexp.MustCompile(`(?i)((?:api[_-]?key|access[_-]?token|auth[_-]?token|password|secret)\s*[:=]\s*)[^\s,"'}]+`),
		regexp.MustCompile(`\b(?:sk|sk-ant|ghp|github_pat|xox[baprs])[-_][A-Za-z0-9_-]{8,}\b`),
	}
)

// Attachment identifies one product-generated message inside Input.Messages.
type Attachment struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Placement    string `json:"placement"`
	Path         string `json:"path,omitempty"`
	Revision     string `json:"revision,omitempty"`
	MessageIndex int    `json:"messageIndex"`
}

// Image contains inspectable metadata without retaining base64 image bytes.
type Image struct {
	MIMEType     string `json:"mimeType"`
	EncodedBytes int    `json:"encodedBytes,omitempty"`
}

// Content is a signature-free, provider-neutral input content block.
type Content struct {
	Type       string         `json:"type"`
	Text       string         `json:"text,omitempty"`
	Thinking   string         `json:"thinking,omitempty"`
	Redacted   bool           `json:"redacted,omitempty"`
	Image      *Image         `json:"image,omitempty"`
	ToolCallID string         `json:"toolCallId,omitempty"`
	ToolName   string         `json:"toolName,omitempty"`
	Arguments  map[string]any `json:"arguments,omitempty"`
}

// Message is one model-input message with only inspectable replay content.
type Message struct {
	Role              string    `json:"role"`
	Content           []Content `json:"content"`
	ProviderRequestID string    `json:"providerRequestId,omitempty"`
	ToolCallID        string    `json:"toolCallId,omitempty"`
	ToolName          string    `json:"toolName,omitempty"`
	IsError           bool      `json:"isError,omitempty"`
}

// Tool is one tool definition advertised to the model.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Strict      *bool           `json:"strict,omitempty"`
}

// Input is the complete inspectable request after context projection.
type Input struct {
	SystemPrompt string    `json:"systemPrompt,omitempty"`
	Messages     []Message `json:"messages"`
	Tools        []Tool    `json:"tools,omitempty"`
}

// Output is the terminal provider message captured after streaming completes.
type Output struct {
	CapturedAt   time.Time `json:"capturedAt"`
	Message      Message   `json:"message"`
	StopReason   string    `json:"stopReason,omitempty"`
	ErrorMessage string    `json:"errorMessage,omitempty"`
}

// Snapshot correlates a provider-neutral exchange with the performance timeline.
type Snapshot struct {
	Version           int          `json:"version"`
	CapturedAt        time.Time    `json:"capturedAt"`
	SessionID         string       `json:"sessionId"`
	RunID             string       `json:"runId"`
	TurnID            string       `json:"turnId"`
	ProviderRequestID string       `json:"providerRequestId"`
	Provider          string       `json:"provider"`
	Model             string       `json:"model"`
	Input             Input        `json:"input"`
	Output            *Output      `json:"output,omitempty"`
	Attachments       []Attachment `json:"attachments,omitempty"`
}

// Writer records request snapshots without coupling the engine to file I/O.
type Writer interface {
	Save(Snapshot) error
	SaveOutput(providerRequestID string, message *llm.AssistantMessage) error
}

// Reader loads one snapshot on demand for the diagnostics API.
type Reader interface {
	Load(providerRequestID string) (Snapshot, error)
}

// SessionCleaner removes every private snapshot owned by one session.
type SessionCleaner interface {
	DeleteSession(sessionID string) error
}

// DiscardWriter keeps snapshot capture optional and fail-open.
type DiscardWriter struct{}

func (DiscardWriter) Save(Snapshot) error                            { return nil }
func (DiscardWriter) SaveOutput(string, *llm.AssistantMessage) error { return nil }
func (DiscardWriter) DeleteSession(string) error                     { return nil }

// OrDiscard replaces a nil writer with a no-op implementation.
func OrDiscard(writer Writer) Writer {
	if writer == nil {
		return DiscardWriter{}
	}
	return writer
}

// Options bounds private local snapshot storage. Zero values use defaults.
type Options struct {
	MaxSnapshots int
	MaxBytes     int64
	MaxFileBytes int64
}

// FileStore stores one private JSON file per provider request.
type FileStore struct {
	mu           sync.Mutex
	dir          string
	maxSnapshots int
	maxBytes     int64
	maxFileBytes int64
}

// NewFileStore creates and secures a local request snapshot directory.
func NewFileStore(dir string, options Options) (*FileStore, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("request snapshot directory is empty")
	}
	if options.MaxSnapshots <= 0 {
		options.MaxSnapshots = DefaultMaxSnapshots
	}
	if options.MaxBytes <= 0 {
		options.MaxBytes = DefaultMaxBytes
	}
	if options.MaxFileBytes <= 0 {
		options.MaxFileBytes = DefaultMaxFileBytes
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	return &FileStore{
		dir: dir, maxSnapshots: options.MaxSnapshots,
		maxBytes: options.MaxBytes, maxFileBytes: options.MaxFileBytes,
	}, nil
}

// NewSnapshot removes replay-only signatures and image payloads while keeping
// every piece of content a person can meaningfully inspect.
func NewSnapshot(
	sessionID, runID, turnID, requestID, provider, model string,
	input llm.Context,
	attachments []Attachment,
) Snapshot {
	return Snapshot{
		Version: CurrentVersion, CapturedAt: time.Now().UTC(),
		SessionID: sessionID, RunID: runID, TurnID: turnID,
		ProviderRequestID: requestID, Provider: provider, Model: model,
		Input:       sanitizeInput(input),
		Attachments: append([]Attachment(nil), attachments...),
	}
}

// Save atomically writes a snapshot and removes the oldest files as needed.
func (store *FileStore) Save(snapshot Snapshot) error {
	if store == nil {
		return nil
	}
	if !validRequestID(snapshot.ProviderRequestID) {
		return ErrInvalidID
	}
	if snapshot.Version == 0 {
		snapshot.Version = CurrentVersion
	}
	if snapshot.CapturedAt.IsZero() {
		snapshot.CapturedAt = time.Now().UTC()
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.saveLocked(snapshot, true)
}

// SaveOutput completes an existing request snapshot without exposing content
// through the privacy-safe performance event log.
func (store *FileStore) SaveOutput(requestID string, message *llm.AssistantMessage) error {
	if store == nil || message == nil {
		return nil
	}
	if !validRequestID(requestID) {
		return ErrInvalidID
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	snapshot, err := store.loadLocked(requestID)
	if err != nil {
		return err
	}
	snapshot.Version = CurrentVersion
	outputMessage := sanitizeMessage(message)
	outputMessage.ProviderRequestID = requestID
	snapshot.Output = &Output{
		CapturedAt: time.Now().UTC(), Message: outputMessage,
		StopReason: string(message.StopReason), ErrorMessage: sanitizeText(message.ErrorMessage),
	}
	return store.saveLocked(snapshot, true)
}

func (store *FileStore) saveLocked(snapshot Snapshot, prune bool) error {
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode request snapshot: %w", err)
	}
	if int64(len(encoded)) > store.maxFileBytes {
		return fmt.Errorf("request snapshot exceeds %d bytes", store.maxFileBytes)
	}
	if err := os.Chmod(store.dir, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(store.dir, ".snapshot-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(encoded); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	path := store.path(snapshot.ProviderRequestID)
	if err := os.Rename(temporaryName, path); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	if prune {
		return store.pruneLocked()
	}
	return nil
}

// Load reads one snapshot by its correlated provider request ID.
func (store *FileStore) Load(requestID string) (Snapshot, error) {
	if store == nil {
		return Snapshot{}, ErrNotFound
	}
	if !validRequestID(requestID) {
		return Snapshot{}, ErrInvalidID
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.loadLocked(requestID)
}

func (store *FileStore) loadLocked(requestID string) (Snapshot, error) {
	if err := os.Chmod(store.dir, 0o700); err != nil {
		return Snapshot{}, err
	}
	path := store.path(requestID)
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, ErrNotFound
	}
	if err != nil {
		return Snapshot{}, err
	}
	defer file.Close()
	if err := os.Chmod(path, 0o600); err != nil {
		return Snapshot{}, err
	}
	limited := io.LimitReader(file, store.maxFileBytes+1)
	encoded, err := io.ReadAll(limited)
	if err != nil {
		return Snapshot{}, err
	}
	if int64(len(encoded)) > store.maxFileBytes {
		return Snapshot{}, errors.New("request snapshot file exceeds size limit")
	}
	var snapshot Snapshot
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("decode request snapshot: %w", err)
	}
	if snapshot.ProviderRequestID != requestID {
		return Snapshot{}, errors.New("request snapshot identity mismatch")
	}
	return snapshot, nil
}

// DeleteSession removes every snapshot correlated with sessionID. The store
// lock prevents pruning, loading, or saving from racing the directory scan.
func (store *FileStore) DeleteSession(sessionID string) error {
	if store == nil {
		return nil
	}
	if strings.TrimSpace(sessionID) == "" {
		return errors.New("request snapshot session ID is empty")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	entries, err := os.ReadDir(store.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(store.dir, entry.Name())
		encoded, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var identity struct {
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(encoded, &identity); err != nil {
			return fmt.Errorf("decode request snapshot identity: %w", err)
		}
		if identity.SessionID != sessionID {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (store *FileStore) path(requestID string) string {
	return filepath.Join(store.dir, requestID+".json")
}

type storedFile struct {
	path    string
	modTime time.Time
	size    int64
}

func (store *FileStore) pruneLocked() error {
	entries, err := os.ReadDir(store.dir)
	if err != nil {
		return err
	}
	files := make([]storedFile, 0, len(entries))
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files = append(files, storedFile{
			path: filepath.Join(store.dir, entry.Name()), modTime: info.ModTime(), size: info.Size(),
		})
		total += info.Size()
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].modTime.Equal(files[j].modTime) {
			return files[i].path < files[j].path
		}
		return files[i].modTime.Before(files[j].modTime)
	})
	for len(files) > store.maxSnapshots || total > store.maxBytes {
		oldest := files[0]
		if err := os.Remove(oldest.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		total -= oldest.size
		files = files[1:]
	}
	return nil
}

func validRequestID(value string) bool { return requestIDPattern.MatchString(value) }

var _ Writer = (*FileStore)(nil)
var _ Reader = (*FileStore)(nil)
var _ SessionCleaner = (*FileStore)(nil)

func sanitizeInput(input llm.Context) Input {
	result := Input{
		SystemPrompt: sanitizeText(input.SystemPrompt),
		Messages:     make([]Message, 0, len(input.Messages)),
		Tools:        make([]Tool, 0, len(input.Tools)),
	}
	for _, message := range input.Messages {
		result.Messages = append(result.Messages, sanitizeMessage(message))
	}
	for _, tool := range input.Tools {
		result.Tools = append(result.Tools, Tool{
			Name: tool.Name, Description: sanitizeText(tool.Description),
			Parameters: sanitizeRawJSON(tool.Parameters), Strict: tool.Strict,
		})
	}
	return result
}

func sanitizeMessage(message llm.Message) Message {
	switch typed := message.(type) {
	case *llm.UserMessage:
		result := Message{Role: "user", Content: make([]Content, 0, len(typed.Content))}
		for _, content := range typed.Content {
			result.Content = append(result.Content, sanitizeContent(content))
		}
		return result
	case *llm.AssistantMessage:
		result := Message{
			Role: "assistant", ProviderRequestID: typed.ProviderRequestID,
			Content: make([]Content, 0, len(typed.Content)),
		}
		for _, content := range typed.Content {
			result.Content = append(result.Content, sanitizeContent(content))
		}
		return result
	case *llm.ToolResultMessage:
		result := Message{
			Role: "toolResult", ToolCallID: typed.ToolCallID,
			ToolName: typed.ToolName, IsError: typed.IsError,
			Content: make([]Content, 0, len(typed.Content)),
		}
		for _, content := range typed.Content {
			result.Content = append(result.Content, sanitizeContent(content))
		}
		return result
	default:
		return Message{Role: "unknown", Content: []Content{}}
	}
}

func sanitizeContent(content any) Content {
	switch typed := content.(type) {
	case *llm.TextContent:
		return Content{Type: "text", Text: sanitizeText(typed.Text)}
	case *llm.ThinkingContent:
		if typed.Redacted {
			return Content{Type: "thinking", Thinking: "[redacted reasoning omitted]", Redacted: true}
		}
		return Content{Type: "thinking", Thinking: sanitizeText(typed.Thinking)}
	case *llm.ImageContent:
		return Content{Type: "image", Image: &Image{
			MIMEType: typed.MIMEType, EncodedBytes: decodedBase64Size(typed.Data),
		}}
	case *llm.ToolCall:
		return Content{
			Type: "toolCall", ToolCallID: typed.ID, ToolName: typed.Name,
			Arguments: sanitizeMap(typed.Arguments),
		}
	default:
		return Content{Type: "unknown"}
	}
}

func decodedBase64Size(data string) int {
	if data == "" {
		return 0
	}
	size := base64.StdEncoding.DecodedLen(len(data))
	padding := 0
	if strings.HasSuffix(data, "==") {
		padding = 2
	} else if strings.HasSuffix(data, "=") {
		padding = 1
	}
	return max(0, size-padding)
}

func sanitizeMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	result := make(map[string]any, len(input))
	for key, value := range input {
		if sensitiveKey(key) {
			result[key] = "[redacted]"
			continue
		}
		result[key] = sanitizeValue(value)
	}
	return result
}

func sanitizeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return sanitizeMap(typed)
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = sanitizeValue(item)
		}
		return result
	case string:
		return sanitizeText(typed)
	default:
		return value
	}
}

func sanitizeRawJSON(input json.RawMessage) json.RawMessage {
	if len(input) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(input, &value); err != nil {
		return json.RawMessage(sanitizeText(string(input)))
	}
	encoded, err := json.Marshal(sanitizeValue(value))
	if err != nil {
		return json.RawMessage(`{"error":"schema unavailable"}`)
	}
	return encoded
}

func sanitizeText(value string) string {
	result := value
	for _, pattern := range secretAssignments {
		if pattern.NumSubexp() > 0 {
			result = pattern.ReplaceAllString(result, `${1}[redacted]`)
		} else {
			result = pattern.ReplaceAllString(result, "[redacted]")
		}
	}
	return result
}

func sensitiveKey(key string) bool {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, key)
	for _, fragment := range []string{
		"apikey", "authorization", "credential", "password", "secret", "cookie",
	} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	if normalized == "token" {
		return true
	}
	for _, tokenKey := range []string{"accesstoken", "authtoken", "bearertoken", "idtoken", "refreshtoken", "sessiontoken"} {
		if strings.HasSuffix(normalized, tokenKey) {
			return true
		}
	}
	return false
}
