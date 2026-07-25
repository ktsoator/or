import {
  webviewBrowserBridge,
  webviewBrowserEnabled,
} from './webviewBrowser'

export type BrowserNavigateInput = {
  tabID: string
  url: string
  revision: number
  kind: 'web' | 'workspace-preview'
}

export type BrowserRuntimeState = {
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

export type BrowserInspection = {
  url: string
  title: string
  pageStatus: 'ready'
  revision: number
  visibleText: string
  truncated: boolean
}

export type BrowserRuntimeBridge = {
  navigate: (input: BrowserNavigateInput) => Promise<BrowserRuntimeState>
  close: (tabID: string) => Promise<void>
  goBack: (tabID: string) => Promise<void>
  goForward: (tabID: string) => Promise<void>
  inspect: (tabID: string) => Promise<BrowserInspection>
  onState: (listener: (state: BrowserRuntimeState) => void) => () => void
}

export type CodingDesktop = {
  platform: string
  browserMode: 'webview'
  chooseDirectory: (initialPath: string, title: string) => Promise<string>
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

// The browser runtime is the renderer's own <webview> registry, so it exists
// exactly when the desktop shell enables the webview tag.
export function hasBrowserRuntime(): boolean {
  return webviewBrowserEnabled()
}

function browserBridge(): BrowserRuntimeBridge | undefined {
  return webviewBrowserEnabled() ? webviewBrowserBridge : undefined
}

export function navigateBrowser(
  input: BrowserNavigateInput,
): Promise<BrowserRuntimeState | undefined> {
  return browserBridge()?.navigate(input) ?? Promise.resolve(undefined)
}

export function closeBrowser(tabID: string): Promise<void> {
  return browserBridge()?.close(tabID) ?? Promise.resolve()
}

export function goBackBrowser(tabID: string): Promise<void> {
  return browserBridge()?.goBack(tabID) ?? Promise.resolve()
}

export function goForwardBrowser(tabID: string): Promise<void> {
  return browserBridge()?.goForward(tabID) ?? Promise.resolve()
}

export function inspectBrowser(
  tabID: string,
): Promise<BrowserInspection | undefined> {
  return browserBridge()?.inspect(tabID) ?? Promise.resolve(undefined)
}

export function onBrowserState(
  listener: (state: BrowserRuntimeState) => void,
): () => void {
  return browserBridge()?.onState(listener) ?? (() => undefined)
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
