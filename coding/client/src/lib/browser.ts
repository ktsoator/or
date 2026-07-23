import { apiURL } from '@/api'

export function normalizeBrowserAddress(value: string): string | undefined {
  const input = value.trim()
  if (!input) return undefined
  const candidate = /^https?:\/\//i.test(input) ? input : `http://${input}`
  try {
    const url = new URL(candidate)
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return undefined
    if (url.username || url.password) return undefined
    return url.href
  } catch {
    return undefined
  }
}

export function isLocalPreviewURL(address: string): boolean {
  try {
    const hostname = new URL(address).hostname.toLowerCase().replace(/\.$/, '')
    return (
      hostname === 'localhost' ||
      hostname === '127.0.0.1' ||
      hostname === '0.0.0.0' ||
      hostname === '::1' ||
      hostname === '::'
    )
  } catch {
    return false
  }
}

export function workspacePreviewURL(sessionID: string, path: string): string {
  const encodedPath = path
    .split('/')
    .filter(Boolean)
    .map((segment) => encodeURIComponent(segment))
    .join('/')
  return apiURL(`/sessions/${encodeURIComponent(sessionID)}/preview/${encodedPath}`)
}

export function workspaceFileURL(path: string): string | undefined {
  const normalized = path.trim().replaceAll('\\', '/')
  if (!normalized.startsWith('/') && !/^[a-z]:\//i.test(normalized)) return undefined
  const pathname = normalized.startsWith('/') ? normalized : `/${normalized}`
  const encoded = pathname
    .split('/')
    .map((segment, index) => {
      if (index === 1 && /^[a-z]:$/i.test(segment)) return segment
      return encodeURIComponent(segment)
    })
    .join('/')
  return `file://${encoded}`
}
