import { expect, test, type Page } from '@playwright/test'

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
    legacyHealth?: boolean
    browserResultFailures?: number
    existingSession?: boolean
    historyEvents?: unknown[]
    historyRunning?: boolean
    historyEventSeq?: number
    backgroundTasks?: unknown[]
    secondarySession?: boolean
    secondaryHistoryEvents?: unknown[]
    modelName?: string
    modelThinkingLevels?: Array<'off' | 'minimal' | 'low' | 'medium' | 'high' | 'xhigh' | 'max'>
    modelThinkingVisibility?: 'visible' | 'hidden'
    composerUpdateDelayMs?: number
    nativeDirectory?: string
  } = {},
) {
  const requests: Array<{ method: string; path: string; body?: unknown }> = []
  const modelThinkingLevels = options.modelThinkingLevels ?? ['medium']
  const modelThinkingLevel = modelThinkingLevels[0] ?? 'off'
  const createdSession = {
    id: 'test-session',
    title: 'New session',
    workspacePath: '/tmp/test-session',
    workspaceName: 'test-session',
    scope: 'chat',
    workspaceKind: 'scratch',
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
  let sessionCreated = Boolean(options.existingSession)
  let workbenchSessionCreated = false
  let remainingHealthFailures = options.healthFailures ?? 0
  let remainingBrowserResultFailures = options.browserResultFailures ?? 0

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
    requests.push({ method, path, body: requestBody })

    if (path === '/api/health') {
      if (remainingHealthFailures > 0) {
        remainingHealthFailures--
        await route.fulfill({ status: 503 })
      } else {
        await route.fulfill({ status: options.legacyHealth ? 404 : 204 })
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
    if (path === '/api/sessions') {
      body = sessionCreated
        ? [
            ...(workbenchSessionCreated ? [workbenchSession] : []),
            createdSession,
            ...(options.secondarySession ? [secondarySession] : []),
          ]
        : []
    }
    if (path === '/api/sessions/test-session/history') {
      body = {
        events: options.historyEvents ?? [],
        tasks: options.backgroundTasks ?? [],
        queue: [],
        context: {},
        running: options.historyRunning ?? false,
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
    const chats = page.getByRole('navigation', { name: 'Chats' })
    await chats.getByRole('button', { name: createdSession.title, exact: true }).click()
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

test('desktop external links open in the system browser without leaving Coding', async ({ page }) => {
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

test('Coding API startup retries recover the Composer automatically', async ({ page }) => {
  const requests = await openDesktopClient(page, { healthFailures: 2, legacyHealth: true })
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

test('browser workspaces show only the selected session tabs and restore them on return', async ({
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

test('browser workspace restores a page navigation instead of the original URL', async ({
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

  const chats = page.getByRole('navigation', { name: 'Chats' })
  await chats.getByRole('button', { name: 'Secondary task', exact: true }).click()
  await expect.poll(async () => browserRuntimeView(page, 'tab-1')).toBeUndefined()

  await chats.getByRole('button', { name: 'New session', exact: true }).click()
  await expect.poll(async () => (await browserRuntimeView(page, 'tab-1'))?.url).toBe(
    videoURL,
  )
  expect(await browserRuntimeView(page, 'tab-1')).toMatchObject({
    loadCalls: [videoURL],
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

test('browser inspection cannot cross session boundaries for same-named tabs', async ({
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
  await expect.poll(async () => (await browserRuntimeView(page, 'tab-1'))?.url).toBe(
    'https://secondary.example/',
  )
  await expect(page.locator('[data-browser-tab-id="tab-1"]')).toHaveAttribute(
    'data-browser-runtime-tab-id',
    'workspace:secondary-session:tab:tab-1',
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
      tabID: 'tab-1',
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
      error: 'Browser tab is not open',
    },
  })
  expect(await browserRuntimeView(page, 'tab-1')).toMatchObject({
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

  await expect(permission).toHaveClass(/text-ink-muted/)
  await expect(page.getByTestId('model-settings-name')).toHaveClass(/text-ink-muted/)
  await expect(page.getByTestId('model-settings-effort')).toBeHidden()
  await expect(page.getByTestId('permission-mode-label')).toBeHidden()
  await expect(page.getByTestId('model-settings-name')).toHaveCSS('text-overflow', 'ellipsis')
  const modelNameLayout = await page.getByTestId('model-settings-name').evaluate((element) => ({
    clientWidth: element.clientWidth,
    scrollWidth: element.scrollWidth,
    whiteSpace: getComputedStyle(element).whiteSpace,
  }))
  expect(modelNameLayout.whiteSpace).toBe('nowrap')
  expect(modelNameLayout.scrollWidth).toBeGreaterThan(modelNameLayout.clientWidth)
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
  })

  const trigger = page.getByTestId('model-settings-trigger')
  await expect(page.getByTestId('model-settings-effort')).toHaveText('Off')
  await trigger.click()

  const toggle = page.getByTestId('model-thinking-toggle')
  await expect(toggle).toHaveRole('menuitemcheckbox')
  await expect(toggle).toHaveText(/Thinking.*Off/)
  await expect(toggle).toHaveAttribute('aria-checked', 'false')

  await toggle.click()
  await expect.poll(() =>
    requests.find(
      (request) =>
        request.path === '/api/sessions/test-session/settings' && request.method === 'PATCH',
    )?.body,
  ).toEqual({ provider: 'openai', model: 'test-model', thinkingLevel: 'high' })
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

test('Composer closes the add menu when the input becomes disabled', async ({ page }) => {
  await openDesktopClient(page, {
    existingSession: true,
    composerUpdateDelayMs: 500,
  })

  const composer = page.getByTestId('composer')
  const input = composer.locator('textarea')
  const addContent = composer.getByRole('button', { name: 'Add content' })
  await addContent.click()
  await expect(composer.getByRole('listbox', { name: 'Add content' })).toBeVisible()

  await composer.getByTestId('permission-mode-trigger').click()
  await page.getByRole('menuitemradio', { name: /Auto edit/ }).click()

  await expect(input).toBeDisabled()
  await expect(addContent).toBeDisabled()
  await expect(composer.getByRole('listbox', { name: 'Add content' })).toHaveCount(0)
  await expect(input).toBeEnabled()
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
    await composer.getByTestId(control.testID).click()
    await expect(page.getByRole('menu')).toBeVisible()

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
  await expect(projectPicker).toHaveClass(/text-ink-muted/)
  await projectPicker.click()
  await page.getByText('New project', { exact: true }).hover()
  await page.getByText('Use an existing folder', { exact: true }).click()

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
