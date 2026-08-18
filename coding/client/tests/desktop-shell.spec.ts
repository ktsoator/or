import { expect, test, type Page } from '@playwright/test'
import type { ContextUsage, UsageEventPage, UsageReport } from '../src/types'

type BrowserRuntimeRecord = {
  tabID: string
  url: string
  title: string
  bounds: { x: number; y: number; width: number; height: number }
  visible: boolean
  status: string | null
  loadCalls: string[]
  reloadCalls: number
  inspectCalls: number
}

// Reads what the renderer's real webview bridge did to a tab's guest, so the
// assertions describe the shipped path instead of a test-only adapter.
async function browserRuntimeView(
  page: Page,
  tabID: string,
): Promise<BrowserRuntimeRecord | undefined> {
  return page.evaluate((id) => {
    const host = document.querySelector(`[data-browser-tab-id="${CSS.escape(id)}"]`)
    if (!host) return undefined
    const guest = host.querySelector('webview') as
      | (HTMLElement & {
          guestURL?: string
          guestTitle?: string
          loadCalls?: string[]
          reloadCalls?: number
          inspectCalls?: number
        })
      | null
    const bounds = host.getBoundingClientRect()
    return {
      tabID: id,
      url: guest?.guestURL ?? '',
      title: guest?.guestTitle ?? '',
      bounds: {
        x: bounds.x,
        y: bounds.y,
        width: bounds.width,
        height: bounds.height,
      },
      visible: host.checkVisibility({
        checkVisibilityCSS: true,
        opacityProperty: true,
      }),
      status: host.getAttribute('data-status'),
      loadCalls: guest?.loadCalls ?? [],
      reloadCalls: guest?.reloadCalls ?? 0,
      inspectCalls: guest?.inspectCalls ?? 0,
    }
  }, tabID)
}

// Drives the fake guest the way a page drives a real one: the guest commits a
// navigation the renderer never requested.
async function guestNavigatesItself(
  page: Page,
  tabID: string,
  url: string,
  title = '',
): Promise<void> {
  await page.evaluate(({ id, target, pageTitle }) => {
    const host = document.querySelector(`[data-browser-tab-id="${CSS.escape(id)}"]`)
    const guest = host?.querySelector('webview') as
      | (HTMLElement & {
          guestURL: string
          guestTitle: string
          history: string[]
          historyIndex: number
        })
      | null
    if (!guest) throw new Error(`no browser guest for ${id}`)
    guest.guestURL = target
    guest.guestTitle = pageTitle
    guest.history = [...guest.history.slice(0, guest.historyIndex + 1), target]
    guest.historyIndex = guest.history.length - 1
    const navigated = new Event('did-navigate') as Event & { url: string }
    navigated.url = target
    guest.dispatchEvent(navigated)
    guest.dispatchEvent(new Event('did-stop-loading'))
  }, { id: tabID, target: url, pageTitle: title })
}

async function setGuestControls(
  page: Page,
  controls: { failNextLoad?: string; loadDelayMs?: number; pageTitle?: string },
): Promise<void> {
  await page.evaluate((next) => {
    const controlsWindow = window as Window & {
      __guestControls?: Record<string, unknown>
    }
    Object.assign(controlsWindow.__guestControls ?? {}, next)
  }, controls)
}

async function emitSessionEvent(page: Page, sessionID: string, payload: unknown): Promise<void> {
  await page.evaluate(({ id, event }) => {
    const emit = (window as Window & {
      __emitSessionSSE?: (targetSessionID: string, value: unknown) => void
    }).__emitSessionSSE
    emit?.(id, event)
  }, { id: sessionID, event: payload })
}

const models = {
  models: [
    {
      provider: 'openai',
      id: 'test-model',
      name: 'Test model',
      contextWindow: 128000,
      thinkingLevels: ['medium'],
      supportsImages: true,
    },
  ],
  defaultProvider: 'openai',
  defaultModel: 'test-model',
  defaultThinkingLevel: 'medium',
}

function longThreadHistory(prefix: string) {
  return Array.from({ length: 18 }, (_, index) => [
    {
      type: 'user_message',
      id: `${prefix}-user-${index}`,
      text: `Question ${index + 1} with enough content to exercise the conversation layout`,
      images: [],
    },
    {
      type: 'run_start',
      id: `${prefix}-run-${index}`,
      startedAt: `2026-07-22T00:00:${String(index).padStart(2, '0')}Z`,
      durationMs: 2000,
    },
    {
      type: 'message_end',
      text: `Response ${index + 1}. This completed answer makes the restored transcript tall enough to require its own scroll container.`,
      finalResponse: true,
      modelName: 'Test model',
      completedAt: `2026-07-22T00:01:${String(index).padStart(2, '0')}Z`,
    },
  ]).flat()
}

async function openDesktopClient(
  page: Page,
  options: {
    failCreate?: boolean
    healthFailures?: number
    browserResultFailures?: number
    existingSession?: boolean
    historyEvents?: unknown[]
    historyRunning?: boolean
    historyEventSeq?: number
    backgroundTasks?: unknown[]
    secondarySession?: boolean
    secondaryHistoryEvents?: unknown[]
    contextUsage?: ContextUsage
    modelName?: string
    modelThinkingLevels?: Array<'off' | 'minimal' | 'low' | 'medium' | 'high' | 'xhigh' | 'max'>
    modelThinkingVisibility?: 'visible' | 'hidden'
    sessionScope?: 'chat' | 'project'
    sessionTitle?: string
    composerUpdateDelayMs?: number
    nativeDirectory?: string
    usageReport?: UsageReport
    usageEventPages?: UsageEventPage[]
    usageEventPagesByOffset?: Record<number, UsageEventPage>
    usageEventDelayMs?: number
    diagnosticsTrace?: (
      requestNumber: number,
      sessionID: string,
      query: URLSearchParams,
    ) => unknown | Promise<unknown>
    diagnosticsStatus?: number
    skills?: Array<{
      name: string
      description: string
      source: 'user' | 'project'
      dir: string
      path?: string
    }>
  } = {},
) {
  const requests: Array<{ method: string; path: string; url: string; body?: unknown }> = []
  const modelThinkingLevels = options.modelThinkingLevels ?? ['medium']
  const modelThinkingLevel = modelThinkingLevels[0] ?? 'off'
  const createdSession = {
    id: 'test-session',
    title: options.sessionTitle ?? 'New session',
    workspacePath: '/tmp/test-session',
    workspaceName: 'test-session',
    scope: options.sessionScope ?? 'chat',
    workspaceKind: options.sessionScope === 'project' ? 'folder' : 'scratch',
    createdAt: '2026-07-22T00:00:00Z',
    updatedAt: '2026-07-22T00:00:00Z',
    running: false,
    hasApproval: false,
    modelProvider: 'openai',
    modelId: 'test-model',
    modelName: options.modelName ?? 'Test model',
    thinkingLevel: modelThinkingLevel,
    permissionMode: 'ask',
  }
  const secondarySession = {
    ...createdSession,
    id: 'secondary-session',
    title: 'Secondary task',
    workspacePath: '/tmp/secondary-session',
    workspaceName: 'secondary-session',
    createdAt: '2026-07-21T00:00:00Z',
    updatedAt: '2026-07-21T00:00:00Z',
  }
  const workbenchSession = {
    ...createdSession,
    id: 'workbench-session',
    workspacePath: '/tmp/workbench-session',
    workspaceName: 'workbench-session',
    createdAt: '2026-07-23T00:00:00Z',
    updatedAt: '2026-07-23T00:00:00Z',
  }
  const branchSession = {
    ...createdSession,
    id: 'branch-session',
    title: 'New session (branch)',
    forkedFromSessionId: createdSession.id,
    forkedFromMessageId: 'assistant-branch',
    updatedAt: '2026-07-24T00:00:00Z',
  }
  let sessionCreated = Boolean(options.existingSession)
  let workbenchSessionCreated = false
  let branchSessionCreated = false
  let sessionHistoryEvents = options.historyEvents ?? []
  let sessionHistoryRunning = options.historyRunning ?? false
  let remainingHealthFailures = options.healthFailures ?? 0
  let remainingBrowserResultFailures = options.browserResultFailures ?? 0
  let diagnosticTraceRequestCount = 0
  const usageEventRangeKeys: string[] = []

  await page.addInitScript(({ nativeDirectory }) => {
    // A stand-in for Electron's <webview>: it attaches asynchronously and
    // commits its own about:blank document before any requested load, which is
    // the sequence the renderer bridge has to survive. The tag name has no
    // hyphen, so it cannot be registered as a custom element.
    type FakeGuest = HTMLElement & {
      attached: boolean
      guestURL: string
      guestTitle: string
      history: string[]
      historyIndex: number
      loadCalls: string[]
      reloadCalls: number
      inspectCalls: number
      stopCalls: number
    }
    const navigateEvent = (url: string, name = 'did-navigate') => {
      const event = new Event(name) as Event & { url: string }
      event.url = url
      return event
    }
    const commitGuest = (guest: FakeGuest, url: string, title: string) => {
      guest.guestURL = url
      guest.guestTitle = title
      guest.dispatchEvent(navigateEvent(url))
      guest.dispatchEvent(new Event('did-stop-loading'))
    }
    const installFakeGuest = (element: HTMLElement) => {
      const guest = element as FakeGuest
      guest.attached = false
      guest.guestURL = ''
      guest.guestTitle = ''
      guest.history = []
      guest.historyIndex = -1
      guest.loadCalls = []
      guest.reloadCalls = 0
      guest.inspectCalls = 0
      guest.stopCalls = 0
      Object.assign(guest, {
        getWebContentsId() {
          if (!guest.attached) throw new Error('The WebView must be attached to the DOM')
          return 1
        },
        getURL: () => guest.guestURL,
        getTitle: () => guest.guestTitle,
        canGoBack: () => guest.historyIndex > 0,
        canGoForward: () => guest.historyIndex < guest.history.length - 1,
        stop() {
          guest.stopCalls += 1
        },
        goBack() {
          if (guest.historyIndex <= 0) return
          guest.historyIndex -= 1
          commitGuest(guest, guest.history[guest.historyIndex] ?? '', guestControls.pageTitle ?? '')
        },
        goForward() {
          if (guest.historyIndex >= guest.history.length - 1) return
          guest.historyIndex += 1
          commitGuest(guest, guest.history[guest.historyIndex] ?? '', guestControls.pageTitle ?? '')
        },
        reload() {
          guest.reloadCalls += 1
          window.setTimeout(() => {
            guest.dispatchEvent(new Event('did-stop-loading'))
          }, 5)
        },
        loadURL(url: string) {
          guest.loadCalls.push(url)
          return new Promise<void>((resolve, reject) => {
            window.setTimeout(() => {
              if (guestControls.failNextLoad === url) {
                guestControls.failNextLoad = undefined
                const failure = new Event('did-fail-load') as Event & {
                  errorCode: number
                  errorDescription: string
                }
                failure.errorCode = -105
                failure.errorDescription = 'ERR_NAME_NOT_RESOLVED'
                guest.dispatchEvent(failure)
                reject(new Error('ERR_NAME_NOT_RESOLVED'))
                return
              }
              guest.history = [...guest.history.slice(0, guest.historyIndex + 1), url]
              guest.historyIndex = guest.history.length - 1
              commitGuest(guest, url, guestControls.pageTitle ?? '')
              resolve()
            }, guestControls.loadDelayMs ?? 5)
          })
        },
        executeJavaScript() {
          guest.inspectCalls += 1
          return Promise.resolve({
            visibleText: `Visible content for ${guest.guestURL}`,
            truncated: false,
          })
        },
      })
      window.setTimeout(() => {
        guest.attached = true
        guest.guestURL = 'about:blank'
        guest.guestTitle = 'about:blank'
        guest.dispatchEvent(new Event('dom-ready'))
        guest.dispatchEvent(navigateEvent('about:blank'))
        guest.dispatchEvent(new Event('did-finish-load'))
        guest.dispatchEvent(new Event('did-stop-loading'))
      }, 0)
    }
    const nativeCreateElement = document.createElement.bind(document)
    document.createElement = ((tag: string, options?: ElementCreationOptions) => {
      const element = nativeCreateElement(tag, options)
      if (tag === 'webview') installFakeGuest(element)
      return element
    }) as typeof document.createElement
    type GuestControls = {
      failNextLoad?: string
      loadDelayMs?: number
      pageTitle?: string
    }
    const guestControls: GuestControls = {}
    ;(window as Window & { __guestControls?: GuestControls }).__guestControls =
      guestControls

    Object.defineProperty(navigator, 'platform', {
      configurable: true,
      get: () => 'MacIntel',
    })
    Object.defineProperty(window, 'codingDesktop', {
      configurable: true,
      value: {
        platform: 'darwin',
        browserMode: 'webview',
        openExternalURL(url: string) {
          const testWindow = window as Window & { __openedURL?: string }
          testWindow.__openedURL = url
        },
        chooseDirectory(initialPath: string, title: string) {
          const testWindow = window as Window & {
            __directoryArgs?: { initialPath: string; title: string }
          }
          testWindow.__directoryArgs = { initialPath, title }
          return Promise.resolve(nativeDirectory ?? '')
        },
        revealPath(target: string) {
          const testWindow = window as Window & { __revealedPath?: string }
          testWindow.__revealedPath = target
          return Promise.resolve()
        },
      },
    })

    class TestEventSource {
      onopen: ((event: Event) => void) | null = null
      onerror: ((event: Event) => void) | null = null
      onmessage: ((event: MessageEvent) => void) | null = null
      readonly url: string
      closed = false

      constructor(url: string) {
        this.url = url
        const testWindow = window as Window & { __eventSources?: TestEventSource[] }
        testWindow.__eventSources = [...(testWindow.__eventSources ?? []), this]
        window.setTimeout(() => this.onopen?.(new Event('open')), 0)
      }

      close() {
        this.closed = true
      }
    }

    Object.defineProperty(window, 'EventSource', {
      configurable: true,
      value: TestEventSource,
    })
    Object.defineProperty(window, '__emitSSE', {
      configurable: true,
      value: (payload: unknown) => {
        const sources = (window as Window & { __eventSources?: TestEventSource[] }).__eventSources
        sources?.findLast((source) => !source.closed)?.onmessage?.(
          new MessageEvent('message', { data: JSON.stringify(payload) }),
        )
      },
    })
    Object.defineProperty(window, '__emitSessionSSE', {
      configurable: true,
      value: (sessionID: string, payload: unknown) => {
        const sources = (window as Window & { __eventSources?: TestEventSource[] }).__eventSources
        sources
          ?.findLast((source) => !source.closed && source.url.includes(`/sessions/${sessionID}/events`))
          ?.onmessage?.(new MessageEvent('message', { data: JSON.stringify(payload) }))
      },
    })
  }, { nativeDirectory: options.nativeDirectory })

  await page.route('**/api/**', async (route) => {
    const request = route.request()
    const path = new URL(route.request().url()).pathname
    const method = request.method()
    const postData = request.postData()
    const requestBody = postData ? JSON.parse(postData) : undefined
    requests.push({ method, path, url: request.url(), body: requestBody })

    if (path === '/api/health') {
      if (remainingHealthFailures > 0) {
        remainingHealthFailures--
        await route.fulfill({ status: 503 })
      } else {
        await route.fulfill({ status: 204 })
      }
      return
    }

    if (path === '/api/sessions' && method === 'POST') {
      if (options.failCreate) {
        await route.fulfill({
          status: 400,
          contentType: 'application/json',
          body: JSON.stringify({ error: 'invalid session settings' }),
        })
        return
      }
      const created = sessionCreated ? workbenchSession : createdSession
      if (created.id === workbenchSession.id) workbenchSessionCreated = true
      sessionCreated = true
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify(created),
      })
      return
    }

    if (path === '/api/sessions/test-session/forks' && method === 'POST') {
      branchSessionCreated = true
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify(branchSession),
      })
      return
    }

    if (path === '/api/sessions/test-session/message-edits' && method === 'POST') {
      const edit = requestBody as { messageID: string; text: string }
      const target = sessionHistoryEvents.findIndex((event) => {
        if (!event || typeof event !== 'object') return false
        const candidate = event as { type?: string; messageID?: string }
        return candidate.type === 'user_message' && candidate.messageID === edit.messageID
      })
      if (target < 0) {
        await route.fulfill({
          status: 404,
          contentType: 'application/json',
          body: JSON.stringify({ error: 'message not found' }),
        })
        return
      }
      const original = sessionHistoryEvents[target] as Record<string, unknown>
      sessionHistoryEvents = [
        ...sessionHistoryEvents.slice(0, target),
        {
          ...original,
          messageID: 'edited-user-message',
          text: edit.text,
        },
      ]
      if (branchSessionCreated) {
        delete (branchSession as { forkedFromMessageId?: string }).forkedFromMessageId
      }
      sessionHistoryRunning = true
      createdSession.running = true
      await route.fulfill({
        status: 202,
        contentType: 'application/json',
        body: JSON.stringify(createdSession),
      })
      await page.evaluate(() => {
        const emit = (window as Window & {
          __emitSessionSSE?: (sessionID: string, payload: unknown) => void
        }).__emitSessionSSE
        emit?.('test-session', { type: 'sync_required' })
      })
      return
    }

    if (path === '/api/workspaces' && method === 'POST') {
      const workspacePath = (requestBody as { path: string }).path
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({
          path: workspacePath,
          name: workspacePath.split('/').filter(Boolean).at(-1),
          addedAt: '2026-07-22T00:00:00Z',
        }),
      })
      return
    }

    if (path === '/api/preview/check' && method === 'POST') {
      const previewURL = (requestBody as { url: string }).url
      const unavailable = previewURL.includes(':4311')
      await route.fulfill({
        status: unavailable ? 400 : 200,
        contentType: 'application/json',
        body: JSON.stringify(
          unavailable ? { error: 'local server is not reachable' } : { url: previewURL },
        ),
      })
      return
    }

    if (/\/api\/sessions\/[^/]+\/browser\/[^/]+\/result$/.test(path) && method === 'POST') {
      if (remainingBrowserResultFailures > 0) {
        remainingBrowserResultFailures -= 1
        await route.fulfill({
          status: 503,
          contentType: 'application/json',
          body: JSON.stringify({ error: 'browser result temporarily unavailable' }),
        })
      } else {
        await route.fulfill({ status: 204 })
      }
      return
    }

    if (/\/api\/sessions\/[^/]+\/tasks\/[^/]+\/output$/.test(path) && method === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          content: 'server listening on http://127.0.0.1:8080\nrequest completed',
          truncated: false,
        }),
      })
      return
    }

    if (/\/api\/sessions\/[^/]+\/tasks\/[^/]+\/stop$/.test(path) && method === 'POST') {
      await route.fulfill({ status: 204 })
      return
    }

    if (path === '/api/sessions/test-session/previews/test-grant/index.html' && method === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'text/html',
        body: '<!doctype html><title>Static page</title><main>Direct HTML preview</main>',
      })
      return
    }
    if (path === '/api/sessions/secondary-session/previews/secondary-grant/index.html' && method === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'text/html',
        body: '<!doctype html><title>Secondary preview</title><main>Secondary page</main>',
      })
      return
    }

    let body: unknown = []
    let status = 200
    if (path === '/api/models') {
      body = {
        ...models,
        defaultThinkingLevel: modelThinkingLevel,
        models: models.models.map((model) => ({
          ...model,
          name: options.modelName ?? model.name,
          thinkingLevels: modelThinkingLevels,
          ...(options.modelThinkingVisibility
            ? { thinkingVisibility: options.modelThinkingVisibility }
            : {}),
        })),
      }
    }
    if (path === '/api/diagnostics/trace') {
      status = options.diagnosticsStatus ?? status
      body = {
        version: 2,
        generatedAt: '2026-07-22T00:00:05Z',
        sessionId: 'test-session',
        selectedTaskId: 'run-diagnostics',
        page: { hasMore: false },
        tasks: [{
          id: 'run-diagnostics',
          status: 'completed',
          prompt: 'Create a short release note',
          startedAt: '2026-07-22T00:00:00Z',
          updatedAt: '2026-07-22T00:00:05Z',
          durationMs: 5000,
          timeToFirstOutputMs: 1200,
          inputTokens: 260,
          outputTokens: 80,
          cacheReadTokens: 80,
          totalTokens: 420,
          retries: 0,
          contextRecoveries: 0,
          rawEvents: [],
          requests: [
            {
              id: 'request-1',
              number: 1,
              turnId: 'turn-diagnostics-1',
              stepId: 'step-diagnostics-1',
              status: 'completed',
              lifecycle: 'complete',
              startedAt: '2026-07-22T00:00:00Z',
              completedAt: '2026-07-22T00:00:02Z',
              durationMs: 2000,
              timeToFirstOutputMs: 900,
              model: 'test-model',
              inputTokens: 180,
              outputTokens: 70,
              totalTokens: 250,
              attempts: [],
              checkpoints: [],
              snapshotState: 'available',
              rawEvents: [],
              input: {
                systemPrompt: 'You are a coding agent.',
                messages: [
                  { role: 'user', content: [{ type: 'text', text: '<or-context kind="base">\nCurrent runtime context.\n</or-context>' }] },
                  { role: 'user', content: [{ type: 'text', text: '<or-context kind="skill_listing">\nAvailable release skills.\n</or-context>' }] },
                  { role: 'user', content: [{ type: 'text', text: 'Create a short release note' }] },
                ],
                tools: [{ name: 'write', description: 'Write one file', parameters: { type: 'object' } }],
              },
              attachments: [
                { id: 'context-base-1', kind: 'base', placement: 'prefix', messageIndex: 0 },
                { id: 'skill-listing-1', kind: 'skill_listing', placement: 'prefix', messageIndex: 1 },
              ],
              output: {
                capturedAt: '2026-07-22T00:00:02Z',
                stopReason: 'tool_use',
                message: {
                  role: 'assistant',
                  content: [
                    { type: 'thinking', thinking: 'Prepare the release note before writing it.' },
                    { type: 'text', text: '## Release note\n\nI will write the **release note**.\n\n- Keep it concise' },
                    { type: 'toolCall', toolCallId: 'call-1', toolName: 'write', arguments: { path: 'RELEASE.md' } },
                  ],
                },
              },
              tools: [{
                id: 'call-1',
                name: 'write',
                status: 'success',
                lifecycle: 'complete',
                startedAt: '2026-07-22T00:00:02Z',
                completedAt: '2026-07-22T00:00:02.2Z',
                durationMs: 200,
                executionDurationMs: 200,
                arguments: { path: 'RELEASE.md' },
                result: { role: 'toolResult', toolCallId: 'call-1', toolName: 'write', content: [{ type: 'text', text: 'Created RELEASE.md' }] },
                rawEvents: [{
                  name: 'tool.call.started',
                  timestamp: '2026-07-22T00:00:02Z',
                  toolCallId: 'call-1',
                  toolName: 'write',
                }],
              }],
            },
            {
              id: 'request-2',
              number: 2,
              turnId: 'turn-diagnostics-1',
              stepId: 'step-diagnostics-2',
              status: 'completed',
              lifecycle: 'complete',
              startedAt: '2026-07-22T00:00:02.3Z',
              completedAt: '2026-07-22T00:00:05Z',
              durationMs: 2700,
              timeToFirstOutputMs: 1200,
              model: 'test-model',
              totalTokens: 170,
              attempts: [],
              checkpoints: [],
              tools: [{
                id: 'call-2',
                name: 'bash',
                status: 'success',
                lifecycle: 'complete',
                startedAt: '2026-07-22T00:00:04Z',
                completedAt: '2026-07-22T00:00:04.1Z',
                durationMs: 100,
                executionDurationMs: 100,
                arguments: { command: 'git status' },
                result: { role: 'toolResult', toolCallId: 'call-2', toolName: 'bash', content: [{ type: 'text', text: 'clean' }] },
                rawEvents: [],
              }],
              snapshotState: 'available',
              rawEvents: [],
              input: {
                systemPrompt: 'You are a coding agent.',
                messages: [
                  { role: 'user', content: [{ type: 'text', text: '<or-context kind="base">\nCurrent runtime context.\n</or-context>' }] },
                  { role: 'user', content: [{ type: 'text', text: '<or-context kind="skill_listing">\nAvailable release skills.\n</or-context>' }] },
                  { role: 'user', content: [{ type: 'text', text: '<or-context kind="context_update">\nThe git branch changed to release.\n</or-context>' }] },
                ],
              },
              attachments: [
                { id: 'context-base-1', kind: 'base', placement: 'prefix', messageIndex: 0 },
                { id: 'skill-listing-1', kind: 'skill_listing', placement: 'prefix', messageIndex: 1 },
                { id: 'context-update-1', kind: 'context_update', placement: 'after-current', revision: 'revision-2', messageIndex: 2 },
              ],
              output: {
                capturedAt: '2026-07-22T00:00:05Z',
                stopReason: 'stop',
                message: {
                  role: 'assistant',
                  content: [
                    { type: 'thinking', thinking: 'The file was created successfully.' },
                    { type: 'toolCall', toolCallId: 'call-2', toolName: 'bash', arguments: { command: 'git status' } },
                  ],
                },
              },
            },
          ],
        }, {
          id: 'run-diagnostics-followup',
          status: 'completed',
          prompt: 'Check the release status',
          startedAt: '2026-07-22T00:01:00Z',
          updatedAt: '2026-07-22T00:01:02Z',
          durationMs: 2000,
          inputTokens: 50,
          outputTokens: 30,
          cacheReadTokens: 40,
          totalTokens: 120,
          retries: 0,
          contextRecoveries: 0,
          rawEvents: [],
          requests: [{
            id: 'request-3',
            number: 3,
            turnId: 'turn-diagnostics-2',
            stepId: 'step-diagnostics-3',
            status: 'completed',
            lifecycle: 'complete',
            startedAt: '2026-07-22T00:01:00Z',
            completedAt: '2026-07-22T00:01:02Z',
            durationMs: 2000,
            model: 'test-model',
            totalTokens: 120,
            attempts: [],
            checkpoints: [],
            tools: [],
            snapshotState: 'available',
            rawEvents: [],
            input: {
              systemPrompt: 'You are a coding agent.',
              messages: [
                { role: 'user', content: [{ type: 'text', text: '<or-context kind="base">\nCurrent runtime context.\n</or-context>' }] },
                { role: 'user', content: [{ type: 'text', text: '<or-context kind="skill_listing">\nAvailable release skills.\n</or-context>' }] },
                { role: 'user', content: [{ type: 'text', text: '<or-context kind="activated_skill" name="git-workflow">\nThis Skill was activated earlier in the conversation. Its exact instructions remain in force.\n\n<loaded_skill name="git-workflow" root="/tmp/skills/git-workflow">\n# Git Workflow\n\n## Scope\n\nUse this skill only inside the repository.\n\n## Workflow order\n\n1. Confirm the working tree is clean.\n2. Create a focused branch from `main`.\n</loaded_skill>\n</or-context>' }] },
                { role: 'user', content: [{ type: 'text', text: 'Check the release status' }] },
              ],
            },
            attachments: [
              { id: 'context-base-1', kind: 'base', placement: 'prefix', messageIndex: 0 },
              { id: 'skill-listing-1', kind: 'skill_listing', placement: 'prefix', messageIndex: 1 },
              { id: 'activated-skill-1', kind: 'activated_skill', placement: 'prefix', messageIndex: 2 },
            ],
            output: {
              capturedAt: '2026-07-22T00:01:02Z',
              stopReason: 'stop',
              message: { role: 'assistant', content: [{ type: 'text', text: 'The release is available.' }] },
            },
          }],
        }],
      }
      if (options.diagnosticsTrace) {
        diagnosticTraceRequestCount += 1
        const query = new URL(request.url()).searchParams
        body = await options.diagnosticsTrace(
          diagnosticTraceRequestCount,
          query.get('sessionId') ?? '',
          query,
        )
      }
    }
    if (path === '/api/providers') {
      body = {
        providers: [
          {
            id: 'openai',
            name: 'OpenAI',
            configured: true,
            models: 1,
            officialBaseURL: 'https://api.openai.com/v1',
            effectiveBaseURL: 'https://api.openai.com/v1',
            activeConnectionId: 'official',
            connections: [
              {
                id: 'official',
                name: 'Official',
                baseURL: 'https://api.openai.com/v1',
                official: true,
                activeKeyId: 'default',
                keys: [{ id: 'default', name: 'Default', preview: 'sk-test' }],
              },
            ],
          },
        ],
        activeModel: {
          provider: 'openai',
          model: 'test-model',
          thinkingLevel: modelThinkingLevel,
        },
      }
    }
    if (path === '/api/workspaces') {
      body = options.sessionScope === 'project'
        ? [{ path: createdSession.workspacePath, name: createdSession.workspaceName }]
        : []
    }
    if (path === '/api/skills') {
      body = {
        user: options.skills?.filter((item) => item.source === 'user') ?? [],
        project: options.skills?.filter((item) => item.source === 'project') ?? [],
        diagnostics: [],
      }
    }
    if (path === '/api/usage' && options.usageReport) {
      body = options.usageReport
    }
    if (path === '/api/usage/events' && options.usageEventPages) {
      const rangeKey = new URL(request.url()).searchParams.get('since') ?? ''
      if (!usageEventRangeKeys.includes(rangeKey)) usageEventRangeKeys.push(rangeKey)
      const index = Math.min(usageEventRangeKeys.indexOf(rangeKey), options.usageEventPages.length - 1)
      if (index > 0 && options.usageEventDelayMs) {
        await new Promise((resolve) => setTimeout(resolve, options.usageEventDelayMs))
      }
      body = options.usageEventPages[index]
    }
    if (path === '/api/usage/events' && options.usageEventPagesByOffset) {
      const offset = Number(new URL(request.url()).searchParams.get('offset') ?? 0)
      body = options.usageEventPagesByOffset[offset] ?? options.usageEventPagesByOffset[0]
    }
    if (path === '/api/sessions') {
      body = sessionCreated
        ? [
            ...(branchSessionCreated ? [branchSession] : []),
            ...(workbenchSessionCreated ? [workbenchSession] : []),
            createdSession,
            ...(options.secondarySession ? [secondarySession] : []),
          ]
        : []
    }
    if (path === '/api/sessions/test-session/history') {
      body = {
        events: sessionHistoryEvents,
        tasks: options.backgroundTasks ?? [],
        queue: [],
        context: options.contextUsage ?? {},
        running: sessionHistoryRunning,
        eventSeq: options.historyEventSeq ?? 0,
      }
    }
    if (path === '/api/sessions/secondary-session/history') {
      body = {
        events: options.secondaryHistoryEvents ?? [],
        queue: [],
        context: {},
        running: false,
        eventSeq: 0,
      }
    }
    if (path === '/api/sessions/workbench-session/history') {
      body = {
        events: [],
        queue: [],
        context: {},
        running: false,
        eventSeq: 0,
      }
    }
    if (path === '/api/sessions/branch-session/history') {
      body = {
        events: options.historyEvents ?? [],
        queue: [],
        context: options.contextUsage ?? {},
        running: false,
        eventSeq: 0,
      }
    }
    if (path === '/api/sessions/test-session/prompt') {
      body = {}
      status = 202
    }
    if (path === '/api/sessions/secondary-session/prompt') {
      body = {}
      status = 202
    }
    if (path === '/api/sessions/workbench-session/prompt') {
      body = {}
      status = 202
    }
    if (
      /\/api\/sessions\/[^/]+\/(?:settings|permission-mode)$/.test(path) &&
      method === 'PATCH'
    ) {
      await new Promise((resolve) =>
        setTimeout(resolve, options.composerUpdateDelayMs ?? 0),
      )
    }
    const permissionModeMatch = path.match(
      /^\/api\/sessions\/([^/]+)\/permission-mode$/,
    )
    if (permissionModeMatch && method === 'PATCH') {
      const mode = (requestBody as { mode: string }).mode
      const target =
        permissionModeMatch[1] === secondarySession.id
          ? secondarySession
          : permissionModeMatch[1] === workbenchSession.id
            ? workbenchSession
            : createdSession
      target.permissionMode = mode
      body = target
    }
    await route.fulfill({
      status,
      contentType: 'application/json',
      body: JSON.stringify(body),
    })
  })

  await page.goto('/')
  await expect(page.locator('html')).toHaveClass(/desktop-macos/)
  await expect(page.getByTestId('conversation-header')).toBeVisible()
  if (options.existingSession) {
    const sessions = page.getByRole('navigation', {
      name: options.sessionScope === 'project' ? 'Or sessions' : 'Chats',
    })
    await sessions.getByRole('button', { name: createdSession.title, exact: true }).click()
    await expect.poll(() =>
      page.evaluate(
        () =>
          (window as Window & { __eventSources?: unknown[] }).__eventSources?.some(
            (source) =>
              Boolean(
                source &&
                  typeof source === 'object' &&
                  'url' in source &&
                  typeof source.url === 'string' &&
                  source.url.includes('/sessions/test-session/events'),
              ),
          ) ?? false,
      ),
    ).toBe(true)
  }
  return requests
}

