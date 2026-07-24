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
  session,
  shell,
} from 'electron'
import { NativeBrowserManager, type DesktopEndpoint } from './nativeBrowser.js'

const isDevelopment = process.argv.includes('--dev')
const sidecarReadyTimeoutMs = 15_000
const rendererReadyTimeoutMs = 30_000

type ReadyMessage = {
  type: 'ready'
  url: string
  cookieName: string
}

let mainWindow: BrowserWindow | null = null
let sidecar: ChildProcessWithoutNullStreams | null = null
let rendererDevServer: ChildProcessWithoutNullStreams | null = null
let nativeBrowser: NativeBrowserManager | null = null
let desktopEndpoint: DesktopEndpoint | null = null
let quitting = false
let rendererURL = ''

if (!app.requestSingleInstanceLock()) {
  app.quit()
} else {
  app.on('second-instance', focusMainWindow)
  app.whenReady().then(start).catch(failStartup)
}

app.on('activate', () => {
  if (BrowserWindow.getAllWindows().length === 0 && rendererURL) createWindow(rendererURL)
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
  registerIPC()
  const desktop = await startSidecar()
  desktopEndpoint = desktop
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
  if (isDevelopment) {
    rendererURL = await startRendererDevServer(desktop.url)
  }
  createWindow(rendererURL)
}

function createWindow(url: string): void {
  const window = new BrowserWindow({
    title: 'Coding',
    width: 1280,
    height: 820,
    minWidth: 960,
    minHeight: 640,
    show: false,
    backgroundColor: '#fbfbfa',
    titleBarStyle: process.platform === 'darwin' ? 'hiddenInset' : 'default',
    trafficLightPosition: process.platform === 'darwin' ? { x: 16, y: 18 } : undefined,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      devTools: isDevelopment,
    },
  })
  mainWindow = window
  if (!desktopEndpoint) throw new Error('desktop endpoint is unavailable')
  nativeBrowser = new NativeBrowserManager(window, desktopEndpoint)

  window.once('ready-to-show', () => window.show())
  window.on('closed', () => {
    nativeBrowser?.destroy()
    nativeBrowser = null
    if (mainWindow === window) mainWindow = null
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
  ipcMain.handle('desktop:browser:navigate', (event, input: unknown) =>
    browserFor(event.sender).navigate(input))
  ipcMain.handle('desktop:browser:set-viewport', (event, input: unknown) => {
    browserFor(event.sender).setViewport(input)
  })
  ipcMain.handle('desktop:browser:close', (event, tabID: unknown) => {
    browserFor(event.sender).close(tabID)
  })
  ipcMain.handle('desktop:browser:go-back', (event, tabID: unknown) => {
    browserFor(event.sender).goBack(tabID)
  })
  ipcMain.handle('desktop:browser:go-forward', (event, tabID: unknown) => {
    browserFor(event.sender).goForward(tabID)
  })
  ipcMain.handle('desktop:browser:inspect', (event, tabID: unknown) =>
    browserFor(event.sender).inspect(tabID))
}

function browserFor(sender: Electron.WebContents): NativeBrowserManager {
  if (!mainWindow || sender !== mainWindow.webContents || !nativeBrowser) {
    throw new Error('native browser is unavailable')
  }
  return nativeBrowser
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
    : path.resolve(__dirname, '../../client')

  const child = spawn(executable, ['-assets', assets], {
    env: { ...process.env, CODING_DESKTOP_TOKEN: token },
    stdio: ['pipe', 'pipe', 'pipe'],
    windowsHide: true,
  })
  sidecar = child
  child.stderr.on('data', (chunk: Buffer) => process.stderr.write(`[coding-sidecar] ${chunk}`))

  const ready = await waitForReadyMessage(child)
  if (child.exitCode !== null || child.signalCode !== null) {
    throw new Error(`Coding sidecar exited after startup (${child.signalCode ?? child.exitCode})`)
  }
  child.once('exit', (code, signal) => {
    if (!quitting) failStartup(new Error(`Coding sidecar exited (${signal ?? code ?? 'unknown'})`))
  })
  return { ...ready, token }
}

function waitForReadyMessage(child: ChildProcessWithoutNullStreams): Promise<ReadyMessage> {
  return new Promise((resolve, reject) => {
    const lines = readline.createInterface({ input: child.stdout })
    const timer = setTimeout(() => finish(new Error('timed out waiting for Coding sidecar')), sidecarReadyTimeoutMs)

    const finish = (error?: Error, ready?: ReadyMessage): void => {
      clearTimeout(timer)
      lines.removeAllListeners()
      if (error) reject(error)
      else resolve(ready!)
    }
    lines.once('line', (line) => {
      try {
        const value = JSON.parse(line) as Partial<ReadyMessage>
        if (value.type !== 'ready' || !value.url || !value.cookieName) {
          throw new Error('invalid sidecar ready message')
        }
        finish(undefined, value as ReadyMessage)
      } catch (error) {
        finish(error instanceof Error ? error : new Error(String(error)))
      }
    })
    child.once('error', (error) => finish(error))
    child.once('exit', (code) => finish(new Error(`Coding sidecar exited before ready (${code ?? 'unknown'})`)))
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
  dialog.showErrorBox('Coding could not start', message)
  app.quit()
}
