import { register, init, locale as localeStore, getLocaleFromNavigator } from 'svelte-i18n'

export const SUPPORTED_LOCALES = ['en', 'es'] as const
export type Locale = (typeof SUPPORTED_LOCALES)[number]

const STORAGE_KEY = 'koalmine:locale'
// The app's original/only language before i18n existed — kept as the
// fallback and default so anyone who never picks a language sees the same
// copy they always did.
const DEFAULT_LOCALE: Locale = 'es'

register('en', () => import('./locales/en.json'))
register('es', () => import('./locales/es.json'))

function storedLocale(): Locale | null {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    return stored === 'en' || stored === 'es' ? stored : null
  } catch {
    return null
  }
}

function detectLocale(): Locale {
  const stored = storedLocale()
  if (stored) return stored
  const nav = (getLocaleFromNavigator() ?? '').toLowerCase()
  return nav.startsWith('en') ? 'en' : DEFAULT_LOCALE
}

init({
  fallbackLocale: DEFAULT_LOCALE,
  initialLocale: detectLocale(),
})

export function setLocale(next: Locale) {
  localeStore.set(next)
  try {
    localStorage.setItem(STORAGE_KEY, next)
  } catch {
    // localStorage unavailable — the choice just won't persist across launches.
  }
}
