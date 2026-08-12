// Theme selection. The stylesheet owns every colour; this module only decides
// which of its two value sets applies, by stamping data-theme on the root
// element. "system" removes the attribute so the media query decides.

export type ThemePreference = 'system' | 'light' | 'dark'
export type ResolvedTheme = 'light' | 'dark'

const storageKey = 'or.theme'

export function isThemePreference(value: unknown): value is ThemePreference {
  return value === 'system' || value === 'light' || value === 'dark'
}

/** Reads the stored preference, falling back to following the OS. */
export function readThemePreference(): ThemePreference {
  if (typeof window === 'undefined') return 'system'
  try {
    const saved = window.localStorage.getItem(storageKey)
    return isThemePreference(saved) ? saved : 'system'
  } catch {
    // Private-mode storage denial must not keep the app from rendering.
    return 'system'
  }
}

export function persistThemePreference(preference: ThemePreference) {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(storageKey, preference)
  } catch {
    // A theme that cannot be remembered is still worth applying for this run.
  }
}

/** Reports the theme the OS currently asks for. */
export function systemTheme(): ResolvedTheme {
  if (typeof window === 'undefined' || !window.matchMedia) return 'light'
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function resolveTheme(preference: ThemePreference): ResolvedTheme {
  return preference === 'system' ? systemTheme() : preference
}

/**
 * Applies preference to the document. Under "system" the attribute is removed
 * rather than pinned to the current OS value, so a later OS change is picked up
 * by CSS alone. color-scheme is set so form controls, scrollbars, and the
 * browser's own surfaces match.
 */
export function applyTheme(preference: ThemePreference): ResolvedTheme {
  const resolved = resolveTheme(preference)
  if (typeof document === 'undefined') return resolved
  const root = document.documentElement
  if (preference === 'system') {
    root.removeAttribute('data-theme')
  } else {
    root.setAttribute('data-theme', preference)
  }
  root.style.colorScheme = resolved
  return resolved
}

/**
 * Calls listener whenever the OS theme changes. The caller decides whether the
 * change is relevant; it is not under an explicit light or dark preference.
 */
export function watchSystemTheme(listener: (theme: ResolvedTheme) => void): () => void {
  if (typeof window === 'undefined' || !window.matchMedia) return () => {}
  const query = window.matchMedia('(prefers-color-scheme: dark)')
  const handle = (event: MediaQueryListEvent) => listener(event.matches ? 'dark' : 'light')
  query.addEventListener('change', handle)
  return () => query.removeEventListener('change', handle)
}
