// Package store belongs to the Infrastructure Layer.
// It provides JSON file persistence for SoulContainerSnapshot (.flysoul.json).
package store

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/jhinw/flybrain/flybody/internal/app/runtime"
)

// JSONSnapshotStore belongs to the Infrastructure Layer.
// Primary Responsibility: Serialize and deserialize SoulContainerSnapshot to/from disk.
type JSONSnapshotStore struct{}

// NewJSONSnapshotStore creates a new JSONSnapshotStore.
func NewJSONSnapshotStore() *JSONSnapshotStore {
	return &JSONSnapshotStore{}
}

// Save writes the snapshot as indented JSON to the target path.
func (s *JSONSnapshotStore) Save(path string, snap runtime.SoulContainerSnapshot) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Load reads and unmarshals a SoulContainerSnapshot from the target path.
func (s *JSONSnapshotStore) Load(path string) (runtime.SoulContainerSnapshot, error) {
	var snap runtime.SoulContainerSnapshot
	data, err := os.ReadFile(path)
	if err != nil {
		return snap, err
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		return snap, err
	}
	return snap, nil
}
