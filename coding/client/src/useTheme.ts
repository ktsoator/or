import { useCallback, useEffect, useState } from 'react'
import {
  applyTheme,
  persistThemePreference,
  readThemePreference,
  resolveTheme,
  watchSystemTheme,
  type ResolvedTheme,
  type ThemePreference,
} from './theme'

/**
 * Owns the theme preference and keeps the document in sync with it. The
 * preference is what the user chose; the resolved theme is what is on screen,
 * which under "system" follows the OS while the app is open.
 */
export function useTheme(): {
  preference: ThemePreference
  resolved: ResolvedTheme
  setPreference: (preference: ThemePreference) => void
} {
  const [preference, setPreferenceState] = useState<ThemePreference>(readThemePreference)
  const [resolved, setResolved] = useState<ResolvedTheme>(() => resolveTheme(readThemePreference()))

  useEffect(() => {
    setResolved(applyTheme(preference))
  }, [preference])

  useEffect(() => {
    // Only "system" tracks the OS. An explicit choice stays put when the OS
    // flips, which is the whole point of having made it.
    if (preference !== 'system') return
    return watchSystemTheme(setResolved)
  }, [preference])

  const setPreference = useCallback((next: ThemePreference) => {
    persistThemePreference(next)
    setPreferenceState(next)
  }, [])

  return { preference, resolved, setPreference }
}
