package store

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// SeenState tracks which TaskItem IDs the poller has already notified
// about, so the same item isn't notified again on every poll.
type SeenState struct {
	IDs map[string]int64 `json:"ids"` // TaskItem.ID -> unix timestamp first seen
}

func stateFilePath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

// LoadSeenState reads the persisted dedup state, or an empty one if none exists yet.
func LoadSeenState() (SeenState, error) {
	path, err := stateFilePath()
	if err != nil {
		return SeenState{}, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return SeenState{IDs: map[string]int64{}}, nil
	}
	if err != nil {
		return SeenState{}, err
	}

	var s SeenState
	if err := json.Unmarshal(data, &s); err != nil {
		return SeenState{}, err
	}
	if s.IDs == nil {
		s.IDs = map[string]int64{}
	}
	return s, nil
}

// SaveSeenState persists the dedup state.
func SaveSeenState(s SeenState) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	path, err := stateFilePath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
