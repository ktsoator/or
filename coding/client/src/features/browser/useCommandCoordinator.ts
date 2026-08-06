import { useCallback, useEffect, useRef, type Dispatch } from 'react'
import { agentBrowserTabID, type BrowserTab } from './tabs'
import {
  browserWorkspaceCommandTabID,
  type BrowserWorkspaceAction,
  type BrowserWorkspaceState,
} from './workspace'
import { workspacePreviewURL } from './urls'
import { closeBrowser, type BrowserRuntimeState } from './runtime'
import { browserRuntimeTabID } from './runtimeID'
import type { BrowserCommandState, BrowserResult, PreviewState } from '@/types'

export function browserPreviewKey(
  preview: PreviewState | undefined,
  sessionID?: string,
): string | undefined {
  if (!preview) return undefined
  return [
    sessionID ?? 'unknown',
    preview.revision,
    preview.commandID ?? '',
    preview.url ?? preview.path ?? '',
    preview.grantID ?? '',
    preview.previewPath ?? '',
  ].join(':')
}

export function browserPreviewTarget(
  preview: PreviewState,
  sessionID?: string,
) {
  const workspacePath = preview.path
  const requestedURL = workspacePath && sessionID && preview.grantID && preview.previewPath
    ? workspacePreviewURL(sessionID, preview.grantID, preview.previewPath)
    : preview.url ?? ''
  return {
    requestedURL,
    addressDraft: workspacePath ?? preview.url ?? '',
    kind: workspacePath ? 'workspace-preview' as const : 'web' as const,
    title: preview.title,
    workspacePath,
    commandID: preview.commandID,
  }
}

export function useBrowserCommandCoordinator({
  activatePreview,
  browserCommands,
  dispatch,
  onBrowserResult,
  preview,
  sessionID,
  state,
  workspaceID,
}: {
  activatePreview: boolean
  browserCommands: BrowserCommandState[]
  dispatch: Dispatch<BrowserWorkspaceAction>
  onBrowserResult: (sessionID: string, commandID: string, result: BrowserResult) => void
  preview?: PreviewState
  sessionID?: string
  state: BrowserWorkspaceState
  workspaceID: string
}) {
  const stateRef = useRef(state)
  const onBrowserResultRef = useRef(onBrowserResult)
  stateRef.current = state
  onBrowserResultRef.current = onBrowserResult

  const reportCancelled = useCallback((tab: BrowserTab | undefined) => {
    if (
      !tab?.sessionID ||
      !tab.desired?.commandID ||
      tab.observed.status === 'ready' ||
      tab.observed.status === 'failed'
    ) {
      return
    }
    onBrowserResultRef.current(tab.sessionID, tab.desired.commandID, {
      status: 'cancelled',
      requestedURL: absoluteHTTPURL(tab.desired.requestedURL),
      committedURL: absoluteHTTPURL(tab.observed.committedURL),
    })
  }, [])

  useEffect(() => {
    const command = browserCommands.find((candidate) => {
      const key = `${sessionID ?? 'unknown'}:${candidate.commandID}`
      return !state.commandTargets[key]
    })
    if (!command) return

    const commandKey = `${sessionID ?? 'unknown'}:${command.commandID}`
    const tabID = browserWorkspaceCommandTabID(
      state,
      sessionID,
      command.commandID,
      command.disposition,
    )
    const existing = state.tabs.find((tab) => tab.id === tabID)
    if (
      command.disposition === 'reuse_agent_tab' &&
      existing?.desired?.commandID !== command.commandID
    ) {
      reportCancelled(existing)
    }
    dispatch({
      t: 'agent_command_received',
      commandKey,
      tabID,
      sessionID: sessionID ?? 'unknown',
      target: browserPreviewTarget(command, sessionID),
      activate:
        command.disposition === 'new_foreground_tab' ||
        (command.disposition === 'reuse_agent_tab' && activatePreview),
      selectForAgent: command.disposition !== 'new_background_tab',
    })
  }, [activatePreview, browserCommands, dispatch, reportCancelled, sessionID, state])

  const previewKey = browserPreviewKey(preview, sessionID)
  useEffect(() => {
    if (
      !previewKey ||
      (!preview?.url && !preview?.path) ||
      preview.commandID ||
      preview.disposition ||
      state.handledPreviewKey === previewKey
    ) {
      return
    }
    dispatch({
      t: 'legacy_preview_received',
      previewKey,
      tabID: agentBrowserTabID(sessionID),
      sessionID: sessionID ?? 'unknown',
      target: browserPreviewTarget(preview, sessionID),
      activate: activatePreview,
    })
  }, [activatePreview, dispatch, preview, previewKey, sessionID, state.handledPreviewKey])

  const closeTab = useCallback((tabID: string): boolean => {
    const current = stateRef.current
    reportCancelled(current.tabs.find((tab) => tab.id === tabID))
    void closeBrowser(browserRuntimeTabID(workspaceID, tabID))
    dispatch({ t: 'close_tab', tabID })
    return current.tabs.length === 1 && !current.conversationTabID && !current.taskTabID
  }, [dispatch, reportCancelled, workspaceID])

  const runtimeStateReceived = useCallback((
    tabID: string,
    runtimeState: BrowserRuntimeState,
  ) => {
    const tab = stateRef.current.tabs.find((candidate) => candidate.id === tabID)
    const desired = tab?.desired
    dispatch({
      t: 'tab_action',
      action: {
        t: 'runtime_state_received',
        tabID,
        appliedRevision: runtimeState.appliedRevision,
        committedURL: runtimeState.committedURL,
        title: runtimeState.title,
        status: runtimeState.status,
        canGoBack: runtimeState.canGoBack,
        canGoForward: runtimeState.canGoForward,
        error: runtimeState.error,
      },
    })

    const committedURL = absoluteHTTPURL(runtimeState.committedURL)
    if (
      !tab?.sessionID ||
      !desired?.commandID ||
      runtimeState.appliedRevision !== desired.revision ||
      runtimeState.status === 'navigating' ||
      (runtimeState.status === 'ready' && !committedURL)
    ) {
      return
    }
    onBrowserResultRef.current(tab.sessionID, desired.commandID, {
      status: runtimeState.status === 'ready' ? 'committed' : 'failed',
      requestedURL: absoluteHTTPURL(runtimeState.requestedURL),
      committedURL,
      title: runtimeState.title || undefined,
      error: runtimeState.error,
    })
  }, [dispatch])

  return { closeTab, runtimeStateReceived }
}

function absoluteHTTPURL(value: string): string | undefined {
  try {
    const url = new URL(value)
    return url.protocol === 'http:' || url.protocol === 'https:' ? url.href : undefined
  } catch {
    return undefined
  }
}
