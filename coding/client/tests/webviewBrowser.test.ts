import { beforeEach, describe, expect, test } from 'bun:test'
import { browserRuntimeTabID } from '../src/browserRuntime'

// The webview bridge only touches window for timers and URL resolution.
;(globalThis as unknown as { window: unknown }).window = {
  location: { href: 'http://127.0.0.1:1234/' },
  setTimeout: globalThis.setTimeout.bind(globalThis),
  clearTimeout: globalThis.clearTimeout.bind(globalThis),
}

const { registerWebviewBrowser, webviewBrowserBridge } = await import(
  '../src/lib/webviewBrowser'
)
type BrowserRuntimeState = Awaited<
  ReturnType<typeof webviewBrowserBridge.navigate>
>
const runtimeTabID = browserRuntimeTabID('session-1', 'tab-1')

// A guest that has not attached yet: getWebContentsId throws and getURL is
// empty until the first document commits, exactly like a fresh <webview>.
class FakeWebview extends EventTarget {
  attached = false
  url = ''
  title = ''
  stopped = 0
  reloaded = 0
  loadCalls: string[] = []
  private settles: Array<{ url: string; resolve: () => void }> = []

  getWebContentsId(): number {
    if (!this.attached) throw new Error('The WebView must be attached to the DOM')
    return 7
  }

  getURL(): string {
    return this.url
  }

  getTitle(): string {
    return this.title
  }

  canGoBack(): boolean {
    return false
  }

  canGoForward(): boolean {
    return false
  }

  stop(): void {
    this.stopped += 1
  }

  reload(): void {
    this.reloaded += 1
  }

  loadURL(url: string): Promise<void> {
    this.loadCalls.push(url)
    return new Promise((resolve) => {
      this.settles.push({ url, resolve })
    })
  }

  executeJavaScript(): Promise<unknown> {
    return Promise.resolve({ visibleText: '', truncated: false })
  }

  // Test helpers -----------------------------------------------------------

  attach(): void {
    this.attached = true
    this.emit('dom-ready')
  }

  // The guest's own initial document, committed before any requested load.
  commitInitialDocument(): void {
    this.url = 'about:blank'
    this.title = 'about:blank'
    const event = new Event('did-navigate') as Event & { url: string }
    event.url = 'about:blank'
    this.dispatchEvent(event)
    this.emit('did-finish-load')
    this.emit('did-stop-loading')
  }

  // Reports one requested load back without committing a document, which is
  // how a superseded load resolves late.
  resolveLoad(index: number): void {
    const settle = this.settles[index]
    if (!settle) throw new Error(`no load at ${index}`)
    settle.resolve()
    this.settles = this.settles.filter((entry) => entry !== settle)
  }

  commit(url: string, title = ''): void {
    this.url = url
    this.title = title
    const event = new Event('did-navigate') as Event & { url: string }
    event.url = url
    this.dispatchEvent(event)
    const settle = this.settles.find((entry) => entry.url === url)
    settle?.resolve()
    this.settles = this.settles.filter((entry) => entry !== settle)
  }

  emit(name: string): void {
    this.dispatchEvent(new Event(name))
  }
}

function element(guest: FakeWebview) {
  return guest as unknown as Parameters<typeof registerWebviewBrowser>[1]
}

async function settle(): Promise<void> {
  for (let tick = 0; tick < 6; tick += 1) await Promise.resolve()
  await new Promise((resolve) => setTimeout(resolve, 0))
}

describe('webviewBrowserBridge.navigate', () => {
  let guest: FakeWebview
  let states: BrowserRuntimeState[]
  let unregister: () => void
  let unsubscribe: () => void

  beforeEach(() => {
    guest = new FakeWebview()
    states = []
    unregister?.()
    unsubscribe?.()
    unregister = registerWebviewBrowser(runtimeTabID, element(guest))
    unsubscribe = webviewBrowserBridge.onState((state) => {
      states.push(state)
    })
  })

  test('the guest initial document does not complete a pending navigation', async () => {
    const navigation = webviewBrowserBridge.navigate({
      tabID: runtimeTabID,
      revision: 0,
      url: 'https://www.bilibili.com',
      kind: 'web',
    })
    await settle()

    // The fresh guest attaches and commits its own about:blank document.
    guest.attach()
    guest.commitInitialDocument()
    await settle()

    expect(
      states.filter((state) => state.status !== 'navigating'),
    ).toEqual([])
    expect(guest.loadCalls).toEqual(['https://www.bilibili.com/'])

    guest.commit('https://www.bilibili.com/', 'bilibili')
    guest.emit('did-stop-loading')
    const state = await navigation
    expect(state).toMatchObject({
      appliedRevision: 0,
      status: 'ready',
      committedURL: 'https://www.bilibili.com/',
    })
    expect(
      states.filter(
        (entry) => entry.status === 'ready' && entry.committedURL === '',
      ),
    ).toEqual([])
  })

  test('a superseded navigation cannot commit over the latest one', async () => {
    const first = webviewBrowserBridge.navigate({
      tabID: runtimeTabID,
      revision: 0,
      url: 'https://github.com',
      kind: 'web',
    })
    await settle()
    guest.attach()
    await settle()

    const second = webviewBrowserBridge.navigate({
      tabID: runtimeTabID,
      revision: 1,
      url: 'https://www.bilibili.com',
      kind: 'web',
    })
    await settle()

    // The first load reports back after the second one was issued.
    guest.resolveLoad(0)
    await settle()
    await first
    expect(states.at(-1)).toMatchObject({
      appliedRevision: 1,
      status: 'navigating',
      committedURL: '',
    })

    guest.commit('https://www.bilibili.com/', 'bilibili')
    guest.emit('did-stop-loading')
    expect(await second).toMatchObject({
      appliedRevision: 1,
      status: 'ready',
      committedURL: 'https://www.bilibili.com/',
    })
  })

  test('an explicit reload of the committed URL still completes', async () => {
    const first = webviewBrowserBridge.navigate({
      tabID: runtimeTabID,
      revision: 0,
      url: 'https://example.com',
      kind: 'web',
    })
    await settle()
    guest.attach()
    await settle()
    guest.commit('https://example.com/', 'Example')
    guest.emit('did-stop-loading')
    await first

    const reloaded = webviewBrowserBridge.navigate({
      tabID: runtimeTabID,
      revision: 1,
      url: 'https://example.com',
      kind: 'web',
    })
    await settle()
    expect(guest.reloaded).toBe(1)
    guest.emit('did-stop-loading')
    expect(await reloaded).toMatchObject({
      appliedRevision: 1,
      status: 'ready',
      committedURL: 'https://example.com/',
    })
  })
})
