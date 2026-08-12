export type CodingDesktop = {
  platform: string
  browserMode: 'webview'
  chooseDirectory: (initialPath: string, title: string) => Promise<string>
  revealPath: (target: string) => Promise<void>
  openExternalURL: (url: string) => Promise<void> | void
}

declare global {
  interface Window {
    codingDesktop?: Partial<CodingDesktop>
  }
}

export function hasDesktopRuntime(): boolean {
  return window.codingDesktop !== undefined
}

export function desktopPlatform(): string | undefined {
  return window.codingDesktop?.platform
}

// Opens a URL outside Or when the native runtime is available, with the
// browser's normal new-tab behavior as the web-client fallback.
export function openExternalURL(url: string): void {
  const open = window.codingDesktop?.openExternalURL
  if (typeof open === 'function') {
    void open(url)
    return
  }
  window.open(url, '_blank', 'noopener,noreferrer')
}

// Returns undefined when the browser has no native desktop bridge. An empty
// string is a valid desktop response and means the user cancelled the dialog.
export async function chooseNativeDirectory(
  initialPath: string | undefined,
  title: string,
): Promise<string | undefined> {
  const choose = window.codingDesktop?.chooseDirectory
  if (typeof choose !== 'function') return undefined
  return choose(initialPath ?? '', title)
}

export async function revealNativePath(target: string): Promise<void> {
  const reveal = window.codingDesktop?.revealPath
  if (typeof reveal !== 'function') throw new Error('native file manager is unavailable')
  await reveal(target)
}
