package store

import (
	"testing"

	"koalmine/internal/providers"
)

func TestNamespaceItemLeavesIDAloneWhenTypeIsUnique(t *testing.T) {
	item := providers.TaskItem{ID: "gitlab:issue:42", Provider: "gitlab"}
	integ := Integration{ID: "gitlab", Type: "gitlab", Name: "GitLab"}

	got := NamespaceItem(item, integ, 1)

	if got.ID != "gitlab:issue:42" {
		t.Errorf("expected ID unchanged when only one integration of this type exists, got %q", got.ID)
	}
	if got.IntegrationID != "gitlab" {
		t.Errorf("expected IntegrationID set to %q, got %q", "gitlab", got.IntegrationID)
	}
	if got.Provider != "GitLab" {
		t.Errorf("expected Provider overwritten with the integration's display name, got %q", got.Provider)
	}
}

func TestNamespaceItemPrefixesIDWhenTypeIsAmbiguous(t *testing.T) {
	item := providers.TaskItem{ID: "gitlab:issue:42", Provider: "gitlab"}
	integ := Integration{ID: "gitlab-a1b2c3d4", Type: "gitlab", Name: "GitLab (work)"}

	got := NamespaceItem(item, integ, 2)

	if want := "gitlab-a1b2c3d4/gitlab:issue:42"; got.ID != want {
		t.Errorf("expected ID prefixed with the integration ID, got %q want %q", got.ID, want)
	}
	if got.IntegrationID != "gitlab-a1b2c3d4" {
		t.Errorf("unexpected IntegrationID: %q", got.IntegrationID)
	}
}

func TestDenamespaceIDRoundTrips(t *testing.T) {
	if got := DenamespaceID("gitlab-a1b2c3d4/gitlab:issue:42", "gitlab-a1b2c3d4"); got != "gitlab:issue:42" {
		t.Errorf("expected prefix stripped, got %q", got)
	}
	// A no-op when the ID was never prefixed (the common, single-integration case).
	if got := DenamespaceID("gitlab:issue:42", "gitlab"); got != "gitlab:issue:42" {
		t.Errorf("expected unprefixed ID left alone, got %q", got)
	}
}

func TestIntegrationIDForFallsBackToProviderField(t *testing.T) {
	// A TaskItem cached/starred before multi-integration support existed —
	// no IntegrationID, but Provider still holds the bare provider type
	// name it always used to.
	legacy := providers.TaskItem{Provider: "gitlab"}
	if got := IntegrationIDFor(legacy); got != "gitlab" {
		t.Errorf("expected fallback to Provider field, got %q", got)
	}

	fresh := providers.TaskItem{IntegrationID: "gitlab-a1b2c3d4", Provider: "GitLab (work)"}
	if got := IntegrationIDFor(fresh); got != "gitlab-a1b2c3d4" {
		t.Errorf("expected IntegrationID to take priority, got %q", got)
	}
}
