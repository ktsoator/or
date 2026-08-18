import { randomBytes } from 'node:crypto'
import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process'
import { stat } from 'node:fs/promises'
import http from 'node:http'
import path from 'node:path'
import readline from 'node:readline'
import {
  app,
  BrowserWindow,
  dialog,
  ipcMain,
  nativeTheme,
  session,
  shell,
  type WebContents,
} from 'electron'
import {
  browserPartition,
  guestKindForPartition,
  popupNavigationAction,
  previewNavigationAction,
  previewPartition,
  type GuestKind,
  type GuestNavigationAction,
} from './guestPolicy.js'
import { resolveDesktopEnvironment } from './shellEnvironment.js'

const isDevelopment = process.argv.includes('--dev')
const sidecarReadyTimeoutMs = 15_000
const rendererReadyTimeoutMs = 30_000
const allowedGuestPartitions = new Set([previewPartition, browserPartition])

type ReadyMessage = {
  type: 'ready'
  url: string
  previewURL: string
  cookieName: string
}

let mainWindow: BrowserWindow | null = null
let sidecar: ChildProcessWithoutNullStreams | null = null
let rendererDevServer: ChildProcessWithoutNullStreams | null = null
let quitting = false
let rendererURL = ''
let workspacePreviewURL = ''

if (!app.requestSingleInstanceLock()) {
  app.quit()
} else {
  app.on('second-instance', focusMainWindow)
  app.whenReady().then(start).catch(failStartup)
}

app.on('activate', () => {
  if (BrowserWindow.getAllWindows().length === 0 && rendererURL && workspacePreviewURL) {
    createWindow(rendererURL, workspacePreviewURL)
  }
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit()
})

app.on('before-quit', () => {
  quitting = true
  stopChild(rendererDevServer)
  stopChild(sidecar)
})

process.once('SIGINT', () => app.quit())
process.once('SIGTERM', () => app.quit())

async function start(): Promise<void> {
  applyDevelopmentDockIcon()
  registerIPC()
  const desktop = await startSidecar()
  configureGuestSession(previewPartition)
  configureGuestSession(browserPartition)
  await session.defaultSession.cookies.set({
    url: desktop.url,
    name: desktop.cookieName,
    value: desktop.token,
    httpOnly: true,
    sameSite: 'strict',
    secure: false,
    path: '/',
  })

  rendererURL = desktop.url
  workspacePreviewURL = desktop.previewURL
  if (isDevelopment) {
    rendererURL = await startRendererDevServer(desktop.url)
  }
  createWindow(rendererURL, workspacePreviewURL)
}

/**
 * A packaged build takes its Dock icon from the bundle that electron-builder
 * assembles out of build/appicon.png. An unpackaged `electron .` has no bundle,
 * so it shows Electron's own icon unless the same art is set at runtime.
 */
function applyDevelopmentDockIcon(): void {
  if (app.isPackaged || process.platform !== 'darwin' || !app.dock) return
  const icon = path.resolve(__dirname, '../build/appicon.png')
  try {
    app.dock.setIcon(icon)
  } catch (error) {
    // Cosmetic only: a missing or unreadable icon must not stop startup.
    console.warn(`[desktop] could not set the development dock icon`, error)
  }
}

function createWindow(url: string, previewURL: string): void {
  const window = new BrowserWindow({
    title: 'Or',
    width: 1280,
    height: 820,
    minWidth: 960,
    minHeight: 640,
    show: false,
    // Painted before the renderer has loaded, so it has to match the theme the
    // renderer is about to apply or the window opens on a flash of the wrong
    // canvas. The renderer owns the final answer, including a user override the
    // main process cannot see; this is only the opening frame.
    backgroundColor: nativeTheme.shouldUseDarkColors ? '#1e1e1e' : '#fbfbfa',
    titleBarStyle: process.platform === 'darwin' ? 'hiddenInset' : 'default',
    trafficLightPosition: process.platform === 'darwin' ? { x: 16, y: 18 } : undefined,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      devTools: isDevelopment,
      webviewTag: true,
      additionalArguments: [`--or-preview-origin=${previewURL}`],
    },
  })
  mainWindow = window

  window.once('ready-to-show', () => window.show())
  window.on('closed', () => {
    if (mainWindow === window) mainWindow = null
  })
  window.webContents.on('will-attach-webview', (event, webPreferences, params) => {
    const partition = params.partition
    if (!allowedGuestPartitions.has(partition) || params.src !== 'about:blank') {
      event.preventDefault()
      return
    }
    delete webPreferences.preload
    delete params.preload
    webPreferences.partition = partition
    webPreferences.sandbox = true
    webPreferences.contextIsolation = true
    webPreferences.nodeIntegration = false
    webPreferences.nodeIntegrationInSubFrames = false
    webPreferences.nodeIntegrationInWorker = false
    webPreferences.webSecurity = true
    webPreferences.allowRunningInsecureContent = false
    webPreferences.experimentalFeatures = false
    webPreferences.devTools = isDevelopment
  })
  window.webContents.on('did-attach-webview', (_event, guest) => {
    const partition = guest.session === session.fromPartition(previewPartition)
      ? previewPartition
      : browserPartition
    const kind = guestKindForPartition(partition)!
    configureGuestWindowHandling(window, guest, kind, previewURL)
  })
  window.webContents.setWindowOpenHandler(({ url: target }) => {
    void openExternalURL(target)
    return { action: 'deny' }
  })
  window.webContents.on('will-navigate', (event, target) => {
    if (new URL(target).origin === new URL(url).origin) return
    event.preventDefault()
    void openExternalURL(target)
  })
  void window.loadURL(url).catch(failStartup)
}

