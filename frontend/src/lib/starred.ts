import { writable } from 'svelte/store'
import type { providers } from '../../wailsjs/go/models'

const STORAGE_KEY = 'koalmine:starred'

// Keyed by TaskItem.id. Storing the full item (not just the id) matters:
// a starred item isn't necessarily in the current "assigned to me" poll
// snapshot (it could've come from a search result, or later fallen out of
// scope), so without its own data it would show a count in the sidebar but
// never actually render in Seguimiento.
type StarredMap = Record<string, providers.TaskItem>

function load(): StarredMap {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw)
    // Older versions stored a bare array of ids with no item data to show —
    // that data is unrecoverable, so drop it rather than crash on it.
    return Array.isArray(parsed) ? {} : parsed
  } catch {
    return {}
  }
}

function save(items: StarredMap) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(items))
  } catch {
    // localStorage unavailable — starring just won't persist across launches.
  }
}

export const starredItems = writable<StarredMap>(load())

export function toggleStar(item: providers.TaskItem) {
  starredItems.update((items) => {
    const next = { ...items }
    if (item.id in next) {
      delete next[item.id]
    } else {
      next[item.id] = item
    }
    save(next)
    return next
  })
}

// Replaces a starred item's saved snapshot with fresher data (e.g. from
// RefreshTaskItem), without starring/unstarring it. A no-op if the item
// isn't currently starred, so a refresh that resolves after the user
// unstarred it doesn't resurrect the entry.
export function updateStarredItem(item: providers.TaskItem) {
  starredItems.update((items) => {
    if (!(item.id in items)) return items
    const next = { ...items, [item.id]: item }
    save(next)
    return next
  })
}

// In-memory only (resets on app restart), module-scoped so it survives
// Seguimiento's TaskList instance being torn down and recreated each time
// the sidebar tab is switched. Without this, a growing favorites list would
// re-fetch every single one from its provider on every tab open — see
// needsRefresh.
const lastRefreshedAt = new Map<string, number>()

export function markRefreshed(id: string) {
  lastRefreshedAt.set(id, Date.now())
}

// Whether a starred item's provider data hasn't been re-fetched recently
// enough to trust it as "current" — used to skip re-fetching items that
// were just refreshed a moment ago (e.g. switching tabs back and forth).
export function needsRefresh(id: string, maxAgeMs: number): boolean {
  const last = lastRefreshedAt.get(id)
  return last === undefined || Date.now() - last > maxAgeMs
}
