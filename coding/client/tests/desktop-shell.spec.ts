import { expect, test, type Page } from '@playwright/test'

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

async function openDesktopClient(page: Page, options: { failCreate?: boolean } = {}) {
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
    modelName: 'Test model',
    thinkingLevel: 'medium',
    permissionMode: 'ask',
  }
  let sessionCreated = false

  await page.addInitScript(() => {
    Object.defineProperty(navigator, 'platform', {
      configurable: true,
      get: () => 'MacIntel',
    })
    Object.defineProperty(window, 'runtime', {
      configurable: true,
      value: {
        WindowToggleMaximise() {
          const testWindow = window as Window & { __maximiseCount?: number }
          testWindow.__maximiseCount = (testWindow.__maximiseCount ?? 0) + 1
        },
      },
    })

    class TestEventSource {
      onopen: ((event: Event) => void) | null = null
      onerror: ((event: Event) => void) | null = null
      onmessage: ((event: MessageEvent) => void) | null = null

      constructor() {
        window.setTimeout(() => this.onopen?.(new Event('open')), 0)
      }

      close() {}
    }

    Object.defineProperty(window, 'EventSource', {
      configurable: true,
      value: TestEventSource,
    })
  })

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
      sessionCreated = true
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify(createdSession),
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

    let body: unknown = []
    let status = 200
    if (path === '/api/models') body = models
    if (path === '/api/sessions') body = sessionCreated ? [createdSession] : []
    if (path === '/api/sessions/test-session/history') {
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
    await route.fulfill({
      status,
      contentType: 'application/json',
      body: JSON.stringify(body),
    })
  })

  await page.goto('/')
  await expect(page.locator('html')).toHaveClass(/wails-macos/)
  await expect(page.getByTestId('conversation-header')).toBeVisible()
  return requests
}

test('sidebar collapse keeps the titlebar control stable and clears the divider', async ({
  page,
}) => {
  await openDesktopClient(page)
  const toggle = page.getByTestId('window-sidebar-toggle')
  const sidebar = page.getByTestId('sidebar-viewport')
  const header = page.getByTestId('conversation-header')
  const title = page.getByTestId('conversation-title')

  await expect(toggle).toBeVisible()
  await expect.poll(() => header.evaluate((element) => element.getBoundingClientRect().height)).toBe(45)
  await expect.poll(() => sidebar.evaluate((element) => element.getBoundingClientRect().width)).toBeGreaterThan(200)

  const before = await toggle.boundingBox()
  expect(before).not.toBeNull()

  await toggle.click()
  await page.waitForTimeout(60)
  const during = await toggle.boundingBox()
  expect(during).not.toBeNull()
  expect(during!.x).toBeCloseTo(before!.x, 1)
  expect(during!.y).toBeCloseTo(before!.y, 1)

  await expect.poll(() => sidebar.evaluate((element) => element.getBoundingClientRect().width)).toBeLessThan(1)

  const after = await toggle.boundingBox()
  const titleBox = await title.boundingBox()
  expect(after).not.toBeNull()
  expect(titleBox).not.toBeNull()
  expect(after!.x).toBeCloseTo(before!.x, 1)
  expect(after!.y).toBeCloseTo(before!.y, 1)
  expect(titleBox!.x).toBeGreaterThanOrEqual(after!.x + after!.width + 10)

  const borderColor = await header.evaluate(
    (element) => getComputedStyle(element).borderBottomColor,
  )
  expect(borderColor).toBe('rgba(0, 0, 0, 0)')

  await toggle.click()
  await expect.poll(() => sidebar.evaluate((element) => element.getBoundingClientRect().width)).toBeGreaterThan(200)
})

test('double-clicking a draggable header toggles window maximisation once', async ({ page }) => {
  await openDesktopClient(page)
  const header = page.getByTestId('conversation-header')
  await header.dblclick({ position: { x: 720, y: 20 } })

  await expect.poll(() =>
    page.evaluate(() => (window as Window & { __maximiseCount?: number }).__maximiseCount ?? 0),
  ).toBe(1)
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
  await page.addInitScript(() => {
    Object.defineProperty(window, 'go', {
      configurable: true,
      value: {
        main: {
          DesktopBridge: {
            ChooseDirectory(initialPath: string, title: string) {
              const testWindow = window as Window & {
                __directoryArgs?: { initialPath: string; title: string }
              }
              testWindow.__directoryArgs = { initialPath, title }
              return Promise.resolve('/tmp/native-project')
            },
          },
        },
      },
    })
  })
  const requests = await openDesktopClient(page)

  await page.getByRole('button', { name: 'Choose project' }).click()
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
