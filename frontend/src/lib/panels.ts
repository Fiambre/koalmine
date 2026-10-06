import type { providers, store } from '../../wailsjs/go/models'

// Filters an item against a panel's project/type/status/integration
// dimensions only — not "assigned to". Exported separately so TaskList can
// apply it to the result of its own dedicated "assigned to" fetch (see
// GetPanelAssignedTasks), which is already scoped to the right assignee and
// shouldn't be re-checked against it.
export function matchesPanelFilters(item: providers.TaskItem, panel: store.Panel): boolean {
  if (panel.integrationId && item.integrationId !== panel.integrationId) return false
  // projectKey, not project: project is a display name (e.g. Redmine's
  // project name), which doesn't necessarily match the identifier
  // Panel.project stores (e.g. Redmine's numeric project ID) — see
  // providers.TaskItem's doc comment.
  if (panel.project && item.projectKey !== panel.project) return false
  if (panel.type && item.type !== panel.type) return false
  if (panel.status === 'open' && item.closed) return false
  if (panel.status === 'closed' && !item.closed) return false
  return true
}

// Shared by App.svelte (sidebar badge counts) and TaskList.svelte's default
// rendering — both work off the poller's "assigned to me" snapshot only, so
// a panel with its own "assigned to" filter, or its "created by me" filter,
// can never be correctly answered from it: that snapshot doesn't contain
// other users' items at all (assignedTo), and it's also restricted to
// currently-open items (createdByMe wants closed ones too). Such a panel
// always reports no match here; TaskList instead fetches its items directly
// (GetPanelAssignedTasks / GetPanelCreatedByMeTasks) and filters that result
// with matchesPanelFilters.
export function matchesPanel(item: providers.TaskItem, panel: store.Panel): boolean {
  if (panel.assignedTo) return false
  if (panel.createdByMe) return false
  return matchesPanelFilters(item, panel)
}
