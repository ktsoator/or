export type InterfaceDensity = 'compact' | 'default' | 'comfortable'
export type TextSize = 'small' | 'default' | 'large'

export type AppearancePreferences = {
  density: InterfaceDensity
  chatText: TextSize
  codeText: TextSize
}

const storageKey = 'or.appearance'

const defaults: AppearancePreferences = {
  density: 'default',
  chatText: 'default',
  codeText: 'default',
}

const densitySizes: Record<InterfaceDensity, string> = {
  compact: '14px',
  default: '15px',
  comfortable: '16px',
}

const chatTextSizes: Record<TextSize, string> = {
  small: '0.96875rem',
  default: '1.03125rem',
  large: '1.125rem',
}

const codeTextSizes: Record<TextSize, string> = {
  small: '0.8125rem',
  default: '0.875rem',
  large: '0.9375rem',
}

const toolTextSizes: Record<TextSize, string> = {
  small: '0.6875rem',
  default: '0.75rem',
  large: '0.8125rem',
}

export function readAppearancePreferences(): AppearancePreferences {
  try {
    const stored = JSON.parse(localStorage.getItem(storageKey) ?? '{}') as Partial<AppearancePreferences>
    return {
      density: isDensity(stored.density) ? stored.density : defaults.density,
      chatText: isTextSize(stored.chatText) ? stored.chatText : defaults.chatText,
      codeText: isTextSize(stored.codeText) ? stored.codeText : defaults.codeText,
    }
  } catch {
    return defaults
  }
}

export function saveAppearancePreferences(preferences: AppearancePreferences): void {
  applyAppearancePreferences(preferences)
  try {
    localStorage.setItem(storageKey, JSON.stringify(preferences))
  } catch {
    // The live settings still work when storage is unavailable.
  }
}

export function applyAppearancePreferences(preferences: AppearancePreferences): void {
  const root = document.documentElement
  root.style.setProperty('--ui-root-font-size', densitySizes[preferences.density])
  root.style.setProperty('--chat-font-size', chatTextSizes[preferences.chatText])
  root.style.setProperty('--code-font-size', codeTextSizes[preferences.codeText])
  root.style.setProperty('--tool-font-size', toolTextSizes[preferences.codeText])
}

function isDensity(value: unknown): value is InterfaceDensity {
  return value === 'compact' || value === 'default' || value === 'comfortable'
}

function isTextSize(value: unknown): value is TextSize {
  return value === 'small' || value === 'default' || value === 'large'
}
