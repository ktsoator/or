import { useEffect, useState } from 'react'
import { apiURL } from './api'
import type { ConnectionStatus } from './types'

const healthyProbeDelayMs = 10_000
const retryDelaysMs = [250, 500, 1_000, 2_000, 5_000] as const
const failuresBeforeDisconnect = 3

export type ServiceConnectionHandlers = {
  onStatus: (status: ConnectionStatus) => void
  onInitialized: () => void
}

export type ServiceConnectionDependencies = {
  health: (signal: AbortSignal) => Promise<void>
  sync: (signal: AbortSignal) => Promise<void>
  schedule: (callback: () => void, delayMs: number) => number
  cancelSchedule: (handle: number) => void
}

export type ServiceConnection = {
  retryNow: () => void
  stop: () => void
}

const browserDependencies = (
  sync: ServiceConnectionDependencies['sync'],
): ServiceConnectionDependencies => ({
  health: async (signal) => {
    const response = await fetch(apiURL('/health'), { cache: 'no-store', signal })
    // A 404 still proves that a pre-health-endpoint sidecar is reachable. This
    // matters in development, where Vite updates before the Go binary restarts.
    if (!response.ok && response.status !== 404) {
      throw new Error(`health check failed (${response.status})`)
    }
  },
  sync,
  schedule: (callback, delayMs) => window.setTimeout(callback, delayMs),
  cancelSchedule: (handle) => window.clearTimeout(handle),
})

export function startServiceConnection(
  handlers: ServiceConnectionHandlers,
  dependencies: ServiceConnectionDependencies,
): ServiceConnection {
  let active = true
  let initialized = false
  let connected = false
  let consecutiveFailures = 0
  let revision = 0
  let timer: number | undefined
  let controller: AbortController | undefined

  const clearTimer = () => {
    if (timer === undefined) return
    dependencies.cancelSchedule(timer)
    timer = undefined
  }

  const schedule = (delayMs: number, sync: boolean) => {
    clearTimer()
    timer = dependencies.schedule(() => {
      timer = undefined
      void attempt(sync)
    }, delayMs)
  }

  const finishInitialization = (currentRevision: number) => {
    if (!active || revision !== currentRevision || initialized) return
    initialized = true
    handlers.onInitialized()
  }

  const attempt = async (forceSync: boolean) => {
    if (!active) return
    const currentRevision = ++revision
    controller?.abort()
    controller = new AbortController()

    try {
      await dependencies.health(controller.signal)
      const recovering = consecutiveFailures > 0
      if (!connected || recovering || forceSync) {
        await dependencies.sync(controller.signal)
      }
      if (!active || revision !== currentRevision) return
      connected = true
      consecutiveFailures = 0
      handlers.onStatus('ready')
      schedule(healthyProbeDelayMs, false)
    } catch (error) {
      if (
        !active ||
        revision !== currentRevision ||
        (error instanceof DOMException && error.name === 'AbortError')
      ) {
        return
      }
      consecutiveFailures++
      if (!connected || consecutiveFailures >= failuresBeforeDisconnect) {
        handlers.onStatus('disconnected')
      }
      const retryIndex = Math.min(consecutiveFailures - 1, retryDelaysMs.length - 1)
      schedule(retryDelaysMs[retryIndex] ?? retryDelaysMs.at(-1)!, true)
    } finally {
      finishInitialization(currentRevision)
    }
  }

  const retryNow = () => {
    if (!active) return
    clearTimer()
    void attempt(true)
  }

  handlers.onStatus('connecting')
  void attempt(true)

  return {
    retryNow,
    stop: () => {
      active = false
      revision++
      clearTimer()
      controller?.abort()
    },
  }
}

export function useServiceConnection(
  sync: ServiceConnectionDependencies['sync'],
): { status: ConnectionStatus; initializing: boolean } {
  const [status, setStatus] = useState<ConnectionStatus>('connecting')
  const [initializing, setInitializing] = useState(true)

  useEffect(() => {
    const connection = startServiceConnection(
      {
        onStatus: setStatus,
        onInitialized: () => setInitializing(false),
      },
      browserDependencies(sync),
    )
    const retryWhenVisible = () => {
      if (document.visibilityState === 'visible') connection.retryNow()
    }
    window.addEventListener('focus', connection.retryNow)
    document.addEventListener('visibilitychange', retryWhenVisible)
    return () => {
      window.removeEventListener('focus', connection.retryNow)
      document.removeEventListener('visibilitychange', retryWhenVisible)
      connection.stop()
    }
  }, [sync])

  return { status, initializing }
}