test('sidebar collapse keeps the titlebar control stable and clears the divider', async ({
  page,
}) => {
  await openDesktopClient(page)
  const toggle = page.getByTestId('sidebar-panel-toggle')
  const sidebar = page.getByTestId('sidebar-viewport')
  const header = page.getByTestId('conversation-header')
  const title = page.getByTestId('conversation-title')

  await expect(toggle).toBeVisible()
  await expect(title).toHaveCSS('user-select', 'none')
  await expect.poll(() => header.evaluate((element) => element.getBoundingClientRect().height)).toBe(45)
  await expect.poll(() => sidebar.evaluate((element) => element.getBoundingClientRect().width)).toBeGreaterThan(200)

  const before = await toggle.boundingBox()
  expect(before).not.toBeNull()
  await expect.poll(() =>
    toggle.evaluate((element) => element.closest('.app-sidebar-header') !== null),
  ).toBe(true)

  await toggle.click()
  await page.waitForTimeout(60)
  const during = await toggle.boundingBox()
  expect(during).not.toBeNull()
  expect(during!.x).toBeCloseTo(before!.x, 1)
  expect(during!.y).toBeCloseTo(before!.y, 1)
  await expect.poll(() =>
    toggle.evaluate((element) => element.closest('[data-testid="conversation-header"]') !== null),
  ).toBe(true)

  await expect.poll(() => sidebar.evaluate((element) => element.getBoundingClientRect().width)).toBeLessThan(1)

  const after = await toggle.boundingBox()
  const titleBox = await title.boundingBox()
  expect(after).not.toBeNull()
  expect(titleBox).not.toBeNull()
  expect(after!.x).toBeCloseTo(before!.x, 1)
  expect(after!.y).toBeCloseTo(before!.y, 1)
  await expect.poll(() =>
    toggle.evaluate((element) => element.closest('[data-testid="conversation-header"]') !== null),
  ).toBe(true)
  expect(titleBox!.x).toBeGreaterThanOrEqual(after!.x + after!.width + 10)

  const borderColor = await header.evaluate(
    (element) => getComputedStyle(element).borderBottomColor,
  )
  expect(borderColor).toBe('rgba(0, 0, 0, 0)')

  await toggle.click()
  await expect.poll(() => sidebar.evaluate((element) => element.getBoundingClientRect().width)).toBeGreaterThan(200)
})

test('desktop headers expose native drag regions while controls remain interactive', async ({ page }) => {
  await openDesktopClient(page)
  const header = page.getByTestId('conversation-header')
  const sidebarControl = page.getByTestId('sidebar-panel-toggle')
  const workbenchToggle = page.getByTestId('workbench-panel-toggle')
  await expect(header).toHaveCSS('-webkit-app-region', 'drag')
  await expect(sidebarControl).toHaveCSS('-webkit-app-region', 'no-drag')
  await expect(workbenchToggle).toHaveCSS('-webkit-app-region', 'no-drag')
  await expect.poll(() =>
    sidebarControl.evaluate((element) => element.closest('.window-titlebar') !== null),
  ).toBe(true)
  await expect.poll(() =>
    workbenchToggle.evaluate((element) => element.closest('.window-titlebar') !== null),
  ).toBe(true)
  await workbenchToggle.click()
  await expect(workbenchToggle).toHaveAccessibleName('Hide workbench')
  await workbenchToggle.click()
  await expect(workbenchToggle).toHaveAccessibleName('Show workbench')
})

