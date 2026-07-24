import { createHash } from 'node:crypto'
import {
  BrowserWindow,
  WebContentsView,
  session,
  shell,
  type Rectangle,
  type Session,
} from 'electron'
import {
  workspacePreviewNavigationAllowed,
  workspacePreviewPrefix,
  workspacePreviewRequestAllowed,
} from './workspacePreviewSecurity'

export type DesktopEndpoint = {
  url: string
  cookieName: string
  token: string
}

type BrowserKind = 'web' | 'workspace-preview'

type NavigateBrowserInput = {
  tabID: string
  url: string
  revision: number
  kind: BrowserKind
}

type BrowserViewportInput = {
  tabID: string
  visible: boolean
  bounds?: Rectangle
}

type BrowserEntry = {
  tabID: string
  kind: BrowserKind
  view: WebContentsView
  appliedRevision: number
  requestedURL: string
  committedURL: string
  status: 'navigating' | 'ready' | 'failed'
  pendingOperation?: number
  previewPrefix?: string
  error?: string
}

type BrowserState = {
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

export class NativeBrowserManager {
  readonly #window: BrowserWindow
  readonly #desktop: DesktopEndpoint
  readonly #webSession: Session
  readonly #entries = new Map<string, BrowserEntry>()
  readonly #viewports = new Map<string, BrowserViewportInput>()
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

  async navigate(value: unknown): Promise<BrowserState> {
    const input = parseNavigateInput(value)
    if (this.#destroyed) throw new Error('native browser is destroyed')
    const target = resolveWebURL(input.url, this.#desktop.url)
    const targetPreviewPrefix = input.kind === 'workspace-preview'
      ? workspacePreviewPrefix(target, this.#desktop.url)
      : undefined
    let entry = this.#entries.get(input.tabID)
    if (entry && input.revision < entry.appliedRevision) {
      return this.#state(entry)
    }
    if (entry && input.revision === entry.appliedRevision) {
      if (entry.kind !== input.kind || entry.requestedURL !== target.href) {
        throw new Error('browser navigation revision conflicts with its target')
      }
      return this.#state(entry)
    }

    const operation = this.#beginOperation(input.tabID)
    if (
      entry &&
      (entry.kind !== input.kind || entry.previewPrefix !== targetPreviewPrefix)
    ) {
      this.#removeEntry(entry)
      entry = undefined
    }
    if (!entry) {
      const created = await this.#createEntry(input.tabID, input.kind, target)
      if (!this.#isCurrent(input.tabID, operation)) {
        this.#disposeEntry(created)
        const current = this.#entries.get(input.tabID)
        return current ? this.#state(current) : this.#state(created)
      }
      entry = created
      this.#entries.set(input.tabID, entry)
      this.#window.contentView.addChildView(entry.view)
      this.#applyViewport(entry)
    }

    const contents = entry.view.webContents
    const reload = entry.status === 'ready' && entry.committedURL === target.href
    if (contents.isLoading()) contents.stop()
    entry.appliedRevision = input.revision
    entry.requestedURL = target.href
    entry.status = 'navigating'
    entry.pendingOperation = operation
    entry.error = undefined
    this.#sendState(entry)

    if (reload) {
      try {
        contents.reload()
        entry.pendingOperation = undefined
        return this.#state(entry)
      } catch (error) {
        entry.pendingOperation = undefined
        entry.status = 'failed'
        entry.error = errorMessage(error)
        this.#sendState(entry)
        return this.#state(entry)
      }
    }

    try {
      await contents.loadURL(target.href)
    } catch (error) {
      if (!this.#isCurrent(input.tabID, operation)) {
        const current = this.#entries.get(input.tabID)
        return current ? this.#state(current) : this.#state(entry)
      }
      entry.pendingOperation = undefined
      entry.status = 'failed'
      entry.error = errorMessage(error)
      this.#sendState(entry)
      return this.#state(entry)
    }

    if (!this.#isCurrent(input.tabID, operation)) {
      const current = this.#entries.get(input.tabID)
      return current ? this.#state(current) : this.#state(entry)
    }
    entry.pendingOperation = undefined
    entry.committedURL = contents.getURL() || target.href
    entry.status = 'ready'
    entry.error = undefined
    this.#sendState(entry)
    return this.#state(entry)
  }

  setViewport(value: unknown): void {
    const input = parseViewportInput(value)
    if (this.#destroyed) throw new Error('native browser is destroyed')
    const entry = this.#entries.get(input.tabID)
    if (!entry && !input.visible) {
      this.#viewports.delete(input.tabID)
      return
    }
    this.#viewports.set(input.tabID, input)
    if (entry) this.#applyViewport(entry)
  }

  close(tabID: unknown): void {
    const id = this.#tabID(tabID)
    if (!id) return
    this.#beginOperation(id)
    this.#viewports.delete(id)
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

  destroy(): void {
    this.#destroyed = true
    for (const tabID of this.#operations.keys()) this.#beginOperation(tabID)
    for (const entry of [...this.#entries.values()]) this.#removeEntry(entry)
    this.#viewports.clear()
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
      appliedRevision: -1,
      requestedURL: '',
      committedURL: '',
      status: 'navigating',
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
    const digest = createHash('sha256')
      .update(`${tabID}\x00${previewPrefix}`)
      .digest('hex')
      .slice(0, 20)
    const browserSession = session.fromPartition(`coding-preview-${digest}`)
    browserSession.setPermissionRequestHandler((_contents, _permission, callback) => {
      callback(false)
    })
    browserSession.webRequest.onBeforeRequest((details, callback) => {
      callback({
        cancel: !workspacePreviewRequestAllowed(
          details.url,
          details.method,
          this.#desktop.url,
          previewPrefix,
        ),
      })
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
    const start = () => {
      if (entry.pendingOperation !== undefined) return
      entry.error = undefined
      entry.status = 'navigating'
      send()
    }
    const fail = (error: string) => {
      if (entry.pendingOperation !== undefined) return
      entry.error = error
      entry.status = 'failed'
      send()
    }

    contents.on('did-start-loading', start)
    contents.on('did-stop-loading', () => {
      if (entry.pendingOperation !== undefined) return
      if (!entry.error) entry.status = 'ready'
      send()
    })
    contents.on('did-navigate', (_event, url) => {
      if (entry.pendingOperation !== undefined) return
      entry.committedURL = url
      entry.requestedURL = url
      send()
    })
    contents.on('did-navigate-in-page', (_event, url, isMainFrame) => {
      if (!isMainFrame || entry.pendingOperation !== undefined) return
      entry.committedURL = url
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
      entry.pendingOperation = undefined
      entry.error = `Browser renderer stopped: ${details.reason}`
      entry.status = 'failed'
      send()
    })
    contents.setWindowOpenHandler(({ url }) => {
      if (entry.kind === 'workspace-preview') return { action: 'deny' }
      const target = safeURL(url)
      if (target && (target.protocol === 'http:' || target.protocol === 'https:')) {
        entry.requestedURL = target.href
        entry.status = 'navigating'
        entry.error = undefined
        send()
        void contents.loadURL(target.href).catch((error: unknown) => {
          fail(errorMessage(error))
        })
      } else if (target) {
        void openExternalURL(target)
      }
      return { action: 'deny' }
    })
    contents.on('will-navigate', (event, url) => {
      if (entry.previewPrefix) {
        const allowed = workspacePreviewNavigationAllowed(
          url,
          this.#desktop.url,
          entry.previewPrefix,
        )
        if (!allowed) {
          event.preventDefault()
        } else if (entry.pendingOperation === undefined) {
          entry.requestedURL = url
        }
        return
      }
      const target = safeURL(url)
      if (!target || (target.protocol !== 'http:' && target.protocol !== 'https:')) {
        event.preventDefault()
        if (target) void openExternalURL(target)
        return
      }
      if (entry.pendingOperation === undefined) entry.requestedURL = target.href
    })
  }

  #sendState(entry: BrowserEntry): void {
    if (
      this.#entries.get(entry.tabID) !== entry ||
      entry.view.webContents.isDestroyed() ||
      this.#window.webContents.isDestroyed()
    ) {
      return
    }
    this.#window.webContents.send('desktop:browser:state', this.#state(entry))
  }

  #state(entry: BrowserEntry): BrowserState {
    const contents = entry.view.webContents
    const available = !contents.isDestroyed()
    return {
      tabID: entry.tabID,
      appliedRevision: entry.appliedRevision,
      requestedURL: entry.requestedURL,
      committedURL: entry.committedURL,
      title: available && entry.status !== 'navigating' ? contents.getTitle() : '',
      status: entry.status,
      canGoBack: available && contents.navigationHistory.canGoBack(),
      canGoForward: available && contents.navigationHistory.canGoForward(),
      error: entry.error,
    }
  }

  #applyViewport(entry: BrowserEntry): void {
    const viewport = this.#viewports.get(entry.tabID)
    if (!viewport?.visible || !viewport.bounds) {
      entry.view.setVisible(false)
      return
    }
    for (const candidate of this.#entries.values()) {
      if (candidate !== entry) candidate.view.setVisible(false)
    }
    entry.view.setBounds(clampBounds(viewport.bounds, this.#window))
    entry.view.setVisible(true)
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

function parseNavigateInput(value: unknown): NavigateBrowserInput {
  if (!value || typeof value !== 'object') throw new TypeError('browser input is required')
  const input = value as Partial<NavigateBrowserInput>
  if (typeof input.tabID !== 'string' || !input.tabID) {
    throw new TypeError('browser tab ID is required')
  }
  if (typeof input.url !== 'string' || !input.url) {
    throw new TypeError('browser URL is required')
  }
  if (
    typeof input.revision !== 'number' ||
    !Number.isSafeInteger(input.revision) ||
    input.revision < 0
  ) {
    throw new TypeError('browser navigation revision is invalid')
  }
  if (input.kind !== 'web' && input.kind !== 'workspace-preview') {
    throw new TypeError('browser target kind is invalid')
  }
  return {
    tabID: input.tabID,
    url: input.url,
    revision: input.revision,
    kind: input.kind,
  }
}

function parseViewportInput(value: unknown): BrowserViewportInput {
  if (!value || typeof value !== 'object') throw new TypeError('browser input is required')
  const input = value as Partial<BrowserViewportInput>
  if (typeof input.tabID !== 'string' || !input.tabID) {
    throw new TypeError('browser tab ID is required')
  }
  if (typeof input.visible !== 'boolean') {
    throw new TypeError('browser viewport visibility is invalid')
  }
  if (!input.visible && input.bounds === undefined) {
    return { tabID: input.tabID, visible: false }
  }
  if (!input.bounds || typeof input.bounds !== 'object') {
    throw new TypeError('visible browser bounds are required')
  }
  const bounds = input.bounds as Partial<Rectangle>
  for (const value of [bounds.x, bounds.y, bounds.width, bounds.height]) {
    if (typeof value !== 'number' || !Number.isFinite(value)) {
      throw new TypeError('browser bounds are invalid')
    }
  }
  return {
    tabID: input.tabID,
    visible: input.visible,
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

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

async function openExternalURL(url: URL): Promise<void> {
  if (!['mailto:', 'tel:'].includes(url.protocol)) return
  await shell.openExternal(url.href)
}
