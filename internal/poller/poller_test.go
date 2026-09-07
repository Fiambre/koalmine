package poller

import (
	"testing"

	"koalmine/internal/providers"
)

func TestUnseenItemsFiltersAlreadySeen(t *testing.T) {
	items := []providers.TaskItem{
		{ID: "a", Title: "A"},
		{ID: "b", Title: "B"},
		{ID: "c", Title: "C"},
	}
	seen := map[string]int64{"a": 1, "c": 1}

	got := unseenItems(items, seen)
	if len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("expected only item %q, got %+v", "b", got)
	}
}

func TestUnseenItemsAllNewWhenStateEmpty(t *testing.T) {
	items := []providers.TaskItem{{ID: "a"}, {ID: "b"}}
	got := unseenItems(items, map[string]int64{})
	if len(got) != 2 {
		t.Fatalf("expected all 2 items to be unseen, got %d", len(got))
	}
}

func TestUnseenItemsNoneWhenAllSeen(t *testing.T) {
	items := []providers.TaskItem{{ID: "a"}, {ID: "b"}}
	seen := map[string]int64{"a": 1, "b": 1}
	got := unseenItems(items, seen)
	if len(got) != 0 {
		t.Fatalf("expected no unseen items, got %d", len(got))
	}
}

func TestTypeLabel(t *testing.T) {
	cases := map[providers.ItemType]string{
		providers.ItemTypeIssue:   "Issue asignado",
		providers.ItemTypePR:      "PR para revisar",
		providers.ItemTypeMention: "Mención",
	}
	for itemType, want := range cases {
		if got := typeLabel(itemType); got != want {
			t.Errorf("typeLabel(%q) = %q, want %q", itemType, got, want)
		}
	}
}
