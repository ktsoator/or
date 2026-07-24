package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ktsoator/or/coding/internal/tools"
)

const previewGrantBytes = 32

type previewGrant struct {
	ID          string
	SessionID   string
	Root        string
	EntryPath   string
	PreviewPath string
}

// previewGrantStore keeps workspace files behind process-local, unguessable
// capabilities. A grant exposes only the HTML entry's directory.
type previewGrantStore struct {
	mu      sync.RWMutex
	grants  map[string]previewGrant
	byEntry map[string]string
}

func newPreviewGrantStore() *previewGrantStore {
	return &previewGrantStore{
		grants:  make(map[string]previewGrant),
		byEntry: make(map[string]string),
	}
}

func (s *previewGrantStore) issue(
	sessionID string,
	workspacePath string,
	preview tools.PreviewRequest,
) (tools.PreviewRequest, error) {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(preview.Path) == "" {
		return tools.PreviewRequest{}, fmt.Errorf("workspace preview grant requires a session and HTML path")
	}

	entryPath, relativePath, err := resolvePreviewEntry(workspacePath, preview.Path)
	if err != nil {
		return tools.PreviewRequest{}, err
	}
	if workspacePath == "" && strings.TrimSpace(preview.RelativePath) != "" {
		relativePath = filepath.ToSlash(filepath.Clean(preview.RelativePath))
	}
	if !previewAssetAllowed(relativePath) {
		return tools.PreviewRequest{}, fmt.Errorf("workspace preview entry is not allowed")
	}

	entryKey := sessionID + "\x00" + entryPath
	s.mu.Lock()
	defer s.mu.Unlock()
	if id := s.byEntry[entryKey]; id != "" {
		grant := s.grants[id]
		return enrichPreview(preview, grant, relativePath), nil
	}

	id, err := randomPreviewGrantID()
	if err != nil {
		return tools.PreviewRequest{}, err
	}
	for s.grants[id].ID != "" {
		id, err = randomPreviewGrantID()
		if err != nil {
			return tools.PreviewRequest{}, err
		}
	}
	grant := previewGrant{
		ID:          id,
		SessionID:   sessionID,
		Root:        filepath.Dir(entryPath),
		EntryPath:   entryPath,
		PreviewPath: filepath.Base(entryPath),
	}
	s.grants[id] = grant
	s.byEntry[entryKey] = id
	return enrichPreview(preview, grant, relativePath), nil
}

func resolvePreviewEntry(workspacePath, path string) (string, string, error) {
	if workspacePath != "" {
		return tools.ResolvePreviewDocument(workspacePath, path)
	}
	entryPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", fmt.Errorf("resolve workspace preview: %w", err)
	}
	entryPath, err = filepath.EvalSymlinks(entryPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve workspace preview: %w", err)
	}
	info, err := os.Stat(entryPath)
	if err != nil {
		return "", "", fmt.Errorf("workspace preview is not available: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("workspace preview must be a regular file")
	}
	extension := strings.ToLower(filepath.Ext(entryPath))
	if extension != ".html" && extension != ".htm" {
		return "", "", fmt.Errorf("workspace preview must be an HTML document")
	}
	return entryPath, filepath.Base(entryPath), nil
}

func enrichPreview(
	preview tools.PreviewRequest,
	grant previewGrant,
	relativePath string,
) tools.PreviewRequest {
	preview.Path = grant.EntryPath
	preview.RelativePath = filepath.ToSlash(relativePath)
	preview.GrantID = grant.ID
	preview.PreviewPath = filepath.ToSlash(grant.PreviewPath)
	return preview
}

func (s *previewGrantStore) resolve(sessionID, grantID string) (previewGrant, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	grant, ok := s.grants[grantID]
	return grant, ok && grant.SessionID == sessionID
}

func (s *previewGrantStore) revokeSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, grant := range s.grants {
		if grant.SessionID != sessionID {
			continue
		}
		delete(s.grants, id)
		delete(s.byEntry, sessionID+"\x00"+grant.EntryPath)
	}
}

func randomPreviewGrantID() (string, error) {
	value := make([]byte, previewGrantBytes)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("create workspace preview grant: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func previewAssetAllowed(path string) bool {
	normalized := strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	if normalized == "" || strings.HasPrefix(normalized, "/") {
		return false
	}
	segments := strings.Split(normalized, "/")
	for _, segment := range segments {
		lower := strings.ToLower(segment)
		if segment == "" || segment == "." || segment == ".." || strings.HasPrefix(segment, ".") {
			return false
		}
		if strings.ContainsAny(segment, "\x00\r\n") || sensitivePreviewName(lower) {
			return false
		}
	}
	return true
}

func sensitivePreviewName(name string) bool {
	extension := strings.ToLower(filepath.Ext(name))
	stem := strings.TrimSuffix(name, extension)
	switch stem {
	case "credential", "credentials", "secret", "secrets", "service-account", "service_account", "account-key", "account_key":
		return true
	}
	for _, prefix := range []string{"id_rsa", "id_dsa", "id_ecdsa", "id_ed25519"} {
		if name == prefix || strings.HasPrefix(name, prefix+".") {
			return true
		}
	}
	switch extension {
	case ".pem", ".key", ".p12", ".pfx":
		return true
	default:
		return false
	}
}

func reissuePreviewGrants(
	store *previewGrantStore,
	sessionID string,
	workspacePath string,
	events []wireEvent,
) {
	for index := range events {
		preview := events[index].Preview
		if preview == nil || preview.Path == "" {
			continue
		}
		enriched, err := store.issue(sessionID, workspacePath, tools.PreviewRequest{
			URL:          preview.URL,
			Path:         preview.Path,
			RelativePath: preview.RelativePath,
			Title:        preview.Title,
		})
		if err != nil {
			events[index].Preview = nil
			continue
		}
		events[index].Preview = previewPayload(enriched)
	}
}
