import {
  browserInspectionScript,
  browserInspectionTextLimit,
  isBrowserInspectionText,
} from './inspection'
import type {
  BrowserRuntimeBridge,
  BrowserRuntimeState,
} from './runtime'
import type { BrowserRuntimeTabID } from './runtimeID'

export interface BrowserWebviewElement extends HTMLElement {
  getWebContentsId: () => number
  loadURL: (url: string) => Promise<void>
  reload: () => void
  stop: () => void
  goBack: () => void
  goForward: () => void
  canGoBack: () => boolean
  canGoForward: () => boolean
  getURL: () => string
  getTitle: () => string
  executeJavaScript: (code: string, userGesture?: boolean) => Promise<unknown>
}

type WebviewEvent = Event & {
  errorCode?: number
  errorDescription?: string
  isMainFrame?: boolean
  reason?: string
  title?: string
  url?: string
}

type WebviewEntry = {
  element: BrowserWebviewElement
  operation: number
  pendingNavigation?: PendingNavigation
  ready: Promise<void>
  state: BrowserRuntimeState
}

type PendingNavigation = {
  operation: number
  committed: boolean
  // The load for this navigation has been requested. Until it has, the guest
  // is still showing its own initial document and none of its load events
  // describe this navigation.
  issued: boolean
  resolve: (state: BrowserRuntimeState) => void
  timeout?: number
}

const entries = new Map<BrowserRuntimeTabID, WebviewEntry>()
const listeners = new Set<(state: BrowserRuntimeState) => void>()
const browserInspectionTimeoutMs = 5_000
const browserNavigationTimeoutMs = 15_000

export function webviewBrowserEnabled(): boolean {
  return window.codingDesktop?.browserMode === 'webview'
}

export function registerWebviewBrowser(
  tabID: BrowserRuntimeTabID,
  element: BrowserWebviewElement,
): () => void {
  // A registration always replaces the entry, so any navigation still waiting
  // on the old one is released rather than left pending forever.
  const previous = entries.get(tabID)
  if (previous) {
    entries.delete(tabID)
    previous.operation += 1
    releaseNavigation(previous)
  }

  const entry: WebviewEntry = {
    element,
    operation: 0,
    ready: Promise.resolve(),
    state: {
      tabID,
      appliedRevision: -1,
      requestedURL: '',
      committedURL: '',
      title: '',
      status: 'navigating',
      canGoBack: false,
      canGoForward: false,
    },
  }
  entries.set(tabID, entry)

  const subscriptions: Array<() => void> = []
  entry.ready = new Promise((resolve) => {
    const ready = () => resolve()
    element.addEventListener('dom-ready', ready, { once: true })
    subscriptions.push(() => element.removeEventListener('dom-ready', ready))
    if (safeCall(() => element.getWebContentsId(), -1) >= 0) resolve()
  })
  const listen = (name: string, handler: (event: WebviewEvent) => void) => {
    const listener = handler as EventListener
    element.addEventListener(name, listener)
    subscriptions.push(() => element.removeEventListener(name, listener))
  }
  const updateForPageNavigation = (event: WebviewEvent) => {
    if (!event.url || event.isMainFrame === false) return
    const pending = currentNavigation(entry)
    if (pending && !pending.issued) return
    if (pending) {
      pending.committed = true
    } else {
      entry.state.requestedURL = event.url
    }
    entry.state.committedURL = event.url
    emit(entry)
  }

  listen('did-start-loading', () => {
    entry.state.status = 'navigating'
    entry.state.error = undefined
    emit(entry)
  })
  listen('dom-ready', () => {
    finishLoadedDocument(entry)
  })
  listen('did-finish-load', () => {
    finishLoadedDocument(entry)
  })
  listen('did-stop-loading', () => {
    finishLoadedDocument(entry)
  })
  listen('did-navigate', updateForPageNavigation)
  listen('did-navigate-in-page', (event) => {
    if (event.isMainFrame === false) return
    const unissued = currentNavigation(entry)
    if (unissued && !unissued.issued) return
    updateForPageNavigation(event)
    const pending = currentNavigation(entry)
    if (pending?.committed) {
      finishNavigation(entry, 'ready')
    } else if (!entry.state.error) {
      entry.state.status = 'ready'
      emit(entry)
    }
  })
  listen('page-title-updated', () => emit(entry))
  listen('did-fail-load', (event) => {
    if (
      event.isMainFrame === false ||
      event.errorCode === -3 ||
      unissuedNavigation(entry)
    ) return
    const message = event.errorDescription || 'Page failed to load'
    if (!finishNavigation(entry, 'failed', message)) {
      entry.state.status = 'failed'
      entry.state.error = message
      emit(entry)
    }
  })
  listen('render-process-gone', (event) => {
    if (unissuedNavigation(entry)) return
    const message = `Browser renderer stopped: ${event.reason || 'unknown'}`
    if (!finishNavigation(entry, 'failed', message)) {
      entry.state.status = 'failed'
      entry.state.error = message
      emit(entry)
    }
  })

  return () => {
    for (const unsubscribe of subscriptions) unsubscribe()
    if (entries.get(tabID) === entry) {
      entries.delete(tabID)
      entry.operation += 1
      releaseNavigation(entry)
    }
  }
}

