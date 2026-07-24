import { useCallback, useEffect, useRef, useState } from 'react'
import { isAPIError } from './api'
import { agentBrowserTabID } from './browserTabs'
import { inspectNativeBrowser } from './lib/desktop'
import { sessionCommands } from './sessionCommands'
import type {
  BrowserCommandState,
  BrowserInspectionCommandState,
  BrowserInspectionResult,
} from './types'

const reportRetryDelays = [0, 250, 1000]

export function useBrowserInspectionRequests({
  sessionID,
  browserCommands,
  browserInspections,
  onHandled,
}: {
  sessionID?: string
  browserCommands: BrowserCommandState[]
  browserInspections: BrowserInspectionCommandState[]
  onHandled: (sessionID: string, commandID: string) => void
}) {
  const processedRef = useRef(new Set<string>())
  const reportingRef = useRef(new Set<string>())
  const reportedRef = useRef(new Set<string>())
  const retryTimersRef = useRef(new Map<string, number>())
  const [, setRetryRevision] = useState(0)

  useEffect(() => () => {
    for (const timer of retryTimersRef.current.values()) window.clearTimeout(timer)
    retryTimersRef.current.clear()
  }, [])

  const report = useCallback(async (
    commandSessionID: string,
    commandID: string,
    result: BrowserInspectionResult,
  ): Promise<boolean> => {
    const key = `${commandSessionID}:${commandID}`
    if (reportedRef.current.has(key)) return true
    if (reportingRef.current.has(key)) return false
    reportingRef.current.add(key)
    try {
      for (const delay of reportRetryDelays) {
        if (delay > 0) await new Promise((resolve) => window.setTimeout(resolve, delay))
        try {
          await sessionCommands.reportBrowserInspection(commandSessionID, commandID, result)
          reportedRef.current.add(key)
          onHandled(commandSessionID, commandID)
          return true
        } catch (error) {
          if (isAPIError(error, 'browser_inspection_not_found')) {
            reportedRef.current.add(key)
            onHandled(commandSessionID, commandID)
            return true
          }
        }
      }
      return false
    } finally {
      reportingRef.current.delete(key)
    }
  }, [onHandled])

  useEffect(() => {
    if (!sessionID) return
    const inspection = browserInspections.find(
      (candidate) => !processedRef.current.has(`${sessionID}:${candidate.commandID}`),
    )
    if (!inspection) return

    // A restored history snapshot can contain navigation and inspection
    // requests together. Let the stable Agent tab finish navigating first.
    if (browserCommands.some((command) => command.disposition === 'reuse_agent_tab')) return

    const key = `${sessionID}:${inspection.commandID}`
    processedRef.current.add(key)
    void inspectNativeBrowser(agentBrowserTabID(sessionID))
      .then((observed): BrowserInspectionResult => {
        if (!observed) throw new Error('Native browser inspection is unavailable')
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
        if (await report(sessionID, inspection.commandID, result)) return
        if (retryTimersRef.current.has(key)) return
        const timer = window.setTimeout(() => {
          retryTimersRef.current.delete(key)
          processedRef.current.delete(key)
          setRetryRevision((revision) => revision + 1)
        }, 1000)
        retryTimersRef.current.set(key, timer)
      })
  }, [browserCommands, browserInspections, report, sessionID])
}
