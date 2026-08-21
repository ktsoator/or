import { createRequire } from 'node:module'
import { mkdtemp, rm } from 'node:fs/promises'
import { createServer } from 'node:http'
import type { AddressInfo } from 'node:net'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { _electron as electron, expect, test } from '@playwright/test'

type ElectronWebviewElement = HTMLElement & {
  executeJavaScript: (code: string, userGesture?: boolean) => Promise<unknown>
  getURL: () => string
  loadURL: (url: string) => Promise<void>
}

const require = createRequire(import.meta.url)
const desktopDirectory = path.resolve(import.meta.dirname, '../../desktop')
const electronExecutable = require(
  path.join(desktopDirectory, 'node_modules/electron'),
) as string

test('webview fills its host and renderer menus stay above it', async () => {
  const userDataDirectory = await mkdtemp(
    path.join(tmpdir(), 'coding-electron-webview-'),
  )
  const electronApp = await electron.launch({
    executablePath: electronExecutable,
    args: [
      desktopDirectory,
      `--user-data-dir=${userDataDirectory}`,
    ],
    cwd: desktopDirectory,
  })
  const webServer = createServer((request, response) => {
    response.setHeader('Content-Type', 'text/html; charset=utf-8')
    response.end(`<!doctype html><title>Test page</title><main>${request.url}</main>`)
  })

  try {
    await new Promise<void>((resolve, reject) => {
      webServer.once('error', reject)
      webServer.listen(0, '127.0.0.1', resolve)
    })
    const webAddress = webServer.address() as AddressInfo
    const webURL = `http://127.0.0.1:${webAddress.port}/`
    const page = await electronApp.firstWindow()
    await page.waitForLoadState('domcontentloaded')

    const panelToggle = page.getByTestId('workbench-panel-toggle')
    await expect(page.getByTestId('desktop-titlebar-controls')).toHaveCSS(
      '-webkit-app-region',
      'none',
    )

    const sidebarToggle = page.getByTestId('sidebar-panel-toggle')
    await expect.poll(() => page.locator('.app-sidebar-header').first().evaluate(
      (element) => element.getBoundingClientRect().height,
    )).toBe(45)
    await expect.poll(async () => (await sidebarToggle.boundingBox())?.width).toBe(30)
    await sidebarToggle.hover()
    await expect(sidebarToggle.locator('.header-control-tooltip')).toHaveCSS('opacity', '1')
    if (await sidebarToggle.getAttribute('aria-expanded') === 'true') {
      await sidebarToggle.click()
    }
    const newSession = page.getByTestId('desktop-new-session')
    await expect(newSession).toHaveCSS('opacity', '1')
    await expect(newSession).toHaveCSS('-webkit-app-region', 'no-drag')
    await newSession.click()
    await expect(page.getByTestId('conversation-title')).toContainText('New session')
    await sidebarToggle.click()
    const sidebarResizeHandle = page.getByTestId('sidebar-resize-handle')
    await expect(sidebarResizeHandle).toHaveCSS('-webkit-app-region', 'no-drag')
    await expect.poll(async () => (await sidebarResizeHandle.boundingBox())?.width).toBe(8)

    if (await panelToggle.getAttribute('aria-expanded') === 'false') {
      await panelToggle.click()
    }
    const workbenchResizeHandle = page.getByTestId('workbench-resize-handle')
    await expect(workbenchResizeHandle).toHaveCSS('-webkit-app-region', 'no-drag')
    await expect.poll(async () => (await workbenchResizeHandle.boundingBox())?.width).toBe(8)

    const maximize = page.getByTestId('workbench-maximize')
    await expect.poll(async () => (await page.getByTestId('workbench-add-view').boundingBox())?.width)
      .toBe(30)
    await expect.poll(async () => (await maximize.boundingBox())?.width).toBe(30)
    await maximize.click()
    await expect(maximize).toHaveAttribute('aria-pressed', 'true')
    await maximize.click()
    await expect(maximize).toHaveAttribute('aria-pressed', 'false')

    await page.getByTestId('workbench-add-view').click()
    await page.getByRole('menuitem').first().click()

    const address = page.getByTestId('browser-address')
    await address.fill(webURL)
    await address.press('Enter')

    const surface = page.getByTestId('browser-surface')
    await expect(surface).toHaveAttribute('data-status', 'ready')
    const webview = surface.locator('webview')
    await expect(webview).toBeVisible()
    await expect(webview).toHaveAttribute('allowpopups', 'true')
    await expect(webview).toHaveAttribute('partition', 'persist:or-browser')

    const sizes = await webview.evaluate(async (element) => {
      const guest = element as ElectronWebviewElement
      const viewport = await guest.executeJavaScript(`({
        width: document.documentElement.clientWidth,
        height: document.documentElement.clientHeight
      })`) as { width: number; height: number }
      return {
        display: getComputedStyle(element).display,
        hostWidth: element.clientWidth,
        hostHeight: element.clientHeight,
        guestWidth: viewport.width,
        guestHeight: viewport.height,
      }
    })
    expect(sizes.display).toBe('flex')
    expect(sizes.hostHeight).toBeGreaterThan(400)
    expect(Math.abs(sizes.hostWidth - sizes.guestWidth)).toBeLessThanOrEqual(1)
    expect(Math.abs(sizes.hostHeight - sizes.guestHeight)).toBeLessThanOrEqual(1)

    const tabsBefore = await page.getByTestId('browser-tab').count()
    await page.getByTestId('workbench-add-view').click()
    const menu = page.getByRole('menu')
    await expect(menu).toBeVisible()

    const webviewBox = await webview.boundingBox()
    const menuBox = await menu.boundingBox()
    expect(webviewBox).not.toBeNull()
    expect(menuBox).not.toBeNull()
    const overlapTop = Math.max(webviewBox!.y, menuBox!.y)
    const overlapBottom = Math.min(
      webviewBox!.y + webviewBox!.height,
      menuBox!.y + menuBox!.height,
    )
    expect(overlapBottom).toBeGreaterThan(overlapTop)

    const topRole = await page.evaluate(
      ({ x, y }) => document.elementFromPoint(x, y)?.getAttribute('role'),
      {
        x: menuBox!.x + 12,
        y: overlapTop + Math.min(12, (overlapBottom - overlapTop) / 2),
      },
    )
    expect(topRole).toBe('menuitem')

    await page.getByRole('menuitem').first().click()
    await expect(page.getByTestId('browser-tab')).toHaveCount(tabsBefore + 1)
    await address.fill(webURL)
    await address.press('Enter')
    await expect(surface).toHaveAttribute('data-status', 'ready')

    const popupURL = new URL('/popup', webURL)
    popupURL.searchParams.set('webview-window-open', 'handled')
    await webview.evaluate(async (element, url) => {
      const guest = element as ElectronWebviewElement
      await guest.executeJavaScript(
        `window.open(${JSON.stringify(url)}, '_blank')`,
        true,
      )
    }, popupURL.href)
    await expect(page.getByTestId('browser-tab')).toHaveCount(tabsBefore + 2)
    await expect.poll(() => surface.locator('webview').evaluate((element) =>
      (element as ElectronWebviewElement).getURL(),
    )).toBe(popupURL.href)
    await expect(surface).toHaveAttribute('data-status', 'ready')
    await expect(address).toHaveValue(popupURL.href)

    const pendingLoadURL = new URL(page.url())
    pendingLoadURL.searchParams.set('webview-load-promise', 'pending')
    await webview.evaluate((element) => {
      const guest = element as ElectronWebviewElement
      const loadURL = guest.loadURL.bind(guest)
      guest.loadURL = (url) => {
        void loadURL(url).catch(() => undefined)
        return new Promise<void>(() => undefined)
      }
    })
    await address.fill(pendingLoadURL.href)
    await address.press('Enter')
    await expect(surface).toHaveAttribute('data-status', 'ready', {
      timeout: 5_000,
    })
    await expect(address).toHaveValue(pendingLoadURL.href)
  } finally {
    await electronApp.close()
    await new Promise<void>((resolve) => webServer.close(() => resolve()))
    await rm(userDataDirectory, { recursive: true, force: true })
  }
})