export const webviewBrowserBridge: BrowserRuntimeBridge = {
  async navigate(input) {
    const entry = requiredEntry(input.tabID)
    const target = resolveWebURL(input.url)
    if (input.revision < entry.state.appliedRevision) return snapshot(entry)
    if (input.revision === entry.state.appliedRevision) {
      if (entry.state.requestedURL !== target.href) {
        throw new Error('browser navigation revision conflicts with its target')
      }
      return snapshot(entry)
    }

    const operation = ++entry.operation
    releaseNavigation(entry)
    const reload =
      entry.state.status === 'ready' &&
      entry.state.committedURL === target.href
    entry.state = {
      ...entry.state,
      appliedRevision: input.revision,
      requestedURL: target.href,
      status: 'navigating',
      error: undefined,
    }
    emit(entry)

    // A freshly mounted guest is still loading its initial about:blank
    // document. Claim the navigation before waiting for that document so its
    // load events cannot be read as this revision's completion, and so the
    // wait is bounded by the same navigation timeout.
    const completion = beginNavigation(entry, operation, false)
    await entry.ready
    if (entries.get(input.tabID) !== entry || entry.operation !== operation) {
      return completion
    }

    const pending = currentNavigation(entry)
    if (!pending) return completion
    pending.issued = true
    try {
      if (reload) {
        pending.committed = true
        entry.element.reload()
        return completion
      }
      if (safeCall(() => entry.element.getURL(), '') !== '') entry.element.stop()
      void entry.element.loadURL(target.href).then(
        () => {
          const pending = currentNavigation(entry)
          if (!pending || pending.operation !== operation) return
          pending.committed = true
          finishNavigation(entry, 'ready')
        },
        (error: unknown) => {
          const pending = currentNavigation(entry)
          if (!pending || pending.operation !== operation) return
          if (isNavigationAborted(error)) return
          finishNavigation(entry, 'failed', errorMessage(error))
        },
      )
    } catch (error) {
      finishNavigation(entry, 'failed', errorMessage(error))
    }
    return completion
  },

  async close(tabID) {
    const entry = entries.get(tabID)
    if (!entry) return
    entries.delete(tabID)
    entry.operation += 1
    releaseNavigation(entry)
    safeCall(() => entry.element.stop(), undefined)
  },

  async goBack(tabID) {
    const entry = entries.get(tabID)
    if (entry && safeCall(() => entry.element.canGoBack(), false)) entry.element.goBack()
  },

  async goForward(tabID) {
    const entry = entries.get(tabID)
    if (entry && safeCall(() => entry.element.canGoForward(), false)) entry.element.goForward()
  },

  async inspect(tabID) {
    const entry = entries.get(tabID)
    if (!entry) throw new Error('Browser tab is not open')
    if (entry.state.status !== 'ready' || entry.state.appliedRevision < 0) {
      throw new Error('Browser tab is not ready')
    }

    const revision = entry.state.appliedRevision
    const result = await withTimeout(
      entry.element.executeJavaScript(browserInspectionScript, false),
      browserInspectionTimeoutMs,
      'Browser inspection timed out',
    )
    if (
      entries.get(tabID) !== entry ||
      entry.state.status !== 'ready' ||
      entry.state.appliedRevision !== revision
    ) {
      throw new Error('Agent browser page changed during inspection')
    }
    if (!isBrowserInspectionText(result)) {
      throw new Error('Browser returned an invalid inspection result')
    }
    return {
      url: safeCall(() => entry.element.getURL(), entry.state.committedURL),
      title: safeCall(() => entry.element.getTitle(), entry.state.title),
      pageStatus: 'ready',
      revision,
      visibleText: result.visibleText.slice(0, browserInspectionTextLimit),
      truncated: result.truncated || result.visibleText.length > browserInspectionTextLimit,
    }
  },

  onState(listener) {
    listeners.add(listener)
    return () => listeners.delete(listener)
  },
}

