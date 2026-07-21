package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Summary is a registered project root as the browser sees it.
type Summary struct {
	Path    string    `json:"path"`
	Name    string    `json:"name"`
	AddedAt time.Time `json:"addedAt"`
}

type record struct {
	Path    string    `json:"path"`
	AddedAt time.Time `json:"addedAt"`
}

func (r record) summary() Summary {
	return Summary{
		Path:    r.Path,
		Name:    filepath.Base(r.Path),
		AddedAt: r.AddedAt,
	}
}

// Registry is the durable list of project roots shown in the sidebar. It is
// persisted separately from sessions so an empty project can stay registered.
//
// Lock ordering: a caller that owns both a session lock and this registry must
// always take the session lock first. Registry never calls back into session
// code, which keeps that ordering trivially satisfiable.
type Registry struct {
	path string

	mu      sync.RWMutex
	entries map[string]record
}

// NewRegistry restores the registry from indexPath. Entries whose paths no
// longer normalize are dropped rather than failing startup, matching how the
// sidebar has always tolerated a project that moved.
func NewRegistry(indexPath string) (*Registry, error) {
	r := &Registry{path: indexPath, entries: make(map[string]record)}
	data, err := os.ReadFile(indexPath)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return nil, fmt.Errorf("workspace: read index: %w", err)
	}
	var records []record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("workspace: decode index: %w", err)
	}
	for _, entry := range records {
		cleaned, cleanErr := Clean(entry.Path)
		if cleanErr != nil {
			continue
		}
		entry.Path = cleaned
		r.entries[cleaned] = entry
	}
	return r, nil
}

// Register validates a project root and persists it. Registering a path that
// is already present is a no-op that returns the existing entry.
func (r *Registry) Register(path string) (Summary, error) {
	cleaned, err := Validate(path)
	if err != nil {
		return Summary{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.entries[cleaned]; ok {
		return existing.summary(), nil
	}
	entry := record{Path: cleaned, AddedAt: time.Now().UTC()}
	r.entries[cleaned] = entry
	if err := r.saveLocked(); err != nil {
		delete(r.entries, cleaned)
		return Summary{}, err
	}
	return entry.summary(), nil
}

// Remove drops a project from the sidebar. Transcripts and workspace files are
// intentionally retained; registering the directory again restores the view.
func (r *Registry) Remove(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%w: path is required", ErrInvalid)
	}
	cleaned, err := Clean(path)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[cleaned]
	if !ok {
		return nil
	}
	delete(r.entries, cleaned)
	if err := r.saveLocked(); err != nil {
		r.entries[cleaned] = entry
		return err
	}
	return nil
}

// Ensure adds a project root in memory without persisting it, reporting
// whether this call was the one that added it. Creating a session registers
// its workspace and writes both indexes as one unit, so the caller flushes
// with Save and undoes a failure with Discard.
func (r *Registry) Ensure(path string, addedAt time.Time) (Summary, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.entries[path]; ok {
		return existing.summary(), false
	}
	if addedAt.IsZero() {
		addedAt = time.Now().UTC()
	}
	entry := record{Path: path, AddedAt: addedAt}
	r.entries[path] = entry
	return entry.summary(), true
}

// Discard undoes an Ensure that was never persisted. It is only correct for a
// path Ensure reported as newly added.
func (r *Registry) Discard(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, path)
}

// List returns registered projects newest-added first, including projects that
// currently have no conversations.
func (r *Registry) List() []Summary {
	r.mu.RLock()
	out := make([]Summary, 0, len(r.entries))
	for _, entry := range r.entries {
		out = append(out, entry.summary())
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].AddedAt.Equal(out[j].AddedAt) {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		}
		return out[i].AddedAt.After(out[j].AddedAt)
	})
	return out
}

// Save flushes the registry to disk.
func (r *Registry) Save() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saveLocked()
}

func (r *Registry) saveLocked() error {
	records := make([]record, 0, len(r.entries))
	for _, entry := range r.entries {
		records = append(records, entry)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].AddedAt.Before(records[j].AddedAt) })
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("workspace: encode index: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return fmt.Errorf("workspace: create directory: %w", err)
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("workspace: write index: %w", err)
	}
	if err := os.Rename(tmp, r.path); err != nil {
		return fmt.Errorf("workspace: replace index: %w", err)
	}
	return nil
}