function registerIPC(): void {
  ipcMain.handle(
    'desktop:choose-directory',
    async (event, initialPath: unknown, title: unknown): Promise<string> => {
      const defaultPath = await existingDirectory(initialPath)
      const owner = BrowserWindow.fromWebContents(event.sender) ?? undefined
      const options = {
        title: typeof title === 'string' && title.trim() ? title : 'Choose a workspace folder',
        defaultPath,
        properties: ['openDirectory', 'createDirectory'] as ('openDirectory' | 'createDirectory')[],
      }
      const result = owner
        ? await dialog.showOpenDialog(owner, options)
        : await dialog.showOpenDialog(options)
      return result.canceled ? '' : (result.filePaths[0] ?? '')
    },
  )
  ipcMain.handle('desktop:open-external', async (_event, target: unknown): Promise<void> => {
    if (typeof target !== 'string') throw new TypeError('external URL must be a string')
    await openExternalURL(target)
  })
  ipcMain.handle('desktop:reveal-path', async (_event, target: unknown): Promise<void> => {
    if (typeof target !== 'string' || !path.isAbsolute(target)) {
      throw new TypeError('path to reveal must be an absolute path')
    }
    const info = await stat(target)
    if (!info.isDirectory()) throw new TypeError('path to reveal must be a directory')
    shell.showItemInFolder(target)
  })
}

function configureGuestSession(partition: string): void {
  const guestSession = session.fromPartition(partition)
  guestSession.setPermissionCheckHandler(() => false)
  guestSession.setPermissionRequestHandler((_webContents, _permission, callback) => {
    callback(false)
  })
  guestSession.on('will-download', (event) => event.preventDefault())
}

function configureGuestWindowHandling(
  window: BrowserWindow,
  guest: WebContents,
  kind: GuestKind,
  previewURL: string,
): void {
  guest.setWindowOpenHandler(({ url }) => {
    applyGuestNavigationAction(window, guest, url, popupNavigationAction(url))
    return { action: 'deny' }
  })

  if (kind !== 'preview') return
  const previewOrigin = new URL(previewURL).origin
  const handleNavigation = (event: { preventDefault: () => void }, url: string) => {
    const action = previewNavigationAction(url, previewOrigin)
    if (action === 'allow') return
    event.preventDefault()
    applyGuestNavigationAction(window, guest, url, action)
  }
  guest.on('will-navigate', handleNavigation)
  guest.on('will-redirect', handleNavigation)
}

function applyGuestNavigationAction(
  window: BrowserWindow,
  guest: WebContents,
  url: string,
  action: GuestNavigationAction,
): void {
  if (action === 'open-tab') {
    sendBrowserOpenTab(window, guest, new URL(url).href)
  } else if (action === 'open-external') {
    void openExternalURL(new URL(url).href)
  }
}

function sendBrowserOpenTab(
  window: BrowserWindow,
  guest: WebContents,
  url: string,
): void {
  if (window.isDestroyed() || guest.isDestroyed()) return
  window.webContents.send('desktop:browser-open-tab', {
    openerWebContentsID: guest.id,
    url,
  })
}

async function existingDirectory(value: unknown): Promise<string | undefined> {
  if (typeof value !== 'string' || !value.trim()) return undefined
  try {
    return (await stat(value)).isDirectory() ? value : undefined
  } catch {
    return undefined
  }
}

async function openExternalURL(target: string): Promise<void> {
  const url = new URL(target)
  if (!['http:', 'https:', 'mailto:', 'tel:'].includes(url.protocol)) {
    throw new Error(`unsupported external URL protocol: ${url.protocol}`)
  }
  await shell.openExternal(url.href)
}

