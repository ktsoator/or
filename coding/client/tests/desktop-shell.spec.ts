import { expect, test, type Page } from '@playwright/test'

type NativeBrowserNavigateInput = {
  tabID: string
  url: string
  revision: number
  kind: 'web' | 'workspace-preview'
}

type NativeBrowserViewportInput = {
  tabID: string
  visible: boolean
  bounds?: { x: number; y: number; width: number; height: number }
}

type NativeBrowserState = {
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

type NativeBrowserRecord = {
  tabID: string
  url: string
  bounds: { x: number; y: number; width: number; height: number }
  navigation: number
  workspacePreview: boolean
  visible: boolean
  navigateCalls: number
  viewportCalls: number
  state?: NativeBrowserState
}

async function nativeBrowserView(
  page: Page,
  tabID: string,
): Promise<NativeBrowserRecord | undefined> {
  return page.evaluate(
    (id) =>
      (
        window as Window & {
          __browserViews?: Record<string, NativeBrowserRecord>
        }
      ).__browserViews?.[id],
    tabID,
  )
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

async function openDesktopClient(
  page: Page,
  options: {
    failCreate?: boolean
    existingSession?: boolean
    historyEvents?: unknown[]
    secondarySession?: boolean
    secondaryHistoryEvents?: unknown[]
    modelName?: string
    nativeDirectory?: string
  } = {},
) {
  const requests: Array<{ method: string; path: string; body?: unknown }> = []
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
    thinkingLevel: 'medium',
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

  await page.addInitScript(({ nativeDirectory }) => {
    type BrowserTestWindow = Window & {
      __browserViews?: Record<string, NativeBrowserRecord>
      __browserActions?: Array<{ action: string; tabID: string }>
      __emitBrowserState?: (state: NativeBrowserState) => void
    }
    const browserWindow = window as BrowserTestWindow
    const browserListeners = new Set<(state: NativeBrowserState) => void>()
    const browserViews: Record<string, NativeBrowserRecord> = {}
    const browserViewports: Record<
      string,
      NativeBrowserViewportInput & { calls: number }
    > = {}
    const browserActions: Array<{ action: string; tabID: string }> = []
    browserWindow.__browserViews = browserViews
    browserWindow.__browserActions = browserActions
    browserWindow.__emitBrowserState = (state) => {
      const view = browserViews[state.tabID]
      if (view && state.appliedRevision >= view.navigation) {
        view.url = state.committedURL || state.requestedURL
        view.state = state
      }
      for (const listener of browserListeners) listener(state)
    }

    Object.defineProperty(navigator, 'platform', {
      configurable: true,
      get: () => 'MacIntel',
    })
    Object.defineProperty(window, 'codingDesktop', {
      configurable: true,
      value: {
        platform: 'darwin',
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
        browser: {
          navigate(input: NativeBrowserNavigateInput) {
            const url = new URL(input.url, window.location.href).href
            const previous = browserViews[input.tabID]
            if (previous && input.revision < previous.navigation) {
              return Promise.resolve(previous.state)
            }
            if (previous && input.revision === previous.navigation) {
              return Promise.resolve(previous.state)
            }
            const viewport = browserViewports[input.tabID]
            const state: NativeBrowserState = {
              tabID: input.tabID,
              appliedRevision: input.revision,
              requestedURL: url,
              committedURL: previous?.state?.committedURL ?? '',
              title: '',
              status: 'navigating',
              canGoBack: previous?.state?.canGoBack ?? false,
              canGoForward: false,
            }
            browserViews[input.tabID] = {
              tabID: input.tabID,
              url,
              bounds: viewport?.bounds ?? previous?.bounds ?? {
                x: 0,
                y: 0,
                width: 0,
                height: 0,
              },
              navigation: input.revision,
              workspacePreview: input.kind === 'workspace-preview',
              visible: viewport?.visible ?? previous?.visible ?? false,
              navigateCalls: (previous?.navigateCalls ?? 0) + 1,
              viewportCalls: viewport?.calls ?? previous?.viewportCalls ?? 0,
              state,
            }
            window.setTimeout(() => {
              if (browserViews[input.tabID]?.navigation !== input.revision) return
              browserWindow.__emitBrowserState?.(
                {
                  tabID: input.tabID,
                  appliedRevision: input.revision,
                  requestedURL: url,
                  committedURL: url,
                  title: '',
                  status: 'ready',
                  canGoBack: false,
                  canGoForward: false,
                },
              )
            }, 0)
            return Promise.resolve(state)
          },
          setViewport(input: NativeBrowserViewportInput) {
            const previous = browserViewports[input.tabID]
            browserViewports[input.tabID] = {
              ...input,
              calls: (previous?.calls ?? 0) + 1,
            }
            const view = browserViews[input.tabID]
            if (view) {
              view.visible = input.visible
              view.viewportCalls += 1
              if (input.bounds) view.bounds = input.bounds
              if (input.visible) {
                for (const candidate of Object.values(browserViews)) {
                  if (candidate !== view) candidate.visible = false
                }
              }
            }
            return Promise.resolve()
          },
          close(tabID: string) {
            delete browserViews[tabID]
            delete browserViewports[tabID]
            browserActions.push({ action: 'close', tabID })
            return Promise.resolve()
          },
          goBack(tabID: string) {
            browserActions.push({ action: 'back', tabID })
            return Promise.resolve()
          },
          goForward(tabID: string) {
            browserActions.push({ action: 'forward', tabID })
            return Promise.resolve()
          },
          onState(listener: (state: NativeBrowserState) => void) {
            browserListeners.add(listener)
            return () => browserListeners.delete(listener)
          },
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

    if (path === '/api/sessions/test-session/preview/web/index.html' && method === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'text/html',
        body: '<!doctype html><title>Static page</title><main>Direct HTML preview</main>',
      })
      return
    }
    if (path === '/api/sessions/secondary-session/preview/web/index.html' && method === 'GET') {
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
      body = options.modelName
        ? {
            ...models,
            models: models.models.map((model) => ({ ...model, name: options.modelName })),
          }
        : models
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
        queue: [],
        context: {},
        running: false,
        eventSeq: 0,
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
    await route.fulfill({
      status,
      contentType: 'application/json',
      body: JSON.stringify(body),
    })
  })

  await page.goto('/')
  await expect(page.locator('html')).toHaveClass(/desktop-macos/)
  await expect(page.getByTestId('conversation-header')).toBeVisible()
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
  const address = page.getByRole('textbox', { name: 'Preview address' })
  await address.fill('127.0.0.1:4310')
  await address.press('Enter')
  await expect.poll(async () => (await nativeBrowserView(page, 'tab-1'))?.url).toBe(
    'http://127.0.0.1:4310/',
  )
  const localhostView = await nativeBrowserView(page, 'tab-1')
  expect(localhostView).toMatchObject({
    navigation: 1,
    workspacePreview: false,
    visible: true,
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
  await expect(addViewMenu.getByRole('menuitem')).toHaveCount(2)
  await expect(addViewMenu.getByRole('menuitem', { name: 'Chat' })).toBeEnabled()
  await addViewMenu.getByRole('menuitem', { name: 'Browser' }).click()
  await expect(page.getByRole('tab')).toHaveCount(2)
  await expect(page.getByRole('tab', { name: 'New tab' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect.poll(async () => nativeBrowserView(page, 'tab-2')).toBeUndefined()
  await address.fill('https://example.com')
  await address.press('Enter')
  await expect.poll(async () => (await nativeBrowserView(page, 'tab-2'))?.url).toBe(
    'https://example.com/',
  )
  expect(await nativeBrowserView(page, 'tab-2')).toMatchObject({
    workspacePreview: false,
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
  await expect(page.getByTestId('native-browser-surface')).toHaveAttribute(
    'data-status',
    'ready',
  )

  await page.evaluate(() => {
    const emit = (
      window as Window & {
        __emitBrowserState?: (state: NativeBrowserState) => void
      }
    ).__emitBrowserState
    emit?.({
      tabID: 'tab-2',
      appliedRevision: 1,
      requestedURL: 'https://example.com/search',
      committedURL: 'https://example.com/search',
      title: 'Example',
      status: 'ready',
      canGoBack: true,
      canGoForward: false,
    })
  })
  await expect(address).toHaveValue('https://example.com/search')
  await expect(page.getByRole('tab', { name: 'Example' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  const back = page.getByRole('button', { name: 'Back' })
  await expect(back).toBeEnabled()
  await back.click()
  await expect.poll(() =>
    page.evaluate(() =>
      (
        window as Window & {
          __browserActions?: Array<{ action: string; tabID: string }>
        }
      ).__browserActions?.some(
        (entry) => entry.action === 'back' && entry.tabID === 'tab-2',
      ),
    ),
  ).toBe(true)

  await originalTab.click()
  await expect(originalTab).toHaveAttribute('aria-selected', 'true')
  await expect.poll(async () => (await nativeBrowserView(page, 'tab-1'))?.visible).toBe(true)
  await expect.poll(async () => (await nativeBrowserView(page, 'tab-2'))?.visible).toBe(false)
  await page.getByRole('button', { name: 'Close tab: 127.0.0.1:4310' }).click()
  await expect(page.getByRole('tab')).toHaveCount(1)
  await expect(page.getByRole('tab', { name: 'Example' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await page.getByRole('button', { name: 'Open in browser' }).click()
  await expect.poll(() =>
    page.evaluate(() => (window as Window & { __openedURL?: string }).__openedURL),
  ).toBe('https://example.com/search')
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
        preview: {
          path: '/tmp/secondary-session/web/index.html',
          relativePath: 'web/index.html',
          title: 'Saved preview',
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
    (await nativeBrowserView(page, 'preview:secondary-session'))?.url.endsWith(
      '/api/sessions/secondary-session/preview/web/index.html',
    ),
  ).toBe(true)
  expect(await nativeBrowserView(page, 'preview:secondary-session')).toMatchObject({
    workspacePreview: true,
    visible: true,
  })
})

test('main and right-side chats keep separate preview tabs and workspace routes', async ({
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
      preview: {
        path: '/tmp/test-session/web/index.html',
        relativePath: 'web/index.html',
        title: 'Main preview',
      },
    })
  })

  const workbench = page.getByTestId('workbench-panel')
  await expect(workbench.getByRole('tab', { name: 'Main preview' })).toHaveAttribute(
    'aria-selected',
    'true',
  )

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
      preview: {
        path: '/tmp/secondary-session/web/index.html',
        relativePath: 'web/index.html',
        title: 'Secondary preview',
      },
    })
  })

  await expect(workbench.getByRole('tab')).toHaveCount(3)
  await expect(workbench.getByRole('tab', { name: 'Main preview' })).toHaveAttribute(
    'aria-selected',
    'false',
  )
  await expect(workbench.getByRole('tab', { name: 'Secondary preview' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect(workbench.getByRole('textbox', { name: 'Preview address' })).toHaveValue(
    '/tmp/secondary-session/web/index.html',
  )
  await expect.poll(async () =>
    (await nativeBrowserView(page, 'preview:secondary-session'))?.url.endsWith(
      '/api/sessions/secondary-session/preview/web/index.html',
    ),
  ).toBe(true)
  expect(await nativeBrowserView(page, 'preview:secondary-session')).toMatchObject({
    workspacePreview: true,
    visible: true,
  })
  expect(await nativeBrowserView(page, 'preview:test-session')).toMatchObject({
    workspacePreview: true,
    visible: false,
  })
  expect(
    requests.filter((request) => request.path.includes('/preview/web/index.html')),
  ).toHaveLength(0)
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
      preview: { url: 'http://127.0.0.1:4310', title: 'Narrow preview' },
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
      preview: { url: 'http://127.0.0.1:4310', title: 'Local app' },
    })
  })

  await expect(page.getByTestId('browser-view')).toBeVisible()
  await expect.poll(async () =>
    (await nativeBrowserView(page, 'preview:test-session'))?.url,
  ).toBe('http://127.0.0.1:4310/')
  await expect.poll(async () =>
    (await nativeBrowserView(page, 'preview:test-session'))?.visible,
  ).toBe(true)
  expect(await nativeBrowserView(page, 'preview:test-session')).toMatchObject({
    workspacePreview: false,
  })
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
      preview: { url: 'http://127.0.0.1:4310', title: 'Updated app' },
    })
  })
  await expect(page.getByRole('tab')).toHaveCount(2)
  await expect(page.getByRole('tab', { name: 'Updated app' })).toHaveAttribute(
    'aria-selected',
    'true',
  )

  await page.getByRole('button', { name: 'Open in browser' }).click()
  await expect.poll(() =>
    page.evaluate(() => (window as Window & { __openedURL?: string }).__openedURL),
  ).toBe('http://127.0.0.1:4310/')

  await page.getByTestId('workbench-panel-toggle').click()
  await expect(page.getByTestId('browser-view')).toBeHidden()
  await expect.poll(async () =>
    (await nativeBrowserView(page, 'preview:test-session'))?.visible,
  ).toBe(false)
  await expect(page.getByRole('main')).toBeVisible()
  await expect(page.getByTestId('workbench-panel-toggle')).toHaveAccessibleName('Show workbench')

  await page.getByTestId('workbench-panel-toggle').click()
  await expect(page.getByTestId('browser-view')).toBeVisible()
  await expect.poll(async () =>
    (await nativeBrowserView(page, 'preview:test-session'))?.visible,
  ).toBe(true)
  await page.getByTestId('workbench-panel-toggle').click()
  await expect(page.getByTestId('workbench-panel')).toBeHidden()
  await expect(page.getByRole('main')).toBeVisible()
  await expect(page.getByTestId('workbench-panel-toggle')).toBeVisible()
})

test('AI preview tool opens a public website inside the native Browser', async ({ page }) => {
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
      preview: { url: 'https://www.google.com', title: 'Google' },
    })
  })

  await expect(page.getByRole('tab', { name: 'Google' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect.poll(async () =>
    (await nativeBrowserView(page, 'preview:test-session'))?.url,
  ).toBe('https://www.google.com/')
  expect(await nativeBrowserView(page, 'preview:test-session')).toMatchObject({
    workspacePreview: false,
    visible: true,
  })
  const divider = page.getByTestId('workbench-divider-line')
  await expect.poll(async () => {
    const dividerBox = await divider.boundingBox()
    const nativeView = await nativeBrowserView(page, 'preview:test-session')
    if (!dividerBox || !nativeView) return Number.NEGATIVE_INFINITY
    return nativeView.bounds.x - (dividerBox.x + dividerBox.width)
  }).toBeGreaterThanOrEqual(0)
  await expect(page.getByRole('textbox', { name: 'Preview address' })).toHaveValue(
    'https://www.google.com/',
  )
  expect(requests.filter((request) => request.path === '/api/preview/check')).toHaveLength(0)
  expect(
    await page.evaluate(() => (window as Window & { __openedURL?: string }).__openedURL),
  ).toBeUndefined()
})

test('AI browser keeps the latest revision across stale state and viewport changes', async ({
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
      preview: { url: 'https://github.com', title: 'GitHub' },
    })
  })
  await expect.poll(async () =>
    (await nativeBrowserView(page, 'preview:test-session'))?.url,
  ).toBe('https://github.com/')

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_end',
      id: 'bilibili-preview',
      tool: 'open_preview',
      result: 'Opened preview at https://www.bilibili.com',
      preview: { url: 'https://www.bilibili.com', title: 'Bilibili' },
    })
  })
  await expect.poll(async () =>
    (await nativeBrowserView(page, 'preview:test-session'))?.url,
  ).toBe('https://www.bilibili.com/')
  const navigated = await nativeBrowserView(page, 'preview:test-session')
  expect(navigated).toMatchObject({ navigation: 1, navigateCalls: 2 })

  await page.evaluate(() => {
    const emit = (
      window as Window & {
        __emitBrowserState?: (state: NativeBrowserState) => void
      }
    ).__emitBrowserState
    emit?.({
      tabID: 'preview:test-session',
      appliedRevision: 0,
      requestedURL: 'https://github.com/',
      committedURL: 'https://github.com/',
      title: 'GitHub',
      status: 'ready',
      canGoBack: false,
      canGoForward: false,
    })
  })

  await expect(page.getByRole('textbox', { name: 'Preview address' })).toHaveValue(
    'https://www.bilibili.com/',
  )
  await expect(page.getByRole('tab', { name: 'Bilibili' })).toHaveAttribute(
    'aria-selected',
    'true',
  )

  const navigateCalls = navigated?.navigateCalls
  const viewportCalls = navigated?.viewportCalls ?? 0
  const toggle = page.getByTestId('workbench-panel-toggle')
  await toggle.click()
  await expect.poll(async () =>
    (await nativeBrowserView(page, 'preview:test-session'))?.visible,
  ).toBe(false)
  await toggle.click()
  await expect.poll(async () =>
    (await nativeBrowserView(page, 'preview:test-session'))?.visible,
  ).toBe(true)
  const restored = await nativeBrowserView(page, 'preview:test-session')
  expect(restored?.navigateCalls).toBe(navigateCalls)
  expect(restored?.viewportCalls).toBeGreaterThan(viewportCalls)
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
      bytes: 1024,
    })
    emit?.({
      type: 'tool_input_delta',
      id: 'write-call',
      tool: 'write',
      toolContentIndex: 0,
      bytes: 512,
    })
  })

  await expect(page.getByText('Preparing file content')).toBeVisible()
  await expect(page.getByText('1.5 KB')).toBeVisible()

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
      change: {
        changeType: 'file',
        path: 'src/main.go',
        op: 'create',
        additions: 3,
        deletions: 0,
        bytes: 13,
        hunks: [],
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
      preview: {
        path: '/tmp/test-session/web/index.html',
        relativePath: 'web/index.html',
        title: 'Static page',
      },
    })
  })

  await expect.poll(async () =>
    (await nativeBrowserView(page, 'preview:test-session'))?.url.endsWith(
      '/api/sessions/test-session/preview/web/index.html',
    ),
  ).toBe(true)
  const previewView = await nativeBrowserView(page, 'preview:test-session')
  expect(previewView).toMatchObject({
    navigation: 0,
    workspacePreview: true,
    visible: true,
  })
  await expect(page.getByRole('textbox', { name: 'Preview address' })).toHaveValue(
    '/tmp/test-session/web/index.html',
  )
  const openExternal = page.getByRole('button', { name: 'Open in browser' })
  await expect(openExternal).toBeEnabled()
  await openExternal.click()
  await expect.poll(() =>
    page.evaluate(() => (window as Window & { __openedURL?: string }).__openedURL),
  ).toBe('file:///tmp/test-session/web/index.html')
  expect(requests.filter((request) => request.path === '/api/preview/check')).toHaveLength(0)
  expect(
    requests.filter(
      (request) =>
        request.path === '/api/sessions/test-session/preview/web/index.html',
    ),
  ).toHaveLength(0)

  await page.evaluate(() => {
    const emit = (window as Window & { __emitSSE?: (payload: unknown) => void }).__emitSSE
    emit?.({
      type: 'tool_end',
      id: 'edit-static',
      tool: 'edit',
      result: 'Updated web/index.html',
      change: {
        changeType: 'file',
        path: 'web/index.html',
        op: 'update',
        additions: 1,
        deletions: 1,
        bytes: 128,
        hunks: [],
      },
    })
  })
  await expect.poll(async () =>
    (await nativeBrowserView(page, 'preview:test-session'))?.navigation,
  ).toBe(1)
})