test('conversation diagnostics uses one session-scoped header entry', async ({ page }) => {
  const requests = await openDesktopClient(page, {
    existingSession: true,
    historyEvents: [
      {
        type: 'run_start',
        id: 'run-diagnostics',
        startedAt: '2026-07-22T00:00:00Z',
        durationMs: 1000,
      },
      {
        type: 'message_end',
        text: 'Response with diagnostics',
        finalResponse: true,
      },
    ],
  })

  const diagnosticsButton = page.getByTestId('conversation-diagnostics-button')
  await expect(diagnosticsButton).toBeVisible()
  await expect(diagnosticsButton).toHaveAccessibleName('View conversation usage and diagnostics')
  await expect(page.getByRole('button', { name: 'View diagnostics for this run' })).toHaveCount(0)

  await page.getByRole('button', { name: 'Open profile menu' }).click()
  const profileMenu = page.getByRole('menu')
  await expect(profileMenu).toBeVisible()
  await expect(profileMenu.getByRole('menuitem', { name: 'Run diagnostics' })).toHaveCount(0)
  await page.keyboard.press('Escape')
  await expect(profileMenu).toBeHidden()

  await diagnosticsButton.click()
  await expect(page.getByRole('heading', { name: 'Run diagnostics' })).toBeVisible()
  await expect(diagnosticsButton).toHaveAttribute('aria-pressed', 'true')
  await expect(diagnosticsButton).toHaveAccessibleName('Back to conversation')
  await expect(page.getByTestId('diagnostics-toolbar')).toHaveCount(0)
  await expect(page.getByRole('tab', { name: 'Overview' })).toHaveCount(0)
  await expect(page.getByRole('tab', { name: 'Trajectory' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Refresh diagnostics' })).toBeVisible()
  await expect(page.locator('textarea:visible')).toBeEnabled()
  await expect(page.getByText('Create a short release note')).toBeVisible()
  await expect(page.getByText('Check the release status')).toBeVisible()
  await expect(page.getByRole('combobox', { name: 'Select run' })).toHaveCount(0)
  const firstTurn = page.locator('[data-trajectory-turn-id="turn-diagnostics-1"]')
  const secondTurn = page.locator('[data-trajectory-turn-id="turn-diagnostics-2"]')
  const firstStep = page.locator('[data-trajectory-step-id="step-diagnostics-1"]')
  const secondStep = page.locator('[data-trajectory-step-id="step-diagnostics-2"]')
  await expect(firstTurn).toContainText('Turn 1')
  await expect(firstTurn).toContainText('2 steps')
  await expect(secondTurn).toContainText('Turn 2')
  await expect(secondTurn).toContainText('1 step')
  await expect(page.getByTestId('trajectory-run-header')).toHaveCount(0)
  await expect(firstStep).toContainText('Step 1')
  await expect(firstStep).toContainText('Request #1')
  await expect(firstStep).toContainText('test-model')
  await expect(firstStep).toContainText('TTFT 900 ms')
  await expect(firstStep).toContainText('Token usage 250')
  await expect(firstStep).toContainText('Total 2.00 s')
  await expect(firstStep).toContainText('Completed')
  await expect(secondStep).toContainText('Step 2')
  await expect(secondStep).toContainText('Request #2')
  await expect(secondStep).toContainText('TTFT 1.20 s')
  await expect(secondStep).toContainText('Token usage 170')
  const thirdStep = page.locator('[data-trajectory-step-id="step-diagnostics-3"]')
  await expect(thirdStep).toContainText('Token usage 120')
  await expect(thirdStep).not.toContainText('TTFT')
  const firstRequestBar = page.getByRole('button', { name: 'Assistant · Request #1', exact: true })
  const secondRequestBar = page.getByRole('button', { name: 'Assistant · Request #2', exact: true })
  await expect(firstRequestBar).toBeVisible()
  await expect(secondRequestBar).toBeVisible()
  await expect(firstRequestBar).toHaveAttribute('data-model-timing', 'true')
  await expect(firstRequestBar).toHaveAttribute(
    'title',
    'Assistant · Request #1\nTTFT 900 ms · Generation 1.10 s · Total 2.00 s',
  )
  await expect(firstRequestBar.locator('[data-timeline-segment="ttft"]')).toHaveAttribute('style', 'width: 45%;')
  await expect(firstRequestBar.locator('[data-timeline-segment="generation"]')).toHaveAttribute('style', 'width: 55%;')
  const markdownResponseRow = page.locator('[data-trajectory-item-id="request:request-1:response"]')
  await expect(markdownResponseRow).toContainText('Release note I will write the release note. Keep it concise')
  await expect(markdownResponseRow).not.toContainText('##')
  await expect(markdownResponseRow).not.toContainText('**')
  await expect(page.getByRole('button', { name: 'System · Initial system prompt', exact: true })).toBeVisible()
  await expect(page.getByText('Initial system prompt', { exact: true })).toHaveCount(1)
  await expect(page.getByRole('button', { name: 'Context · Runtime context', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Context · Available skills', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Context · Runtime context update', exact: true })).toBeVisible()
  await expect(page.getByText('Runtime context', { exact: true })).toHaveCount(1)
  await expect(page.getByText('Available skills', { exact: true })).toHaveCount(1)
  const executionTimeline = page.locator('section[aria-label="Execution timeline"]')
  await expect(executionTimeline.getByText('Input', { exact: true })).toBeVisible()
  await expect(executionTimeline.getByText('Context', { exact: true })).toHaveCount(0)
  const systemRow = page.getByRole('button', { name: /System Initial system prompt/ })
  await systemRow.click()
  const systemInspector = page.getByRole('complementary', { name: 'System · Initial system prompt' })
  await expect(systemInspector.getByText('You are a coding agent.', { exact: true })).toBeVisible()
  const initialTools = systemInspector.getByRole('tab', { name: 'Tools 1', exact: true })
  await expect(initialTools).toHaveAttribute('aria-selected', 'false')
  await initialTools.click()
  await expect(initialTools).toHaveAttribute('aria-selected', 'true')
  await expect(systemInspector.getByText('write', { exact: true })).toBeVisible()
  await expect(systemInspector.getByText('Write one file', { exact: true }).first()).toBeVisible()
  const runtimeContextRow = page.getByRole('button', { name: /Context Runtime context.*Current runtime context/ })
  await runtimeContextRow.click()
  const contextInspector = page.getByRole('complementary', { name: 'Context · Runtime context' })
  await expect(contextInspector.getByText('Current runtime context.', { exact: false })).toBeVisible()
  const activatedSkillRow = page.getByRole('button', { name: 'Context · Activated skill', exact: true })
  await activatedSkillRow.click()
  const activatedSkillInspector = page.getByRole('complementary', { name: 'Context · Activated skill' })
  await activatedSkillInspector.getByRole('tab', { name: 'Content' }).click()
  await expect(activatedSkillInspector.getByRole('heading', { name: 'Git Workflow', level: 1 })).toBeVisible()
  await expect(activatedSkillInspector.getByRole('heading', { name: 'Scope', level: 2 })).toBeVisible()
  await expect(activatedSkillInspector.getByRole('listitem').getByText('Confirm the working tree is clean.')).toBeVisible()
  await expect(activatedSkillInspector).not.toContainText('This Skill was activated earlier')
  await expect(activatedSkillInspector).not.toContainText('<loaded_skill')
  await activatedSkillInspector.getByRole('tab', { name: 'Raw' }).click()
  await expect(activatedSkillInspector).toContainText('<or-context kind=\\"activated_skill\\" name=\\"git-workflow\\">')
  const thinkingOnlyRow = page.getByRole('button', { name: /Assistant Thinking · The file was created successfully\./ })
  await expect(thinkingOnlyRow).toBeVisible()
  await expect(thinkingOnlyRow.getByText('Thinking ·', { exact: true })).toBeVisible()
  await expect(thinkingOnlyRow).not.toContainText('bash')
  await thinkingOnlyRow.click()
  const thinkingOnlyInspector = page.getByRole('complementary', { name: 'Assistant · Request #2' })
  const responseToolCalls = thinkingOnlyInspector.getByTestId('diagnostics-response-tool-calls')
  await expect(responseToolCalls).toContainText('Tool calls')
  await expect(responseToolCalls.getByText('bash', { exact: true })).toBeVisible()
  await expect(responseToolCalls).toContainText('{"command":"git status"}')
  await expect(responseToolCalls).toContainText('Success · 100 ms')
  await expect.poll(() => {
    const request = requests.find((candidate) => candidate.path === '/api/diagnostics/trace')
    return request ? new URL(request.url).searchParams.get('sessionId') : undefined
  }).toBe('test-session')
  await firstRequestBar.click()
  const responseInspector = page.getByRole('complementary', { name: 'Assistant · Request #1' })
  await expect(responseInspector.getByText('Turn 1 · Step 1 · Request #1', { exact: true })).toBeVisible()
  await expect(responseInspector.getByRole('tab', { name: 'System prompt' })).toHaveCount(0)
  await expect(responseInspector.getByRole('tab', { name: 'Tools 1' })).toHaveCount(0)
  await expect(responseInspector.getByText('Source', { exact: true })).toBeVisible()
  await expect(responseInspector.getByText('Run ID', { exact: true })).toBeVisible()
  await expect(responseInspector.getByText('run-diagnostics', { exact: true })).toBeVisible()
  await expect(responseInspector.getByText('Status', { exact: true })).toBeVisible()
  await expect(responseInspector.getByText('250 tok', { exact: true })).toBeVisible()
  await expect(responseInspector.getByText('250 tok', { exact: true })).toHaveCSS('font-weight', '400')
  await expect(responseInspector.getByText('180 tok', { exact: true })).toBeVisible()
  await expect(responseInspector.getByText('70 tok', { exact: true })).toBeVisible()
  await expect(responseInspector.getByRole('heading', { name: 'Release note', level: 2 })).toBeVisible()
  await expect(responseInspector.getByText('I will write the release note.')).toBeVisible()
  await expect(responseInspector.getByText('release note', { exact: true })).toHaveCSS('font-weight', '600')
  await expect(responseInspector.getByRole('listitem').getByText('Keep it concise')).toBeVisible()
  await responseInspector.getByRole('tab', { name: 'Content' }).click()
  await expect(responseInspector.getByRole('heading', { name: 'Release note', level: 2 })).toBeVisible()
  await expect(page.getByText('Created RELEASE.md')).toBeVisible()
  await page.getByRole('button', { name: /write.*Created RELEASE\.md/ }).click()
  await expect(page.getByRole('heading', { name: 'Arguments' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Result' })).toBeVisible()
  const argumentsToggle = page.getByRole('button', { name: 'Arguments', exact: true })
  const resultToggle = page.getByRole('button', { name: 'Result', exact: true })
  await expect(argumentsToggle).toHaveAttribute('aria-expanded', 'true')
  await argumentsToggle.click()
  await expect(argumentsToggle).toHaveAttribute('aria-expanded', 'false')
  await expect(resultToggle).toHaveAttribute('aria-expanded', 'true')
  await expect(page.getByText('Session timestamps', { exact: true })).toBeVisible()

  await page.setViewportSize({ width: 700, height: 820 })
  await expect(executionTimeline).toBeVisible()
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
})

test('conversation diagnostics hierarchy fits a narrow viewport', async ({ page }) => {
  await openDesktopClient(page, { existingSession: true })
  await page.getByTestId('conversation-diagnostics-button').click()
  await page.setViewportSize({ width: 390, height: 844 })

  const inspector = page.getByRole('complementary', { name: 'Assistant · Request #2' })
  await inspector.getByRole('button', { name: 'Close' }).click()
  const ledger = page.getByTestId('diagnostics-ledger-scroll')
  const firstStep = page.locator('[data-trajectory-step-id="step-diagnostics-1"]')
  await firstStep.scrollIntoViewIfNeeded()
  await expect(firstStep.getByText('Step 1', { exact: true })).toBeVisible()
  await expect(firstStep.getByText('Request #1', { exact: true })).toBeVisible()
  await expect(firstStep.getByText('test-model', { exact: true })).toBeHidden()
  await expect(firstStep.getByText('TTFT 900 ms', { exact: true })).toBeVisible()
  await expect(firstStep.getByText('Token usage 250', { exact: true })).toBeHidden()
  await expect(firstStep.getByText('Total 2.00 s', { exact: true })).toBeVisible()
  await expect(firstStep.getByText('Completed', { exact: true })).toBeVisible()

  const ledgerBox = await ledger.boundingBox()
  const stepBox = await firstStep.boundingBox()
  expect(ledgerBox).not.toBeNull()
  expect(stepBox).not.toBeNull()
  expect(stepBox!.x).toBeGreaterThanOrEqual(ledgerBox!.x)
  expect(stepBox!.x + stepBox!.width).toBeLessThanOrEqual(ledgerBox!.x + ledgerBox!.width + 1)
})

test('conversation diagnostics treats a missing new-session trace as empty', async ({ page }) => {
  await openDesktopClient(page, {
    existingSession: true,
    diagnosticsStatus: 404,
  })

  await page.getByTestId('conversation-diagnostics-button').click()
  await expect(page.getByRole('heading', { name: 'No runs recorded' })).toBeVisible()
  await expect(page.getByText('Could not load local diagnostics.')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Retry' })).toHaveCount(0)
})

test('conversation diagnostics inspector divider supports pointer and keyboard resizing', async ({ page }) => {
  await page.setViewportSize({ width: 1400, height: 900 })
  await openDesktopClient(page, { existingSession: true })
  await page.getByTestId('conversation-diagnostics-button').click()

  const handle = page.getByTestId('diagnostics-inspector-resize-handle')
  const inspector = page.getByRole('complementary', { name: /Assistant · Request #/ })
  await expect(handle).toBeVisible()
  await expect(handle).toHaveAccessibleName('Resize request details')
  await expect(handle).toHaveAttribute('aria-orientation', 'vertical')

  const [handleBefore, inspectorBefore] = await Promise.all([
    handle.boundingBox(),
    inspector.boundingBox(),
  ])
  expect(handleBefore).not.toBeNull()
  expect(inspectorBefore).not.toBeNull()

  const bodyStylesBefore = await page.evaluate(() => ({
    cursor: document.body.style.cursor,
    userSelect: document.body.style.userSelect,
  }))
  await page.mouse.move(
    handleBefore!.x + handleBefore!.width / 2,
    handleBefore!.y + handleBefore!.height / 2,
  )
  await page.mouse.down()
  await expect.poll(() => page.evaluate(() => ({
    cursor: document.body.style.cursor,
    userSelect: document.body.style.userSelect,
  }))).toEqual({ cursor: 'col-resize', userSelect: 'none' })
  await page.mouse.move(
    handleBefore!.x + handleBefore!.width / 2 - 64,
    handleBefore!.y + handleBefore!.height / 2,
    { steps: 8 },
  )
  await page.mouse.up()

  await expect.poll(async () => (await inspector.boundingBox())?.width)
    .toBeCloseTo(inspectorBefore!.width + 64, 0)
  await expect.poll(() => page.evaluate(() => ({
    cursor: document.body.style.cursor,
    userSelect: document.body.style.userSelect,
  }))).toEqual(bodyStylesBefore)

  const widthAfterPointer = (await inspector.boundingBox())!.width
  await handle.focus()
  await handle.press('ArrowRight')
  await expect.poll(async () => (await inspector.boundingBox())?.width)
    .toBeCloseTo(widthAfterPointer - 16, 0)

  const rememberedWidth = (await inspector.boundingBox())!.width
  await page.getByTestId('conversation-diagnostics-button').click()
  await page.getByTestId('conversation-diagnostics-button').click()
  await expect.poll(async () => (await inspector.boundingBox())?.width)
    .toBeCloseTo(rememberedWidth, 0)

  await page.setViewportSize({ width: 700, height: 820 })
  await expect(handle).toBeHidden()
})

test('conversation diagnostics loads earlier tasks without moving the visible record', async ({ page }) => {
  const task = (group: 'current' | 'older', index: number) => {
    const hour = group === 'current' ? 12 : 10
    const startedAt = `2026-07-22T${hour}:${String(index).padStart(2, '0')}:00Z`
    return {
      id: `run-${group}-${index}`,
      status: 'completed',
      prompt: `${group === 'current' ? 'Current' : 'Older'} task ${index}`,
      startedAt,
      updatedAt: startedAt,
      retries: 0,
      contextRecoveries: 0,
      rawEvents: [],
      requests: [{
        id: `request-${group}-${index}`,
        number: 1,
        status: 'completed',
        lifecycle: 'complete',
        startedAt,
        attempts: [],
        checkpoints: [],
        tools: [],
        snapshotState: 'available',
        rawEvents: [],
        input: { messages: [{ role: 'user', content: [{ type: 'text', text: `${group} input ${index}` }] }] },
        output: {
          capturedAt: startedAt,
          message: { role: 'assistant', content: [{ type: 'text', text: `${group} response ${index}` }] },
        },
      }],
    }
  }
  const requests = await openDesktopClient(page, {
    existingSession: true,
    diagnosticsTrace: (_requestNumber, sessionID, query) => {
      const older = query.get('before') === 'older-cursor'
      return {
        version: 1,
        generatedAt: '2026-07-22T13:00:00Z',
        sessionId: sessionID,
        selectedTaskId: older ? 'run-older-12' : 'run-current-12',
        tasks: Array.from({ length: 12 }, (_, index) => task(older ? 'older' : 'current', index + 1)),
        page: older
          ? { hasMore: false }
          : { hasMore: true, beforeCursor: 'older-cursor' },
      }
    },
  })

  await page.getByTestId('conversation-diagnostics-button').click()
  const loadEarlier = page.getByRole('button', { name: 'Load earlier user tasks' })
  const currentAnchor = page.locator('[data-trajectory-item-id="task:run-current-1:user"]')
  await expect(loadEarlier).toBeVisible()
  await expect(currentAnchor).toBeVisible()
  const beforeBox = await currentAnchor.boundingBox()
  expect(beforeBox).not.toBeNull()

  await loadEarlier.click()
  await expect(page.getByText('Older task 1', { exact: true })).toBeAttached()
  await expect(loadEarlier).toHaveCount(0)
  const afterBox = await currentAnchor.boundingBox()
  expect(afterBox).not.toBeNull()
  expect(Math.abs((afterBox?.y ?? 0) - (beforeBox?.y ?? 0))).toBeLessThanOrEqual(2)

  const traceRequests = requests.filter((request) => request.path === '/api/diagnostics/trace')
  expect(new URL(traceRequests[0]!.url).searchParams.get('limit')).toBe('12')
  const olderRequest = traceRequests.find((request) =>
    new URL(request.url).searchParams.get('before') === 'older-cursor')
  expect(olderRequest).toBeDefined()
})

test('conversation diagnostics virtualizes a thousand trajectory records', async ({ page }) => {
  const task = (group: 'current' | 'older', index: number) => {
    const startedAt = new Date(Date.UTC(
      2026,
      6,
      22,
      group === 'current' ? 12 : 10,
      0,
      index,
    )).toISOString()
    return {
      id: `run-large-${group}-${index}`,
      status: 'completed',
      prompt: `${group === 'current' ? 'Current' : 'Older'} large task ${index}`,
      startedAt,
      updatedAt: startedAt,
      retries: 0,
      contextRecoveries: 0,
      rawEvents: [],
      requests: [{
        id: `request-large-${group}-${index}`,
        number: index,
        status: 'completed',
        lifecycle: 'complete',
        startedAt,
        attempts: [],
        checkpoints: [],
        tools: [],
        snapshotState: 'available',
        rawEvents: [],
        input: { messages: [{ role: 'user', content: [{ type: 'text', text: `${group} input ${index}` }] }] },
        output: {
          capturedAt: startedAt,
          message: { role: 'assistant', content: [{ type: 'text', text: `${group} response ${index}` }] },
        },
      }],
    }
  }
  await openDesktopClient(page, {
    existingSession: true,
    diagnosticsTrace: (_requestNumber, sessionID, query) => {
      const older = query.get('before') === 'large-older-cursor'
      const count = older ? 12 : 500
      return {
        version: 1,
        generatedAt: '2026-07-22T13:00:00Z',
        sessionId: sessionID,
        selectedTaskId: older ? 'run-large-older-12' : 'run-large-current-500',
        tasks: Array.from({ length: count }, (_, index) => task(older ? 'older' : 'current', index + 1)),
        page: older
          ? { hasMore: false }
          : { hasMore: true, beforeCursor: 'large-older-cursor' },
      }
    },
  })

  await page.getByTestId('conversation-diagnostics-button').click()
  const ledger = page.getByTestId('diagnostics-ledger-scroll')
  const timeline = page.getByTestId('diagnostics-timeline-scroll')
  const currentAnchor = ledger.locator('[data-trajectory-item-id="task:run-large-current-1:user"]')
  await expect(ledger).toHaveAttribute('data-virtualized', 'true')
  await expect(timeline).toHaveAttribute('data-virtualized', 'true')
  await expect(page.getByText('1000 items', { exact: true })).toBeVisible()
  await expect(currentAnchor).toBeVisible()
  expect(await ledger.locator('[data-trajectory-item-id]').count()).toBeLessThanOrEqual(160)
  expect(await timeline.locator('[data-timeline-item-id]').count()).toBeLessThanOrEqual(80)

  const beforeBox = await currentAnchor.boundingBox()
  expect(beforeBox).not.toBeNull()
  await page.getByRole('button', { name: 'Load earlier user tasks' }).click()
  await expect(page.getByText('1024 items', { exact: true })).toBeVisible()
  await expect.poll(async () => {
    const afterBox = await currentAnchor.boundingBox()
    return Math.abs((afterBox?.y ?? 0) - (beforeBox?.y ?? 0))
  }).toBeLessThanOrEqual(2)

  const timelineTarget = timeline.locator('[data-timeline-item-id]').last()
  const targetID = await timelineTarget.getAttribute('data-timeline-item-id')
  expect(targetID).not.toBeNull()
  await timelineTarget.click()
  const timelineBox = await timeline.boundingBox()
  const targetBox = await timelineTarget.boundingBox()
  expect(timelineBox).not.toBeNull()
  expect(targetBox).not.toBeNull()
  expect((timelineBox?.x ?? 0) + (timelineBox?.width ?? 0) - ((targetBox?.x ?? 0) + (targetBox?.width ?? 0)))
    .toBeGreaterThanOrEqual(6)
  const targetRow = ledger.locator(`[data-trajectory-item-id="${targetID}"]`)
  await expect(targetRow).toBeVisible()
  await expect(targetRow).toHaveAttribute('aria-expanded', 'true')
  expect(await ledger.locator('[data-trajectory-item-id]').count()).toBeLessThanOrEqual(160)
})

test('conversation diagnostics preserves state independently for each session', async ({ page }) => {
  const traceForSession = (sessionID: string) => {
    const label = sessionID === 'secondary-session' ? 'Secondary' : 'Primary'
    const tasks = Array.from({ length: 14 }, (_, index) => {
      const number = index + 1
      const requestID = `${sessionID}-request-${number}`
      return {
        id: `${sessionID}-run-${number}`,
        status: 'completed',
        prompt: `${label} task ${number}`,
        startedAt: `2026-07-22T00:${String(number).padStart(2, '0')}:00Z`,
        updatedAt: `2026-07-22T00:${String(number).padStart(2, '0')}:02Z`,
        durationMs: 2000,
        totalTokens: 100 + number,
        retries: 0,
        contextRecoveries: 0,
        rawEvents: [],
        requests: [{
          id: requestID,
          number,
          status: 'completed',
          lifecycle: 'complete',
          startedAt: `2026-07-22T00:${String(number).padStart(2, '0')}:00Z`,
          completedAt: `2026-07-22T00:${String(number).padStart(2, '0')}:02Z`,
          durationMs: 2000,
          timeToFirstOutputMs: 300,
          model: 'test-model',
          inputTokens: 80,
          outputTokens: 20 + number,
          totalTokens: 100 + number,
          attempts: [],
          checkpoints: [],
          tools: [],
          snapshotState: 'available',
          rawEvents: [],
          input: {
            systemPrompt: `System prompt for ${label}`,
            messages: [{
              role: 'user',
              content: [{ type: 'text', text: `${label} task ${number}` }],
            }],
            tools: [],
          },
          output: {
            capturedAt: `2026-07-22T00:${String(number).padStart(2, '0')}:02Z`,
            stopReason: 'stop',
            message: {
              role: 'assistant',
              providerRequestId: requestID,
              content: [{ type: 'text', text: `${label} response ${number}` }],
            },
          },
        }],
      }
    })
    return {
      version: 1,
      generatedAt: '2026-07-22T00:15:00Z',
      sessionId: sessionID,
      selectedTaskId: tasks.at(-1)?.id ?? '',
      tasks,
    }
  }

  await openDesktopClient(page, {
    existingSession: true,
    secondarySession: true,
    diagnosticsTrace: (_requestNumber, sessionID) => traceForSession(sessionID),
  })

  const chats = page.getByRole('navigation', { name: 'Chats' })
  await page.getByTestId('conversation-diagnostics-button').click()

  const primarySearch = page.getByPlaceholder('Search trajectory')
  await primarySearch.fill('Primary')
  const primaryResponse = page.getByRole('button', { name: /Assistant Primary response 8/ })
  await primaryResponse.click()
  const primaryInspector = page.getByRole('complementary', { name: 'Assistant · Request #8' })
  await primaryInspector.getByRole('tab', { name: 'Content' }).click()

  const ledger = page.getByTestId('diagnostics-ledger-scroll')
  await ledger.evaluate((element) => {
    element.scrollTop = 240
    element.dispatchEvent(new Event('scroll'))
  })
  const primaryScrollTop = await ledger.evaluate((element) => element.scrollTop)
  expect(primaryScrollTop).toBeGreaterThan(0)

  await chats.getByRole('button', { name: 'Secondary task', exact: true }).click()
  await page.getByTestId('conversation-diagnostics-button').click()
  const secondarySearch = page.getByPlaceholder('Search trajectory')
  await expect(secondarySearch).toHaveValue('')
  await secondarySearch.fill('Secondary')
  await page.getByRole('button', { name: /Assistant Secondary response 4/ }).click()
  const secondaryInspector = page.getByRole('complementary', {
    name: 'Assistant · Request #4',
  })
  await secondaryInspector.getByRole('button', { name: 'Close' }).click()

  await chats.getByRole('button', { name: 'New session', exact: true }).click()
  await expect(page.getByPlaceholder('Search trajectory')).toHaveValue('Primary')
  await expect(primaryResponse).toHaveAttribute('aria-expanded', 'true')
  await expect(primaryInspector).toBeVisible()
  await expect(primaryInspector.getByRole('tab', { name: 'Content' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect.poll(() => ledger.evaluate((element) => element.scrollTop)).toBe(primaryScrollTop)

  await chats.getByRole('button', { name: 'Secondary task', exact: true }).click()
  await expect(page.getByPlaceholder('Search trajectory')).toHaveValue('Secondary')
  await expect(
    page.getByRole('complementary', { name: 'Assistant · Request #4' }),
  ).toHaveCount(0)
})

test('conversation diagnostics ignores an older trace response that finishes last', async ({ page }) => {
  const trace = (requestID: string, text: string, generatedAt: string) => ({
    version: 1,
    generatedAt,
    sessionId: 'test-session',
    selectedTaskId: 'run-race',
    page: { hasMore: false },
    tasks: [{
      id: 'run-race',
      status: 'running',
      prompt: 'Inspect response ordering',
      startedAt: '2026-07-22T00:03:00Z',
      updatedAt: '2026-07-22T00:03:01Z',
      retries: 0,
      contextRecoveries: 0,
      rawEvents: [],
      requests: [{
        id: requestID,
        number: 1,
        status: 'running',
        lifecycle: 'in-progress',
        startedAt: '2026-07-22T00:03:00Z',
        attempts: [],
        checkpoints: [],
        tools: [],
        snapshotState: 'available',
        rawEvents: [],
        output: {
          capturedAt: '2026-07-22T00:03:01Z',
          message: {
            role: 'assistant',
            providerRequestId: requestID,
            content: [{ type: 'text', text }],
          },
        },
      }],
    }],
  })
  const requests = await openDesktopClient(page, {
    existingSession: true,
    historyRunning: true,
    historyEvents: [
      { type: 'user_message', text: 'Inspect response ordering' },
      {
        type: 'run_start',
        id: 'run-race',
        runId: 'run-race',
        startedAt: '2026-07-22T00:03:00Z',
      },
    ],
    diagnosticsTrace: async (_requestNumber, _sessionID, query) => {
      if (query.has('runId')) {
        return trace('request-current', 'Current trace response', '2026-07-22T00:03:02Z')
      }
      await new Promise((resolve) => setTimeout(resolve, 600))
      return trace('request-stale', 'Stale trace response', '2026-07-22T00:03:01Z')
    },
  })

  await page.getByTestId('conversation-diagnostics-button').click()
  await expect.poll(() => requests.filter((request) =>
    request.path === '/api/diagnostics/trace').length).toBeGreaterThan(0)
  await emitSessionEvent(page, 'test-session', {
    type: 'tool_start',
    id: 'call-race',
    tool: 'read',
    args: { path: 'trace.go' },
    providerRequestId: 'request-current',
  })
  await expect(page.getByText('Current trace response', { exact: true })).toBeVisible()
  await page.waitForTimeout(700)
  await expect(page.getByText('Current trace response', { exact: true })).toBeVisible()
  await expect(page.getByText('Stale trace response', { exact: true })).toHaveCount(0)
})

test('conversation diagnostics streams a tool loop and replaces provisional metrics', async ({ page }) => {
  let traceFinalized = false
  const requests = await openDesktopClient(page, {
    existingSession: true,
    diagnosticsTrace: () => traceFinalized ? {
      version: 1,
      generatedAt: '2026-07-22T00:02:02Z',
      sessionId: 'test-session',
      selectedTaskId: 'run-live',
      tasks: [{
        id: 'run-live',
        status: 'completed',
        prompt: 'Inspect the live trace',
        startedAt: '2026-07-22T00:02:00Z',
        updatedAt: '2026-07-22T00:02:02Z',
        durationMs: 2000,
        totalTokens: 60,
        retries: 0,
        contextRecoveries: 0,
        rawEvents: [],
        requests: [{
          id: 'request-live-1',
          number: 1,
          status: 'completed',
          lifecycle: 'complete',
          startedAt: '2026-07-22T00:02:00Z',
          completedAt: '2026-07-22T00:02:01Z',
          durationMs: 1000,
          timeToFirstOutputMs: 150,
          model: 'test-model',
          totalTokens: 18,
          attempts: [],
          checkpoints: [],
          snapshotState: 'available',
          rawEvents: [],
          output: {
            capturedAt: '2026-07-22T00:02:01Z',
            stopReason: 'tool_use',
            message: {
              role: 'assistant',
              providerRequestId: 'request-live-1',
              content: [
                { type: 'thinking', thinking: 'Inspecting workspace' },
                {
                  type: 'toolCall',
                  toolCallId: 'call-live-1',
                  toolName: 'read',
                  arguments: { path: 'trace.go' },
                },
              ],
            },
          },
          tools: [{
            id: 'call-live-1',
            name: 'read',
            status: 'success',
            lifecycle: 'complete',
            startedAt: '2026-07-22T00:02:00.5Z',
            completedAt: '2026-07-22T00:02:01Z',
            durationMs: 500,
            executionDurationMs: 500,
            arguments: { path: 'trace.go' },
            result: {
              role: 'toolResult',
              toolCallId: 'call-live-1',
              toolName: 'read',
              content: [{ type: 'text', text: 'Loaded trace.go' }],
            },
            rawEvents: [],
          }],
        }, {
          id: 'request-live-2',
          number: 2,
          status: 'completed',
          lifecycle: 'complete',
          startedAt: '2026-07-22T00:02:01Z',
          completedAt: '2026-07-22T00:02:01.9Z',
          durationMs: 900,
          timeToFirstOutputMs: 120,
          model: 'test-model',
          inputTokens: 30,
          outputTokens: 12,
          totalTokens: 42,
          attempts: [],
          checkpoints: [],
          tools: [],
          snapshotState: 'available',
          rawEvents: [],
          output: {
            capturedAt: '2026-07-22T00:02:01.9Z',
            stopReason: 'stop',
            message: {
              role: 'assistant',
              providerRequestId: 'request-live-2',
              content: [
                { type: 'thinking', thinking: 'Preparing final response' },
                { type: 'text', text: 'Live trace complete' },
              ],
            },
          },
        }],
      }],
    } : {
      version: 1,
      generatedAt: '2026-07-22T00:02:00Z',
      sessionId: 'test-session',
      tasks: [],
    },
  })

  await page.getByTestId('conversation-diagnostics-button').click()
  const composer = page.getByRole('textbox', { name: 'Ask anything' })
  await composer.fill('Inspect the live trace')
  await page.getByRole('button', { name: 'Send prompt' }).click()
  await expect.poll(() =>
    requests.find((request) => request.path === '/api/sessions/test-session/prompt')?.body,
  ).toEqual({ text: 'Inspect the live trace', images: [] })

  await emitSessionEvent(page, 'test-session', {
    type: 'run_start',
    id: 'run-live',
    runId: 'run-live',
    startedAt: '2026-07-22T00:02:00Z',
  })
  await emitSessionEvent(page, 'test-session', {
    type: 'delta',
    kind: 'thinking',
    delta: 'Inspecting workspace',
    providerRequestId: 'request-live-1',
  })

  const ledger = page.locator('section[aria-label="Trajectory records"]')
  await expect(
    ledger.getByRole('button', { name: /Assistant Thinking · Inspecting workspace/ }),
  ).toBeVisible()

  await emitSessionEvent(page, 'test-session', {
    type: 'tool_start',
    id: 'call-live-1',
    tool: 'read',
    args: { path: 'trace.go' },
    providerRequestId: 'request-live-1',
  })
  await expect(ledger.getByText('read', { exact: true })).toBeVisible()
  await emitSessionEvent(page, 'test-session', {
    type: 'tool_end',
    id: 'call-live-1',
    tool: 'read',
    result: 'Loaded trace.go',
    outcome: { status: 'success' },
    providerRequestId: 'request-live-1',
  })
  await expect(ledger.getByText('Loaded trace.go', { exact: true })).toBeVisible()

  await emitSessionEvent(page, 'test-session', {
    type: 'delta',
    kind: 'thinking',
    delta: 'Preparing final response',
    providerRequestId: 'request-live-2',
  })
  await emitSessionEvent(page, 'test-session', {
    type: 'delta',
    kind: 'text',
    delta: 'Live trace complete',
    providerRequestId: 'request-live-2',
  })
  await expect(ledger.getByText('Live trace complete', { exact: true })).toBeVisible()

  traceFinalized = true
  await emitSessionEvent(page, 'test-session', {
    type: 'message_end',
    text: 'Live trace complete',
    finalResponse: true,
    completedAt: '2026-07-22T00:02:01.9Z',
    providerRequestId: 'request-live-2',
  })
  await emitSessionEvent(page, 'test-session', {
    type: 'done',
    runId: 'run-live',
    startedAt: '2026-07-22T00:02:00Z',
    durationMs: 2000,
  })

  const finalResponse = ledger.getByRole('button', { name: /Assistant.*Live trace complete/ })
  await expect(finalResponse).toBeVisible()
  await finalResponse.click()
  const inspector = page.getByRole('complementary', { name: 'Assistant · Request #2' })
  await expect(inspector.getByText('42 tok', { exact: true })).toBeVisible()
  await expect(inspector.getByText('120 ms', { exact: true })).toBeVisible()
  await expect(composer).toBeEnabled()

  const traceRequests = requests.filter((request) => request.path === '/api/diagnostics/trace')
  expect(new URL(traceRequests[0]!.url).searchParams.get('limit')).toBe('12')
  const fullPageRequests = traceRequests.filter((request) =>
    !new URL(request.url).searchParams.has('runId'))
  const runRequests = traceRequests.filter((request) =>
    new URL(request.url).searchParams.has('runId'))
  expect(fullPageRequests.length).toBeLessThanOrEqual(2)
  expect(runRequests.length).toBeGreaterThan(0)
  expect(runRequests.every((request) =>
    new URL(request.url).searchParams.get('runId') === 'run-live')).toBe(true)
})

test('dark theme uses the cool neutral canvas', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('or.theme', 'dark'))
  await openDesktopClient(page)

  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await expect(page.locator('body')).toHaveCSS('background-color', 'rgb(31, 33, 36)')
})

test('theme and language live in General settings without duplicate profile entries', async ({
  page,
}) => {
  await openDesktopClient(page)
  await page.getByRole('button', { name: 'Open profile menu' }).click()

  const profileMenu = page.getByRole('menu')
  await expect(profileMenu.getByRole('menuitem', { name: 'Usage' })).toHaveCount(0)
  await expect(profileMenu.getByText('Theme', { exact: true })).toHaveCount(0)
  await expect(profileMenu.getByText('Language', { exact: true })).toHaveCount(0)
  await profileMenu.getByRole('menuitem', { name: 'Settings' }).click()

  const theme = page.getByRole('button', { name: 'Theme', exact: true })
  const language = page.getByRole('button', { name: 'Language', exact: true })
  await expect(theme).toBeVisible()
  await expect(language).toBeVisible()

  await theme.click()
  await page.getByRole('menuitemradio', { name: 'Dark', exact: true }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await expect.poll(() => page.evaluate(() => localStorage.getItem('or.theme'))).toBe('dark')
})

test('settings keep an opaque native titlebar above scrolling content', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('or.theme', 'light'))
  await openDesktopClient(page)
  const appTitlebarHeight = await page
    .getByTestId('conversation-header')
    .evaluate((element) => element.getBoundingClientRect().height)
  const appSidebarBackground = await page.locator('.app-sidebar').evaluate(
    (element) => getComputedStyle(element).backgroundColor,
  )
  const appSidebarTitlebarBox = await page.locator('.app-sidebar-header').boundingBox()
  const appSidebarToggleBox = await page.getByTestId('sidebar-panel-toggle').boundingBox()
  await page.getByRole('button', { name: 'Open profile menu' }).click()
  await page.getByRole('menuitem', { name: 'Settings' }).click()

  const titlebar = page.getByTestId('settings-titlebar')
  const sidebarTitlebar = page.getByTestId('settings-sidebar-titlebar')
  const settingsSidebarToggle = page.getByTestId('sidebar-panel-toggle')
  await expect(titlebar).toHaveCSS('-webkit-app-region', 'drag')
  await expect(sidebarTitlebar).toHaveCSS('-webkit-app-region', 'drag')
  await expect(titlebar).toHaveCSS('background-color', 'rgb(255, 255, 255)')
  await expect(titlebar).toHaveCSS('border-bottom-width', '0px')
  const [titlebarBox, sidebarTitlebarBox, settingsSidebarToggleBox, backBox, headingBox] = await Promise.all([
    titlebar.boundingBox(),
    sidebarTitlebar.boundingBox(),
    settingsSidebarToggle.boundingBox(),
    page.getByRole('button', { name: 'Back to app' }).boundingBox(),
    page.getByRole('heading', { name: 'General', level: 1 }).boundingBox(),
  ])
  expect(appSidebarTitlebarBox).not.toBeNull()
  expect(appSidebarToggleBox).not.toBeNull()
  expect(titlebarBox).not.toBeNull()
  expect(sidebarTitlebarBox).not.toBeNull()
  expect(settingsSidebarToggleBox).not.toBeNull()
  expect(backBox).not.toBeNull()
  expect(headingBox).not.toBeNull()
  expect(titlebarBox!.height).toBe(appTitlebarHeight)
  expect(sidebarTitlebarBox!.height).toBe(appSidebarTitlebarBox!.height)
  expect(settingsSidebarToggleBox!.x).toBe(appSidebarToggleBox!.x)
  expect(settingsSidebarToggleBox!.y).toBe(appSidebarToggleBox!.y)
  expect(settingsSidebarToggleBox!.width).toBe(appSidebarToggleBox!.width)
  expect(settingsSidebarToggleBox!.height).toBe(appSidebarToggleBox!.height)
  await expect(page.locator('.settings-sidebar')).toHaveCSS(
    'background-color',
    appSidebarBackground,
  )
  await expect(page.locator('.settings-sidebar')).toHaveCSS('border-right-width', '1px')
  expect(backBox!.y).toBeGreaterThanOrEqual(sidebarTitlebarBox!.y + sidebarTitlebarBox!.height)
  expect(headingBox!.y).toBeGreaterThanOrEqual(titlebarBox!.y + titlebarBox!.height)
  await expect.poll(() =>
    titlebar.evaluate((element) => {
      const box = element.getBoundingClientRect()
      return document.elementFromPoint(box.right - 12, box.top + box.height / 2) === element
    }),
  ).toBe(true)
})

test('settings sidebar collapses without moving its titlebar control', async ({ page }) => {
  await openDesktopClient(page)
  await page.getByRole('button', { name: 'Open profile menu' }).click()
  await page.getByRole('menuitem', { name: 'Settings' }).click()

  const toggle = page.getByTestId('sidebar-panel-toggle')
  const sidebar = page.locator('.settings-sidebar')
  const content = page.getByTestId('settings-content')

  await expect(toggle).toBeVisible()
  await expect(toggle).toHaveAttribute('aria-expanded', 'true')
  await expect.poll(() => sidebar.evaluate((element) => element.getBoundingClientRect().width)).toBeGreaterThan(200)

  const [toggleBefore, contentBefore] = await Promise.all([
    toggle.boundingBox(),
    content.boundingBox(),
  ])
  expect(toggleBefore).not.toBeNull()
  expect(contentBefore).not.toBeNull()

  await toggle.click()
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  await expect.poll(() => sidebar.evaluate((element) => element.getBoundingClientRect().width)).toBeLessThan(1)

  const [toggleAfter, contentAfter] = await Promise.all([
    toggle.boundingBox(),
    content.boundingBox(),
  ])
  expect(toggleAfter).not.toBeNull()
  expect(contentAfter).not.toBeNull()
  expect(toggleAfter!.x).toBeCloseTo(toggleBefore!.x, 1)
  expect(toggleAfter!.y).toBeCloseTo(toggleBefore!.y, 1)
  expect(contentAfter!.width).toBeGreaterThan(contentBefore!.width + 200)

  await toggle.click()
  await expect(toggle).toHaveAttribute('aria-expanded', 'true')
  await expect.poll(() => sidebar.evaluate((element) => element.getBoundingClientRect().width)).toBeGreaterThan(200)
})

test('settings sidebar divider supports pointer and keyboard resizing', async ({ page }) => {
  await openDesktopClient(page)
  await page.getByRole('button', { name: 'Open profile menu' }).click()
  await page.getByRole('menuitem', { name: 'Settings' }).click()

  const sidebar = page.locator('.settings-sidebar')
  const handle = page.getByTestId('settings-sidebar-resize-handle')
  const divider = page.getByTestId('settings-sidebar-divider')
  await expect(handle).toBeVisible()
  await expect(handle).toHaveAttribute('aria-valuemin', '206')
  await expect(handle).toHaveAttribute('aria-valuemax', '338')
  await expect(handle).toHaveAttribute('aria-valuenow', '240')
  await expect(sidebar).toHaveCSS('border-right-width', '1px')

  const [sidebarBefore, handleBefore, dividerBefore] = await Promise.all([
    sidebar.boundingBox(),
    handle.boundingBox(),
    divider.boundingBox(),
  ])
  expect(sidebarBefore).not.toBeNull()
  expect(handleBefore).not.toBeNull()
  expect(dividerBefore).not.toBeNull()
  expect(sidebarBefore!.width).toBeCloseTo(240, 0)
  expect(Math.abs(
    dividerBefore!.x + dividerBefore!.width -
      (sidebarBefore!.x + sidebarBefore!.width),
  )).toBeLessThanOrEqual(1)

  await page.mouse.move(
    handleBefore!.x + handleBefore!.width - 2,
    handleBefore!.y + handleBefore!.height / 2,
  )
  await page.mouse.down()
  await page.mouse.move(
    handleBefore!.x + handleBefore!.width + 38,
    handleBefore!.y + handleBefore!.height / 2,
    { steps: 8 },
  )
  await page.mouse.up()

  await expect(handle).toHaveAttribute('aria-valuenow', '280')
  await expect.poll(async () => (await sidebar.boundingBox())?.width).toBeCloseTo(280, 0)

  await handle.focus()
  await handle.press('ArrowRight')
  await expect(handle).toHaveAttribute('aria-valuenow', '288')
  await expect.poll(async () => (await sidebar.boundingBox())?.width).toBeCloseTo(288, 0)

  const bodyStylesBeforeLostCapture = await page.evaluate(() => ({
    cursor: document.body.style.cursor,
    userSelect: document.body.style.userSelect,
  }))
  const handleAfterKeyboard = await handle.boundingBox()
  expect(handleAfterKeyboard).not.toBeNull()
  await handle.evaluate((element) => {
    element.addEventListener('pointerdown', (event) => {
      element.setAttribute('data-test-pointer-id', String(event.pointerId))
    }, { once: true })
  })
  await page.mouse.move(
    handleAfterKeyboard!.x + handleAfterKeyboard!.width - 2,
    handleAfterKeyboard!.y + handleAfterKeyboard!.height / 2,
  )
  await page.mouse.down()
  await expect.poll(() => page.evaluate(() => ({
    cursor: document.body.style.cursor,
    userSelect: document.body.style.userSelect,
  }))).toEqual({ cursor: 'col-resize', userSelect: 'none' })

  const pointerID = Number(await handle.getAttribute('data-test-pointer-id'))
  expect(Number.isInteger(pointerID)).toBe(true)
  await handle.evaluate((element, activePointerID) => {
    element.dispatchEvent(new PointerEvent('lostpointercapture', {
      bubbles: true,
      pointerId: activePointerID,
      pointerType: 'mouse',
    }))
  }, pointerID)
  await expect.poll(() => page.evaluate(() => ({
    cursor: document.body.style.cursor,
    userSelect: document.body.style.userSelect,
  }))).toEqual(bodyStylesBeforeLostCapture)
  await page.mouse.up()
})

test('desktop external links open in the system browser without leaving Or', async ({ page }) => {
  await openDesktopClient(page)
  const appURL = page.url()

  await page.evaluate(() => {
    const anchor = document.createElement('a')
    anchor.href = 'http://localhost:3000'
    anchor.textContent = 'Open preview'
    document.body.append(anchor)
  })
  await page.getByRole('link', { name: 'Open preview' }).click()

  await expect.poll(() =>
    page.evaluate(() => (window as Window & { __openedURL?: string }).__openedURL),
  ).toBe('http://localhost:3000/')
  expect(page.url()).toBe(appURL)
  await expect(page.getByTestId('conversation-header')).toBeVisible()
})

test('Or API startup retries recover the Composer automatically', async ({ page }) => {
  const requests = await openDesktopClient(page, { healthFailures: 2 })
  const input = page.getByTestId('composer').locator('textarea')

  await expect(input).toBeDisabled()
  await expect(input).toBeEnabled()
  await expect(input).toHaveAttribute('placeholder', 'Ask anything')
  await expect.poll(
    () => requests.filter((request) => request.path === '/api/health').length,
  ).toBeGreaterThanOrEqual(3)
})

test('approval replaces the Composer with a complete command review', async ({ page }) => {
  const command = [
    "python3 - <<'EOF'",
    'def ratio(a, b):',
    '    hi, lo = max(a, b), min(a, b)',
    '    return (hi + 0.05) / (lo + 0.05)',
    '',
    'colors = ["#1a1a18", "#6f6c66", "#a3a098", "#3159a8"]',
    'for color in colors:',
    '    print(color)',
    'EOF',
  ].join('\n')
  const requests = await openDesktopClient(page, {
    existingSession: true,
    historyRunning: true,
    historyEvents: [
      {
        type: 'user_message',
        id: 'approval-user',
        text: 'Check the interface contrast',
        images: [],
      },
      {
        type: 'run_start',
        id: 'approval-run',
        startedAt: '2026-07-30T12:00:00Z',
      },
      {
        type: 'tool_start',
        id: 'approval-tool',
        tool: 'bash',
        args: {
          command,
          description: 'Inspect interface contrast',
        },
      },
      {
        type: 'approval_request',
        id: 'approval-layout',
        summary: "bash: python3 - <<'EOF' …",
        reason: 'shell commands require approval',
        command,
        commandSegments: 17,
      },
    ],
  })

  const transcript = page.getByTestId('conversation-transcript')
  const composer = page.getByTestId('composer')
  const approval = composer.getByTestId('approval')
  const deny = approval.getByRole('button', { name: 'Deny' })
  const allow = approval.getByRole('button', { name: 'Allow once' })
  const input = composer.locator('textarea')
  await expect(approval).toBeVisible()
  await expect(composer).toBeVisible()
  await expect(transcript.getByTestId('approval')).toHaveCount(0)
  await expect(input).toBeHidden()
  await expect(input).toBeDisabled()
  await expect(input).toHaveAttribute('placeholder', 'Resolve the approval above to continue…')
  await expect(approval).toHaveCSS('border-radius', '24px')
  await expect(approval.getByText('Terminal', { exact: true })).toBeVisible()
  await expect(approval.getByRole('heading', { name: 'Allow this command to run?' })).toBeVisible()
  await expect(approval.getByText('17 commands', { exact: true })).toBeVisible()
  await expect(approval.locator('pre')).toContainText(command)
  await expect
    .poll(() =>
      approval.locator('pre').evaluate((element) => parseFloat(getComputedStyle(element).maxHeight)),
    )
    .toBeLessThanOrEqual(144)
  await expect
    .poll(() => deny.evaluate((element) => element.getBoundingClientRect().height))
    .toBeGreaterThanOrEqual(32)
  await expect
    .poll(() => allow.evaluate((element) => element.getBoundingClientRect().height))
    .toBeGreaterThanOrEqual(32)
  const desktopApprovalBox = await approval.boundingBox()
  const transcriptBox = await transcript.boundingBox()
  expect(desktopApprovalBox).not.toBeNull()
  expect(transcriptBox).not.toBeNull()
  expect(desktopApprovalBox!.y).toBeGreaterThanOrEqual(
    transcriptBox!.y + transcriptBox!.height,
  )

  await page.setViewportSize({ width: 500, height: 800 })
  const denyBox = await deny.boundingBox()
  const allowBox = await allow.boundingBox()
  const approvalBox = await approval.boundingBox()
  expect(denyBox).not.toBeNull()
  expect(allowBox).not.toBeNull()
  expect(approvalBox).not.toBeNull()
  expect(denyBox!.y).toBeCloseTo(allowBox!.y, 1)
  expect(allowBox!.x + allowBox!.width).toBeLessThanOrEqual(
    approvalBox!.x + approvalBox!.width,
  )
  expect(denyBox!.width + allowBox!.width).toBeLessThan(approvalBox!.width * 0.6)

  await allow.click()
  await expect(approval).toHaveCount(0)
  await expect(input).toBeVisible()
  await expect(input).toBeEnabled()
  await expect(input).toHaveAttribute('placeholder', 'Guide the current run…')
  await expect.poll(
    () =>
      requests.find(
        (request) =>
          request.method === 'POST' &&
          request.path === '/api/sessions/test-session/approvals/approval-layout',
      )?.body,
  ).toEqual({ choice: 'allow_once' })
})

test('right-panel approval replaces its Composer without duplicating in the transcript', async ({ page }) => {
  const command = 'ls -la coding/client/src/components'
  const requests = await openDesktopClient(page, {
    existingSession: true,
    secondarySession: true,
    secondaryHistoryEvents: [
      {
        type: 'user_message',
        id: 'secondary-approval-user',
        text: 'Inspect component files',
        images: [],
      },
      {
        type: 'run_start',
        id: 'secondary-approval-run',
        startedAt: '2026-07-30T12:00:00Z',
      },
      {
        type: 'tool_start',
        id: 'secondary-approval-tool',
        tool: 'bash',
        args: { command, description: 'List component files' },
      },
      {
        type: 'approval_request',
        id: 'secondary-approval',
        summary: `bash: ${command}`,
        reason: 'shell commands require approval',
        command,
        commandSegments: 1,
      },
    ],
  })

  await page.getByRole('button', { name: 'Actions for Secondary task' }).click()
  await page.getByRole('menuitem', { name: 'Open in right panel' }).click()

  const conversation = page
    .getByTestId('workbench-panel')
    .getByTestId('workbench-conversation')
  const transcript = conversation.getByTestId('workbench-conversation-transcript')
  const composer = conversation.getByTestId('composer')
  const approval = composer.getByTestId('approval')
  await expect(approval).toBeVisible()
  await expect(approval.locator('pre')).toHaveText(command)
  await expect(transcript.getByTestId('approval')).toHaveCount(0)
  await expect(composer).toBeVisible()
  await expect(composer.locator('textarea')).toBeHidden()
  await approval.getByRole('button', { name: 'Deny' }).click()
  await expect(approval).toHaveCount(0)
  await expect(composer.locator('textarea')).toBeVisible()
  await expect.poll(
    () =>
      requests.find(
        (request) =>
          request.method === 'POST' &&
          request.path === '/api/sessions/secondary-session/approvals/secondary-approval',
      )?.body,
  ).toEqual({ choice: 'deny' })
})

test('guided questions replace the Composer with a clear selected state and progress', async ({ page }) => {
  const questions = [
    {
      header: 'Checks',
      question: 'Which checks should run?',
      multiSelect: true,
      options: [
        { label: 'Lint', description: 'Catch static analysis issues' },
        { label: 'UI tests', description: 'Exercise the desktop workflow' },
      ],
    },
    {
      header: 'Location',
      question: 'Where should the prompt appear?',
      options: [
        { label: 'Composer', description: 'Replace the input temporarily' },
        { label: 'Transcript', description: 'Place it beside the messages' },
      ],
    },
    {
      header: 'Confirm',
      question: 'Apply this direction?',
      options: [
        { label: 'Apply', description: 'Continue with the selected direction' },
        { label: 'Revisit', description: 'Return to the previous choices' },
      ],
    },
  ]
  const requests = await openDesktopClient(page, {
    existingSession: true,
    historyEvents: [
      {
        type: 'user_message',
        id: 'question-user',
        text: 'Polish the guided question UI',
        images: [],
      },
      {
        type: 'run_start',
        id: 'question-run',
        startedAt: '2026-07-30T12:00:00Z',
      },
      {
        type: 'question_request',
        id: 'question-layout',
        questions,
      },
    ],
  })

  const composer = page.getByTestId('composer')
  const question = composer.getByTestId('question')
  const input = composer.locator('textarea')
  await expect(question).toBeVisible()
  await expect(question).toHaveCSS('border-radius', '24px')
  await expect(input).toBeHidden()
  await expect(question.getByText('1 / 3', { exact: true })).toBeVisible()

  const lint = question.getByRole('button', { name: /Lint/ })
  await lint.click()
  await expect(lint).toHaveAttribute('aria-pressed', 'true')
  await question.getByRole('button', { name: 'Next question' }).click()
  await expect(question.getByText('2 / 3', { exact: true })).toBeVisible()

  await question.getByRole('button', { name: /Composer/ }).click()
  await expect(question.getByText('3 / 3', { exact: true })).toBeVisible()
  await question.getByRole('button', { name: /Apply/ }).click()
  await question.getByRole('button', { name: 'Send answer' }).click()

  await expect(question).toHaveCount(0)
  await expect(input).toBeVisible()
  await expect.poll(
    () =>
      requests.find(
        (request) =>
          request.method === 'POST' &&
          request.path === '/api/sessions/test-session/questions/question-layout',
      )?.body,
  ).toEqual({
    answers: [
      { question: questions[0].question, values: ['Lint'] },
      { question: questions[1].question, values: ['Composer'] },
      { question: questions[2].question, values: ['Apply'] },
    ],
  })
})

test('workbench opens before a preview and launches Browser without hiding Chat', async ({
  page,
}) => {
  await page.route('http://127.0.0.1:4310/**', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'text/html',
      body: '<!doctype html><title>Preview fixture</title><main>Local preview ready</main>',
    })
  })
  const requests = await openDesktopClient(page, { existingSession: true })

  const workbenchToggle = page.getByTestId('workbench-panel-toggle')
  await expect(workbenchToggle).toBeVisible()
  await expect(workbenchToggle).toHaveAccessibleName('Show workbench')
  const togglePosition = await workbenchToggle.boundingBox()
  expect(togglePosition).not.toBeNull()
  await workbenchToggle.click()
  await page.waitForTimeout(60)
  const toggleDuringOpen = await workbenchToggle.boundingBox()
  expect(toggleDuringOpen).not.toBeNull()
  expect(toggleDuringOpen!.x).toBeCloseTo(togglePosition!.x, 1)
  expect(toggleDuringOpen!.y).toBeCloseTo(togglePosition!.y, 1)

  const workbench = page.getByTestId('workbench-panel')
  await expect(workbench).toBeVisible()
  await expect(workbenchToggle).toHaveAccessibleName('Hide workbench')
  const conversationHeaderColor = await page
    .getByTestId('conversation-header')
    .evaluate((element) => getComputedStyle(element).backgroundColor)
  await expect(workbench.getByTestId('workbench-titlebar')).toHaveCSS(
    'background-color',
    conversationHeaderColor,
  )
  await expect(workbench.getByTestId('workbench-titlebar')).toHaveCSS(
    'border-bottom-width',
    '0px',
  )
  const settledWorkbench = await workbench.boundingBox()
  expect(settledWorkbench).not.toBeNull()
  await expect(workbench.getByTestId('workbench-empty')).toContainText('No open views')
  await expect(workbench.getByRole('button', { name: 'Browser' })).toHaveCount(0)
  await expect(workbench.getByRole('button', { name: 'Chat' })).toHaveCount(0)

  await workbench.getByRole('button', { name: 'Add view' }).click()
  await page.getByRole('menu').getByRole('menuitem', { name: 'Browser' }).click()
  await expect(page.getByTestId('browser-view')).toBeVisible()
  await expect(page.getByText('New tab', { exact: true })).toBeVisible()
  await expect(page.getByTestId('browser-titlebar')).toHaveCSS('user-select', 'none')
  await expect(page.getByTestId('browser-titlebar')).toHaveCSS(
    'background-color',
    conversationHeaderColor,
  )
  await expect(page.getByTestId('browser-titlebar')).toHaveCSS('border-bottom-width', '0px')
  const address = page.getByRole('textbox', { name: 'Address' })
  await address.fill('127.0.0.1:4310')
  await address.press('Enter')
  await expect.poll(async () => (await browserRuntimeView(page, 'tab-1'))?.url).toBe(
    'http://127.0.0.1:4310/',
  )
  const localhostView = await browserRuntimeView(page, 'tab-1')
  expect(localhostView).toMatchObject({
    visible: true,
    loadCalls: ['http://127.0.0.1:4310/'],
  })
  expect(localhostView?.bounds.width).toBeGreaterThan(0)
  expect(localhostView?.bounds.height).toBeGreaterThan(0)
  await expect.poll(
    () => requests.filter((request) => request.path === '/api/preview/check').length,
  ).toBe(1)
  await expect(page.getByRole('main')).toBeVisible()

  const originalTab = page.getByRole('tab', { name: '127.0.0.1:4310' })
  await expect(originalTab).toHaveAttribute('aria-selected', 'true')
  await page.getByRole('button', { name: 'Add view' }).click()
  const addViewMenu = page.getByRole('menu')
  await expect.poll(() =>
    addViewMenu.evaluate((element) => element.getBoundingClientRect().width),
  ).toBe(232.5)
  await expect.poll(() =>
    addViewMenu
      .getByRole('menuitem', { name: 'Browser' })
      .evaluate((element) => element.getBoundingClientRect().height),
  ).toBe(30)
  await expect(addViewMenu.getByRole('menuitem')).toHaveCount(3)
  await expect(addViewMenu.getByRole('menuitem', { name: 'Chat' })).toBeEnabled()
  await addViewMenu.getByRole('menuitem', { name: 'Browser' }).click()
  await expect(page.getByRole('tab')).toHaveCount(2)
  await expect(page.getByRole('tab', { name: 'New tab' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  expect(await browserRuntimeView(page, 'tab-2')).toMatchObject({
    status: 'idle',
    loadCalls: [],
  })
  await address.fill('https://example.com')
  await address.press('Enter')
  await expect.poll(async () => (await browserRuntimeView(page, 'tab-2'))?.url).toBe(
    'https://example.com/',
  )
  expect(await browserRuntimeView(page, 'tab-2')).toMatchObject({
    visible: true,
  })
  expect(
    await page.evaluate(() => (window as Window & { __openedURL?: string }).__openedURL),
  ).toBeUndefined()
  await expect(page.getByRole('tab', { name: 'example.com' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  expect(requests.filter((request) => request.path === '/api/preview/check')).toHaveLength(1)
  await expect(page.getByTestId('browser-surface')).toHaveAttribute(
    'data-status',
    'ready',
  )

  await guestNavigatesItself(page, 'tab-2', 'https://example.com/search', 'Example')
  await expect(address).toHaveValue('https://example.com/search')
  await expect(page.getByRole('tab', { name: 'Example' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  const back = page.getByRole('button', { name: 'Back' })
  await expect(back).toBeEnabled()
  await back.click()
  await expect.poll(async () => (await browserRuntimeView(page, 'tab-2'))?.url).toBe(
    'https://example.com/',
  )

  await originalTab.click()
  await expect(originalTab).toHaveAttribute('aria-selected', 'true')
  await expect.poll(async () => (await browserRuntimeView(page, 'tab-1'))?.visible).toBe(true)
  await expect.poll(async () => (await browserRuntimeView(page, 'tab-2'))?.visible).toBe(false)
  await page.getByRole('button', { name: 'Close tab: 127.0.0.1:4310' }).click()
  await expect(page.getByRole('tab')).toHaveCount(1)
  await expect(page.getByRole('tab', { name: 'Example' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await page.getByRole('button', { name: 'Open in system browser' }).click()
  await expect.poll(() =>
    page.evaluate(() => (window as Window & { __openedURL?: string }).__openedURL),
  ).toBe('https://example.com/')
  await page.getByRole('button', { name: 'Close tab: Example' }).click()
  await expect(page.getByTestId('browser-view')).toHaveCount(0)
  await expect(workbench.getByTestId('workbench-empty')).toContainText('No open views')
  await workbenchToggle.click()
  await page.waitForTimeout(60)
  const toggleDuringClose = await workbenchToggle.boundingBox()
  const workbenchDuringClose = await workbench.boundingBox()
  expect(toggleDuringClose).not.toBeNull()
  expect(workbenchDuringClose).not.toBeNull()
  expect(toggleDuringClose!.x).toBeCloseTo(togglePosition!.x, 1)
  expect(toggleDuringClose!.y).toBeCloseTo(togglePosition!.y, 1)
  expect(workbenchDuringClose!.width).toBeCloseTo(settledWorkbench!.width, 1)
  await expect(workbench).toBeHidden()
  await expect(workbenchToggle).toBeVisible()
  await expect(workbenchToggle).toHaveAccessibleName('Show workbench')
  const toggleAfterClose = await workbenchToggle.boundingBox()
  expect(toggleAfterClose).not.toBeNull()
  expect(toggleAfterClose!.x).toBeCloseTo(togglePosition!.x, 1)
  expect(toggleAfterClose!.y).toBeCloseTo(togglePosition!.y, 1)
})

test('background tasks open in one persistent workbench tab with responsive logs', async ({
  page,
}) => {
  const requests = await openDesktopClient(page, {
    existingSession: true,
    backgroundTasks: [
      {
        id: 'task-dev-server',
        command: 'bun run dev',
        description: 'Development server',
        status: 'running',
        outputPath: '/tmp/task-dev-server.log',
        startedAt: '2026-07-22T00:00:00Z',
      },
      {
        id: 'task-tests',
        command: 'bun test',
        description: 'Unit tests',
        status: 'succeeded',
        outputPath: '/tmp/task-tests.log',
        exitCode: 0,
        startedAt: '2026-07-21T23:59:00Z',
        completedAt: '2026-07-21T23:59:03Z',
      },
    ],
  })

  await page.getByTestId('conversation-actions-trigger').click()
  const taskSubmenu = page.getByRole('menuitem', { name: /Background tasks/ })
  await taskSubmenu.focus()
  await taskSubmenu.press('ArrowRight')
  await page.getByRole('menuitem', { name: /Development server/ }).press('Enter')

  const workbench = page.getByTestId('workbench-panel')
  const taskView = workbench.getByTestId('background-tasks-view')
  await expect(workbench).toBeVisible()
  await expect(workbench.getByRole('tab', { name: /Background tasks/ })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect(taskView.getByRole('heading', { name: 'Development server' })).toBeVisible()
  await expect(taskView.getByRole('button', { name: /Unit tests/ })).toBeVisible()
  await expect(taskView.getByText('server listening on http://127.0.0.1:8080')).toBeVisible()

  const list = taskView.locator('.background-tasks-list')
  const output = taskView.locator('pre')
  const [narrowListBox, narrowOutputBox] = await Promise.all([
    list.boundingBox(),
    output.boundingBox(),
  ])
  expect(narrowListBox).not.toBeNull()
  expect(narrowOutputBox).not.toBeNull()
  expect(narrowOutputBox!.y).toBeGreaterThan(narrowListBox!.y)

  await workbench.getByRole('button', { name: 'Maximize workbench' }).click()
  const [wideListBox, wideOutputBox] = await Promise.all([
    list.boundingBox(),
    output.boundingBox(),
  ])
  expect(wideListBox).not.toBeNull()
  expect(wideOutputBox).not.toBeNull()
  expect(wideOutputBox!.x).toBeGreaterThan(wideListBox!.x)

  await workbench.getByRole('button', { name: 'Close background tasks' }).click()
  await expect(taskView).toHaveCount(0)
  expect(
    requests.some(
      (request) =>
        request.method === 'POST' &&
        request.path === '/api/sessions/test-session/tasks/task-dev-server/stop',
    ),
  ).toBe(false)

  await workbench.getByRole('button', { name: 'Add view' }).click()
  await page.getByRole('menuitem', { name: 'Background tasks' }).click()
  await expect(workbench.getByTestId('background-tasks-view')).toBeVisible()
  await expect.poll(() =>
    requests.filter(
      (request) =>
        request.method === 'GET' &&
        request.path === '/api/sessions/test-session/tasks/task-dev-server/output',
    ).length,
  ).toBeGreaterThan(0)
})

test('Add view creates a chat directly in the right panel', async ({ page }) => {
  const requests = await openDesktopClient(page, {
    existingSession: true,
    historyEvents: [
      {
        type: 'user_message',
        id: 'main-user',
        text: 'Keep this conversation on the left',
        images: [],
      },
      {
        type: 'message_end',
        text: 'Main answer remains visible',
        finalResponse: true,
        modelName: 'Test model',
      },
    ],
  })

  const mainConversation = page.getByTestId('conversation-pane')
  await expect(mainConversation.getByText('Main answer remains visible')).toBeVisible()

  await page.getByTestId('workbench-panel-toggle').click()
  const workbench = page.getByTestId('workbench-panel')
  await workbench.getByRole('button', { name: 'Add view' }).click()
  const chatItem = page.getByRole('menuitem', { name: 'Chat' })
  await expect(chatItem).toBeEnabled()
  await chatItem.click()

  await expect.poll(() =>
    requests.find(
      (request) => request.path === '/api/sessions' && request.method === 'POST',
    )?.body,
  ).toEqual({
    scope: 'chat',
    provider: 'openai',
    model: 'test-model',
    thinkingLevel: 'medium',
    permissionMode: 'ask',
  })
  await expect(workbench.getByRole('tab', { name: 'New session' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect(mainConversation.getByText('Main answer remains visible')).toBeVisible()

  const sideConversation = workbench.getByTestId('workbench-conversation')
  const input = sideConversation.getByPlaceholder('Ask anything')
  await expect(input).toBeEnabled()
  await input.fill('Start on the right')
  await input.press('Enter')
  await expect(sideConversation.getByText('Start on the right')).toBeVisible()
  await expect(mainConversation.getByText('Start on the right')).toHaveCount(0)
  await expect.poll(() =>
    requests.find(
      (request) => request.path === '/api/sessions/workbench-session/prompt',
    )?.body,
  ).toEqual({ text: 'Start on the right', images: [] })

  await workbench.getByRole('button', { name: 'Close conversation view' }).click()
  expect(
    requests.filter(
      (request) =>
        request.method === 'DELETE' && request.path === '/api/sessions/workbench-session',
    ),
  ).toHaveLength(0)
})

test('an existing chat opens and remains interactive in the right panel', async ({ page }) => {
  const requests = await openDesktopClient(page, {
    existingSession: true,
    secondarySession: true,
    secondaryHistoryEvents: [
      {
        type: 'user_message',
        id: 'secondary-user',
        text: 'Secondary history',
        images: [],
      },
      {
        type: 'run_start',
        id: 'secondary-run',
        startedAt: '2026-07-22T00:00:00Z',
        durationMs: 1200,
      },
      {
        type: 'message_end',
        text: 'Secondary answer',
        finalResponse: true,
        modelName: 'Test model',
      },
      { type: 'done', durationMs: 1200 },
    ],
  })

  await page.getByRole('button', { name: 'Actions for Secondary task' }).click()
  await page.getByRole('menuitem', { name: 'Open in right panel' }).click()

  const mainConversation = page.getByTestId('conversation-pane')
  const workbench = page.getByTestId('workbench-panel')
  const sideConversation = workbench.getByTestId('workbench-conversation')
  await expect(workbench.getByRole('tab', { name: 'Secondary task' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect(sideConversation.getByText('Secondary history')).toBeVisible()
  await expect(sideConversation.getByText('Secondary answer')).toBeVisible()
  await expect(mainConversation.getByText('Secondary history')).toHaveCount(0)
  await expect
    .poll(() =>
      requests.filter(
        (request) => request.path === '/api/sessions/secondary-session/history',
      ).length,
    )
    .toBe(1)

  const input = sideConversation.getByPlaceholder('Ask anything')
  await input.fill('Continue on the right')
  await input.press('Enter')
  await expect(sideConversation.getByText('Continue on the right')).toBeVisible()
  await expect(mainConversation.getByText('Continue on the right')).toHaveCount(0)
  await expect
    .poll(() =>
      requests.filter(
        (request) => request.path === '/api/sessions/secondary-session/prompt',
      ).length,
    )
    .toBe(1)

  await page.evaluate(() => {
    const emit = (
      window as Window & {
        __emitSessionSSE?: (sessionID: string, payload: unknown) => void
      }
    ).__emitSessionSSE
    emit?.('secondary-session', {
      type: 'delta',
      kind: 'text',
      delta: 'Streamed on the right',
    })
  })
  await expect(sideConversation.getByText('Streamed on the right')).toBeVisible()
  await expect(mainConversation.getByText('Streamed on the right')).toHaveCount(0)

  await workbench.getByRole('button', { name: 'Close conversation view' }).click()
  await expect(workbench.getByTestId('workbench-empty')).toContainText('No open views')
  await expect
    .poll(() =>
      page.evaluate(() => {
        const sources = (
          window as Window & {
            __eventSources?: Array<{ url: string; closed: boolean }>
          }
        ).__eventSources
        return sources
          ?.filter((source) => source.url.includes('/sessions/secondary-session/events'))
          .every((source) => source.closed)
      }),
    )
    .toBe(true)
  expect(
    requests.filter(
      (request) =>
        request.method === 'DELETE' && request.path === '/api/sessions/secondary-session',
    ),
  ).toHaveLength(0)
  await expect(mainConversation).toBeVisible()
})

test('a restored right-side preview stays available without taking focus from Chat', async ({
  page,
}) => {
  await openDesktopClient(page, {
    existingSession: true,
    secondarySession: true,
    secondaryHistoryEvents: [
      {
        type: 'tool_start',
        id: 'restored-preview',
        tool: 'open_preview',
        args: {
          url: '/tmp/secondary-session/web/index.html',
          title: 'Saved preview',
        },
      },
      {
        type: 'tool_end',
        id: 'restored-preview',
        tool: 'open_preview',
        result: 'Opened preview at /tmp/secondary-session/web/index.html',
        outcome: {
          status: 'success',
          data: {
            path: '/tmp/secondary-session/web/index.html',
            relativePath: 'web/index.html',
            grantID: 'secondary-grant',
            previewPath: 'index.html',
            title: 'Saved preview',
          },
        },
      },
    ],
  })

  await page.getByRole('button', { name: 'Actions for Secondary task' }).click()
  await page.getByRole('menuitem', { name: 'Open in right panel' }).click()

  const workbench = page.getByTestId('workbench-panel')
  await expect(workbench.getByRole('tab', { name: 'Secondary task' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  const previewTab = workbench.getByRole('tab', { name: 'Saved preview' })
  await expect(previewTab).toHaveAttribute('aria-selected', 'false')

  await previewTab.click()
  await expect.poll(async () =>
    (await browserRuntimeView(page, 'preview:secondary-session'))?.url.endsWith(
      '/api/sessions/secondary-session/previews/secondary-grant/index.html',
    ),
  ).toBe(true)
  expect(await browserRuntimeView(page, 'preview:secondary-session')).toMatchObject({
    visible: true,
  })
})

test('Agent preview tabs stay scoped to their selected session', async ({
  page,
}) => {
  const requests = await openDesktopClient(page, {
    existingSession: true,
    secondarySession: true,
  })

  await page.getByRole('button', { name: 'Actions for Secondary task' }).click()
  await page.getByRole('menuitem', { name: 'Open in right panel' }).click()

  await page.evaluate(() => {
    const emit = (
      window as Window & {
        __emitSessionSSE?: (sessionID: string, payload: unknown) => void
      }
    ).__emitSessionSSE
    emit?.('test-session', {
      type: 'tool_end',
      id: 'main-preview',
      tool: 'open_preview',
      result: 'Opened preview at /tmp/test-session/web/index.html',
      outcome: {
        status: 'success',
        data: {
          path: '/tmp/test-session/web/index.html',
          relativePath: 'web/index.html',
          grantID: 'main-grant',
          previewPath: 'index.html',
          title: 'Main preview',
        },
      },
    })
  })

  const workbench = page.getByTestId('workbench-panel')
  await expect(workbench.getByRole('tab', { name: 'Main preview' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect(workbench.getByRole('tab')).toHaveCount(1)

  await page.evaluate(() => {
    const emit = (
      window as Window & {
        __emitSessionSSE?: (sessionID: string, payload: unknown) => void
      }
    ).__emitSessionSSE
    emit?.('secondary-session', {
      type: 'tool_end',
      id: 'secondary-preview',
      tool: 'open_preview',
      result: 'Opened preview at /tmp/secondary-session/web/index.html',
      outcome: {
        status: 'success',
        data: {
          path: '/tmp/secondary-session/web/index.html',
          relativePath: 'web/index.html',
          grantID: 'secondary-grant',
          previewPath: 'index.html',
          title: 'Secondary preview',
        },
      },
    })
  })

  await expect(workbench.getByRole('tab')).toHaveCount(2)
  await expect(workbench.getByRole('tab', { name: 'Main preview' })).toHaveCount(0)
  await expect(workbench.getByRole('tab', { name: 'Secondary preview' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect(workbench.getByRole('textbox', { name: 'Address' })).toHaveValue(
    '/tmp/secondary-session/web/index.html',
  )
  await expect.poll(async () =>
    (await browserRuntimeView(page, 'preview:secondary-session'))?.url.endsWith(
      '/api/sessions/secondary-session/previews/secondary-grant/index.html',
    ),
  ).toBe(true)
  expect(await browserRuntimeView(page, 'preview:secondary-session')).toMatchObject({
    visible: true,
  })
  expect(await browserRuntimeView(page, 'preview:test-session')).toBeUndefined()
  expect(
    requests.filter((request) => request.path.includes('/previews/secondary-grant/index.html')),
  ).toHaveLength(0)

  await page.evaluate(() => {
    const emit = (
      window as Window & {
        __emitSessionSSE?: (sessionID: string, payload: unknown) => void
      }
    ).__emitSessionSSE
    emit?.('test-session', {
      type: 'tool_end',
      id: 'main-preview-update',
      tool: 'open_preview',
      result: 'Restored preview at /tmp/test-session/web/index.html',
      outcome: {
        status: 'success',
        data: {
          path: '/tmp/test-session/web/index.html',
          relativePath: 'web/index.html',
          grantID: 'main-grant',
          previewPath: 'index.html',
          title: 'Main preview',
        },
      },
    })
  })

  await expect(workbench.getByRole('tab')).toHaveCount(1)
  await expect(workbench.getByRole('tab', { name: 'Secondary preview' })).toHaveCount(0)
  await expect(workbench.getByRole('tab', { name: 'Main preview' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect(workbench.getByRole('textbox', { name: 'Address' })).toHaveValue(
    '/tmp/test-session/web/index.html',
  )
})

test('opening a conversation in the workbench preserves user browser tabs', async ({
  page,
}) => {
  await openDesktopClient(page, {
    existingSession: true,
    secondarySession: true,
  })

  await page.getByTestId('workbench-panel-toggle').click()
  const workbench = page.getByTestId('workbench-panel')
  await workbench.getByRole('button', { name: 'Add view' }).click()
  await page.getByRole('menuitem', { name: 'Browser' }).click()

  const address = workbench.getByRole('textbox', { name: 'Address' })
  await address.fill('https://www.bilibili.com')
  await address.press('Enter')
  await expect.poll(async () => (await browserRuntimeView(page, 'tab-1'))?.url).toBe(
    'https://www.bilibili.com/',
  )

  const videoURL = 'https://www.bilibili.com/video/BV1test'
  await guestNavigatesItself(page, 'tab-1', videoURL, 'Bilibili video')
  await expect(address).toHaveValue(videoURL)

  await workbench.getByRole('button', { name: 'Add view' }).click()
  await page.getByRole('menuitem', { name: 'Browser' }).click()
  await address.fill('https://github.com')
  await address.press('Enter')
  await expect.poll(async () => (await browserRuntimeView(page, 'tab-2'))?.url).toBe(
    'https://github.com/',
  )

  await page.getByRole('button', { name: 'Actions for Secondary task' }).click()
  await page.getByRole('menuitem', { name: 'Open in right panel' }).click()

  await expect(workbench.getByTestId('browser-tab')).toHaveCount(2)
  await expect(workbench.getByRole('tab', { name: 'Secondary task' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  expect(await browserRuntimeView(page, 'tab-1')).toMatchObject({
    url: videoURL,
    visible: false,
    loadCalls: ['https://www.bilibili.com/'],
  })
  expect(await browserRuntimeView(page, 'tab-2')).toMatchObject({
    url: 'https://github.com/',
    visible: false,
    loadCalls: ['https://github.com/'],
  })

  await workbench.getByRole('tab', { name: 'Bilibili video' }).click()
  expect(await browserRuntimeView(page, 'tab-1')).toMatchObject({
    url: videoURL,
    visible: true,
    loadCalls: ['https://www.bilibili.com/'],
  })
})

test('workbench divider resizes the panel without moving the corner control', async ({
  page,
}) => {
  await openDesktopClient(page, { existingSession: true })
  const toggle = page.getByTestId('workbench-panel-toggle')
  await toggle.click()

  const viewport = page.getByTestId('workbench-viewport')
  const handle = page.getByTestId('workbench-resize-handle')
  const divider = page.getByTestId('workbench-divider-line')
  await expect(handle).toBeVisible()
  await expect.poll(() => divider.evaluate((element) => {
    const color = getComputedStyle(element).backgroundColor
    return color !== 'transparent' && color !== 'rgba(0, 0, 0, 0)'
  })).toBe(true)
  await expect.poll(async () => (await viewport.boundingBox())?.width).toBeGreaterThan(490)
  const before = await viewport.boundingBox()
  const handleBefore = await handle.boundingBox()
  const dividerBefore = await divider.boundingBox()
  const toggleBefore = await toggle.boundingBox()
  expect(before).not.toBeNull()
  expect(handleBefore).not.toBeNull()
  expect(dividerBefore).not.toBeNull()
  expect(toggleBefore).not.toBeNull()
  expect(Math.abs(handleBefore!.x + handleBefore!.width - before!.x)).toBeLessThanOrEqual(1)
  expect(dividerBefore!.height).toBeCloseTo(before!.height, 0)
  expect(
    Math.abs(dividerBefore!.x + dividerBefore!.width - before!.x),
  ).toBeLessThanOrEqual(1)

  await page.mouse.move(handleBefore!.x + 2, handleBefore!.y + handleBefore!.height / 2)
  await page.mouse.down()
  await page.mouse.move(handleBefore!.x - 70, handleBefore!.y + handleBefore!.height / 2, {
    steps: 8,
  })
  await page.mouse.up()

  await expect.poll(async () => (await viewport.boundingBox())?.width).toBeCloseTo(
    before!.width + 72,
    0,
  )
  const afterDrag = await viewport.boundingBox()
  const handleAfterDrag = await handle.boundingBox()
  const toggleAfterDrag = await toggle.boundingBox()
  expect(afterDrag).not.toBeNull()
  expect(handleAfterDrag).not.toBeNull()
  expect(toggleAfterDrag).not.toBeNull()
  expect(
    Math.abs(handleAfterDrag!.x + handleAfterDrag!.width - afterDrag!.x),
  ).toBeLessThanOrEqual(1)
  expect(toggleAfterDrag!.x).toBeCloseTo(toggleBefore!.x, 1)
  expect(toggleAfterDrag!.y).toBeCloseTo(toggleBefore!.y, 1)

  await handle.focus()
  await handle.press('ArrowRight')
  await expect.poll(async () => (await viewport.boundingBox())?.width).toBeCloseTo(
    afterDrag!.width - 16,
    0,
  )
})

test('long threads coalesce workbench resize events into one animation frame', async ({
  page,
}) => {
  await openDesktopClient(page, {
    existingSession: true,
    historyEvents: Array.from({ length: 8 }, (_, index) =>
      longThreadHistory(`resize-${index}`),
    ).flat(),
  })
  await expect(page.getByTestId('assistant-message')).toHaveCount(144)

  await page.getByTestId('workbench-panel-toggle').click()
  const viewport = page.getByTestId('workbench-viewport')
  const handle = page.getByTestId('workbench-resize-handle')
  await expect.poll(async () => (await viewport.boundingBox())?.width).toBeGreaterThan(490)
  const before = await viewport.boundingBox()
  const handleBox = await handle.boundingBox()
  expect(before).not.toBeNull()
  expect(handleBox).not.toBeNull()

  const startX = handleBox!.x + handleBox!.width / 2
  const pointerY = handleBox!.y + handleBox!.height / 2
  await handle.evaluate((element, { clientX, clientY }) => {
    const capturedPointers = new Set<number>()
    const originalSetPointerCapture = element.setPointerCapture.bind(element)
    const originalHasPointerCapture = element.hasPointerCapture.bind(element)
    const originalReleasePointerCapture = element.releasePointerCapture.bind(element)
    const testWindow = window as Window & { __restoreResizePointerCapture?: () => void }
    element.setPointerCapture = (pointerID) => capturedPointers.add(pointerID)
    element.hasPointerCapture = (pointerID) => capturedPointers.has(pointerID)
    element.releasePointerCapture = (pointerID) => capturedPointers.delete(pointerID)
    testWindow.__restoreResizePointerCapture = () => {
      element.setPointerCapture = originalSetPointerCapture
      element.hasPointerCapture = originalHasPointerCapture
      element.releasePointerCapture = originalReleasePointerCapture
    }
    element.dispatchEvent(new PointerEvent('pointerdown', {
      bubbles: true,
      button: 0,
      buttons: 1,
      clientX,
      clientY,
      pointerId: 77,
    }))
  }, { clientX: startX, clientY: pointerY })

  await page.evaluate(() => {
    const nativeRequestAnimationFrame = window.requestAnimationFrame.bind(window)
    const nativeCancelAnimationFrame = window.cancelAnimationFrame.bind(window)
    const callbacks = new Map<number, FrameRequestCallback>()
    let nextFrameID = 100_000
    const testWindow = window as Window & {
      __resizeFrameTest?: {
        count: () => number
        flush: () => void
        restore: () => void
      }
    }
    const restore = () => {
      window.requestAnimationFrame = nativeRequestAnimationFrame
      window.cancelAnimationFrame = nativeCancelAnimationFrame
    }
    window.requestAnimationFrame = (callback) => {
      const frameID = nextFrameID
      nextFrameID += 1
      callbacks.set(frameID, callback)
      return frameID
    }
    window.cancelAnimationFrame = (frameID) => {
      callbacks.delete(frameID)
    }
    testWindow.__resizeFrameTest = {
      count: () => callbacks.size,
      flush: () => {
        const pending = [...callbacks.values()]
        callbacks.clear()
        restore()
        for (const callback of pending) callback(performance.now())
      },
      restore,
    }
  })

  try {
    await handle.evaluate((element, { startX, pointerY }) => {
      for (let step = 1; step <= 40; step += 1) {
        element.dispatchEvent(new PointerEvent('pointermove', {
          bubbles: true,
          buttons: 1,
          clientX: startX - step * 2,
          clientY: pointerY,
          pointerId: 77,
        }))
      }
    }, { startX, pointerY })

    await expect.poll(() => page.evaluate(() => (
      window as Window & { __resizeFrameTest?: { count: () => number } }
    ).__resizeFrameTest?.count())).toBe(1)
    expect((await viewport.boundingBox())?.width).toBeCloseTo(before!.width, 0)

    await page.evaluate(() => (
      window as Window & { __resizeFrameTest?: { flush: () => void } }
    ).__resizeFrameTest?.flush())
    await expect.poll(async () => (await viewport.boundingBox())?.width).toBeCloseTo(
      before!.width + 80,
      0,
    )
  } finally {
    await page.evaluate(() => (
      window as Window & { __resizeFrameTest?: { restore: () => void } }
    ).__resizeFrameTest?.restore())
    await handle.evaluate((element, { clientX, clientY }) => {
      element.dispatchEvent(new PointerEvent('pointerup', {
        bubbles: true,
        buttons: 0,
        clientX,
        clientY,
        pointerId: 77,
      }))
    }, { clientX: startX - 80, clientY: pointerY })
    await page.evaluate(() => (
      window as Window & { __restoreResizePointerCapture?: () => void }
    ).__restoreResizePointerCapture?.())
  }
})

test('workbench restores after an automatic collapse but respects a manual close', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1728, height: 1000 })
  await openDesktopClient(page, { existingSession: true })
  const toggle = page.getByTestId('workbench-panel-toggle')
  const viewport = page.getByTestId('workbench-viewport')
  const conversation = page.getByTestId('conversation-pane')

  await toggle.click()
  await expect(viewport.getByRole('button', { name: 'Maximize workbench' })).toBeVisible()
  await expect.poll(async () => (await viewport.boundingBox())?.width).toBeGreaterThan(700)
  const originalWorkbenchWidth = (await viewport.boundingBox())!.width

  await page.setViewportSize({ width: 960, height: 700 })
  await expect(toggle).toHaveAccessibleName('Show workbench')
  const collapsedLayout = await page.getByTestId('workbench-layout').boundingBox()
  const expandedConversation = await conversation.boundingBox()
  expect(collapsedLayout).not.toBeNull()
  expect(expandedConversation).not.toBeNull()
  expect(expandedConversation!.width).toBeCloseTo(collapsedLayout!.width, 1)
  await expect(page.getByTestId('workbench-panel')).toBeHidden()

  // 520-560 px is a dead band: growing slightly does not flip back to split mode.
  await page.setViewportSize({ width: 1490, height: 900 })
  await expect(toggle).toHaveAccessibleName('Show workbench')

  await page.setViewportSize({ width: 1728, height: 1000 })
  await expect(toggle).toHaveAccessibleName('Hide workbench')
  await expect(viewport.getByRole('button', { name: 'Maximize workbench' })).toBeVisible()
  await expect.poll(async () => (await viewport.boundingBox())?.width).toBeCloseTo(
    originalWorkbenchWidth,
    0,
  )
  await expect(conversation).toHaveAttribute('aria-hidden', 'false')

  await toggle.click()
  await page.setViewportSize({ width: 960, height: 700 })
  await page.setViewportSize({ width: 1728, height: 1000 })
  await expect(toggle).toHaveAccessibleName('Show workbench')
})

test('manual and AI workbench opens cover Chat when the layout is constrained', async ({ page }) => {
  await page.setViewportSize({ width: 960, height: 700 })
  await openDesktopClient(page, { existingSession: true })
  const toggle = page.getByTestId('workbench-panel-toggle')
  const layout = page.getByTestId('workbench-layout')
  const viewport = page.getByTestId('workbench-viewport')
  const conversation = page.getByTestId('conversation-pane')
  const sidebar = page.getByTestId('sidebar-viewport')

  await toggle.click()
  await expect(viewport.getByRole('button', { name: 'Restore workbench' })).toBeVisible()
  await expect.poll(async () => (await viewport.boundingBox())?.width).toBeCloseTo(
    (await layout.boundingBox())!.width,
    0,
  )
  await expect(conversation).toHaveAttribute('aria-hidden', 'true')
  await expect(sidebar).toBeVisible()

  await toggle.click()
  await expect.poll(() =>
    page.evaluate(
      () => (window as Window & { __eventSources?: unknown[] }).__eventSources?.length ?? 0,
    ),
  ).toBeGreaterThan(0)
  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_end',
      id: 'narrow-preview',
      tool: 'open_preview',
      result: 'Opened preview at http://127.0.0.1:4310',
      outcome: {
        status: 'success',
        data: { url: 'http://127.0.0.1:4310', title: 'Narrow preview' },
      },
    })
  })

  await expect(page.getByTestId('browser-view')).toBeVisible()
  await expect(viewport.getByRole('button', { name: 'Restore workbench' })).toBeVisible()
  await expect.poll(async () => (await viewport.boundingBox())?.width).toBeCloseTo(
    (await layout.boundingBox())!.width,
    0,
  )
  await expect(conversation).toHaveAttribute('aria-hidden', 'true')
})

test('empty workbench keeps header actions and can cover Chat without hiding the sidebar', async ({
  page,
}) => {
  await openDesktopClient(page, { existingSession: true })
  await page.getByTestId('workbench-panel-toggle').click()

  const layout = page.getByTestId('workbench-layout')
  const viewport = page.getByTestId('workbench-viewport')
  const conversation = page.getByTestId('conversation-pane')
  const sidebar = page.getByTestId('sidebar-viewport')
  const resizeHandle = page.getByTestId('workbench-resize-handle')
  const addView = viewport.getByRole('button', { name: 'Add view' })
  const maximize = viewport.getByRole('button', { name: 'Maximize workbench' })

  await expect.poll(async () => (await viewport.boundingBox())?.width).toBeGreaterThan(490)
  const normalWidth = (await viewport.boundingBox())!.width
  const normalConversationWidth = (await conversation.boundingBox())!.width
  await expect(addView).toBeVisible()
  await expect(maximize).toBeVisible()
  await addView.click()
  await expect(page.getByRole('menu').getByRole('menuitem', { name: 'Browser' })).toBeVisible()
  await page.keyboard.press('Escape')

  await maximize.click()
  await expect(viewport.getByRole('button', { name: 'Restore workbench' })).toBeVisible()
  const layoutWidth = (await layout.boundingBox())!.width
  await expect.poll(async () => (await viewport.boundingBox())?.width).toBeCloseTo(
    layoutWidth,
    0,
  )
  expect((await conversation.boundingBox())!.width).toBeCloseTo(normalConversationWidth, 0)
  await expect(conversation).toHaveAttribute('aria-hidden', 'true')
  await expect(resizeHandle).toHaveCount(0)
  await expect(sidebar).toBeVisible()
  expect((await sidebar.boundingBox())!.width).toBeGreaterThan(200)

  await viewport.getByRole('button', { name: 'Restore workbench' }).click()
  await expect.poll(async () => (await viewport.boundingBox())?.width).toBeCloseTo(
    normalWidth,
    0,
  )
  await expect(conversation).toHaveAttribute('aria-hidden', 'false')
  await expect(resizeHandle).toBeVisible()
})

test('AI preview tool opens Browser in the workbench beside Chat', async ({ page }) => {
  await page.route('http://127.0.0.1:4310/**', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'text/html',
      body: '<!doctype html><title>Preview fixture</title><main>Local preview ready</main>',
    })
  })
  await openDesktopClient(page, { existingSession: true })
  await expect.poll(() =>
    page.evaluate(
      () =>
        (window as Window & { __eventSources?: unknown[] }).__eventSources?.length ?? 0,
    ),
  ).toBeGreaterThan(0)

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_start',
      id: 'preview-call',
      tool: 'open_preview',
      args: { url: 'http://127.0.0.1:4310', title: 'Local app' },
    })
    emit?.({
      type: 'tool_end',
      id: 'preview-call',
      tool: 'open_preview',
      result: 'Opened preview at http://127.0.0.1:4310',
      outcome: {
        status: 'success',
        data: { url: 'http://127.0.0.1:4310', title: 'Local app' },
      },
    })
  })

  await expect(page.getByTestId('browser-view')).toBeVisible()
  await expect.poll(async () =>
    (await browserRuntimeView(page, 'preview:test-session'))?.url,
  ).toBe('http://127.0.0.1:4310/')
  await expect.poll(async () =>
    (await browserRuntimeView(page, 'preview:test-session'))?.visible,
  ).toBe(true)
  await expect(page.locator('webview')).toHaveAttribute('partition', 'persist:or-browser')
  await expect(page.getByRole('main')).toBeVisible()
  await expect.poll(async () => {
    const chatBox = await page.getByRole('main').boundingBox()
    const browserBox = await page.getByTestId('workbench-viewport').boundingBox()
    if (!chatBox || !browserBox) return Number.POSITIVE_INFINITY
    return chatBox.x + chatBox.width - browserBox.x
  }).toBeLessThanOrEqual(1)

  await expect(page.getByRole('tab', { name: 'Local app' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await page.getByRole('button', { name: 'Add view' }).click()
  await page.getByRole('menu').getByRole('menuitem', { name: 'Browser' }).click()
  await expect(page.getByRole('tab')).toHaveCount(2)
  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_end',
      id: 'preview-call-2',
      tool: 'open_preview',
      result: 'Updated preview at http://127.0.0.1:4310',
      outcome: {
        status: 'success',
        data: { url: 'http://127.0.0.1:4310', title: 'Updated app' },
      },
    })
  })
  await expect(page.getByRole('tab')).toHaveCount(2)
  await expect(page.getByRole('tab', { name: 'Updated app' })).toHaveAttribute(
    'aria-selected',
    'true',
  )

  await page.getByRole('button', { name: 'Open in system browser' }).click()
  await expect.poll(() =>
    page.evaluate(() => (window as Window & { __openedURL?: string }).__openedURL),
  ).toBe('http://127.0.0.1:4310/')

  await page.getByTestId('workbench-panel-toggle').click()
  await expect(page.getByTestId('browser-view')).toBeHidden()
  await expect.poll(async () =>
    (await browserRuntimeView(page, 'preview:test-session'))?.visible,
  ).toBe(false)
  await expect(page.getByRole('main')).toBeVisible()
  await expect(page.getByTestId('workbench-panel-toggle')).toHaveAccessibleName('Show workbench')

  await page.getByTestId('workbench-panel-toggle').click()
  await expect(page.getByTestId('browser-view')).toBeVisible()
  await expect.poll(async () =>
    (await browserRuntimeView(page, 'preview:test-session'))?.visible,
  ).toBe(true)
  await page.getByTestId('workbench-panel-toggle').click()
  await expect(page.getByTestId('workbench-panel')).toBeHidden()
  await expect(page.getByRole('main')).toBeVisible()
  await expect(page.getByTestId('workbench-panel-toggle')).toBeVisible()
})

test('AI preview tool opens a public website inside the Browser', async ({ page }) => {
  const requests = await openDesktopClient(page, { existingSession: true })
  await expect.poll(() =>
    page.evaluate(
      () =>
        (window as Window & { __eventSources?: unknown[] }).__eventSources?.length ?? 0,
    ),
  ).toBeGreaterThan(0)

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_end',
      id: 'public-preview',
      tool: 'open_preview',
      result: 'Opened preview at https://www.google.com',
      outcome: {
        status: 'success',
        data: { url: 'https://www.google.com', title: 'Google' },
      },
    })
  })

  await expect(page.getByRole('tab', { name: 'Google' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect.poll(async () =>
    (await browserRuntimeView(page, 'preview:test-session'))?.url,
  ).toBe('https://www.google.com/')
  await expect.poll(async () =>
    (await browserRuntimeView(page, 'preview:test-session'))?.visible,
  ).toBe(true)
  const divider = page.getByTestId('workbench-divider-line')
  await expect.poll(async () => {
    const dividerBox = await divider.boundingBox()
    const runtimeView = await browserRuntimeView(page, 'preview:test-session')
    if (!dividerBox || !runtimeView) return Number.NEGATIVE_INFINITY
    return runtimeView.bounds.x - (dividerBox.x + dividerBox.width)
  }).toBeGreaterThanOrEqual(0)
  await expect(page.getByRole('textbox', { name: 'Address' })).toHaveValue(
    'https://www.google.com/',
  )
  expect(requests.filter((request) => request.path === '/api/preview/check')).toHaveLength(0)
  expect(
    await page.evaluate(() => (window as Window & { __openedURL?: string }).__openedURL),
  ).toBeUndefined()
})

test('AI browser request reports the committed Electron navigation exactly once', async ({
  page,
}) => {
  const requests = await openDesktopClient(page, { existingSession: true })
  await expect.poll(() =>
    page.evaluate(
      () =>
        (window as Window & { __eventSources?: unknown[] }).__eventSources?.length ?? 0,
    ),
  ).toBeGreaterThan(0)

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'browser_request',
      id: 'browser-command-1',
      disposition: 'reuse_agent_tab',
      preview: { url: 'https://github.com', title: 'GitHub' },
    })
  })

  await expect.poll(async () =>
    (await browserRuntimeView(page, 'preview:test-session'))?.url,
  ).toBe('https://github.com/')
  await expect.poll(() =>
    requests.filter(
      (request) =>
        request.path ===
        '/api/sessions/test-session/browser/browser-command-1/result',
    ).length,
  ).toBe(1)
  const resultRequest = requests.find(
    (request) =>
      request.path === '/api/sessions/test-session/browser/browser-command-1/result',
  )
  expect(resultRequest).toMatchObject({
    method: 'POST',
    body: {
      status: 'committed',
      requestedURL: 'https://github.com/',
      committedURL: 'https://github.com/',
    },
  })

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_end',
      id: 'preview-call-1',
      tool: 'open_preview',
      result: 'Opened preview at https://github.com/',
      outcome: {
        status: 'success',
        data: { url: 'https://github.com', title: 'GitHub' },
      },
    })
  })
  await page.waitForTimeout(50)
  expect(await browserRuntimeView(page, 'preview:test-session')).toMatchObject({
    loadCalls: ['https://github.com/'],
    reloadCalls: 0,
  })
})

test('AI browser result outbox survives Browser closure and retries without navigating again', async ({
  page,
}) => {
  const requests = await openDesktopClient(page, {
    existingSession: true,
    browserResultFailures: 3,
  })
  await expect.poll(() =>
    page.evaluate(
      () =>
        (window as Window & { __eventSources?: unknown[] }).__eventSources?.length ?? 0,
    ),
  ).toBeGreaterThan(0)

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'browser_request',
      id: 'browser-retry',
      disposition: 'reuse_agent_tab',
      preview: { url: 'https://github.com', title: 'GitHub' },
    })
  })

  const resultRequests = () => requests.filter(
    (request) =>
      request.path === '/api/sessions/test-session/browser/browser-retry/result',
  )
  await expect.poll(() => resultRequests().length).toBe(1)
  expect(await browserRuntimeView(page, 'preview:test-session')).toMatchObject({
    url: 'https://github.com/',
    loadCalls: ['https://github.com/'],
  })

  await page.getByRole('button', { name: 'Close tab: GitHub' }).click()
  await expect(page.getByTestId('workbench-empty')).toBeVisible()
  await expect.poll(() => resultRequests().length, { timeout: 8_000 }).toBe(4)
  await page.waitForTimeout(300)
  expect(resultRequests()).toHaveLength(4)
  await expect(page.getByTestId('workbench-empty')).toBeVisible()
})

test('a guest initial document cannot consume the AI browser command result', async ({
  page,
}) => {
  const requests = await openDesktopClient(page, { existingSession: true })
  await expect.poll(() =>
    page.evaluate(
      () =>
        (window as Window & { __eventSources?: unknown[] }).__eventSources?.length ?? 0,
    ),
  ).toBeGreaterThan(0)

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'browser_request',
      id: 'browser-command-1',
      disposition: 'reuse_agent_tab',
      preview: { url: 'https://github.com', title: 'GitHub' },
    })
  })

  const results = () =>
    requests.filter(
      (request) =>
        request.path ===
        '/api/sessions/test-session/browser/browser-command-1/result',
    )
  await expect.poll(() => results().length).toBe(1)
  expect(results()[0]).toMatchObject({
    method: 'POST',
    body: {
      status: 'committed',
      requestedURL: 'https://github.com/',
      committedURL: 'https://github.com/',
    },
  })
  await page.waitForTimeout(300)
  expect(results()).toHaveLength(1)
})

test('the first agent navigation of a session commits in a real webview guest', async ({
  page,
}) => {
  const requests = await openDesktopClient(page, { existingSession: true })
  await expect.poll(() =>
    page.evaluate(
      () =>
        (window as Window & { __eventSources?: unknown[] }).__eventSources?.length ?? 0,
    ),
  ).toBeGreaterThan(0)

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'browser_request',
      id: 'browser-command-1',
      disposition: 'reuse_agent_tab',
      preview: { url: 'https://github.com', title: 'GitHub' },
    })
  })

  const surface = page.getByTestId('browser-surface')
  await expect(surface).toHaveAttribute('data-status', 'ready')
  await expect.poll(() =>
    page.evaluate(() => {
      const guest = document.querySelector('webview') as
        | (HTMLElement & { getURL?: () => string })
        | null
      return guest?.getURL?.() ?? ''
    }),
  ).toBe('https://github.com/')

  const results = () =>
    requests.filter(
      (request) =>
        request.path ===
        '/api/sessions/test-session/browser/browser-command-1/result',
    )
  await expect.poll(() => results().length).toBe(1)
  expect(results()[0]).toMatchObject({
    method: 'POST',
    body: {
      status: 'committed',
      requestedURL: 'https://github.com/',
      committedURL: 'https://github.com/',
    },
  })
})

test('AI browser restores and acknowledges a pending history command', async ({ page }) => {
  const requests = await openDesktopClient(page, {
    existingSession: true,
    historyEvents: [
      {
        type: 'browser_inspect_request',
        id: 'restored-browser-inspection',
      },
      {
        type: 'browser_request',
        id: 'restored-browser-command',
        disposition: 'reuse_agent_tab',
        preview: { url: 'https://example.com/restored', title: 'Restored' },
      },
    ],
  })

  await expect.poll(async () =>
    (await browserRuntimeView(page, 'preview:test-session'))?.url,
  ).toBe('https://example.com/restored')
  await expect.poll(() =>
    requests.filter(
      (request) =>
        request.path ===
        '/api/sessions/test-session/browser/restored-browser-command/result',
    ).length,
  ).toBe(1)
  await expect(page.getByRole('tab', { name: 'Restored' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect.poll(() =>
    requests.filter(
      (request) =>
        request.path ===
        '/api/sessions/test-session/browser/inspect/restored-browser-inspection/result',
    ).length,
  ).toBe(1)
  expect(
    requests.find(
      (request) =>
        request.path ===
        '/api/sessions/test-session/browser/inspect/restored-browser-inspection/result',
    )?.body,
  ).toMatchObject({
    status: 'completed',
    url: 'https://example.com/restored',
    pageStatus: 'ready',
    visibleText: 'Visible content for https://example.com/restored',
  })
})

test('AI browser opens foreground and background tabs and reuses the active Agent tab', async ({
  page,
}) => {
  const requests = await openDesktopClient(page, { existingSession: true })
  await expect.poll(() =>
    page.evaluate(
      () =>
        (window as Window & { __eventSources?: unknown[] }).__eventSources?.length ?? 0,
    ),
  ).toBeGreaterThan(0)

  const emitBrowserRequest = async (
    id: string,
    disposition: 'reuse_agent_tab' | 'new_foreground_tab' | 'new_background_tab',
    url: string,
    title: string,
  ) => {
    await page.evaluate(
      ({ id, disposition, url, title }) => {
        const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
        emit?.({
          type: 'browser_request',
          id,
          disposition,
          preview: { url, title },
        })
      },
      { id, disposition, url, title },
    )
  }

  await emitBrowserRequest(
    'browser-reuse',
    'reuse_agent_tab',
    'https://github.com',
    'GitHub',
  )
  await expect.poll(async () =>
    (await browserRuntimeView(page, 'preview:test-session'))?.url,
  ).toBe('https://github.com/')

  await emitBrowserRequest(
    'browser-foreground',
    'new_foreground_tab',
    'https://www.bilibili.com',
    'Bilibili',
  )
  const foregroundTabID = 'preview:test-session:command:browser-foreground'
  await expect.poll(async () =>
    (await browserRuntimeView(page, foregroundTabID))?.url,
  ).toBe('https://www.bilibili.com/')
  await expect(page.getByRole('tab', { name: 'Bilibili' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect.poll(async () =>
    (await browserRuntimeView(page, 'preview:test-session'))?.visible,
  ).toBe(false)
  await expect.poll(() =>
    requests.filter(
      (request) =>
        request.path ===
        '/api/sessions/test-session/browser/browser-foreground/result',
    ).length,
  ).toBe(1)
  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_end',
      id: 'preview-foreground',
      tool: 'open_preview',
      result: 'Opened preview at https://www.bilibili.com/',
      outcome: {
        status: 'success',
        data: { url: 'https://www.bilibili.com', title: 'Bilibili' },
      },
    })
  })
  await expect(page.getByRole('tab', { name: 'Bilibili' })).toHaveCount(1)
  expect(await browserRuntimeView(page, 'preview:test-session')).toMatchObject({
    url: 'https://github.com/',
    loadCalls: ['https://github.com/'],
    reloadCalls: 0,
  })

  await emitBrowserRequest(
    'browser-background',
    'new_background_tab',
    'https://www.google.com',
    'Google',
  )
  const backgroundTabID = 'preview:test-session:command:browser-background'
  await expect.poll(async () =>
    (await browserRuntimeView(page, backgroundTabID))?.url,
  ).toBe('https://www.google.com/')
  await expect(page.getByRole('tab', { name: 'Bilibili' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  expect(await browserRuntimeView(page, backgroundTabID)).toMatchObject({ visible: false })
  expect(await browserRuntimeView(page, foregroundTabID)).toMatchObject({ visible: true })
  expect(await browserRuntimeView(page, 'preview:test-session')).toMatchObject({
    url: 'https://github.com/',
  })

  await emitBrowserRequest(
    'browser-reuse-active',
    'reuse_agent_tab',
    'https://chat.deepseek.com',
    'DeepSeek',
  )
  await expect.poll(async () =>
    (await browserRuntimeView(page, foregroundTabID))?.url,
  ).toBe('https://chat.deepseek.com/')
  await expect(page.getByRole('tab', { name: 'DeepSeek' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  expect(await browserRuntimeView(page, 'preview:test-session')).toMatchObject({
    url: 'https://github.com/',
    loadCalls: ['https://github.com/'],
    reloadCalls: 0,
  })
  expect(await browserRuntimeView(page, backgroundTabID)).toMatchObject({
    url: 'https://www.google.com/',
    visible: false,
  })

  await expect.poll(() =>
    requests.filter((request) =>
      request.path.startsWith('/api/sessions/test-session/browser/browser-') &&
      request.path.endsWith('/result'),
    ).length,
  ).toBe(4)
})

test('AI browser lists session tabs and explicitly inspects a non-active Agent tab', async ({
  page,
}) => {
  const requests = await openDesktopClient(page, { existingSession: true })
  await setGuestControls(page, { pageTitle: 'github.com' })
  await expect.poll(() =>
    page.evaluate(
      () =>
        (window as Window & { __eventSources?: unknown[] }).__eventSources?.length ?? 0,
    ),
  ).toBeGreaterThan(0)

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'browser_request',
      id: 'browser-agent',
      disposition: 'reuse_agent_tab',
      preview: { url: 'https://github.com', title: 'GitHub' },
    })
  })
  await expect.poll(async () =>
    (await browserRuntimeView(page, 'preview:test-session'))?.url,
  ).toBe('https://github.com/')

  await setGuestControls(page, { pageTitle: 'bilibili.com' })
  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'browser_request',
      id: 'browser-command-tab',
      disposition: 'new_foreground_tab',
      preview: { url: 'https://www.bilibili.com', title: 'Bilibili' },
    })
  })
  await expect.poll(async () =>
    (await browserRuntimeView(
      page,
      'preview:test-session:command:browser-command-tab',
    ))?.url,
  ).toBe('https://www.bilibili.com/')

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({ type: 'browser_tabs_request', id: 'tabs-1' })
    emit?.({ type: 'browser_tabs_request', id: 'tabs-1' })
    emit?.({
      type: 'browser_inspect_request',
      id: 'inspection-1',
      tabID: 'preview:test-session',
    })
    emit?.({
      type: 'browser_inspect_request',
      id: 'inspection-1',
      tabID: 'preview:test-session',
    })
  })

  const tabsResultPath = '/api/sessions/test-session/browser/tabs/tabs-1/result'
  await expect.poll(() =>
    requests.filter((request) => request.path === tabsResultPath).length,
  ).toBe(1)
  expect(requests.find((request) => request.path === tabsResultPath)).toMatchObject({
    method: 'POST',
    body: {
      status: 'completed',
      openTabs: [
        {
          tabID: 'preview:test-session',
          url: 'https://github.com/',
          title: 'github.com',
          status: 'ready',
        },
        {
          tabID: 'preview:test-session:command:browser-command-tab',
          url: 'https://www.bilibili.com/',
          title: 'bilibili.com',
          status: 'ready',
        },
      ],
      controlledTabs: [
        {
          tabID: 'preview:test-session',
          capabilities: ['read', 'navigate'],
        },
        {
          tabID: 'preview:test-session:command:browser-command-tab',
          capabilities: ['read', 'navigate'],
        },
      ],
      selected: 'preview:test-session:command:browser-command-tab',
    },
  })
  const resultPath =
    '/api/sessions/test-session/browser/inspect/inspection-1/result'
  await expect.poll(() =>
    requests.filter((request) => request.path === resultPath).length,
  ).toBe(1)
  expect(requests.find((request) => request.path === resultPath)).toMatchObject({
    method: 'POST',
    body: {
      status: 'completed',
      url: 'https://github.com/',
      title: 'github.com',
      pageStatus: 'ready',
      revision: 0,
      visibleText: 'Visible content for https://github.com/',
      truncated: false,
    },
  })
  expect(
    await page.evaluate(() =>
      Array.from(document.querySelectorAll('[data-browser-tab-id]'))
        .map((host) => ({
          tabID: host.getAttribute('data-browser-tab-id'),
          inspectCalls:
            (host.querySelector('webview') as (HTMLElement & { inspectCalls?: number }) | null)
              ?.inspectCalls ?? 0,
        }))
        .filter((entry) => entry.inspectCalls > 0)
        .map((entry) => entry.tabID),
    ),
  ).toEqual(['preview:test-session'])
})

test('AI browser temporarily attaches read control to an existing open tab', async ({
  page,
}) => {
  const requests = await openDesktopClient(page, { existingSession: true })
  await expect.poll(() =>
    page.evaluate(
      () =>
        (window as Window & { __eventSources?: unknown[] }).__eventSources?.length ?? 0,
    ),
  ).toBeGreaterThan(0)

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'browser_request',
      id: 'browser-agent',
      disposition: 'reuse_agent_tab',
      preview: { url: 'https://github.com', title: 'GitHub' },
    })
  })
  await expect.poll(async () =>
    (await browserRuntimeView(page, 'preview:test-session'))?.url,
  ).toBe('https://github.com/')

  await page.getByTestId('workbench-add-view').click()
  await page.getByRole('menuitem').first().click()
  await setGuestControls(page, { pageTitle: 'example.com' })
  await page.getByRole('textbox', { name: 'Address' }).fill('https://example.com')
  await page.getByRole('textbox', { name: 'Address' }).press('Enter')
  await expect.poll(async () => (await browserRuntimeView(page, 'tab-1'))?.url).toBe(
    'https://example.com/',
  )
  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({ type: 'browser_tabs_request', id: 'tabs-with-user' })
    emit?.({
      type: 'browser_inspect_request',
      id: 'inspection-user-tab',
      tabID: 'tab-1',
    })
  })

  const tabsResultPath =
    '/api/sessions/test-session/browser/tabs/tabs-with-user/result'
  await expect.poll(() =>
    requests.filter((request) => request.path === tabsResultPath).length,
  ).toBe(1)
  expect(requests.find((request) => request.path === tabsResultPath)?.body).toMatchObject({
    status: 'completed',
    openTabs: [
      {
        tabID: 'preview:test-session',
      },
      {
        tabID: 'tab-1',
        url: 'https://example.com/',
      },
    ],
    controlledTabs: [
      {
        tabID: 'preview:test-session',
        capabilities: ['read', 'navigate'],
      },
    ],
    selected: 'preview:test-session',
  })
  const resultPath =
    '/api/sessions/test-session/browser/inspect/inspection-user-tab/result'
  await expect.poll(() =>
    requests.filter((request) => request.path === resultPath).length,
  ).toBe(1)
  expect(requests.find((request) => request.path === resultPath)).toMatchObject({
    method: 'POST',
    body: {
      status: 'completed',
      url: 'https://example.com/',
      title: 'example.com',
      pageStatus: 'ready',
      revision: 1,
      visibleText: 'Visible content for https://example.com/',
      truncated: false,
    },
  })
  expect(
    await page.evaluate(() =>
      Array.from(document.querySelectorAll('[data-browser-tab-id]'))
        .map((host) => ({
          tabID: host.getAttribute('data-browser-tab-id'),
          inspectCalls:
            (host.querySelector('webview') as (HTMLElement & { inspectCalls?: number }) | null)
              ?.inspectCalls ?? 0,
        }))
        .filter((entry) => entry.inspectCalls > 0)
        .map((entry) => entry.tabID),
    ),
  ).toEqual(['tab-1'])

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({ type: 'browser_tabs_request', id: 'tabs-after-inspection' })
  })
  const afterInspectionPath =
    '/api/sessions/test-session/browser/tabs/tabs-after-inspection/result'
  await expect.poll(() =>
    requests.filter((request) => request.path === afterInspectionPath).length,
  ).toBe(1)
  expect(requests.find((request) => request.path === afterInspectionPath)?.body)
    .toMatchObject({
      controlledTabs: [
        {
          tabID: 'preview:test-session',
          capabilities: ['read', 'navigate'],
        },
      ],
      selected: 'preview:test-session',
    })
  await expect(page.locator('[data-browser-tab-id]')).toHaveCount(2)
})

