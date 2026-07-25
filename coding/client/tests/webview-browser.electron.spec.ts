import { createRequire } from 'node:module'
import { mkdtemp, rm } from 'node:fs/promises'
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

  try {
    const page = await electronApp.firstWindow()
    await page.waitForLoadState('domcontentloaded')

    const panelToggle = page.getByTestId('workbench-panel-toggle')
    if (await panelToggle.getAttribute('aria-expanded') === 'false') {
      await panelToggle.click()
    }

    await page.getByTestId('workbench-add-view').click()
    await page.getByRole('menuitem').first().click()

    const address = page.getByTestId('browser-address')
    await address.fill(page.url())
    await address.press('Enter')

    const surface = page.getByTestId('browser-surface')
    await expect(surface).toHaveAttribute('data-status', 'ready')
    const webview = surface.locator('webview')
    await expect(webview).toBeVisible()
    await expect(webview).toHaveAttribute('allowpopups', 'true')

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
    await address.fill(page.url())
    await address.press('Enter')
    await expect(surface).toHaveAttribute('data-status', 'ready')

    const popupURL = new URL(page.url())
    popupURL.hash = 'webview-window-open-handled'
    await webview.evaluate(async (element, url) => {
      const guest = element as ElectronWebviewElement
      await guest.executeJavaScript(
        `window.open(${JSON.stringify(url)}, '_blank')`,
        true,
      )
    }, popupURL.href)
    await expect.poll(() => webview.evaluate((element) =>
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
    await rm(userDataDirectory, { recursive: true, force: true })
  }
})
