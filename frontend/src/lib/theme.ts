// nameKey is an i18n key under "accent.*" — the display name is resolved
// at render time so it stays in sync with the current language.
export type AccentColor = { nameKey: string; value: string }

export const ACCENT_COLORS: AccentColor[] = [
  { nameKey: 'accent.red', value: '#db4c3f' },
  { nameKey: 'accent.orange', value: '#e58b2e' },
  { nameKey: 'accent.yellow', value: '#c9a227' },
  { nameKey: 'accent.green', value: '#4a9c5d' },
  { nameKey: 'accent.teal', value: '#2d9d94' },
  { nameKey: 'accent.blue', value: '#4a90e2' },
  { nameKey: 'accent.indigo', value: '#6c5ce7' },
  { nameKey: 'accent.violet', value: '#9b59b6' },
  { nameKey: 'accent.pink', value: '#e0538b' },
  { nameKey: 'accent.gray', value: '#8a8a88' },
]

const DEFAULT_ACCENT = ACCENT_COLORS[0].value
const STORAGE_KEY = 'koalmine:accent'

function hexToRgb(hex: string): [number, number, number] {
  const clean = hex.replace('#', '')
  const num = parseInt(clean, 16)
  return [(num >> 16) & 255, (num >> 8) & 255, num & 255]
}

function shade(hex: string, factor: number): string {
  const [r, g, b] = hexToRgb(hex)
  const clamp = (n: number) => Math.max(0, Math.min(255, Math.round(n)))
  return `rgb(${clamp(r * factor)}, ${clamp(g * factor)}, ${clamp(b * factor)})`
}

export function applyAccent(hex: string) {
  const root = document.documentElement
  const [r, g, b] = hexToRgb(hex)
  root.style.setProperty('--accent', hex)
  root.style.setProperty('--accent-hover', shade(hex, 0.85))
  root.style.setProperty('--accent-soft', `rgba(${r}, ${g}, ${b}, 0.16)`)
}

export function loadAccent(): string {
  try {
    return localStorage.getItem(STORAGE_KEY) || DEFAULT_ACCENT
  } catch {
    return DEFAULT_ACCENT
  }
}

export function saveAccent(hex: string) {
  try {
    localStorage.setItem(STORAGE_KEY, hex)
  } catch {
    // localStorage unavailable — accent just won't persist across launches.
  }
  applyAccent(hex)
}
