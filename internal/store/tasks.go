package store

import (
	"encoding/json"
	"os"
	"path/filepath"

	"koalmine/internal/providers"
)

func tasksFilePath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tasks.json"), nil
}

// LoadCachedTasks returns the last persisted snapshot of tasks, or an empty
// slice if none has been saved yet — so the window has something to show
// immediately on launch, before the first poll completes.
func LoadCachedTasks() ([]providers.TaskItem, error) {
	path, err := tasksFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []providers.TaskItem{}, nil
	}
	if err != nil {
		return nil, err
	}

	var tasks []providers.TaskItem
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

// SaveCachedTasks persists the latest snapshot of tasks, wholesale replacing
// whatever was cached before.
func SaveCachedTasks(tasks []providers.TaskItem) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	path, err := tasksFilePath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
