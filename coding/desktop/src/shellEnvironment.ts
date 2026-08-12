import { execFile } from 'node:child_process'
import { userInfo } from 'node:os'

const captureTimeoutMs = 5_000
const captureMaxBytes = 4 * 1024 * 1024
const environmentStart = Buffer.from('\x1eOR_DESKTOP_ENV_START\x1f\0')
const environmentEnd = Buffer.from('\x1eOR_DESKTOP_ENV_END\x1f\0')
const captureCommand =
  "/usr/bin/printf '\\036OR_DESKTOP_ENV_START\\037\\000'; " +
  "/usr/bin/env -0; " +
  "/usr/bin/printf '\\036OR_DESKTOP_ENV_END\\037\\000'"

type CaptureOptions = {
  shell: string
  environment: NodeJS.ProcessEnv
  timeoutMs?: number
}

type DesktopEnvironmentOptions = {
  packaged: boolean
  platform?: NodeJS.Platform
  environment?: NodeJS.ProcessEnv
  shell?: string
  capture?: (options: CaptureOptions) => Promise<NodeJS.ProcessEnv>
  warn?: (message: string, error: unknown) => void
}

export async function resolveDesktopEnvironment({
  packaged,
  platform = process.platform,
  environment = process.env,
  shell = loginShell(environment),
  capture = captureLoginShellEnvironment,
  warn = (message, error) => console.warn(message, error),
}: DesktopEnvironmentOptions): Promise<NodeJS.ProcessEnv> {
  if (!packaged || platform !== 'darwin') return environment

  try {
    const captured = await capture({ shell, environment })
    return { ...environment, ...captured }
  } catch (error) {
    warn('[desktop] could not load the login shell environment; using the app environment', error)
    return environment
  }
}

export function captureLoginShellEnvironment({
  shell,
  environment,
  timeoutMs = captureTimeoutMs,
}: CaptureOptions): Promise<NodeJS.ProcessEnv> {
  return new Promise((resolve, reject) => {
    execFile(
      shell,
      ['-ilc', captureCommand],
      {
        encoding: 'buffer',
        env: environment,
        maxBuffer: captureMaxBytes,
        timeout: timeoutMs,
      },
      (error, stdout) => {
        if (error) {
          reject(error)
          return
        }
        try {
          resolve(parseLoginShellEnvironment(stdout))
        } catch (parseError) {
          reject(parseError)
        }
      },
    )
  })
}

export function parseLoginShellEnvironment(output: Buffer): NodeJS.ProcessEnv {
  const start = output.indexOf(environmentStart)
  const contentStart = start + environmentStart.length
  const end = start < 0 ? -1 : output.indexOf(environmentEnd, contentStart)
  if (start < 0 || end < 0) throw new Error('login shell output did not contain environment markers')

  const environment: NodeJS.ProcessEnv = {}
  for (const entry of output.subarray(contentStart, end).toString('utf8').split('\0')) {
    if (!entry) continue
    const separator = entry.indexOf('=')
    if (separator <= 0) throw new Error('login shell returned an invalid environment entry')
    environment[entry.slice(0, separator)] = entry.slice(separator + 1)
  }
  return environment
}

function loginShell(environment: NodeJS.ProcessEnv): string {
  try {
    const shell = userInfo().shell?.trim()
    if (shell) return shell
  } catch {
    // Fall through to the inherited shell when account lookup is unavailable.
  }
  return environment.SHELL?.trim() || '/bin/zsh'
}
