import { createHash } from 'node:crypto'
import {
  BrowserWindow,
  WebContentsView,
  session,
  shell,
  type Rectangle,
  type Session,
} from 'electron'

export type DesktopEndpoint = {
  url: string
  cookieName: string
  token: string
}

type BrowserKind = 'web' | 'workspace-preview'

type ShowBrowserInput = {
  tabID: string
  url: string
  bounds: Rectangle
  navigation: number
  workspacePreview: boolean
}

type BrowserEntry = {
  tabID: string
  kind: BrowserKind
  view: WebContentsView
  requestedURL: string
  navigation: number
  previewPrefix?: string
  error?: string
}

type BrowserState = {
  tabID: string
  url: string
  title: string
  loading: boolean
  canGoBack: boolean
  canGoForward: boolean
  error?: string
}

export class NativeBrowserManager {
  readonly #window: BrowserWindow
  readonly #desktop: DesktopEndpoint
  readonly #webSession: Session
  readonly #entries = new Map<string, BrowserEntry>()
  readonly #operations = new Map<string, number>()
  #destroyed = false

  constructor(window: BrowserWindow, desktop: DesktopEndpoint) {
    this.#window = window
    this.#desktop = desktop
    this.#webSession = session.fromPartition('persist:coding-browser')
    this.#webSession.setPermissionRequestHandler((_contents, _permission, callback) => {
      callback(false)
    })
  }

  async show(value: unknown): Promise<void> {
    const input = parseShowInput(value)
    if (this.#destroyed) throw new Error('native browser is destroyed')
    const operation = this.#beginOperation(input.tabID)
    const target = resolveWebURL(input.url, this.#desktop.url)
    const kind: BrowserKind = input.workspacePreview ? 'workspace-preview' : 'web'
    let entry = this.#entries.get(input.tabID)
    if (entry && entry.kind !== kind) {
      this.#removeEntry(entry)
      entry = undefined
    }
    if (!entry) {
      const created = await this.#createEntry(input.tabID, kind, target)
      if (!this.#isCurrent(input.tabID, operation)) {
        this.#disposeEntry(created)
        return
      }
      entry = created
      this.#entries.set(input.tabID, entry)
    }

    for (const candidate of this.#entries.values()) {
      if (candidate !== entry) candidate.view.setVisible(false)
    }
    this.#window.contentView.addChildView(entry.view)
    entry.view.setBounds(clampBounds(input.bounds, this.#window))
    entry.view.setVisible(true)

    const reload = entry.requestedURL === target.href && entry.navigation !== input.navigation
    entry.navigation = input.navigation
    if (entry.requestedURL !== target.href) {
      entry.requestedURL = target.href
      try {
        await entry.view.webContents.loadURL(target.href)
      } catch (error) {
        if (!this.#isCurrent(input.tabID, operation)) return
        throw error
      }
    } else if (reload) {
      entry.view.webContents.reload()
    } else {
      this.#sendState(entry)
    }
  }

  hide(tabID: unknown): void {
    const id = this.#tabID(tabID)
    if (!id) return
    this.#beginOperation(id)
    this.#entries.get(id)?.view.setVisible(false)
  }

  close(tabID: unknown): void {
    const id = this.#tabID(tabID)
    if (!id) return
    this.#beginOperation(id)
    const entry = this.#entries.get(id)
    if (!entry) return
    this.#removeEntry(entry)
  }

  goBack(tabID: unknown): void {
    const entry = this.#entry(tabID)
    if (entry?.view.webContents.navigationHistory.canGoBack()) {
      entry.view.webContents.navigationHistory.goBack()
    }
  }

  goForward(tabID: unknown): void {
    const entry = this.#entry(tabID)
    if (entry?.view.webContents.navigationHistory.canGoForward()) {
      entry.view.webContents.navigationHistory.goForward()
    }
  }

  reload(tabID: unknown): void {
    this.#entry(tabID)?.view.webContents.reload()
  }

  destroy(): void {
    this.#destroyed = true
    for (const tabID of this.#operations.keys()) this.#beginOperation(tabID)
    for (const entry of [...this.#entries.values()]) this.#removeEntry(entry)
  }

  async #createEntry(
    tabID: string,
    kind: BrowserKind,
    target: URL,
  ): Promise<BrowserEntry> {
    const previewPrefix = kind === 'workspace-preview'
      ? workspacePreviewPrefix(target, this.#desktop.url)
      : undefined
    const browserSession = previewPrefix
      ? await this.#workspaceSession(tabID, target, previewPrefix)
      : this.#webSession
    const view = new WebContentsView({
      webPreferences: {
        session: browserSession,
        sandbox: true,
        contextIsolation: true,
        nodeIntegration: false,
        webSecurity: true,
      },
    })
    view.setBackgroundColor('#ffffff')
    view.setVisible(false)
    const entry: BrowserEntry = {
      tabID,
      kind,
      view,
      requestedURL: '',
      navigation: -1,
      previewPrefix,
    }
    this.#wireEntry(entry)
    return entry
  }

  async #workspaceSession(
    tabID: string,
    target: URL,
    previewPrefix: string,
  ): Promise<Session> {
    const digest = createHash('sha256').update(tabID).digest('hex').slice(0, 20)
    const browserSession = session.fromPartition(`coding-preview-${digest}`)
    browserSession.setPermissionRequestHandler((_contents, _permission, callback) => {
      callback(false)
    })
    browserSession.webRequest.onBeforeRequest((details, callback) => {
      const requestURL = safeURL(details.url)
      const desktopOrigin = new URL(this.#desktop.url).origin
      if (
        requestURL?.origin === desktopOrigin &&
        (
          !requestURL.pathname.startsWith(previewPrefix) ||
          (details.method !== 'GET' && details.method !== 'HEAD')
        )
      ) {
        callback({ cancel: true })
        return
      }
      callback({})
    })
    await browserSession.cookies.set({
      url: target.href,
      name: this.#desktop.cookieName,
      value: this.#desktop.token,
      httpOnly: true,
      sameSite: 'strict',
      secure: false,
      path: previewPrefix,
    })
    return browserSession
  }

  #wireEntry(entry: BrowserEntry): void {
    const contents = entry.view.webContents
    const send = () => this.#sendState(entry)
    const clearError = () => {
      entry.error = undefined
      send()
    }
    const fail = (error: string) => {
      entry.error = error
      send()
    }

    contents.on('did-start-loading', clearError)
    contents.on('did-stop-loading', () => send())
    contents.on('did-navigate', (_event, url) => {
      entry.requestedURL = url
      send()
    })
    contents.on('did-navigate-in-page', (_event, url) => {
      entry.requestedURL = url
      send()
    })
    contents.on('page-title-updated', () => send())
    contents.on(
      'did-fail-load',
      (_event, errorCode, errorDescription, _validatedURL, isMainFrame) => {
        if (isMainFrame && errorCode !== -3) fail(errorDescription)
      },
    )
    contents.on('render-process-gone', (_event, details) => {
      fail(`Browser renderer stopped: ${details.reason}`)
    })
    contents.setWindowOpenHandler(({ url }) => {
      const target = safeURL(url)
      if (target && (target.protocol === 'http:' || target.protocol === 'https:')) {
        void contents.loadURL(target.href)
      } else if (target) {
        void openExternalURL(target)
      }
      return { action: 'deny' }
    })
    contents.on('will-navigate', (event, url) => {
      const target = safeURL(url)
      if (!target || (target.protocol !== 'http:' && target.protocol !== 'https:')) {
        event.preventDefault()
        if (target) void openExternalURL(target)
        return
      }
      if (
        entry.previewPrefix &&
        target.origin === new URL(this.#desktop.url).origin &&
        !target.pathname.startsWith(entry.previewPrefix)
      ) {
        event.preventDefault()
      }
    })
  }

  #sendState(entry: BrowserEntry): void {
    const contents = entry.view.webContents
    if (contents.isDestroyed() || this.#window.webContents.isDestroyed()) return
    const state: BrowserState = {
      tabID: entry.tabID,
      url: contents.getURL() || entry.requestedURL,
      title: contents.getTitle(),
      loading: contents.isLoading(),
      canGoBack: contents.navigationHistory.canGoBack(),
      canGoForward: contents.navigationHistory.canGoForward(),
      error: entry.error,
    }
    this.#window.webContents.send('desktop:browser:state', state)
  }

  #entry(tabID: unknown): BrowserEntry | undefined {
    const id = this.#tabID(tabID)
    return id ? this.#entries.get(id) : undefined
  }

  #tabID(value: unknown): string | undefined {
    return typeof value === 'string' && value ? value : undefined
  }

  #beginOperation(tabID: string): number {
    const operation = (this.#operations.get(tabID) ?? 0) + 1
    this.#operations.set(tabID, operation)
    return operation
  }

  #isCurrent(tabID: string, operation: number): boolean {
    return !this.#destroyed && this.#operations.get(tabID) === operation
  }

  #removeEntry(entry: BrowserEntry): void {
    if (this.#entries.get(entry.tabID) === entry) {
      this.#entries.delete(entry.tabID)
      this.#window.contentView.removeChildView(entry.view)
    }
    this.#disposeEntry(entry)
  }

  #disposeEntry(entry: BrowserEntry): void {
    if (!entry.view.webContents.isDestroyed()) {
      entry.view.webContents.close({ waitForBeforeUnload: false })
    }
  }
}

function parseShowInput(value: unknown): ShowBrowserInput {
  if (!value || typeof value !== 'object') throw new TypeError('browser input is required')
  const input = value as Partial<ShowBrowserInput>
  if (typeof input.tabID !== 'string' || !input.tabID) {
    throw new TypeError('browser tab ID is required')
  }
  if (typeof input.url !== 'string' || !input.url) {
    throw new TypeError('browser URL is required')
  }
  if (typeof input.navigation !== 'number' || !Number.isSafeInteger(input.navigation)) {
    throw new TypeError('browser navigation revision is invalid')
  }
  if (typeof input.workspacePreview !== 'boolean') {
    throw new TypeError('browser preview mode is invalid')
  }
  if (!input.bounds || typeof input.bounds !== 'object') {
    throw new TypeError('browser bounds are required')
  }
  const bounds = input.bounds as Partial<Rectangle>
  for (const value of [bounds.x, bounds.y, bounds.width, bounds.height]) {
    if (typeof value !== 'number' || !Number.isFinite(value)) {
      throw new TypeError('browser bounds are invalid')
    }
  }
  return {
    tabID: input.tabID,
    url: input.url,
    navigation: input.navigation,
    workspacePreview: input.workspacePreview,
    bounds: bounds as Rectangle,
  }
}

function resolveWebURL(value: string, base: string): URL {
  const target = new URL(value, base)
  if (target.protocol !== 'http:' && target.protocol !== 'https:') {
    throw new TypeError(`unsupported browser URL protocol: ${target.protocol}`)
  }
  return target
}

function workspacePreviewPrefix(target: URL, desktopURL: string): string {
  if (target.origin !== new URL(desktopURL).origin) {
    throw new TypeError('workspace preview must use the desktop origin')
  }
  const match = target.pathname.match(/^\/api\/sessions\/[^/]+\/preview\//)
  if (!match) throw new TypeError('workspace preview URL is invalid')
  return match[0]
}

function clampBounds(bounds: Rectangle, window: BrowserWindow): Rectangle {
  const [windowWidth, windowHeight] = window.getContentSize()
  const x = clamp(Math.ceil(bounds.x), 0, Math.max(0, windowWidth - 1))
  const y = clamp(Math.ceil(bounds.y), 0, Math.max(0, windowHeight - 1))
  const right = Math.floor(bounds.x + bounds.width)
  const bottom = Math.floor(bounds.y + bounds.height)
  return {
    x,
    y,
    width: clamp(right - x, 1, windowWidth - x),
    height: clamp(bottom - y, 1, windowHeight - y),
  }
}

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.max(minimum, Math.min(value, Math.max(minimum, maximum)))
}

function safeURL(value: string): URL | undefined {
  try {
    return new URL(value)
  } catch {
    return undefined
  }
}

async function openExternalURL(url: URL): Promise<void> {
  if (!['mailto:', 'tel:'].includes(url.protocol)) return
  await shell.openExternal(url.href)
}