test('Browser replaces a failed local preview probe with a retry state', async ({ page }) => {
  const requests = await openDesktopClient(page, { existingSession: true })
  await page.getByTestId('workbench-panel-toggle').click()
  await page.getByTestId('workbench-panel').getByRole('button', { name: 'Add view' }).click()
  await page.getByRole('menu').getByRole('menuitem', { name: 'Browser' }).click()

  const address = page.getByRole('textbox', { name: 'Preview address' })
  await address.fill('127.0.0.1:4311')
  await address.press('Enter')

  await expect(page.getByRole('alert')).toContainText('Preview unavailable')
  expect(await nativeBrowserView(page, 'tab-1')).toBeUndefined()
  await expect(page.getByRole('alert')).toContainText(
    'Check that the local server is running, then try again.',
  )
  await expect(page.getByRole('button', { name: 'Retry' })).toBeVisible()
  await expect(page.getByRole('status')).toHaveCount(0)

  await page.getByRole('button', { name: 'Retry' }).click()
  await expect.poll(
    () => requests.filter((request) => request.path === '/api/preview/check').length,
  ).toBe(2)
  await expect(page.getByRole('alert')).toContainText('Preview unavailable')
})

test('long threads keep the titlebar and Composer fixed while the transcript scrolls', async ({
  page,
}) => {
  const historyEvents = Array.from({ length: 18 }, (_, index) => [
    {
      type: 'user_message',
      id: `user-${index}`,
      text: `Question ${index + 1} with enough content to exercise the conversation layout`,
      images: [],
    },
    {
      type: 'run_start',
      id: `run-${index}`,
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

  await openDesktopClient(page, { existingSession: true, historyEvents })

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

  await expect(permission).toHaveClass(/text-stone-500/)
  await expect(page.getByTestId('model-settings-name')).toHaveClass(/text-stone-500/)
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
  await expect(page.getByText(message, { exact: true })).toHaveCount(0)
})

test('desktop project browsing uses the native directory picker', async ({ page }) => {
  const requests = await openDesktopClient(page, { nativeDirectory: '/tmp/native-project' })

  const projectPicker = page.getByRole('button', { name: 'Choose project' })
  await expect(projectPicker).toHaveClass(/text-stone-500/)
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