test('browser inspection cannot inspect another session\'s user tab', async ({
  page,
}) => {
  const requests = await openDesktopClient(page, {
    existingSession: true,
    secondarySession: true,
  })

  await page.getByTestId('workbench-panel-toggle').click()
  const workbench = page.getByTestId('workbench-panel')
  await workbench.getByRole('button', { name: 'Add view' }).click()
  await page.getByRole('menuitem', { name: 'Browser' }).click()
  let address = workbench.getByRole('textbox', { name: 'Address' })
  await address.fill('https://main.example')
  await address.press('Enter')
  await expect.poll(async () => (await browserRuntimeView(page, 'tab-1'))?.url).toBe(
    'https://main.example/',
  )

  await page.getByRole('button', { name: 'Actions for Secondary task' }).click()
  await page.getByRole('menuitem', { name: 'Open in right panel' }).click()
  await workbench.getByRole('button', { name: 'Add view' }).click()
  await page.getByRole('menuitem', { name: 'Browser' }).click()
  address = workbench.getByRole('textbox', { name: 'Address' })
  await address.fill('https://secondary.example')
  await address.press('Enter')
  await expect.poll(async () => (await browserRuntimeView(page, 'tab-2'))?.url).toBe(
    'https://secondary.example/',
  )
  await expect(page.locator('[data-browser-tab-id="tab-2"]')).toHaveAttribute(
    'data-browser-runtime-tab-id',
    'workspace:workbench:tab:tab-2',
  )

  await page.evaluate(() => {
    const emit = (
      window as Window & {
        __emitSessionSSE?: (sessionID: string, payload: unknown) => void
      }
    ).__emitSessionSSE
    emit?.('test-session', {
      type: 'browser_inspect_request',
      id: 'cross-session-inspection',
      tabID: 'tab-2',
    })
  })

  const resultPath =
    '/api/sessions/test-session/browser/inspect/cross-session-inspection/result'
  await expect.poll(() =>
    requests.filter((request) => request.path === resultPath).length,
  ).toBe(1)
  expect(requests.find((request) => request.path === resultPath)).toMatchObject({
    method: 'POST',
    body: {
      status: 'failed',
      revision: 0,
      error: 'Browser tab is not open in this session',
    },
  })
  expect(await browserRuntimeView(page, 'tab-2')).toMatchObject({
    url: 'https://secondary.example/',
    inspectCalls: 0,
  })
})

