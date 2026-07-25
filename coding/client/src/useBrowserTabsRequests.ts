import { useEffect, useRef, useState } from 'react'
import {
  browserWorkspaceContext,
  type BrowserWorkspaceState,
} from './browserWorkspace'
import { sessionCommands } from './sessionCommands'
import { useCommandResultReporter } from './useCommandResultReporter'
import type { BrowserTabsCommandState, BrowserTabsResult } from './types'

const reportRetryMs = 1_000

export function useBrowserTabsRequests({
  workspace,
  sessionID,
  requests,
  onHandled,
}: {
  workspace?: BrowserWorkspaceState
  sessionID?: string
  requests: BrowserTabsCommandState[]
  onHandled: (sessionID: string, commandID: string) => void
}) {
  const processedRef = useRef(new Set<string>())
  const timersRef = useRef(new Map<string, number>())
  const [, setRetryRevision] = useState(0)
  const report = useCommandResultReporter<BrowserTabsResult>({
    send: sessionCommands.reportBrowserTabs,
    resolvedCode: 'browser_tabs_not_found',
    onHandled,
  })

  useEffect(() => () => {
    for (const timer of timersRef.current.values()) window.clearTimeout(timer)
    timersRef.current.clear()
  }, [])

  useEffect(() => {
    if (!sessionID) return
    const request = requests.find(
      (candidate) => !processedRef.current.has(`${sessionID}:${candidate.commandID}`),
    )
    if (!request) return

    const key = `${sessionID}:${request.commandID}`
    processedRef.current.add(key)
    const context = browserWorkspaceContext(workspace)
    const result: BrowserTabsResult = {
      status: 'completed',
      openTabs: context.openTabs,
      controlledTabs: context.controlledTabs,
      selected: context.selected,
    }
    void report(sessionID, request.commandID, result).then((settled) => {
      if (settled || timersRef.current.has(key)) return
      const timer = window.setTimeout(() => {
        timersRef.current.delete(key)
        processedRef.current.delete(key)
        setRetryRevision((revision) => revision + 1)
      }, reportRetryMs)
      timersRef.current.set(key, timer)
    })
  }, [report, requests, sessionID, workspace])
}
