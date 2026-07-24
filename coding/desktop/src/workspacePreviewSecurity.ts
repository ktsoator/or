export function workspacePreviewPrefix(target: URL, desktopURL: string): string {
  if (target.origin !== new URL(desktopURL).origin) {
    throw new TypeError('workspace preview must use the desktop origin')
  }
  const match = target.pathname.match(/^\/api\/sessions\/[^/]+\/previews\/[^/]+\//)
  if (!match) throw new TypeError('workspace preview URL is invalid')
  return match[0]
}
export function workspacePreviewRequestAllowed(
  value: string,
  method: string,
  desktopURL: string,
  previewPrefix: string,
): boolean {
  const target = safeURL(value)
  if (!target || (method !== 'GET' && method !== 'HEAD')) return false
  if (target.protocol === 'data:' || target.protocol === 'blob:') return true
  return (
    target.origin === new URL(desktopURL).origin &&
    target.pathname.startsWith(previewPrefix)
  )
}

export function workspacePreviewNavigationAllowed(
  value: string,
  desktopURL: string,
  previewPrefix: string,
): boolean {
  const target = safeURL(value)
  return Boolean(
    target &&
    (target.protocol === 'http:' || target.protocol === 'https:') &&
    target.origin === new URL(desktopURL).origin &&
    target.pathname.startsWith(previewPrefix),
  )
}

function safeURL(value: string): URL | undefined {
  try {
    return new URL(value)
  } catch {
    return undefined
  }
}