test('AI browser inspection fails promptly without reopening a closed Agent tab', async ({
  page,
}) => {
  const requests = await openDesktopClient(page, { existingSession: true })
  await expect.poll(() =>
    page.evaluate(
      () =>
        (window as Window & { __eventSources?: unknown[] }).__eventSources?.length ?? 0,
    ),
  ).toBeGreaterThan(0)

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({ type: 'browser_inspect_request', id: 'inspection-without-tab' })
  })

  const resultPath =
    '/api/sessions/test-session/browser/inspect/inspection-without-tab/result'
  await expect.poll(() =>
    requests.filter((request) => request.path === resultPath).length,
  ).toBe(1)
  expect(requests.find((request) => request.path === resultPath)).toMatchObject({
    method: 'POST',
    body: {
      status: 'failed',
      revision: 0,
      error: 'Browser tab is not open',
    },
  })
  await expect(page.getByTestId('browser-view')).toHaveCount(0)
  expect(await browserRuntimeView(page, 'preview:test-session')).toBeUndefined()
})

test('browser tools use page-focused labels and keep inspection text collapsed', async ({
  page,
}) => {
  await openDesktopClient(page, { existingSession: true })
  await expect.poll(() =>
    page.evaluate(
      () =>
        (window as Window & { __eventSources?: unknown[] }).__eventSources?.length ?? 0,
    ),
  ).toBeGreaterThan(0)

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_start',
      id: 'open-browser-ui',
      tool: 'open_preview',
      args: { url: 'https://github.com/' },
    })
  })
  await expect(page.getByText('Opening', { exact: true })).toBeVisible()
  await expect(page.getByText('github.com', { exact: true })).toBeVisible()

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_end',
      id: 'open-browser-ui',
      tool: 'open_preview',
      result: 'Opened preview at https://github.com/',
      outcome: {
        status: 'success',
        data: { url: 'https://github.com/', title: 'GitHub' },
      },
    })
  })
  await expect(page.getByText('Opened', { exact: true })).toBeVisible()

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_start',
      id: 'tabs-context-ui',
      tool: 'tabs_context',
      args: {},
    })
  })

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_start',
      id: 'inspect-browser-ui',
      tool: 'inspect_browser',
      args: {},
    })
  })
  const stepSummary = page.getByText(
    'Opened a browser page, checked browser tabs, read a browser page',
    { exact: true },
  )
  await expect(stepSummary).toBeVisible()
  await stepSummary.click()
  await expect(page.getByText('Checking', { exact: true })).toBeVisible()
  await expect(page.getByText('browser tabs', { exact: true })).toBeVisible()
  await expect(page.getByText('Reading', { exact: true })).toBeVisible()
  await expect(page.getByText('current page', { exact: true })).toBeVisible()

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_end',
      id: 'tabs-context-ui',
      tool: 'tabs_context',
      result: '{"tabs":[]}',
    })
  })
  await expect(page.getByText('Checked', { exact: true })).toBeVisible()

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_end',
      id: 'inspect-browser-ui',
      tool: 'inspect_browser',
      result: [
        'Browser URL: https://github.com/',
        'Title: GitHub',
        'Page status: ready',
        'Visible text:',
        'Repositories',
        'Issues',
      ].join('\n'),
    })
  })
  await expect(page.getByText('Read', { exact: true })).toBeVisible()
  await expect(page.getByText('github.com', { exact: true })).toHaveCount(2)
  await expect(page.getByText('https://github.com/', { exact: true })).toHaveCount(0)

  await page.getByRole('button', { name: /Read github\.com/ }).click()
  await expect(page.getByRole('region', { name: 'Visible page text' })).toContainText(
    'Repositories',
  )
  await expect(page.getByText('https://github.com/', { exact: true })).toBeVisible()
})

