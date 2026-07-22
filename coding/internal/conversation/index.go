package conversation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func (m *Manager) loadRecords() ([]record, error) {
	data, err := os.ReadFile(m.indexPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("session: read session index: %w", err)
	}
	var records []record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("session: decode session index: %w", err)
	}
	return records, nil
}

func (m *Manager) saveLocked() error {
	// Both indexes move together: a session that registered a new workspace must
	// not be persisted while that workspace is missing from the sidebar.
	if err := m.workspaces.Save(); err != nil {
		return err
	}
	records := make([]record, 0, len(m.sessions))
	for _, runtime := range m.sessions {
		records = append(records, runtime.record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt.Before(records[j].CreatedAt) })
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("session: encode session index: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(m.indexPath), 0o755); err != nil {
		return fmt.Errorf("session: create session directory: %w", err)
	}
	tmp := m.indexPath + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("session: write session index: %w", err)
	}
	if err := os.Rename(tmp, m.indexPath); err != nil {
		return fmt.Errorf("session: replace session index: %w", err)
	}
	return nil
}
