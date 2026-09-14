package store

import (
	"strings"

	"koalmine/internal/providers"
)

// NamespaceItem stamps a TaskItem fetched from integ's provider with the
// integration it came from, so it can always be routed back to the right
// configured connection. providers.Provider implementations have no
// notion of "which configured instance am I" — they always produce the
// same TaskItem.ID shape for a given remote item (e.g. "gitlab:issue:42"),
// which is fine as long as at most one integration of that type exists,
// but would collide if two integrations of the same type happened to
// reference the same remote numeric ID.
//
// sameTypeCount is the number of configured integrations sharing integ's
// provider type (see Config.SameTypeCount). The ID is only prefixed with
// the integration's own ID when that's more than one — so the
// overwhelmingly common case (one integration per type) never changes
// TaskItem.ID at all, preserving already-starred items and notification
// dedup state across an upgrade from single- to multi-integration support.
// Provider is also overwritten with the integration's own display Name, so
// the UI shows e.g. "GitLab (work)" instead of the bare connector type.
func NamespaceItem(item providers.TaskItem, integ Integration, sameTypeCount int) providers.TaskItem {
	item.IntegrationID = integ.ID
	if sameTypeCount > 1 {
		item.ID = integ.ID + "/" + item.ID
	}
	item.Provider = integ.Name
	return item
}

// NamespaceItems applies NamespaceItem to every item in items.
func NamespaceItems(items []providers.TaskItem, integ Integration, sameTypeCount int) []providers.TaskItem {
	result := make([]providers.TaskItem, len(items))
	for i, item := range items {
		result[i] = NamespaceItem(item, integ, sameTypeCount)
	}
	return result
}

// DenamespaceID strips the integration-ID prefix NamespaceItem may have
// added, back to the raw ID shape the item's own provider implementation
// expects. A no-op when the ID was never prefixed (sameTypeCount was 1 at
// fetch time), so it's safe to call unconditionally.
func DenamespaceID(id string, integrationID string) string {
	return strings.TrimPrefix(id, integrationID+"/")
}

// IntegrationIDFor returns the integration ID a TaskItem should route back
// to: its own IntegrationID if set, falling back to its Provider field.
// This covers a TaskItem that was cached or starred by the frontend before
// multi-integration support existed — its JSON has no "integrationId" key
// at all (so it unmarshals to ""), and its Provider field holds the bare
// provider type name it always used to hold, which resolves correctly
// because migrateLegacyConfig deliberately reuses that same string as the
// migrated integration's own ID.
func IntegrationIDFor(item providers.TaskItem) string {
	if item.IntegrationID != "" {
		return item.IntegrationID
	}
	return item.Provider
}