test('AI browser applies the latest agent navigation and never renavigates on hide', async ({
  page,
}) => {
  await openDesktopClient(page, { existingSession: true })
  await expect.poll(() =>
    page.evaluate(
      () =>
        (window as Window & { __eventSources?: unknown[] }).__eventSources?.length ?? 0,
    ),
  ).toBeGreaterThan(0)

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_end',
      id: 'github-preview',
      tool: 'open_preview',
      result: 'Opened preview at https://github.com',
      outcome: {
        status: 'success',
        data: { url: 'https://github.com', title: 'GitHub' },
      },
    })
  })
  await expect.poll(async () =>
    (await browserRuntimeView(page, 'preview:test-session'))?.url,
  ).toBe('https://github.com/')

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_end',
      id: 'bilibili-preview',
      tool: 'open_preview',
      result: 'Opened preview at https://www.bilibili.com',
      outcome: {
        status: 'success',
        data: { url: 'https://www.bilibili.com', title: 'Bilibili' },
      },
    })
  })
  await expect.poll(async () =>
    (await browserRuntimeView(page, 'preview:test-session'))?.url,
  ).toBe('https://www.bilibili.com/')
  const navigated = await browserRuntimeView(page, 'preview:test-session')
  expect(navigated?.loadCalls).toEqual([
    'https://github.com/',
    'https://www.bilibili.com/',
  ])
  await expect(page.getByRole('textbox', { name: 'Address' })).toHaveValue(
    'https://www.bilibili.com/',
  )
  await expect(page.getByRole('tab', { name: 'Bilibili' })).toHaveAttribute(
    'aria-selected',
    'true',
  )

  const toggle = page.getByTestId('workbench-panel-toggle')
  await toggle.click()
  await expect.poll(async () =>
    (await browserRuntimeView(page, 'preview:test-session'))?.visible,
  ).toBe(false)
  await toggle.click()
  await expect.poll(async () =>
    (await browserRuntimeView(page, 'preview:test-session'))?.visible,
  ).toBe(true)
  expect((await browserRuntimeView(page, 'preview:test-session'))?.loadCalls).toEqual(
    navigated?.loadCalls,
  )
})

test('streaming tool input shows write progress without duplicating the tool row', async ({
  page,
}) => {
  await openDesktopClient(page, { existingSession: true })
  await expect.poll(() =>
    page.evaluate(
      () =>
        (window as Window & { __eventSources?: unknown[] }).__eventSources?.length ?? 0,
    ),
  ).toBeGreaterThan(0)

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_input_start',
      tool: 'write',
      toolContentIndex: 0,
    })
    emit?.({
      type: 'tool_input_delta',
      tool: 'write',
      toolContentIndex: 0,
      delta: '{"path":"src/main.go","content":"one',
      bytes: 1024,
    })
    emit?.({
      type: 'tool_input_delta',
      id: 'write-call',
      tool: 'write',
      toolContentIndex: 0,
      delta: '\\ntwo',
      bytes: 512,
    })
  })

  await expect(page.getByText('Preparing file content')).toBeVisible()
  await expect(page.getByText('1.5 KB')).toBeVisible()
  await page.getByText('Preparing file content').click()
  await expect(
    page.getByText('{"path":"src/main.go","content":"one\\ntwo', { exact: true }),
  ).toBeVisible()

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_input_end',
      id: 'write-call',
      tool: 'write',
      toolContentIndex: 0,
      args: { path: 'src/main.go', content: 'one\ntwo\nthree' },
    })
    emit?.({
      type: 'tool_start',
      id: 'write-call',
      tool: 'write',
      args: { path: 'src/main.go', content: 'one\ntwo\nthree' },
    })
  })

  await expect(page.getByText('src/main.go', { exact: true })).toHaveCount(1)
  await expect(page.getByText('3 lines')).toBeVisible()

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_end',
      id: 'write-call',
      tool: 'write',
      result: 'Created src/main.go',
      outcome: {
        status: 'success',
        data: {
          changeType: 'file',
          path: 'src/main.go',
          op: 'create',
          additions: 3,
          deletions: 0,
          bytes: 13,
          hunks: [],
        },
      },
    })
    emit?.({
      type: 'tool_input_start',
      id: 'abandoned-call',
      tool: 'write',
      toolContentIndex: 1,
    })
  })
  await expect(page.getByText('Preparing file content')).toBeVisible()

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({ type: 'done' })
  })
  await expect(page.getByText('Preparing file content')).toHaveCount(0)
})

test('a leading bold reasoning line becomes the collapsible thinking title', async ({ page }) => {
  await openDesktopClient(page, {
    existingSession: true,
    historyEvents: [
      { type: 'user_message', text: 'Review the workspace' },
      { type: 'run_start', startedAt: '2026-08-02T12:00:00Z' },
      {
        type: 'delta',
        kind: 'thinking',
        delta:
          '**Considering workspace modifications**\n\nI need to inspect the **current files** first.',
      },
      { type: 'delta', kind: 'text', delta: 'I reviewed the workspace.' },
      { type: 'message_end', text: 'I reviewed the workspace.', finalResponse: true },
    ],
  })

  const title = page.getByRole('button', {
    name: 'Considering workspace modifications',
    exact: true,
  })
  await expect(title).toBeVisible()
  await expect(page.getByText('**Considering workspace modifications**')).toHaveCount(0)

  await title.click()
  await expect(page.getByText('I need to inspect the current files first.')).toBeVisible()
  await expect(page.getByText('current files', { exact: true })).toHaveCSS('font-weight', '600')
})

test('history restores in-flight assistant text and tool input after reload', async ({ page }) => {
  await openDesktopClient(page, {
    existingSession: true,
    historyRunning: true,
    historyEventSeq: 44,
    historyEvents: [
      { type: 'user_message', text: 'Update the file' },
      { type: 'run_start', startedAt: '2026-07-27T12:00:00Z' },
      { type: 'delta', kind: 'text', delta: 'Restored partial answer' },
      { type: 'tool_input_start', tool: 'write', toolContentIndex: 0 },
      {
        type: 'tool_input_delta',
        tool: 'write',
        toolContentIndex: 0,
        bytes: 1536,
      },
    ],
  })

  await expect(page.getByText('Restored partial answer')).toBeVisible()
  await expect(page.getByText('Preparing file content')).toBeVisible()
  await expect(page.getByText('1.5 KB')).toBeVisible()

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_input_end',
      id: 'restored-write',
      tool: 'write',
      toolContentIndex: 0,
      args: { path: 'README.md', content: 'updated' },
    })
    emit?.({ type: 'message_end', text: 'Restored partial answer' })
    emit?.({
      type: 'tool_start',
      id: 'restored-write',
      tool: 'write',
      args: { path: 'README.md', content: 'updated' },
    })
    emit?.({
      type: 'tool_end',
      id: 'restored-write',
      tool: 'write',
      result: 'Updated README.md',
      outcome: {
        status: 'success',
        data: {
          changeType: 'file',
          path: 'README.md',
          op: 'update',
          additions: 1,
          deletions: 0,
          bytes: 7,
          hunks: [],
        },
      },
    })
  })

  await expect(page.getByText('Restored partial answer')).toHaveCount(1)
  await expect(page.getByText('README.md', { exact: true })).toHaveCount(1)
})

test('AI preview opens workspace HTML directly without starting or probing a server', async ({
  page,
}) => {
  const requests = await openDesktopClient(page, { existingSession: true })
  await expect.poll(() =>
    page.evaluate(
      () =>
        (window as Window & { __eventSources?: unknown[] }).__eventSources?.length ?? 0,
    ),
  ).toBeGreaterThan(0)

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_end',
      id: 'preview-static',
      tool: 'open_preview',
      result: 'Opened preview at /tmp/test-session/web/index.html',
      outcome: {
        status: 'success',
        data: {
          path: '/tmp/test-session/web/index.html',
          relativePath: 'web/index.html',
          grantID: 'test-grant',
          previewPath: 'index.html',
          title: 'Static page',
        },
      },
    })
  })

  await expect.poll(async () =>
    (await browserRuntimeView(page, 'preview:test-session'))?.url.endsWith(
      '/api/sessions/test-session/previews/test-grant/index.html',
    ),
  ).toBe(true)
  await expect.poll(async () =>
    (await browserRuntimeView(page, 'preview:test-session'))?.visible,
  ).toBe(true)
  await expect(page.locator('webview')).toHaveAttribute('partition', 'or-preview')
  await expect(page.getByRole('textbox', { name: 'Address' })).toHaveValue(
    '/tmp/test-session/web/index.html',
  )
  const openExternal = page.getByRole('button', { name: 'Open in system browser' })
  await expect(openExternal).toBeEnabled()
  await openExternal.click()
  await expect.poll(() =>
    page.evaluate(() => (window as Window & { __openedURL?: string }).__openedURL),
  ).toBe('file:///tmp/test-session/web/index.html')
  expect(requests.filter((request) => request.path === '/api/preview/check')).toHaveLength(0)
  expect(
    requests.filter(
      (request) =>
        request.path === '/api/sessions/test-session/previews/test-grant/index.html',
    ),
  ).toHaveLength(0)

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_end',
      id: 'edit-static',
      tool: 'edit',
      result: 'Updated web/index.html',
      outcome: {
        status: 'success',
        data: {
          changeType: 'file',
          path: 'web/index.html',
          op: 'update',
          additions: 1,
          deletions: 1,
          bytes: 128,
          hunks: [],
        },
      },
    })
  })
  await expect.poll(async () =>
    (await browserRuntimeView(page, 'preview:test-session'))?.reloadCalls,
  ).toBe(1)
})

test('Browser replaces a failed local preview probe with a retry state', async ({ page }) => {
  const requests = await openDesktopClient(page, { existingSession: true })
  await page.getByTestId('workbench-panel-toggle').click()
  await page.getByTestId('workbench-panel').getByRole('button', { name: 'Add view' }).click()
  await page.getByRole('menu').getByRole('menuitem', { name: 'Browser' }).click()

  const address = page.getByRole('textbox', { name: 'Address' })
  await address.fill('127.0.0.1:4311')
  await address.press('Enter')

  await expect(page.getByRole('alert')).toContainText('Page unavailable')
  expect((await browserRuntimeView(page, 'tab-1'))?.loadCalls).toEqual([])
  await expect(page.getByRole('alert')).toContainText(
    'Check that the local server is running, then try again.',
  )
  await expect(page.getByRole('button', { name: 'Retry' })).toBeVisible()
  await expect(page.getByRole('status')).toHaveCount(0)

  await page.getByRole('button', { name: 'Retry' }).click()
  await expect.poll(
    () => requests.filter((request) => request.path === '/api/preview/check').length,
  ).toBe(2)
  await expect(page.getByRole('alert')).toContainText('Page unavailable')
})

test('long threads keep the titlebar and Composer fixed while the transcript scrolls', async ({
  page,
}) => {
  await openDesktopClient(page, {
    existingSession: true,
    historyEvents: longThreadHistory('main'),
  })

  const header = page.getByTestId('conversation-header')
  const transcript = page.getByTestId('conversation-transcript')
  const composer = page.getByTestId('composer')
  const viewport = page.viewportSize()
  const headerBox = await header.boundingBox()
  const composerBox = await composer.boundingBox()
  const scrollSize = await transcript.evaluate((element) => ({
    clientHeight: element.clientHeight,
    scrollHeight: element.scrollHeight,
  }))

  expect(viewport).not.toBeNull()
  expect(headerBox).not.toBeNull()
  expect(composerBox).not.toBeNull()
  expect(headerBox!.y).toBeGreaterThanOrEqual(0)
  expect(headerBox!.height).toBe(45)
  expect(composerBox!.y + composerBox!.height).toBeLessThanOrEqual(viewport!.height)
  expect(scrollSize.clientHeight).toBeGreaterThan(0)
  expect(scrollSize.scrollHeight).toBeGreaterThan(scrollSize.clientHeight)

  await transcript.evaluate((element) => {
    element.scrollTop = 0
  })
  await transcript.hover()
  await page.mouse.wheel(0, 480)
  await expect.poll(() => transcript.evaluate((element) => element.scrollTop)).toBeGreaterThan(0)
  await page.mouse.wheel(0, -10_000)
  await expect.poll(() => transcript.evaluate((element) => element.scrollTop)).toBe(0)

  await expect(
    page.getByRole('button', { name: 'Jump to latest', exact: true }),
  ).toBeVisible()
  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'run_start',
      id: 'new-main-run',
      startedAt: '2026-07-30T12:00:00Z',
    })
    emit?.({ type: 'delta', kind: 'text', delta: 'A newly streamed response' })
  })

  const jumpToLatest = page.getByRole('button', {
    name: 'New content available. Jump to latest',
    exact: true,
  })
  await expect(jumpToLatest).toBeVisible()
  await jumpToLatest.click()
  await expect
    .poll(() =>
      transcript.evaluate(
        (element) => element.scrollHeight - element.scrollTop - element.clientHeight,
      ),
    )
    .toBeLessThan(2)
  await expect(jumpToLatest).toHaveCount(0)
})

test('streaming output does not reclaim a transcript after a small upward scroll', async ({
  page,
}) => {
  await openDesktopClient(page, {
    existingSession: true,
    historyEvents: longThreadHistory('streaming'),
    historyRunning: true,
  })

  const transcript = page.getByTestId('conversation-transcript')
  await transcript.hover()
  await page.mouse.wheel(0, -24)
  await expect
    .poll(() =>
      transcript.evaluate(
        (element) => element.scrollHeight - element.scrollTop - element.clientHeight,
      ),
    )
    .toBeGreaterThan(2)

  const pausedScrollTop = await transcript.evaluate((element) => element.scrollTop)
  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({ type: 'delta', kind: 'text', delta: 'Streaming without taking the scroll position' })
  })

  await expect(
    page.getByRole('button', {
      name: 'New content available. Jump to latest',
      exact: true,
    }),
  ).toBeVisible()
  await expect
    .poll(() => transcript.evaluate((element) => element.scrollTop))
    .toBeLessThanOrEqual(pausedScrollTop + 1)
})

