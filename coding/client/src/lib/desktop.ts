export type NativeBrowserBounds = {
  x: number
  y: number
  width: number
  height: number
}

export type NativeBrowserNavigateInput = {
  tabID: string
  url: string
  revision: number
  kind: 'web' | 'workspace-preview'
}

export type NativeBrowserViewportInput = {
  tabID: string
  visible: boolean
  bounds?: NativeBrowserBounds
}

export type NativeBrowserState = {
  tabID: string
  appliedRevision: number
  requestedURL: string
  committedURL: string
  title: string
  status: 'navigating' | 'ready' | 'failed'
  canGoBack: boolean
  canGoForward: boolean
  error?: string
}

export type NativeBrowserInspection = {
  url: string
  title: string
  pageStatus: 'ready'
  revision: number
  visibleText: string
  truncated: boolean
}

type NativeBrowserBridge = {
  navigate: (input: NativeBrowserNavigateInput) => Promise<NativeBrowserState>
  setViewport: (input: NativeBrowserViewportInput) => Promise<void>
  close: (tabID: string) => Promise<void>
  goBack: (tabID: string) => Promise<void>
  goForward: (tabID: string) => Promise<void>
  inspect: (tabID: string) => Promise<NativeBrowserInspection>
  onState: (listener: (state: NativeBrowserState) => void) => () => void
}

export type CodingDesktop = {
  platform: string
  chooseDirectory: (initialPath: string, title: string) => Promise<string>
  openExternalURL: (url: string) => Promise<void> | void
  browser: NativeBrowserBridge
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

export function hasNativeBrowser(): boolean {
  return (
    typeof window.codingDesktop?.browser?.navigate === 'function' &&
    typeof window.codingDesktop.browser.setViewport === 'function'
  )
}

export function navigateNativeBrowser(
  input: NativeBrowserNavigateInput,
): Promise<NativeBrowserState | undefined> {
  return window.codingDesktop?.browser?.navigate(input) ?? Promise.resolve(undefined)
}

export function setNativeBrowserViewport(
  input: NativeBrowserViewportInput,
): Promise<void> {
  return window.codingDesktop?.browser?.setViewport(input) ?? Promise.resolve()
}

export function closeNativeBrowser(tabID: string): Promise<void> {
  return window.codingDesktop?.browser?.close(tabID) ?? Promise.resolve()
}

export function goBackNativeBrowser(tabID: string): Promise<void> {
  return window.codingDesktop?.browser?.goBack(tabID) ?? Promise.resolve()
}

export function goForwardNativeBrowser(tabID: string): Promise<void> {
  return window.codingDesktop?.browser?.goForward(tabID) ?? Promise.resolve()
}

export function inspectNativeBrowser(
  tabID: string,
): Promise<NativeBrowserInspection | undefined> {
  return window.codingDesktop?.browser?.inspect(tabID) ?? Promise.resolve(undefined)
}

export function onNativeBrowserState(
  listener: (state: NativeBrowserState) => void,
): () => void {
  return window.codingDesktop?.browser?.onState(listener) ?? (() => undefined)
}

// Opens a URL outside Coding when the native runtime is available, with the
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
