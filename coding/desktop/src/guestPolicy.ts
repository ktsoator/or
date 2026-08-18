export const previewPartition = 'or-preview'
export const browserPartition = 'persist:or-browser'

export type GuestKind = 'preview' | 'web'
export type GuestNavigationAction = 'allow' | 'open-tab' | 'open-external' | 'deny'

export function guestKindForPartition(partition: string): GuestKind | undefined {
  if (partition === previewPartition) return 'preview'
  if (partition === browserPartition) return 'web'
  return undefined
}

export function popupNavigationAction(rawURL: string): GuestNavigationAction {
  const target = safeURL(rawURL)
  if (target?.protocol === 'http:' || target?.protocol === 'https:') return 'open-tab'
  if (target?.protocol === 'mailto:' || target?.protocol === 'tel:') return 'open-external'
  return 'deny'
}

export function previewNavigationAction(
  rawURL: string,
  previewOrigin: string,
): GuestNavigationAction {
  const target = safeURL(rawURL)
  if (!target) return 'deny'
  if (
    (target.protocol === 'http:' || target.protocol === 'https:') &&
    target.origin === previewOrigin
  ) return 'allow'
  return popupNavigationAction(target.href)
}

function safeURL(value: string): URL | undefined {
  try {
    return new URL(value)
  } catch {
    return undefined
  }
}