test('right-panel conversations can return to new content in long threads', async ({ page }) => {
  await openDesktopClient(page, {
    existingSession: true,
    secondarySession: true,
    secondaryHistoryEvents: longThreadHistory('secondary'),
  })

  await page.getByRole('button', { name: 'Actions for Secondary task' }).click()
  await page.getByRole('menuitem', { name: 'Open in right panel' }).click()

  const conversation = page
    .getByTestId('workbench-panel')
    .getByTestId('workbench-conversation')
  const transcript = conversation.getByTestId('workbench-conversation-transcript')
  await expect(conversation.getByText('Response 18.')).toBeVisible()
  await transcript.evaluate((element) => {
    element.scrollTop = 0
    element.dispatchEvent(new Event('scroll'))
  })

  await expect(
    conversation.getByRole('button', { name: 'Jump to latest', exact: true }),
  ).toBeVisible()
  await page.evaluate(() => {
    const emit = (
      window as Window & {
        __emitSessionSSE?: (sessionID: string, payload: unknown) => void
      }
    ).__emitSessionSSE
    emit?.('secondary-session', {
      type: 'run_start',
      id: 'new-secondary-run',
      startedAt: '2026-07-30T12:00:00Z',
    })
    emit?.('secondary-session', {
      type: 'delta',
      kind: 'text',
      delta: 'New workbench output',
    })
  })

  const jumpToLatest = conversation.getByRole('button', {
    name: 'New content available. Jump to latest',
    exact: true,
  })
  await expect(jumpToLatest).toBeVisible()
  await jumpToLatest.click()
  await expect
    .poll(() =>
      transcript.evaluate(
        (element) => element.scrollHeight - element.scrollTop - element.clientHeight,
      ),
    )
    .toBeLessThan(2)
  await expect(jumpToLatest).toHaveCount(0)
})

test('user actions appear on hover while assistant actions stay visible', async ({ page }) => {
  const now = new Date()
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 10, 52)
  const earlier = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1, 8, 15)
  const expectedToday = new Intl.DateTimeFormat('en-US', {
    hour: 'numeric',
    minute: '2-digit',
  }).format(today)
  const expectedEarlier = new Intl.DateTimeFormat('en-US', {
    ...(earlier.getFullYear() === now.getFullYear() ? {} : { year: 'numeric' as const }),
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  }).format(earlier)

  await page.addInitScript(() => {
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: {
        writeText: async (value: string) => {
          localStorage.setItem('test.clipboard', value)
        },
      },
    })
  })
  await openDesktopClient(page, {
    existingSession: true,
    historyEvents: [
      {
        type: 'user_message',
        id: 'earlier-user',
        text: 'Earlier user message',
        images: [],
      },
      {
        type: 'run_start',
        id: 'earlier-run',
        startedAt: earlier.toISOString(),
        durationMs: 1000,
      },
      {
        type: 'message_end',
        text: 'Earlier assistant response',
        finalResponse: true,
        completedAt: new Date(earlier.getTime() + 1000).toISOString(),
      },
      {
        type: 'user_message',
        id: 'today-user',
        text: 'Copy this user message',
        images: [],
      },
      {
        type: 'run_start',
        id: 'today-run',
        startedAt: today.toISOString(),
        durationMs: 1000,
      },
      {
        type: 'message_end',
        text: 'Today assistant response',
        finalResponse: true,
        completedAt: new Date(today.getTime() + 1000).toISOString(),
      },
    ],
  })

  const earlierUser = page
    .getByTestId('user-message')
    .filter({ hasText: 'Earlier user message' })
  const todayUser = page
    .getByTestId('user-message')
    .filter({ hasText: 'Copy this user message' })
  const userActions = todayUser.getByTestId('user-message-actions')

  await expect(userActions).toHaveCSS('opacity', '0')
  await expect(userActions).toHaveCSS('transition-duration', '0s')
  await expect(userActions.locator('time')).toHaveText(expectedToday)
  await todayUser.hover()
  await expect(userActions).toHaveCSS('opacity', '1')
  await expect(userActions.locator('time')).toBeVisible()
  await todayUser.getByRole('button', { name: 'Copy', exact: true }).click()
  await expect.poll(() => page.evaluate(() => localStorage.getItem('test.clipboard'))).toBe(
    'Copy this user message',
  )
  await earlierUser.hover()
  await expect(earlierUser.getByTestId('user-message-actions').locator('time')).toHaveText(
    expectedEarlier,
  )

  const assistant = page
    .getByTestId('assistant-message')
    .filter({ hasText: 'Today assistant response' })
  const assistantActions = assistant.getByTestId('response-message-actions')
  await expect(assistantActions).toHaveCSS('opacity', '1')
  await expect(assistantActions.locator('time')).toHaveText(expectedToday)
  await expect(assistant.getByRole('button', { name: 'Copy response' })).toBeVisible()
})

test('response usage stays on one line and truncates when Chat is narrow', async ({ page }) => {
  await page.setViewportSize({ width: 960, height: 700 })
  await openDesktopClient(page, {
    existingSession: true,
    historyEvents: [
      {
        type: 'user_message',
        id: 'user-usage',
        text: 'Show a compact response footer',
        images: [],
      },
      {
        type: 'run_start',
        id: 'run-usage',
        startedAt: '2026-07-22T20:45:49Z',
        durationMs: 11000,
      },
      {
        type: 'message_end',
        text: 'Completed response',
        finalResponse: true,
        modelName: 'DeepSeek V4 Pro Extended Preview Model',
        completedAt: '2026-07-22T20:46:00Z',
        usage: {
          input: 750000,
          output: 11000,
          cacheRead: 0,
          cacheWrite: 0,
          totalTokens: 761000,
          cost: {
            input: 0.01,
            output: 0.003,
            cacheRead: 0,
            cacheWrite: 0,
            total: 0.013,
          },
        },
      },
    ],
  })
  const actions = page.getByTestId('response-actions')
  const usageTrigger = page.getByTestId('response-usage-trigger')
  const [actionsBox, usageTriggerBox] = await Promise.all([
    actions.boundingBox(),
    usageTrigger.boundingBox(),
  ])
  expect(actionsBox).not.toBeNull()
  expect(usageTriggerBox).not.toBeNull()
  expect(usageTriggerBox!.width).toBeLessThan(actionsBox!.width - 40)

  await page.getByTestId('workbench-panel-toggle').click()
  await page
    .getByTestId('workbench-viewport')
    .getByRole('button', { name: 'Restore workbench' })
    .click()
  await expect.poll(async () =>
    (await page.getByTestId('workbench-viewport').boundingBox())?.width,
  ).toBeGreaterThan(330)

  const summary = page.getByTestId('response-usage-summary')
  await expect(summary).toBeVisible()
  const layout = await summary.evaluate((element) => {
    const style = getComputedStyle(element)
    return {
      clientWidth: element.clientWidth,
      scrollWidth: element.scrollWidth,
      whiteSpace: style.whiteSpace,
      textOverflow: style.textOverflow,
    }
  })
  expect(layout.whiteSpace).toBe('nowrap')
  expect(layout.textOverflow).toBe('ellipsis')
  expect(layout.scrollWidth).toBeGreaterThan(layout.clientWidth)
  await expect(actions).toHaveCSS('overflow', 'hidden')
})

test('branching from an assistant response requires confirmation', async ({ page }) => {
  const requests = await openDesktopClient(page, {
    existingSession: true,
    historyEvents: [
      {
        type: 'user_message',
        messageID: 'user-branch',
        text: `Earlier context\n${'Line before the branch point\n'.repeat(42)}`,
        images: [],
      },
      {
        type: 'run_start',
        id: 'run-branch',
        startedAt: '2026-07-22T20:45:49Z',
        durationMs: 3000,
      },
      {
        type: 'delta',
        kind: 'thinking',
        delta: '**Thought process**\nReviewed the requested branch point.',
      },
      {
        type: 'message_end',
        messageID: 'assistant-branch',
        text: 'Response to branch from',
        finalResponse: true,
      },
      {
        type: 'user_message',
        messageID: 'user-after-branch',
        text: `Later context\n${'Line after the branch point\n'.repeat(42)}`,
        images: [],
      },
    ],
  })
  await page.setViewportSize({ width: 390, height: 720 })

  const branchButton = page.getByRole('button', { name: 'Branch from this response' })
  await branchButton.click()

  const dialog = page.getByRole('dialog', { name: 'Create a new branch?' })
  await expect(dialog).toBeVisible()
  await expect(dialog.getByTestId('branch-session-details')).toContainText('Conversation history')
  expect(requests.some((request) => request.path.endsWith('/forks'))).toBe(false)

  await dialog.getByRole('button', { name: 'Cancel' }).click()
  await expect(dialog).toBeHidden()
  expect(requests.some((request) => request.path.endsWith('/forks'))).toBe(false)

  await branchButton.click()
  await dialog.getByRole('button', { name: 'Create branch' }).click()
  await expect.poll(() => requests.find((request) => request.path.endsWith('/forks'))?.body).toEqual({
    messageID: 'assistant-branch',
    mode: 'after_assistant',
  })
  await expect(page.getByTestId('conversation-title')).toContainText('New session (branch)')

  const branchNavigation = page.getByTestId('conversation-branch-navigation')
  await branchNavigation.getByRole('button', { name: 'Return to New session' }).click()
  await expect(page.getByTestId('conversation-title')).toContainText('New session')

  const branchPoint = page.locator(
    '[data-branch-point-message-id="assistant-branch"]',
  )
  await expect(branchPoint).toHaveAttribute('data-branch-point-highlighted', 'true')
  await expect(branchPoint).toContainText('Worked for 3s')
  await expect(branchPoint).toContainText('Thought process')
  await expect(branchPoint).toContainText('Response to branch from')
  expect(
    await branchPoint.evaluate((element) => getComputedStyle(element).backgroundColor),
  ).toBe(
    await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue('--surface-hover').trim(),
    ),
  )
  await expect(
    branchPoint.getByTestId('assistant-message'),
  ).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
  await expect.poll(async () => {
    const messageBox = await branchPoint.boundingBox()
    const transcriptBox = await page.getByTestId('conversation-transcript').boundingBox()
    return Boolean(
      messageBox &&
      transcriptBox &&
      messageBox.y >= transcriptBox.y &&
      messageBox.y + messageBox.height <= transcriptBox.y + transcriptBox.height,
    )
  }).toBe(true)

  await page.getByRole('button', { name: 'Open sessions' }).click()
  await page.getByRole('button', { name: 'Actions for New session', exact: true }).click()
  await page.getByRole('menuitem', { name: 'Delete' }).click()
  const deleteDialog = page.getByRole('dialog', { name: 'Delete session?' })
  await expect(deleteDialog).toContainText('1 branch will be kept as an independent session.')
  await deleteDialog.getByRole('button', { name: 'Cancel' }).click()
  await page.getByRole('button', { name: 'Collapse sidebar' }).click()

  await branchNavigation.getByRole('button', { name: 'View 1 branch' }).click()
  await page.getByRole('menuitem', { name: 'New session (branch)' }).click()
  await expect(page.getByTestId('conversation-title')).toContainText('New session (branch)')
})

test('editing a historical message rewrites the current session after confirmation', async ({ page }) => {
  const requests = await openDesktopClient(page, {
    existingSession: true,
    historyEvents: [
      {
        type: 'user_message',
        messageID: 'user-edit-target',
        text: 'Original question',
        images: [],
      },
      {
        type: 'message_end',
        messageID: 'assistant-old-answer',
        text: 'Old answer that should be removed',
        finalResponse: true,
      },
      {
        type: 'user_message',
        messageID: 'user-later-message',
        text: 'Later question that should be removed',
        images: [],
      },
    ],
  })

  const userMessage = page.getByTestId('user-message').filter({ hasText: 'Original question' })
  await userMessage.hover()
  await userMessage.getByRole('button', { name: 'Edit message' }).click()
  const editor = userMessage.getByRole('textbox', { name: 'Edit message' })
  await editor.fill('Rewritten question')
  await page.getByRole('button', { name: 'Send edited message' }).click()

  const dialog = page.getByRole('dialog', { name: 'Edit this message?' })
  await expect(dialog).toBeVisible()
  await expect(dialog).toContainText('Workspace files will stay as they are.')
  expect(requests.some((request) => request.path.endsWith('/message-edits'))).toBe(false)

  await dialog.getByRole('button', { name: 'Edit and continue' }).click()
  await expect.poll(() => requests.find((request) => request.path.endsWith('/message-edits'))?.body).toEqual({
    messageID: 'user-edit-target',
    text: 'Rewritten question',
  })
  await expect(dialog).toBeHidden()
  await expect(page.getByTestId('conversation-title')).toContainText('New session')
  await expect(page.getByText('Rewritten question')).toBeVisible()
  await expect(page.getByText('Old answer that should be removed')).toHaveCount(0)
  await expect(page.getByText('Later question that should be removed')).toHaveCount(0)
  expect(requests.filter((request) => request.path.endsWith('/forks'))).toHaveLength(0)
  expect(requests.filter((request) => request.path === '/api/sessions' && request.method === 'POST')).toHaveLength(0)
})

test('a branch returns without locating a message after its source history is edited', async ({ page }) => {
  await openDesktopClient(page, {
    existingSession: true,
    historyEvents: [
      {
        type: 'user_message',
        messageID: 'user-branch-origin',
        text: 'Question before branching',
        images: [],
      },
      {
        type: 'message_end',
        messageID: 'assistant-branch',
        text: 'Answer used to create a branch',
        finalResponse: true,
      },
    ],
  })

  await page.getByRole('button', { name: 'Branch from this response' }).click()
  await page.getByRole('dialog', { name: 'Create a new branch?' })
    .getByRole('button', { name: 'Create branch' })
    .click()
  const branchNavigation = page.getByTestId('conversation-branch-navigation')
  await branchNavigation.getByRole('button', { name: 'Return to New session' }).click()

  const userMessage = page.getByTestId('user-message').filter({ hasText: 'Question before branching' })
  await userMessage.hover()
  await userMessage.getByRole('button', { name: 'Edit message' }).click()
  await userMessage.getByRole('textbox', { name: 'Edit message' }).fill('Replacement branch point')
  await page.getByRole('button', { name: 'Send edited message' }).click()
  await page.getByRole('dialog', { name: 'Edit this message?' })
    .getByRole('button', { name: 'Edit and continue' })
    .click()
  await expect(page.getByText('Replacement branch point')).toBeVisible()

  await branchNavigation.getByRole('button', { name: 'View 1 branch' }).click()
  await page.getByRole('menuitem', { name: 'New session (branch)' }).click()
  await expect(branchNavigation.getByText('Original branch point edited')).toBeVisible()
  await branchNavigation.getByRole('button', { name: 'Return to New session' }).click()

  const replacement = page.locator('[data-branch-point-message-id="edited-user-message"]')
  await expect(replacement).toContainText('Replacement branch point')
  await expect(replacement).not.toHaveAttribute('data-branch-point-highlighted', 'true')
})

test('unknown provider input is shown as unavailable', async ({ page }) => {
  await openDesktopClient(page, {
    existingSession: true,
    historyEvents: [
      {
        type: 'message_end',
        text: 'Completed response',
        finalResponse: true,
        usage: {
          input: 0,
          inputUnknown: true,
          output: 3486,
          cacheRead: 21224,
          cacheWrite: 0,
          totalTokens: 24710,
          cost: { input: 0, output: 0.0042, cacheRead: 0.0013, cacheWrite: 0, total: 0.0055 },
        },
      },
    ],
  })

  await page.getByTestId('response-usage-trigger').hover()
  const tooltip = page.getByRole('tooltip')
  await expect(tooltip).toContainText(/Uncached input\s*--/)
  await expect(tooltip).not.toContainText('Cache hit')
})

test('Composer controls stay separate and compact when Chat is narrow', async ({ page }) => {
  await page.setViewportSize({ width: 960, height: 700 })
  await openDesktopClient(page, {
    existingSession: true,
    modelName: 'DeepSeek V4 Pro Extended Preview Model',
  })
  await page.getByTestId('workbench-panel-toggle').click()
  await page
    .getByTestId('workbench-viewport')
    .getByRole('button', { name: 'Restore workbench' })
    .click()
  await expect.poll(async () =>
    (await page.getByTestId('workbench-viewport').boundingBox())?.width,
  ).toBeGreaterThan(330)

  const composer = page.getByTestId('composer')
  const permission = page.getByTestId('permission-mode-trigger')
  const model = page.getByTestId('model-settings-trigger')
  const send = page.getByTestId('composer-send')
  const [composerBox, permissionBox, modelBox, sendBox] = await Promise.all([
    composer.boundingBox(),
    permission.boundingBox(),
    model.boundingBox(),
    send.boundingBox(),
  ])

  expect(composerBox).not.toBeNull()
  expect(permissionBox).not.toBeNull()
  expect(modelBox).not.toBeNull()
  expect(sendBox).not.toBeNull()
  expect(permissionBox!.x + permissionBox!.width).toBeLessThanOrEqual(modelBox!.x)
  expect(modelBox!.x + modelBox!.width).toBeLessThanOrEqual(sendBox!.x)
  expect(sendBox!.x + sendBox!.width).toBeLessThanOrEqual(
    composerBox!.x + composerBox!.width,
  )
  expect(modelBox!.width).toBeGreaterThan(40)
  await expect(permission.locator('.lucide-chevron-down')).toHaveCount(0)
  await expect(model.locator('.lucide-chevron-down')).toHaveCount(0)

  await expect(permission).toHaveCSS('color', 'rgb(138, 139, 141)')
  await expect(page.getByTestId('model-settings-name')).toHaveCSS(
    'color',
    'rgb(138, 139, 141)',
  )
  await expect(page.getByTestId('model-settings-effort')).toBeHidden()
  await expect(page.getByTestId('permission-mode-label')).toBeVisible()
  await expect(page.getByTestId('model-settings-name')).toHaveCSS('text-overflow', 'ellipsis')
  const modelNameLayout = await page.getByTestId('model-settings-name').evaluate((element) => ({
    clientWidth: element.clientWidth,
    scrollWidth: element.scrollWidth,
    whiteSpace: getComputedStyle(element).whiteSpace,
  }))
  expect(modelNameLayout.whiteSpace).toBe('nowrap')
  expect(modelNameLayout.scrollWidth).toBeGreaterThan(modelNameLayout.clientWidth)
})

test('Composer context ring opens the measured context breakdown', async ({ page }) => {
  await openDesktopClient(page, {
    existingSession: true,
    contextUsage: {
      provider: 'openai',
      model: 'test-model',
      usedTokens: 64_000,
      contextWindow: 128_000,
      measured: true,
      breakdown: {
        messages: 40_000,
        systemTools: 10_000,
        systemPrompt: 6_000,
        skills: 4_000,
        projectContext: 4_000,
      },
    },
  })

  const trigger = page.getByTestId('context-window-trigger')
  const composerSurface = page.getByTestId('composer-surface')
  const modelTrigger = page.getByTestId('model-settings-trigger')
  const sendButton = page.getByTestId('composer-send')
  await expect(trigger).toBeVisible()
  await expect(trigger).toHaveAccessibleName('Model context window usage')
  await expect(trigger).toHaveAttribute('aria-expanded', 'false')
  const [composerBox, modelBox, triggerBox, sendBox] = await Promise.all([
    composerSurface.boundingBox(),
    modelTrigger.boundingBox(),
    trigger.boundingBox(),
    sendButton.boundingBox(),
  ])
  expect(composerBox).not.toBeNull()
  expect(modelBox).not.toBeNull()
  expect(triggerBox).not.toBeNull()
  expect(sendBox).not.toBeNull()
  expect(triggerBox!.x).toBeGreaterThanOrEqual(composerBox!.x)
  expect(triggerBox!.x + triggerBox!.width).toBeLessThanOrEqual(composerBox!.x + composerBox!.width)
  expect(triggerBox!.x + triggerBox!.width).toBeLessThanOrEqual(modelBox!.x)
  expect(triggerBox!.x + triggerBox!.width).toBeLessThanOrEqual(sendBox!.x)

  await trigger.click()
  await expect(trigger).toHaveAttribute('aria-expanded', 'true')
  const summary = page.getByTestId('context-window-summary')
  await expect(summary).toContainText('Context window')
  await expect(summary).toContainText(/64k\s*\/\s*128k\s*·\s*50%/)
  await expect(page.getByTestId('context-window-ring')).toHaveAttribute('stroke-dasharray', '50 100')
  const breakdown = page.getByTestId('context-window-breakdown')
  await expect(breakdown).toBeVisible()
  await expect(breakdown).toContainText('Estimated breakdown')
  await expect(page.getByTestId('context-breakdown-messages')).toContainText(/Messages\s*40k\s*31\.3%/)
  await expect(page.getByTestId('context-breakdown-tools')).toContainText(/System tools\s*10k\s*7\.8%/)
  await expect(page.getByTestId('context-breakdown-prompt')).toContainText(/System prompt\s*6k\s*4\.7%/)
  await expect(page.getByTestId('context-breakdown-skills')).toContainText(/Skills\s*4k\s*3\.1%/)
  await expect(page.getByTestId('context-breakdown-project')).toContainText(/Project context\s*4k\s*3\.1%/)
  await expect(page.getByTestId('context-breakdown-free')).toContainText(/Free space\s*64k\s*50\.0%/)
})

test('Composer shows the full model name when space is available', async ({ page }) => {
  const modelName = 'DeepSeek V4 Flash Chat Preview'
  await openDesktopClient(page, { existingSession: true, modelName })

  const label = page.getByTestId('model-settings-name')
  await expect(label).toHaveText(modelName)
  await expect(label).toHaveAttribute('title', modelName)
  const layout = await label.evaluate((element) => ({
    clientWidth: element.clientWidth,
    scrollWidth: element.scrollWidth,
  }))
  expect(layout.scrollWidth).toBe(layout.clientWidth)
})

test('Models settings show full configured model names when space is available', async ({
  page,
}) => {
  const modelName = 'DeepSeek V4 Flash (New)'
  await page.setViewportSize({ width: 1440, height: 800 })
  await openDesktopClient(page, { existingSession: true, modelName })

  await page.getByRole('button', { name: 'Open profile menu' }).click()
  await page.getByRole('menuitem', { name: 'Settings' }).click()
  await page.getByRole('button', { name: 'Models', exact: true }).click()

  const labels = page.getByTitle(modelName)
  await expect(labels).toHaveCount(1)
  for (const label of await labels.all()) {
    const layout = await label.evaluate((element) => ({
      clientWidth: element.clientWidth,
      scrollWidth: element.scrollWidth,
    }))
    expect(layout.scrollWidth).toBe(layout.clientWidth)
  }
  const controls = page.getByTestId('default-model-controls').locator('button')
  await expect(controls.first()).toHaveCSS('border-radius', '10px')
  await expect(controls.nth(1)).toHaveCSS('border-radius', '10px')
  const gap = await controls.evaluateAll(([first, second]) => {
    const firstRect = first.getBoundingClientRect()
    const secondRect = second.getBoundingClientRect()
    return secondRect.left - firstRect.right
  })
  expect(gap).toBeGreaterThan(0)

  const defaultsLayout = page.getByTestId('model-defaults-section').locator(':scope > div')
  await expect(defaultsLayout).toHaveCSS('border-top-width', '0px')
  await expect(defaultsLayout).toHaveCSS('border-bottom-width', '0px')
  const connectionHeader = page
    .getByTestId('connection-editor-panel')
    .locator(':scope > section > div')
    .first()
  await expect(connectionHeader).toHaveCSS('border-bottom-width', '0px')
})

test('fixed hidden thinking is shown as a read-only model capability', async ({ page }) => {
  await openDesktopClient(page, {
    existingSession: true,
    modelThinkingLevels: ['high'],
    modelThinkingVisibility: 'hidden',
  })

  const trigger = page.getByTestId('model-settings-trigger')
  const compactStatus = trigger.getByTestId('fixed-thinking-status')
  await expect(compactStatus).toBeVisible()
  await expect(compactStatus).toHaveText('')
  await trigger.click()

  const menu = page.getByRole('menu')
  const status = menu.getByTestId('fixed-thinking-status')
  await expect(status).toHaveText('Fixed thinking')
  await expect(menu.getByRole('menuitem', { name: /Effort/ })).toHaveCount(0)

  await status.hover()
  await expect(page.getByRole('tooltip')).toContainText(
    'The model reasons internally, but the provider does not return its reasoning process.',
  )
})

test('fixed provider thinking is shown without an effort menu', async ({ page }) => {
  await openDesktopClient(page, {
    existingSession: true,
    modelThinkingLevels: ['high'],
    modelThinkingVisibility: 'visible',
  })

  const trigger = page.getByTestId('model-settings-trigger')
  await trigger.click()

  const menu = page.getByRole('menu')
  const status = menu.getByTestId('fixed-thinking-status')
  await expect(status).toHaveText('Fixed thinking')
  await expect(menu.getByRole('menuitem', { name: /Effort/ })).toHaveCount(0)

  await status.hover()
  await expect(page.getByRole('tooltip')).toContainText(
    "This model's thinking mode is fixed by the provider and cannot be changed.",
  )
})

test('binary thinking capability is presented as an off/on switch', async ({ page }) => {
  const requests = await openDesktopClient(page, {
    existingSession: true,
    modelThinkingLevels: ['off', 'high'],
    composerUpdateDelayMs: 500,
  })

  const trigger = page.getByTestId('model-settings-trigger')
  await expect(page.getByTestId('model-settings-effort')).toHaveText('Off')
  await trigger.click()

  const toggle = page.getByTestId('model-thinking-toggle')
  await expect(toggle).toHaveRole('switch')
  await expect(toggle).toHaveText('Off')
  await expect(toggle).toHaveAttribute('aria-checked', 'false')

  await page.getByTestId('model-thinking-row').click({ position: { x: 5, y: 15 } })
  expect(
    requests.filter(
      (request) =>
        request.path === '/api/sessions/test-session/settings' &&
        request.method === 'PATCH',
    ),
  ).toHaveLength(0)
  await expect(page.getByRole('menu')).toBeVisible()

  await toggle.click()
  await expect(page.getByRole('menu')).toBeVisible()
  await expect(toggle).toBeDisabled()
  await expect(trigger).toBeDisabled()
  await expect.poll(() =>
    requests.find(
      (request) =>
        request.path === '/api/sessions/test-session/settings' && request.method === 'PATCH',
    )?.body,
  ).toEqual({ provider: 'openai', model: 'test-model', thinkingLevel: 'high' })
  await expect(toggle).toBeEnabled()
  await expect(page.getByRole('menu')).toBeVisible()
})

test('usage range refresh keeps request details mounted while new data loads', async ({ page }) => {
  const cost = { input: 0.001, output: 0.001, cacheRead: 0, cacheWrite: 0, total: 0.002 }
  const usageReport: UsageReport = {
    total: {
      requests: 2,
      input: 3000,
      output: 30,
      cacheRead: 0,
      cacheWrite: 0,
      totalTokens: 3030,
      cost,
    },
    models: [
      {
        provider: 'openai',
        model: 'test-model',
        name: 'Test model',
        lastUsedAt: '2026-07-22T10:00:00Z',
        requests: 2,
        input: 3000,
        output: 30,
        cacheRead: 0,
        cacheWrite: 0,
        totalTokens: 3030,
        cost,
      },
    ],
    generatedAt: '2026-07-22T10:00:00Z',
  }
  const usageEventPage = (id: string, input: number): UsageEventPage => ({
    events: [
      {
        id,
        sessionId: 'test-session',
        provider: 'openai',
        model: 'test-model',
        timestamp: '2026-07-22T10:00:00Z',
        usage: {
          input,
          output: 10,
          cacheRead: 0,
          cacheWrite: 0,
          totalTokens: input + 10,
          cost,
        },
      },
    ],
    total: 1,
    limit: 10,
    offset: 0,
  })

  await openDesktopClient(page, {
    existingSession: true,
    usageReport,
    usageEventPages: [usageEventPage('30-day-event', 1111), usageEventPage('7-day-event', 2222)],
    usageEventDelayMs: 500,
  })
  await page.getByRole('button', { name: 'Open profile menu' }).click()
  await page.getByRole('menuitem', { name: 'Settings' }).click()
  await page.getByRole('button', { name: 'Usage', exact: true }).click()
  await page.getByRole('button', { name: 'Test model', exact: true }).click()

  const previousRow = page.getByRole('cell', { name: '1,111', exact: true })
  await expect(previousRow).toBeVisible()
  await page.getByRole('button', { name: 'Usage time range' }).click()
  await page.getByRole('menuitemradio', { name: '7 days' }).click()

  expect(await previousRow.isVisible()).toBe(true)
  await expect(page.getByRole('cell', { name: '2,222', exact: true })).toBeVisible()
  await expect(previousRow).toBeHidden()
})

