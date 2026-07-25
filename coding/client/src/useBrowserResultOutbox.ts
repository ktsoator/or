import { useEffect, useRef } from 'react'
import { isAPIError } from './api'
import { sessionCommands } from './sessionCommands'
import type { ThreadsState } from './sessionReducer'
import type { BrowserResultOutboxEntry } from './types'

type PendingBrowserResult = BrowserResultOutboxEntry & {
  sessionID: string
}

type ReporterWorker = {
  cancelled: boolean
  failures: number
  timer?: number
  wake?: () => void
}

const retryDelays = [250, 1_000, 2_000, 5_000, 10_000]

export function useBrowserResultOutbox(
  threads: ThreadsState,
  onAcknowledged: (sessionID: string, commandID: string) => void,
): void {
  const pendingRef = useRef(new Map<string, PendingBrowserResult>())
  const workersRef = useRef(new Map<string, ReporterWorker>())
  const onAcknowledgedRef = useRef(onAcknowledged)
  onAcknowledgedRef.current = onAcknowledged

  useEffect(() => {
    const pending = collectPendingBrowserResults(threads)
    pendingRef.current = pending

    for (const [key, worker] of workersRef.current) {
      if (!pending.has(key)) {
        cancelWorker(worker)
        workersRef.current.delete(key)
      }
    }

    for (const [key] of pending) {
      if (workersRef.current.has(key)) continue
      const worker: ReporterWorker = { cancelled: false, failures: 0 }
      workersRef.current.set(key, worker)
      void reportUntilAcknowledged(key, worker)
    }

    async function reportUntilAcknowledged(key: string, worker: ReporterWorker) {
      try {
        while (!worker.cancelled) {
          const entry = pendingRef.current.get(key)
          if (!entry) return
          try {
            await sessionCommands.reportBrowserResult(
              entry.sessionID,
              entry.commandID,
              entry.result,
            )
            if (!worker.cancelled) {
              onAcknowledgedRef.current(entry.sessionID, entry.commandID)
            }
            return
          } catch (error) {
            if (isAPIError(error, 'browser_command_not_found')) {
              if (!worker.cancelled) {
                onAcknowledgedRef.current(entry.sessionID, entry.commandID)
              }
              return
            }
          }

          const delay = retryDelays[Math.min(worker.failures, retryDelays.length - 1)]!
          worker.failures += 1
          await waitForRetry(worker, delay)
        }
      } finally {
        if (workersRef.current.get(key) === worker) workersRef.current.delete(key)
      }
    }
  }, [threads])

  useEffect(() => () => {
    pendingRef.current.clear()
    for (const worker of workersRef.current.values()) cancelWorker(worker)
    workersRef.current.clear()
  }, [])
}

function collectPendingBrowserResults(
  threads: ThreadsState,
): Map<string, PendingBrowserResult> {
  const pending = new Map<string, PendingBrowserResult>()
  for (const [sessionID, thread] of Object.entries(threads)) {
    for (const entry of Object.values(thread.browserResultOutbox)) {
      pending.set(`${sessionID}:${entry.commandID}`, { sessionID, ...entry })
    }
  }
  return pending
}

function waitForRetry(worker: ReporterWorker, delay: number): Promise<void> {
  return new Promise((resolve) => {
    const wake = () => {
      if (worker.timer !== undefined) window.clearTimeout(worker.timer)
      worker.timer = undefined
      worker.wake = undefined
      resolve()
    }
    worker.wake = wake
    worker.timer = window.setTimeout(wake, delay)
  })
}

function cancelWorker(worker: ReporterWorker): void {
  worker.cancelled = true
  worker.wake?.()
}
