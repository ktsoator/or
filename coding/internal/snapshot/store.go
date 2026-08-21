package snapshot

import (
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

	"github.com/ktsoator/or/llm"
)

const (
	DefaultMaxSnapshots       = 200
	DefaultMaxBytes     int64 = 128 << 20
	DefaultMaxFileBytes       = 32 << 20
)

var (
	ErrNotFound      = errors.New("request snapshot not found")
	ErrInvalidID     = errors.New("request snapshot ID is invalid")
	requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
)

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

// NewFileStore creates and secures a local request snapshot directory. It is
// retained for legacy fixtures and cleanup; production no longer writes new
// request snapshot files.
func NewFileStore(dir string, options Options) (*FileStore, error) {
	return newFileStore(dir, options, true)
}

// OpenLegacyFileStore opens an existing request snapshot directory without
// creating one. Production uses it only to cascade session deletion to files
// left by older releases.
func OpenLegacyFileStore(dir string) (*FileStore, error) {
	return newFileStore(dir, Options{}, false)
}

func newFileStore(dir string, options Options, create bool) (*FileStore, error) {
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
	if create {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	} else {
		info, err := os.Stat(dir)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("request snapshot path %q is not a directory", dir)
		}
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	return &FileStore{
		dir: dir, maxSnapshots: options.MaxSnapshots,
		maxBytes: options.MaxBytes, maxFileBytes: options.MaxFileBytes,
	}, nil
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

// LoadForSession implements Reader while keeping file-backed fixtures and
// legacy cleanup scoped to the owning conversation.
func (store *FileStore) LoadForSession(
	sessionID, requestID string,
) (Snapshot, error) {
	record, err := store.Load(requestID)
	if err != nil {
		return Snapshot{}, err
	}
	if sessionID != "" && record.SessionID != sessionID {
		return Snapshot{}, ErrNotFound
	}
	return record, nil
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

var _ Reader = (*FileStore)(nil)
var _ SessionCleaner = (*FileStore)(nil)
