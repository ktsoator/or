import { useEffect, useRef, useState } from 'react'
import { agentBrowserTabID, type ActiveBrowserTab } from './browserTabs'
import { inspectBrowser } from './lib/desktop'
import { sessionCommands } from './sessionCommands'
import { useCommandResultReporter } from './useCommandResultReporter'
import type {
  BrowserCommandState,
  BrowserInspectionCommandState,
  BrowserInspectionResult,
} from './types'

// An inspection waits for the foreground Agent navigation to finish, but only
// for this long. A command that can never be acknowledged must not block every
// later inspection of the session.
const pendingNavigationWaitMs = 10_000
const reportRetryMs = 1_000

export function useBrowserInspectionRequests({
  activeTab,
  sessionID,
  browserCommands,
  browserInspections,
  onHandled,
}: {
  activeTab?: ActiveBrowserTab
  sessionID?: string
  browserCommands: BrowserCommandState[]
  browserInspections: BrowserInspectionCommandState[]
  onHandled: (sessionID: string, commandID: string) => void
}) {
  const processedRef = useRef(new Set<string>())
  const waitStartedRef = useRef(new Map<string, number>())
  const timersRef = useRef(new Map<string, number>())
  const [, setRetryRevision] = useState(0)

  const report = useCommandResultReporter<BrowserInspectionResult>({
    send: sessionCommands.reportBrowserInspection,
    resolvedCode: 'browser_inspection_not_found',
    onHandled,
  })

  useEffect(() => () => {
    for (const timer of timersRef.current.values()) window.clearTimeout(timer)
    timersRef.current.clear()
  }, [])

  useEffect(() => {
    if (!sessionID) return
    const inspection = browserInspections.find(
      (candidate) => !processedRef.current.has(`${sessionID}:${candidate.commandID}`),
    )
    if (!inspection) return

    const key = `${sessionID}:${inspection.commandID}`
    const retry = (delay: number) => {
      if (timersRef.current.has(key)) return
      const timer = window.setTimeout(() => {
        timersRef.current.delete(key)
        processedRef.current.delete(key)
        setRetryRevision((revision) => revision + 1)
      }, delay)
      timersRef.current.set(key, timer)
    }

    // A restored history snapshot can contain navigation and inspection
    // requests together. Let the tab that will become foreground finish first.
    if (browserCommands.some((command) => command.disposition !== 'new_background_tab')) {
      const started = waitStartedRef.current.get(key) ?? Date.now()
      waitStartedRef.current.set(key, started)
      const remaining = pendingNavigationWaitMs - (Date.now() - started)
      if (remaining > 0) {
        // Stay unprocessed so an acknowledged command releases the wait
        // immediately; the timer only bounds how long it can last.
        retry(remaining)
        return
      }
    }

    processedRef.current.add(key)
    const inspectionTabID = resolveInspectionTabID(sessionID, activeTab)
    const inspectionRequest = inspectionTabID
      ? inspectBrowser(inspectionTabID)
      : Promise.reject(new Error('Current browser tab is user-owned and cannot be inspected'))
    void inspectionRequest
      .then((observed): BrowserInspectionResult => {
        if (!observed) throw new Error('Browser inspection is unavailable')
        return {
          status: 'completed',
          url: observed.url,
          title: observed.title,
          pageStatus: observed.pageStatus,
          revision: observed.revision,
          visibleText: observed.visibleText,
          truncated: observed.truncated,
        }
      })
      .catch((error: unknown): BrowserInspectionResult => ({
        status: 'failed',
        revision: 0,
        error: error instanceof Error ? error.message : String(error),
      }))
      .then(async (result) => {
        if (await report(sessionID, inspection.commandID, result)) {
          waitStartedRef.current.delete(key)
          return
        }
        retry(reportRetryMs)
      })
  }, [activeTab, browserCommands, browserInspections, report, sessionID])
}

function resolveInspectionTabID(
  sessionID: string,
  activeTab: ActiveBrowserTab | undefined,
): string | undefined {
  if (!activeTab) return agentBrowserTabID(sessionID)
  if (activeTab.owner === 'user') return undefined
  return activeTab.sessionID === sessionID
    ? activeTab.id
    : agentBrowserTabID(sessionID)
}
