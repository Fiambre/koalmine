import type { providers, store } from '../../wailsjs/go/models'

// Shared by App.svelte (sidebar badge counts) and TaskList.svelte (the
// actual filtered list), so both always agree on what belongs to a panel.
export function matchesPanel(item: providers.TaskItem, panel: store.Panel): boolean {
  if (panel.integrationId && item.integrationId !== panel.integrationId) return false
  if (panel.project && item.project !== panel.project) return false
  if (panel.type && item.type !== panel.type) return false
  if (panel.status === 'open' && item.closed) return false
  if (panel.status === 'closed' && !item.closed) return false
  return true
}