async function startSidecar(): Promise<ReadyMessage & { token: string }> {
  const token = randomBytes(32).toString('hex')
  const binaryName = process.platform === 'win32' ? 'coding-sidecar.exe' : 'coding-sidecar'
  const executable = app.isPackaged
    ? path.join(process.resourcesPath, 'bin', binaryName)
    : path.join(__dirname, 'sidecar', binaryName)
  const assets = app.isPackaged
    ? path.join(process.resourcesPath, 'client')
    : path.resolve(__dirname, '../../client', isDevelopment ? '' : 'dist')
  const environment = await resolveDesktopEnvironment({ packaged: app.isPackaged })

  const child = spawn(executable, ['-assets', assets], {
    env: { ...environment, CODING_DESKTOP_TOKEN: token },
    stdio: ['pipe', 'pipe', 'pipe'],
    windowsHide: true,
  })
  sidecar = child
  child.stderr.on('data', (chunk: Buffer) => process.stderr.write(`[coding-sidecar] ${chunk}`))

  const ready = await waitForReadyMessage(child)
  if (child.exitCode !== null || child.signalCode !== null) {
    throw new Error(`Or service exited after startup (${child.signalCode ?? child.exitCode})`)
  }
  child.once('exit', (code, signal) => {
    if (!quitting) failStartup(new Error(`Or service exited (${signal ?? code ?? 'unknown'})`))
  })
  return { ...ready, token }
}

function waitForReadyMessage(child: ChildProcessWithoutNullStreams): Promise<ReadyMessage> {
  return new Promise((resolve, reject) => {
    const lines = readline.createInterface({ input: child.stdout })
    const timer = setTimeout(() => finish(new Error('timed out waiting for Or service')), sidecarReadyTimeoutMs)

    const finish = (error?: Error, ready?: ReadyMessage): void => {
      clearTimeout(timer)
      lines.removeAllListeners()
      if (error) reject(error)
      else resolve(ready!)
    }
    lines.once('line', (line) => {
      try {
        const value = JSON.parse(line) as Partial<ReadyMessage>
        if (value.type !== 'ready' || !value.url || !value.previewURL || !value.cookieName) {
          throw new Error('invalid sidecar ready message')
        }
        finish(undefined, value as ReadyMessage)
      } catch (error) {
        finish(error instanceof Error ? error : new Error(String(error)))
      }
    })
    child.once('error', (error) => finish(error))
    child.once('exit', (code) => finish(new Error(`Or service exited before ready (${code ?? 'unknown'})`)))
  })
}

async function startRendererDevServer(apiURL: string): Promise<string> {
  const clientDirectory = path.resolve(__dirname, '../../client')
  const url = 'http://127.0.0.1:5173'
  const child = spawn(
    process.env.BUN_EXEC_PATH ?? 'bun',
    ['run', 'dev', '--', '--host', '127.0.0.1', '--port', '5173', '--strictPort'],
    {
      cwd: clientDirectory,
      env: { ...process.env, CODING_API_PROXY: apiURL },
      stdio: ['pipe', 'pipe', 'pipe'],
      windowsHide: true,
    },
  )
  rendererDevServer = child
  child.stdout.on('data', (chunk: Buffer) => process.stdout.write(`[vite] ${chunk}`))
  child.stderr.on('data', (chunk: Buffer) => process.stderr.write(`[vite] ${chunk}`))
  await waitForHTTP(url, rendererReadyTimeoutMs)
  if (child.exitCode !== null || child.signalCode !== null) {
    throw new Error(`Vite renderer exited after startup (${child.signalCode ?? child.exitCode})`)
  }
  child.once('exit', (code, signal) => {
    if (!quitting) failStartup(new Error(`Vite renderer exited (${signal ?? code ?? 'unknown'})`))
  })
  return url
}

async function waitForHTTP(url: string, timeoutMs: number): Promise<void> {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      await new Promise<void>((resolve, reject) => {
        const request = http.get(url, (response) => {
          response.resume()
          if (response.statusCode && response.statusCode < 500) resolve()
          else reject(new Error(`renderer returned ${response.statusCode ?? 'no status'}`))
        })
        request.once('error', reject)
        request.setTimeout(1_000, () => request.destroy(new Error('renderer request timed out')))
      })
      return
    } catch {
      await new Promise((resolve) => setTimeout(resolve, 100))
    }
  }
  throw new Error('timed out waiting for Vite renderer')
}

function focusMainWindow(): void {
  if (!mainWindow) return
  if (mainWindow.isMinimized()) mainWindow.restore()
  mainWindow.show()
  mainWindow.focus()
}

function stopChild(child: ChildProcessWithoutNullStreams | null): void {
  if (child && child.exitCode === null && child.signalCode === null) child.kill('SIGTERM')
}

function failStartup(error: unknown): void {
  const message = error instanceof Error ? error.message : String(error)
  dialog.showErrorBox('Or could not start', message)
  app.quit()
}
