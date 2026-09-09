import { writable } from 'svelte/store'

const STORAGE_KEY = 'koalmine:starred'

function load(): Set<string> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    return raw ? new Set(JSON.parse(raw)) : new Set()
  } catch {
    return new Set()
  }
}

function save(ids: Set<string>) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify([...ids]))
  } catch {
    // localStorage unavailable — starring just won't persist across launches.
  }
}

export const starredIds = writable<Set<string>>(load())

export function toggleStar(id: string) {
  starredIds.update((ids) => {
    const next = new Set(ids)
    if (next.has(id)) {
      next.delete(id)
    } else {
      next.add(id)
    }
    save(next)
    return next
  })
}