test('usage request column headers stay fixed while paging', async ({ page }) => {
  const cost = { input: 0.001, output: 0.001, cacheRead: 0, cacheWrite: 0, total: 0.002 }
  const usageReport: UsageReport = {
    total: {
      requests: 11,
      input: 10_900_017,
      output: 319,
      cacheRead: 0,
      cacheWrite: 0,
      totalTokens: 10_900_336,
      cost,
    },
    models: [
      {
        provider: 'openai',
        model: 'test-model',
        name: 'Test model',
        lastUsedAt: '2026-07-22T10:00:00Z',
        requests: 11,
        input: 10_900_017,
        output: 319,
        cacheRead: 0,
        cacheWrite: 0,
        totalTokens: 10_900_336,
        cost,
      },
    ],
    generatedAt: '2026-07-22T10:00:00Z',
  }
  const usageEventPage = (offset: number, inputs: number[]): UsageEventPage => ({
    events: inputs.map((input, index) => ({
      id: `event-${offset + index}`,
      sessionId: 'test-session',
      provider: 'openai',
      model: 'test-model',
      timestamp: new Date(Date.UTC(2026, 6, 22, 10, offset + index)).toISOString(),
      usage: {
        input,
        output: 29,
        cacheRead: 0,
        cacheWrite: 0,
        totalTokens: input + 29,
        cost,
      },
    })),
    total: 11,
    limit: 10,
    offset,
  })

  await openDesktopClient(page, {
    existingSession: true,
    usageReport,
    usageEventPagesByOffset: {
      0: usageEventPage(0, [9_999_999, 100_001, 100_002, 100_003, 100_004, 100_005, 100_006, 100_007, 100_008, 100_009]),
      10: usageEventPage(10, [17]),
    },
  })
  await page.getByRole('button', { name: 'Open profile menu' }).click()
  await page.getByRole('menuitem', { name: 'Settings' }).click()
  await page.getByRole('button', { name: 'Usage', exact: true }).click()
  await page.getByRole('button', { name: 'Test model', exact: true }).click()

  const details = page.getByTestId('usage-request-details')
  await expect(details.getByRole('cell', { name: '9,999,999', exact: true })).toBeVisible()
  const headerGeometry = () => details.getByRole('columnheader').evaluateAll((headers) =>
    headers.map((header) => {
      const box = header.getBoundingClientRect()
      return { x: box.x, y: box.y, width: box.width }
    }),
  )
  const scrollRegion = details.locator('[aria-busy]')
  await expect(scrollRegion).toHaveAttribute('aria-busy', 'false')
  const before = await headerGeometry()

  await scrollRegion.evaluate((element) => {
    element.scrollTop = element.scrollHeight
  })
  await expect.poll(() => scrollRegion.evaluate((element) => element.scrollTop)).toBeGreaterThan(0)
  const afterScroll = await headerGeometry()
  afterScroll.forEach((header, index) => {
    expect(header.x).toBe(before[index]?.x)
    expect(header.width).toBe(before[index]?.width)
    expect(Math.abs(header.y - (before[index]?.y ?? header.y))).toBeLessThan(1)
  })
  const headerAlphas = await details.getByRole('columnheader').evaluateAll((headers) =>
    headers.map((header) => {
      const background = getComputedStyle(header).backgroundColor
      if (background === 'transparent') return 0
      const rgbaAlpha = background.match(/^rgba\([^,]+,[^,]+,[^,]+,\s*([\d.]+)\)$/)?.[1]
      return rgbaAlpha ? Number(rgbaAlpha) : 1
    }),
  )
  expect(headerAlphas.every((alpha) => alpha === 1)).toBe(true)

  await details.getByRole('button', { name: 'Next page' }).click()
  await expect(details.getByRole('cell', { name: '17', exact: true })).toBeVisible()

  const afterPaging = await headerGeometry()
  afterPaging.forEach((header, index) => {
    expect(header.x).toBe(before[index]?.x)
    expect(header.width).toBe(before[index]?.width)
  })
})

test('first send creates a session and renders the user message', async ({ page }) => {
  const requests = await openDesktopClient(page)
  const message = 'Desktop first-send regression'
  const composer = page.getByRole('textbox', { name: 'Ask anything' })
  const send = page.getByRole('button', { name: 'Send prompt' })

  await expect(composer).toBeVisible()
  await composer.fill(message)
  await send.click()

  await expect(page.getByRole('main').getByText(message, { exact: true })).toBeVisible()
  await expect.poll(() =>
    requests.find((request) => request.path === '/api/sessions' && request.method === 'POST')
      ?.body,
  ).toEqual({
    scope: 'chat',
    provider: 'openai',
    model: 'test-model',
    thinkingLevel: 'medium',
    permissionMode: 'ask',
  })
  await expect.poll(() =>
    requests.find((request) => request.path === '/api/sessions/test-session/prompt')?.body,
  ).toEqual({ text: message, images: [] })
})

test('sidebar session actions leave catalog pages for the conversation', async ({ page }) => {
  await openDesktopClient(page, { existingSession: true })
  const conversation = page.getByTestId('conversation-pane')
  const sidebar = page.locator('aside')

  await sidebar.getByRole('button', { name: 'Skills', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Skills', exact: true })).toBeVisible()
  await page
    .getByRole('navigation', { name: 'Chats' })
    .getByRole('button', { name: 'New session', exact: true })
    .click()
  await expect(conversation).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Skills', exact: true })).toBeHidden()

})

test('sidebar session hover shows conversation details with a small row gap', async ({
  page,
}) => {
  const title = 'Analyze message branching without changing the original conversation'
  await openDesktopClient(page, { existingSession: true, sessionTitle: title })

  const trigger = page.getByRole('button', { name: title, exact: true })
  await trigger.hover()

  const hoverCard = page.getByTestId('session-hover-card')
  await expect(hoverCard).toBeVisible()
  await expect(hoverCard.getByRole('heading', { name: title })).toBeVisible()
  await expect(hoverCard).toContainText('Model')
  await expect(hoverCard).toContainText('Test model')
  await expect(hoverCard).toContainText('Updated')
  await expect(hoverCard).not.toContainText('/tmp/test-session')

  const [triggerBox, hoverCardBox] = await Promise.all([
    trigger.boundingBox(),
    hoverCard.boundingBox(),
  ])
  expect(triggerBox).not.toBeNull()
  expect(hoverCardBox).not.toBeNull()
  expect(Math.abs((triggerBox?.x ?? 0) + (triggerBox?.width ?? 0) + 6 - (hoverCardBox?.x ?? 0))).toBeLessThan(1)
})

test('sidebar workspace hover shows project details with a small row gap', async ({
  page,
}) => {
  await openDesktopClient(page, { existingSession: true, sessionScope: 'project' })

  const trigger = page.getByRole('button', { name: 'test-session', exact: true })
  await trigger.hover()

  const hoverCard = page.getByTestId('workspace-hover-card')
  await expect(hoverCard).toBeVisible()
  await expect(hoverCard.getByRole('heading', { name: 'test-session' })).toBeVisible()
  await expect(hoverCard).toContainText('1 chat')
  await expect(hoverCard).toContainText('/tmp/test-session')
  await expect(page.getByTestId('session-hover-card')).toHaveCount(0)

  const contentLeftEdges = await Promise.all([
    hoverCard.getByRole('heading', { name: 'test-session' }).evaluate((element) => element.getBoundingClientRect().x),
    hoverCard.getByText('1 chat', { exact: true }).evaluate((element) => element.getBoundingClientRect().x),
    hoverCard.getByText('/tmp/test-session', { exact: true }).evaluate((element) => element.getBoundingClientRect().x),
  ])
  expect(Math.max(...contentLeftEdges) - Math.min(...contentLeftEdges)).toBeLessThan(1)

  const [triggerBox, hoverCardBox] = await Promise.all([
    trigger.boundingBox(),
    hoverCard.boundingBox(),
  ])
  expect(triggerBox).not.toBeNull()
  expect(hoverCardBox).not.toBeNull()
  expect(Math.abs((triggerBox?.x ?? 0) + (triggerBox?.width ?? 0) + 6 - (hoverCardBox?.x ?? 0))).toBeLessThan(1)
})

test('sidebar workspace action reveals the project in the native file manager', async ({
  page,
}) => {
  await openDesktopClient(page, { existingSession: true, sessionScope: 'project' })

  const project = page.getByRole('button', { name: 'test-session', exact: true })
  await project.hover()
  const actions = page.getByRole('button', { name: 'Actions for test-session' })
  await actions.click()
  await page.getByRole('menuitem', { name: 'Reveal in Finder' }).click()

  await expect.poll(() =>
    page.evaluate(() => (window as Window & { __revealedPath?: string }).__revealedPath),
  ).toBe('/tmp/test-session')
  await expect(actions).not.toBeFocused()
})

test('sidebar rename selects once and preserves normal backspace editing', async ({ page }) => {
  const title = 'Greeting session in Chinese'
  await openDesktopClient(page, { existingSession: true, sessionTitle: title })

  const sessionRow = page.getByRole('button', { name: title, exact: true })
  await sessionRow.hover()
  await page.getByRole('button', { name: `Actions for ${title}` }).click()
  await page.getByRole('menuitem', { name: 'Rename' }).click()

  const renameInput = page.getByRole('textbox', { name: `Rename ${title}` })
  await expect(renameInput).toBeVisible()
  await renameInput.press('End')
  await renameInput.press('Backspace')
  await expect(renameInput).toHaveValue(title.slice(0, -1))
  await renameInput.press('Backspace')
  await expect(renameInput).toHaveValue(title.slice(0, -2))
  const visibleShadows = await renameInput.evaluate((element) => {
    const shadow = getComputedStyle(element).boxShadow
    if (shadow === 'none') return []
    return [...shadow.matchAll(/rgba?\(([^)]+)\)/g)].filter(([, channels]) => {
      const values = channels?.split(',').map((value) => value.trim()) ?? []
      return values.length < 4 || Number(values[3]) > 0
    })
  })
  expect(visibleShadows).toHaveLength(0)
  await expect(renameInput).toHaveCSS('outline-style', 'none')
})

test('skill invocations render and copy as file references', async ({
  page,
}) => {
  const requests = await openDesktopClient(page, {
    existingSession: true,
    skills: [
      {
        name: 'frontend-design',
        description: 'Build polished interfaces',
        source: 'user',
        dir: '/tmp/skills/frontend-design',
        path: '/tmp/skills/frontend-design/SKILL.md',
      },
    ],
  })
  const composer = page.getByTestId('composer')
  const input = composer.locator('textarea')

  await input.fill('/front hello')
  const suggestions = page.getByRole('listbox', { name: 'Commands and resources' })
  const skill = suggestions.getByRole('option', { name: /frontend-design/ })
  await expect(skill).toBeVisible()
  await skill.click()

  await expect(composer.getByText('frontend-design', { exact: true })).toBeVisible()
  await expect(input).toHaveValue('hello')
  await input.press('Enter')

  const reference = page.getByTestId('conversation-transcript').getByTestId('skill-reference')
  await expect(reference).toContainText('frontend-design')
  await expect(reference).not.toContainText('$frontend-design')
  await expect(reference.locator('svg')).toBeVisible()
  await expect(reference).toHaveAttribute('title', '/tmp/skills/frontend-design/SKILL.md')
  await expect(reference.locator('xpath=..')).toContainText('hello')
  const copied = await reference.evaluate((element) => {
    const range = document.createRange()
    range.selectNodeContents(element)
    const selection = window.getSelection()
    selection?.removeAllRanges()
    selection?.addRange(range)
    const clipboardData = new DataTransfer()
    element.dispatchEvent(
      new ClipboardEvent('copy', { bubbles: true, cancelable: true, clipboardData }),
    )
    selection?.removeAllRanges()
    return clipboardData.getData('text/plain')
  })
  expect(copied).toBe(
    '[$frontend-design](/tmp/skills/frontend-design/SKILL.md)',
  )
  await expect.poll(() =>
    requests.find((request) => request.path === '/api/sessions/test-session/prompt')
      ?.body,
  ).toEqual({
    text: '[$frontend-design](/tmp/skills/frontend-design/SKILL.md) hello',
    images: [],
  })
})

test('composer slash catalog lists skills, refreshes, and follows keyboard navigation', async ({
  page,
}) => {
  const requests = await openDesktopClient(page, {
    existingSession: true,
    skills: Array.from({ length: 18 }, (_, index) => ({
      name: `skill-${index + 1}`,
      description: `Skill ${index + 1} description`,
      source: 'user' as const,
      dir: `/tmp/skills/skill-${index + 1}`,
    })),
  })
  const input = page.getByTestId('composer').locator('textarea')
  const skillRequests = () =>
    requests.filter((request) => request.path === '/api/skills').length
  const skillsBeforeOpen = skillRequests()
  await input.fill('$skill')
  await expect(page.getByRole('listbox', { name: 'Commands and resources' })).toBeHidden()
  await input.fill('/')

  const suggestions = page.getByRole('listbox', { name: 'Commands and resources' })
  await expect(suggestions.getByRole('option', { name: /skill-1/ })).toBeVisible()
  const scrollArea = suggestions.locator(':scope > div').first()
  await expect(suggestions.getByRole('option', { name: /Code review/ })).toBeVisible()
  await expect.poll(skillRequests).toBe(skillsBeforeOpen + 1)
  await expect.poll(() => scrollArea.evaluate((element) => element.scrollHeight)).toBeGreaterThan(
    await scrollArea.evaluate((element) => element.clientHeight),
  )

  const selectedOption = suggestions.locator('[role="option"][aria-selected="true"]')
  await expect(selectedOption).toContainText('Code review')
  await input.press('ArrowUp')
  await expect(selectedOption).toContainText('skill-8')
  await expect(selectedOption).toBeInViewport()
  expect(await scrollArea.evaluate((element) => element.scrollTop)).toBe(
    await scrollArea.evaluate((element) => element.scrollHeight - element.clientHeight),
  )
  expect(
    await selectedOption.evaluate((element) => getComputedStyle(element).transitionDuration),
  ).toBe('0s')
  expect(await suggestions.evaluate((element) => getComputedStyle(element).cursor)).toBe('default')
  expect(await selectedOption.evaluate((element) => getComputedStyle(element).cursor)).toBe(
    'pointer',
  )
  await selectedOption.dispatchEvent('mousemove', {
    bubbles: true,
    clientX: 400,
    clientY: 300,
    movementX: 0,
    movementY: 0,
  })
  expect(
    await selectedOption.evaluate((element) => getComputedStyle(element).transitionDuration),
  ).toBe('0s')
  await input.press('ArrowDown')
  await expect(selectedOption).toContainText('Code review')
  expect(await scrollArea.evaluate((element) => element.scrollTop)).toBe(0)
  for (const label of ['Compact', 'Continue in new chat', 'Plan mode', 'skill-1']) {
    await input.press('ArrowDown')
    await expect(selectedOption).toContainText(label)
  }
  for (const label of ['Plan mode', 'Continue in new chat', 'Compact', 'Code review']) {
    await input.press('ArrowUp')
    await expect(selectedOption).toContainText(label)
  }

  for (let index = 0; index < 11; index++) await input.press('ArrowDown')

  await expect.poll(() => scrollArea.evaluate((element) => element.scrollTop)).toBeGreaterThan(0)
  await expect(suggestions.locator('[role="option"][aria-selected="true"]')).toBeInViewport()

  for (let index = 0; index < 11; index++) await input.press('ArrowUp')

  await expect.poll(() => scrollArea.evaluate((element) => element.scrollTop)).toBe(0)

  await input.fill('ordinary text')
  await expect(suggestions).toBeHidden()
  await input.fill('/')
  await expect.poll(skillRequests).toBe(skillsBeforeOpen + 2)
})

test('composer slash catalog groups project skills before system skills without horizontal scroll', async ({
  page,
}) => {
  const requests = await openDesktopClient(page, {
    existingSession: true,
    skills: [
      {
        name: 'system-skill',
        description: 'A system skill with a description that should truncate inside the panel',
        source: 'user',
        dir: '/tmp/system-skills/system-skill',
      },
      {
        name: 'project-skill',
        description: 'A project skill with a description that should truncate inside the panel',
        source: 'project',
        dir: '/tmp/test-session/.agents/skills/project-skill',
      },
    ],
  })
  const input = page.getByTestId('composer').locator('textarea')
  await input.fill('/')

  const suggestions = page.getByRole('listbox', { name: 'Commands and resources' })
  const projectGroup = suggestions.getByRole('group', { name: 'Project skills' })
  const systemGroup = suggestions.getByRole('group', { name: 'System skills' })
  await expect(projectGroup.getByRole('option', { name: /project-skill/ })).toBeVisible()
  await expect(systemGroup.getByRole('option', { name: /system-skill/ })).toBeVisible()
  await expect.poll(() =>
    requests.some((request) =>
      request.url.includes('/api/skills?workspace=%2Ftmp%2Ftest-session'),
    ),
  ).toBe(true)
  expect(
    await suggestions.getByRole('group').evaluateAll((groups) =>
      groups.map((group) => group.getAttribute('aria-label')),
    ),
  ).toEqual(['Project skills', 'System skills'])

  const scrollArea = suggestions.locator(':scope > div').first()
  expect(await scrollArea.evaluate((element) => element.scrollWidth)).toBe(
    await scrollArea.evaluate((element) => element.clientWidth),
  )
  const commandRects = await suggestions
    .getByRole('option')
    .evaluateAll((options) => options.slice(0, 4).map((option) => {
      const rect = option.getBoundingClientRect()
      return { top: rect.top, bottom: rect.bottom, height: rect.height }
    }))
  expect(commandRects.every((rect) => Math.abs(rect.height - 30) < 0.1)).toBe(true)
  expect(commandRects.slice(1).every((rect, index) => rect.top >= commandRects[index].bottom)).toBe(
    true,
  )
})

test('failed first send keeps the draft and shows the server error', async ({ page }) => {
  await openDesktopClient(page, { failCreate: true })
  const message = 'Keep this draft after a failed send'
  const composer = page.getByRole('textbox', { name: 'Ask anything' })

  await composer.fill(message)
  await page.getByRole('button', { name: 'Send prompt' }).click()

  await expect(composer).toHaveValue(message)
  await expect(page.getByRole('alert')).toHaveText('invalid session settings')
  await expect(
    page
      .getByTestId('conversation-transcript')
      .locator('section')
      .getByText(message, { exact: true }),
  ).toHaveCount(0)
})

test('Composer adds and removes a text attachment from the add menu', async ({ page }) => {
  await openDesktopClient(page, { existingSession: true })

  await page.getByRole('button', { name: 'Add content' }).click()
  const fileChooser = page.waitForEvent('filechooser')
  await page.getByRole('option', { name: /Add files/ }).click()
  await (await fileChooser).setFiles({
    name: 'main.go',
    mimeType: 'text/plain',
    buffer: Buffer.from('package main\n'),
  })

  const composer = page.getByTestId('composer')
  await expect(composer.getByText('main.go')).toBeVisible()
  await expect(composer.getByText('13 B')).toBeVisible()
  await composer.getByRole('button', { name: 'Remove main.go' }).click()
  await expect(composer.getByText('main.go')).toBeHidden()
})

for (const setting of ['permission', 'model'] as const) {
  test(`Composer keeps the editor stable while ${setting} settings update`, async ({ page }) => {
    await openDesktopClient(page, {
      existingSession: true,
      composerUpdateDelayMs: 500,
      modelThinkingLevels: ['off', 'high'],
    })

    const composer = page.getByTestId('composer')
    const input = composer.locator('textarea')
    const addContent = composer.locator('button[aria-label="Add content"]')
    const permission = composer.getByTestId('permission-mode-trigger')
    const model = composer.getByTestId('model-settings-trigger')
    const send = composer.getByTestId('composer-send')
    const draft = `Keep this draft during the ${setting} update`
    await input.fill(draft)

    if (setting === 'permission') {
      await permission.click()
      await page.getByRole('menuitemradio', { name: /Auto edit/ }).click()
    } else {
      await model.click()
      await page.getByTestId('model-thinking-toggle').click()
    }

    await expect(permission).toBeDisabled()
    await expect(model).toBeDisabled()
    await expect(send).toBeDisabled()
    await expect(input).toBeEnabled()
    await expect(input).toHaveValue(draft)
    await expect(input).toHaveAttribute('placeholder', 'Ask anything')
    await expect(addContent).toBeEnabled()

    await expect(permission).toBeEnabled()
    await expect(model).toBeEnabled()
    await expect(send).toBeEnabled()
    await expect(input).toHaveValue(draft)
  })
}

test('settings updates are isolated between the main and right-panel composers', async ({
  page,
}) => {
  const requests = await openDesktopClient(page, {
    existingSession: true,
    secondarySession: true,
    composerUpdateDelayMs: 500,
  })

  await page.getByRole('button', { name: 'Actions for Secondary task' }).click()
  await page.getByRole('menuitem', { name: 'Open in right panel' }).click()

  const mainComposer = page.getByTestId('conversation-pane').getByTestId('composer')
  const sideComposer = page
    .getByTestId('workbench-panel')
    .getByTestId('workbench-conversation')
    .getByTestId('composer')
  const mainPermission = mainComposer.getByTestId('permission-mode-trigger')
  const sidePermission = sideComposer.getByTestId('permission-mode-trigger')

  await mainPermission.click()
  await page.getByRole('menuitemradio', { name: /Auto edit/ }).click()
  await expect(mainPermission).toBeDisabled()

  await expect(sideComposer.locator('textarea')).toBeEnabled()
  await expect(sideComposer.getByRole('button', { name: 'Add content' })).toBeEnabled()
  await expect(sideComposer.getByTestId('model-settings-trigger')).toBeEnabled()
  await expect(sidePermission).toBeEnabled()
  await expect(sideComposer.getByTestId('composer-send')).toBeEnabled()

  await sidePermission.click()
  await page.getByRole('menuitemradio', { name: /Auto edit/ }).click()
  await expect(sidePermission).toBeDisabled()

  await expect.poll(() =>
    requests
      .filter(
        (request) =>
          request.method === 'PATCH' && request.path.endsWith('/permission-mode'),
      )
      .map((request) => ({ path: request.path, body: request.body })),
  ).toEqual([
    {
      path: '/api/sessions/test-session/permission-mode',
      body: { mode: 'auto_edit' },
    },
    {
      path: '/api/sessions/secondary-session/permission-mode',
      body: { mode: 'auto_edit' },
    },
  ])
  await expect(mainPermission).toBeEnabled()
  await expect(sidePermission).toBeEnabled()
})

test('Full access requires confirmation before updating the session', async ({ page }) => {
  const requests = await openDesktopClient(page, {
    existingSession: true,
    composerUpdateDelayMs: 500,
  })
  const composer = page.getByTestId('composer')
  const permission = composer.getByTestId('permission-mode-trigger')

  await permission.click()
  await page.getByRole('menuitemradio', { name: /^Full access/ }).click()

  const confirmation = page.getByRole('dialog', { name: 'Enable full access?' })
  await expect(confirmation).toBeVisible()
  await expect(confirmation).toHaveCSS('width', '510px')
  await expect(confirmation.getByText('Files and folders', { exact: true })).toBeVisible()
  await expect(confirmation.getByText('Terminal commands', { exact: true })).toBeVisible()
  await expect(
    confirmation.getByText('Internet and connected apps', { exact: true }),
  ).toBeVisible()
  expect(
    requests.filter((request) => request.path.endsWith('/permission-mode')),
  ).toHaveLength(0)

  await confirmation.getByRole('button', { name: 'Cancel' }).click()
  await expect(confirmation).toHaveCount(0)
  await expect(composer.getByTestId('permission-mode-label')).toHaveText('Ask')

  await permission.click()
  await page.getByRole('menuitemradio', { name: /^Full access/ }).click()
  const enableFullAccess = confirmation.getByRole('button', {
    name: 'Enable full access',
  })
  await enableFullAccess.click()

  await expect(confirmation).toBeVisible()
  await expect(confirmation.getByRole('button', { name: 'Enabling…' })).toBeDisabled()

  await expect.poll(() =>
    requests.find(
      (request) =>
        request.path === '/api/sessions/test-session/permission-mode' &&
        request.method === 'PATCH',
    )?.body,
  ).toEqual({ mode: 'full_access' })
  await expect(page.getByRole('dialog', { name: 'Enable full access?' })).toHaveCount(0)
  await expect(composer.getByTestId('permission-mode-label')).toHaveText('Full access')
  await expect(permission).toHaveClass(/text-warning/)
})

test('Full access confirmation closes when a run starts', async ({ page }) => {
  const requests = await openDesktopClient(page, { existingSession: true })
  const composer = page.getByTestId('composer')
  const permission = composer.getByTestId('permission-mode-trigger')

  await permission.click()
  await page.getByRole('menuitemradio', { name: /^Full access/ }).click()
  await expect(page.getByRole('dialog', { name: 'Enable full access?' })).toBeVisible()

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({ type: 'run_start', id: 'full-access-lock', startedAt: '2026-08-04T12:00:00Z' })
  })

  await expect(permission).toBeDisabled()
  await expect(page.getByRole('dialog', { name: 'Enable full access?' })).toHaveCount(0)
  expect(
    requests.filter((request) => request.path.endsWith('/permission-mode')),
  ).toHaveLength(0)
})

for (const control of [
  { name: 'permission', testID: 'permission-mode-trigger' },
  { name: 'model', testID: 'model-settings-trigger' },
]) {
  test(`Composer closes the ${control.name} menu when a run starts`, async ({ page }) => {
    await openDesktopClient(page, { existingSession: true })

    const composer = page.getByTestId('composer')
    const permission = composer.getByTestId('permission-mode-trigger')
    const model = composer.getByTestId('model-settings-trigger')
    const trigger = composer.getByTestId(control.testID)
    await expect(trigger).toHaveCSS('color', 'rgb(138, 139, 141)')
    await trigger.click()
    const menu = page.getByRole('menu')
    await expect(menu).toBeVisible()
    await expect.poll(async () => {
      const [triggerBox, menuBox] = await Promise.all([
        trigger.boundingBox(),
        menu.boundingBox(),
      ])
      if (!triggerBox || !menuBox) return Number.POSITIVE_INFINITY
      return Math.abs(triggerBox.y - (menuBox.y + menuBox.height) - 2)
    }).toBeLessThanOrEqual(0.5)
    if (control.name === 'permission') {
      await expect(menu).toHaveCSS('width', '450px')
      const autoEdit = page.getByRole('menuitemradio', { name: /Auto edit/ })
      await expect(autoEdit.getByText('Auto edit', { exact: true })).toHaveCSS(
        'font-weight',
        '400',
      )
      await autoEdit.hover()
      await expect(autoEdit).toHaveCSS('background-color', 'rgb(244, 244, 244)')
    } else {
      const provider = page.getByRole('menuitem', { name: /Provider/ })
      await provider.hover()
      await expect(provider).toHaveCSS('background-color', 'rgb(244, 244, 244)')
      await provider.focus()
      await provider.press('ArrowRight')
      const selectedProvider = page.getByRole('menuitemradio', { name: 'OpenAI' })
      await expect(selectedProvider).toBeVisible()
      await expect(selectedProvider).toHaveAttribute('data-state', 'checked')
      await expect(selectedProvider).toHaveAttribute('data-highlighted', '')
      await expect(selectedProvider).toHaveCSS(
        'background-color',
        'rgb(244, 244, 244)',
      )
    }

    await page.evaluate((runID) => {
      const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
      emit?.({ type: 'run_start', id: runID, startedAt: '2026-08-04T12:00:00Z' })
    }, `configuration-lock-${control.name}`)

    await expect(permission).toBeDisabled()
    await expect(model).toBeDisabled()
    await expect(page.getByRole('menu')).toHaveCount(0)
    await expect(
      composer.getByRole('button', { name: 'Choose how this message is delivered' }),
    ).toBeEnabled()
  })
}

test('desktop project browsing uses the native directory picker', async ({ page }) => {
  const requests = await openDesktopClient(page, { nativeDirectory: '/tmp/native-project' })

  const projectPicker = page.getByRole('button', { name: 'Choose project' })
  await expect(projectPicker).toHaveClass(/text-\[rgb\(138,139,141\)\]/)
  await expect(projectPicker.locator('.lucide-chevron-down')).toHaveCount(0)
  await projectPicker.click()
  await expect(page.getByRole('textbox', { name: 'Search projects' })).toHaveCSS('height', '30px')
  const newProject = page.getByRole('menuitem', { name: 'New project' })
  await expect(newProject).toHaveCSS('height', '30px')
  await newProject.hover()
  const existingFolder = page.getByRole('menuitem', { name: 'Use an existing folder' })
  await expect(existingFolder).toHaveCSS('height', '30px')
  await existingFolder.click()

  await expect.poll(() =>
    page.evaluate(() =>
      (window as Window & { __directoryArgs?: { initialPath: string; title: string } })
        .__directoryArgs,
    ),
  ).toEqual({ initialPath: '', title: 'Choose a workspace folder' })
  await expect.poll(() =>
    requests.find((request) => request.path === '/api/workspaces' && request.method === 'POST')
      ?.body,
  ).toEqual({ path: '/tmp/native-project' })
  await expect(page.getByRole('button', { name: 'Choose project' })).toContainText('native-project')
  await expect(page.getByRole('dialog')).toHaveCount(0)
})
