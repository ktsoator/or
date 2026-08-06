import {
  webviewBrowserBridge,
  webviewBrowserEnabled,
} from './webviewBrowser'
import type { BrowserRuntimeTabID } from './runtimeID'

export type BrowserNavigateInput = {
  tabID: BrowserRuntimeTabID
  url: string
  revision: number
  kind: 'web' | 'workspace-preview'
}

export type BrowserRuntimeState = {
  tabID: BrowserRuntimeTabID
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
  close: (tabID: BrowserRuntimeTabID) => Promise<void>
  goBack: (tabID: BrowserRuntimeTabID) => Promise<void>
  goForward: (tabID: BrowserRuntimeTabID) => Promise<void>
  inspect: (tabID: BrowserRuntimeTabID) => Promise<BrowserInspection>
  onState: (listener: (state: BrowserRuntimeState) => void) => () => void
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

export function closeBrowser(tabID: BrowserRuntimeTabID): Promise<void> {
  return browserBridge()?.close(tabID) ?? Promise.resolve()
}

export function goBackBrowser(tabID: BrowserRuntimeTabID): Promise<void> {
  return browserBridge()?.goBack(tabID) ?? Promise.resolve()
}

export function goForwardBrowser(tabID: BrowserRuntimeTabID): Promise<void> {
  return browserBridge()?.goForward(tabID) ?? Promise.resolve()
}

export function inspectBrowser(
  tabID: BrowserRuntimeTabID,
): Promise<BrowserInspection | undefined> {
  return browserBridge()?.inspect(tabID) ?? Promise.resolve(undefined)
}

export function onBrowserState(
  listener: (state: BrowserRuntimeState) => void,
): () => void {
  return browserBridge()?.onState(listener) ?? (() => undefined)
}