function requiredEntry(tabID: BrowserRuntimeTabID): WebviewEntry {
  const entry = entries.get(tabID)
  if (!entry) throw new Error(`Browser tab is not mounted: ${tabID}`)
  return entry
}

function emit(entry: WebviewEntry): void {
  entry.state.title =
    entry.state.status === 'navigating'
      ? ''
      : safeCall(() => entry.element.getTitle(), entry.state.title)
  entry.state.canGoBack = safeCall(() => entry.element.canGoBack(), false)
  entry.state.canGoForward = safeCall(() => entry.element.canGoForward(), false)
  // A guest that has never been asked to navigate is still showing its own
  // initial document. It describes no requested navigation, so publishing it
  // would put about:blank in the tab's address and title.
  if (entry.state.appliedRevision < 0) return
  const state = snapshot(entry)
  for (const listener of listeners) listener(state)
}

function snapshot(entry: WebviewEntry): BrowserRuntimeState {
  return { ...entry.state }
}

function beginNavigation(
  entry: WebviewEntry,
  operation: number,
  committed: boolean,
): Promise<BrowserRuntimeState> {
  return new Promise((resolve) => {
    const pending: PendingNavigation = {
      operation,
      committed,
      issued: false,
      resolve,
    }
    pending.timeout = window.setTimeout(() => {
      if (currentNavigation(entry) !== pending) return
      if (pending.committed) {
        finishNavigation(entry, 'ready')
      } else {
        finishNavigation(entry, 'failed', 'Browser navigation timed out')
      }
    }, browserNavigationTimeoutMs)
    entry.pendingNavigation = pending
  })
}

function currentNavigation(entry: WebviewEntry): PendingNavigation | undefined {
  const pending = entry.pendingNavigation
  return pending?.operation === entry.operation ? pending : undefined
}

// True while a navigation is claimed but its load has not been requested yet,
// which is the window where the guest's own initial document loads.
function unissuedNavigation(entry: WebviewEntry): boolean {
  const pending = currentNavigation(entry)
  return Boolean(pending && !pending.issued)
}

function finishLoadedDocument(entry: WebviewEntry): void {
  const pending = currentNavigation(entry)
  if (pending) {
    if (!pending.committed) return
    finishNavigation(entry, 'ready')
    return
  }
  if (entry.state.error) return
  const currentURL = safeCall(() => entry.element.getURL(), entry.state.committedURL)
  if (currentURL) entry.state.committedURL = currentURL
  entry.state.status = 'ready'
  emit(entry)
}

function finishNavigation(
  entry: WebviewEntry,
  status: 'ready' | 'failed',
  error?: string,
): boolean {
  const pending = currentNavigation(entry)
  if (!pending) return false
  entry.pendingNavigation = undefined
  if (pending.timeout !== undefined) window.clearTimeout(pending.timeout)
  if (status === 'ready') {
    entry.state.committedURL =
      safeCall(() => entry.element.getURL(), entry.state.committedURL) ||
      entry.state.committedURL ||
      entry.state.requestedURL
    entry.state.status = 'ready'
    entry.state.error = undefined
  } else {
    entry.state.status = 'failed'
    entry.state.error = error || 'Page failed to load'
  }
  emit(entry)
  pending.resolve(snapshot(entry))
  return true
}

function releaseNavigation(entry: WebviewEntry): void {
  const pending = entry.pendingNavigation
  if (!pending) return
  entry.pendingNavigation = undefined
  if (pending.timeout !== undefined) window.clearTimeout(pending.timeout)
  pending.resolve(snapshot(entry))
}

function resolveWebURL(value: string): URL {
  const target = new URL(value, window.location.href)
  if (target.protocol !== 'http:' && target.protocol !== 'https:') {
    throw new TypeError(`unsupported browser URL protocol: ${target.protocol}`)
  }
  return target
}

function safeCall<T>(call: () => T, fallback: T): T {
  try {
    return call()
  } catch {
    return fallback
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

function isNavigationAborted(error: unknown): boolean {
  return errorMessage(error).includes('ERR_ABORTED')
}

function withTimeout<T>(
  promise: Promise<T>,
  timeoutMs: number,
  message: string,
): Promise<T> {
  return new Promise((resolve, reject) => {
    const timer = window.setTimeout(() => reject(new Error(message)), timeoutMs)
    promise.then(
      (value) => {
        window.clearTimeout(timer)
        resolve(value)
      },
      (error: unknown) => {
        window.clearTimeout(timer)
        reject(error)
      },
    )
  })
}
