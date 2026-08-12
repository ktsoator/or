import { describe, expect, test } from 'bun:test'
import {
  captureLoginShellEnvironment,
  parseLoginShellEnvironment,
  resolveDesktopEnvironment,
} from '../src/shellEnvironment'

describe('desktop shell environment', () => {
  test('keeps the inherited environment outside a packaged macOS app', async () => {
    const environment = { PATH: '/usr/bin:/bin' }
    let captures = 0
    const capture = async () => {
      captures++
      return { PATH: '/opt/homebrew/bin:/usr/bin:/bin' }
    }

    expect(
      await resolveDesktopEnvironment({
        packaged: false,
        platform: 'darwin',
        environment,
        capture,
      }),
    ).toBe(environment)
    expect(
      await resolveDesktopEnvironment({
        packaged: true,
        platform: 'linux',
        environment,
        capture,
      }),
    ).toBe(environment)
    expect(captures).toBe(0)
  })

  test('uses the login shell environment for a packaged macOS app', async () => {
    const environment = { HOME: '/Users/example', PATH: '/usr/bin:/bin' }
    let capturedShell = ''
    const resolved = await resolveDesktopEnvironment({
      packaged: true,
      platform: 'darwin',
      environment,
      shell: '/bin/zsh',
      capture: async ({ shell }) => {
        capturedShell = shell
        return {
          PATH: '/Users/example/.volta/bin:/opt/homebrew/bin:/usr/bin:/bin',
          VOLTA_HOME: '/Users/example/.volta',
        }
      },
    })

    expect(capturedShell).toBe('/bin/zsh')
    expect(resolved).toEqual({
      HOME: '/Users/example',
      PATH: '/Users/example/.volta/bin:/opt/homebrew/bin:/usr/bin:/bin',
      VOLTA_HOME: '/Users/example/.volta',
    })
  })

  test('falls back without blocking startup when shell capture fails', async () => {
    const environment = { PATH: '/usr/bin:/bin' }
    const warnings: unknown[][] = []
    const resolved = await resolveDesktopEnvironment({
      packaged: true,
      platform: 'darwin',
      environment,
      shell: '/bin/zsh',
      capture: async () => {
        throw new Error('timed out')
      },
      warn: (...values) => warnings.push(values),
    })

    expect(resolved).toBe(environment)
    expect(warnings).toHaveLength(1)
    expect(String(warnings[0]?.[1])).toContain('timed out')
  })
})

describe('login shell environment parser', () => {
  test('ignores shell startup output and preserves equals signs in values', () => {
    const output = Buffer.concat([
      Buffer.from('startup message\n'),
      Buffer.from('\x1eOR_DESKTOP_ENV_START\x1f\0'),
      Buffer.from('PATH=/opt/homebrew/bin:/usr/bin\0TOKEN=left=right\0'),
      Buffer.from('\x1eOR_DESKTOP_ENV_END\x1f\0'),
      Buffer.from('logout message\n'),
    ])

    expect(parseLoginShellEnvironment(output)).toEqual({
      PATH: '/opt/homebrew/bin:/usr/bin',
      TOKEN: 'left=right',
    })
  })

  test('rejects output without a complete marked environment', () => {
    expect(() => parseLoginShellEnvironment(Buffer.from('PATH=/usr/bin\0'))).toThrow(
      'environment markers',
    )
  })

  test('rejects when the login shell cannot run the capture command', async () => {
    await expect(
      captureLoginShellEnvironment({
        shell: '/usr/bin/false',
        environment: { PATH: '/usr/bin:/bin' },
        timeoutMs: 100,
      }),
    ).rejects.toBeDefined()
  })
})
