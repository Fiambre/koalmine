package store

import (
	"testing"
	"time"

	"koalmine/internal/providers"
)

func TestLoadCachedTasksReturnsEmptyWhenNoFileExists(t *testing.T) {
	withTempConfigDir(t)

	tasks, err := LoadCachedTasks()
	if err != nil {
		t.Fatalf("LoadCachedTasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected an empty slice, got %d items", len(tasks))
	}
}

func TestSaveThenLoadCachedTasksRoundTrips(t *testing.T) {
	withTempConfigDir(t)

	want := []providers.TaskItem{
		{ID: "redmine:1", Provider: "redmine", Type: providers.ItemTypeIssue, Title: "Arreglar el build", UpdatedAt: time.Now().Truncate(time.Second)},
		{ID: "github:2", Provider: "github", Type: providers.ItemTypePR, Title: "Revisar PR"},
	}
	if err := SaveCachedTasks(want); err != nil {
		t.Fatalf("SaveCachedTasks: %v", err)
	}

	got, err := LoadCachedTasks()
	if err != nil {
		t.Fatalf("LoadCachedTasks: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d tasks, got %d", len(want), len(got))
	}
	if got[0].Title != want[0].Title || !got[0].UpdatedAt.Equal(want[0].UpdatedAt) {
		t.Errorf("unexpected first task after round-trip: %+v", got[0])
	}
	if got[1].ID != want[1].ID {
		t.Errorf("unexpected second task after round-trip: %+v", got[1])
	}
}

func TestSaveCachedTasksOverwritesPreviousSnapshot(t *testing.T) {
	withTempConfigDir(t)

	if err := SaveCachedTasks([]providers.TaskItem{{ID: "a"}, {ID: "b"}}); err != nil {
		t.Fatalf("SaveCachedTasks (first): %v", err)
	}
	if err := SaveCachedTasks([]providers.TaskItem{{ID: "c"}}); err != nil {
		t.Fatalf("SaveCachedTasks (second): %v", err)
	}

	got, err := LoadCachedTasks()
	if err != nil {
		t.Fatalf("LoadCachedTasks: %v", err)
	}
	if len(got) != 1 || got[0].ID != "c" {
		t.Errorf("expected the cache to be wholesale replaced, got %+v", got)
	}
}
