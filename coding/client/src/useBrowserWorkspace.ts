import { useCallback, useEffect, useReducer, useRef } from 'react'
import {
  agentBrowserTabID,
  browserTabNavigationURL,
  createBrowserTab,
  type BrowserTabsAction,
} from './browserTabs'
import {
  browserWorkspaceRegistryReducer,
  createBrowserWorkspaceRegistryState,
  createBrowserWorkspaceState,
  selectedBrowserTab,
  type BrowserWorkspaceAction,
  type BrowserWorkspaceState,
} from './browserWorkspace'
import {
  browserPreviewKey,
  browserPreviewTarget,
  useBrowserCommandCoordinator,
} from './useBrowserCommandCoordinator'
import { normalizeBrowserAddress, workspaceFileURL } from './lib/browser'
import { browserRuntimeTabID } from './browserRuntime'
import {
  goBackBrowser,
  goForwardBrowser,
  hasBrowserRuntime,
  openExternalURL,
} from './lib/desktop'
import type {
  BrowserCommandState,
  BrowserControlCapability,
  BrowserResult,
  PreviewState,
} from './types'

export function conversationWorkbenchTabID(sessionID?: string): string | undefined {
  return sessionID ? `conversation:${sessionID}` : undefined
}

export function useBrowserWorkspace({
  activatePreview,
  browserCommands,
  conversationID,
  onBrowserResult,
  preview,
  sessionID,
}: {
  activatePreview: boolean
  browserCommands: BrowserCommandState[]
  conversationID?: string
  onBrowserResult: (sessionID: string, commandID: string, result: BrowserResult) => void
  preview?: PreviewState
  sessionID?: string
}) {
  const conversationTabID = conversationWorkbenchTabID(conversationID)
  const workspaceID = sessionID ?? conversationID ?? 'unknown'
  const fallbackWorkspace = createInitialBrowserWorkspace({
    activatePreview,
    browserCommands,
    conversationID,
    conversationTabID,
    preview,
    sessionID,
  })
  const [registry, dispatchRegistry] = useReducer(
    browserWorkspaceRegistryReducer,
    undefined,
    () => createBrowserWorkspaceRegistryState(workspaceID, fallbackWorkspace),
  )
  const state = registry.workspaces[workspaceID] ?? fallbackWorkspace
  const registryRef = useRef(registry)
  registryRef.current = registry
  const initialStateRef = useRef(state)
  initialStateRef.current = state
  const dispatch = useCallback((action: BrowserWorkspaceAction) => {
    dispatchRegistry({
      t: 'workspace_action',
      workspaceID,
      initialState: initialStateRef.current,
      action,
    })
  }, [workspaceID])

  const dispatchWorkspace = useCallback((
    targetWorkspaceID: string,
    action: BrowserWorkspaceAction,
  ) => {
    const initialState = targetWorkspaceID === workspaceID
      ? initialStateRef.current
      : registryRef.current.workspaces[targetWorkspaceID] ??
        createBrowserWorkspaceState()
    dispatchRegistry({
      t: 'workspace_action',
      workspaceID: targetWorkspaceID,
      initialState,
      action,
    })
  }, [workspaceID])

  useEffect(() => {
    dispatch({ t: 'sync_conversation', conversationTabID })
  }, [conversationTabID, dispatch, workspaceID])

  const coordinator = useBrowserCommandCoordinator({
    activatePreview,
    browserCommands,
    dispatch,
    onBrowserResult,
    preview,
    sessionID,
    state,
    workspaceID,
  })
  const activeTab = selectedBrowserTab(state)
  const conversationActive = state.activeItemID === state.conversationTabID
  const activeDesired = activeTab?.desired
  const activeObserved = activeTab?.observed
  const activeNavigationURL = activeTab ? browserTabNavigationURL(activeTab) : ''
  const activeExternalURL = activeDesired
    ? activeDesired.workspacePath
      ? workspaceFileURL(activeDesired.workspacePath)
      : activeNavigationURL
    : ''
  const workspaceForSession = useCallback((targetSessionID?: string) => {
    const targetWorkspaceID = targetSessionID ?? 'unknown'
    return targetWorkspaceID === workspaceID
      ? state
      : registry.workspaces[targetWorkspaceID]
  }, [registry.workspaces, state, workspaceID])

  const dispatchTabAction = useCallback((action: BrowserTabsAction) => {
    dispatch({ t: 'tab_action', action })
  }, [dispatch])

  const attachControl = useCallback((
    targetSessionID: string,
    leaseID: string,
    tabID: string,
    capabilities: BrowserControlCapability[],
  ) => {
    dispatchWorkspace(targetSessionID, {
      t: 'attach_control',
      leaseID,
      tabID,
      capabilities,
    })
  }, [dispatchWorkspace])

  const releaseControl = useCallback((targetSessionID: string, leaseID: string) => {
    dispatchWorkspace(targetSessionID, { t: 'release_control', leaseID })
  }, [dispatchWorkspace])

  const reload = useCallback((tabID = activeTab?.id) => {
    if (!tabID) return
    dispatchTabAction({ t: 'reload', tabID })
  }, [activeTab?.id, dispatchTabAction])

  const navigateActiveAddress = useCallback(() => {
    if (!activeTab) return
    if (
      activeDesired?.workspacePath &&
      activeTab.addressDraft === activeDesired.workspacePath
    ) {
      reload(activeTab.id)
      return
    }
    const next = normalizeBrowserAddress(activeTab.addressDraft)
    if (!next) {
      dispatchTabAction({ t: 'reject_address', tabID: activeTab.id })
      return
    }
    dispatchTabAction({
      t: 'submit_navigation',
      tabID: activeTab.id,
      source: 'address',
      target: {
        requestedURL: next,
        addressDraft: next,
        kind: 'web',
      },
    })
  }, [activeDesired?.workspacePath, activeTab, dispatchTabAction, reload])

  const openExternal = useCallback(() => {
    if (activeExternalURL) openExternalURL(activeExternalURL)
  }, [activeExternalURL])

  return {
    state,
    tabs: state.tabs,
    workspaceID,
    activeItemID: state.activeItemID,
    conversationTabID: state.conversationTabID,
    conversationActive,
    activeTab,
    activeDesired,
    activeObserved,
    activeExternalURL,
    workspaceForSession,
    attachControl,
    releaseControl,
    browserRuntime: hasBrowserRuntime(),
    selectItem: (itemID: string) => dispatch({ t: 'select_item', itemID }),
    newTab: () => dispatch({ t: 'create_user_tab' }),
    closeTab: coordinator.closeTab,
    reload,
    navigateActiveAddress,
    editAddress: (tabID: string, address: string) =>
      dispatchTabAction({ t: 'edit_address', tabID, address }),
    resolveNavigation: (tabID: string, revision: number, url: string) =>
      dispatchTabAction({ t: 'resolve_navigation', tabID, revision, url }),
    runtimeStateReceived: coordinator.runtimeStateReceived,
    goBack: () => {
      if (!activeTab) return
      dispatch({ t: 'release_tab_control', tabID: activeTab.id })
      void goBackBrowser(browserRuntimeTabID(workspaceID, activeTab.id))
    },
    goForward: () => {
      if (!activeTab) return
      dispatch({ t: 'release_tab_control', tabID: activeTab.id })
      void goForwardBrowser(browserRuntimeTabID(workspaceID, activeTab.id))
    },
    openExternal,
  }
}

export type BrowserWorkspaceController = ReturnType<typeof useBrowserWorkspace>

function createInitialBrowserWorkspace({
  activatePreview,
  browserCommands,
  conversationID,
  conversationTabID,
  preview,
  sessionID,
}: {
  activatePreview: boolean
  browserCommands: BrowserCommandState[]
  conversationID?: string
  conversationTabID?: string
  preview?: PreviewState
  sessionID?: string
}): BrowserWorkspaceState {
  const initialTab = browserCommands.length === 0 && preview
      ? createBrowserTab({
        id: agentBrowserTabID(sessionID),
        sessionID: sessionID ?? 'unknown',
        target: browserPreviewTarget(preview, sessionID),
        source: 'agent',
      })
    : undefined
  return createBrowserWorkspaceState({
    initialTab,
    conversationTabID,
    activeItemID:
      activatePreview && initialTab
        ? initialTab.id
        : conversationID
          ? conversationTabID
          : initialTab?.id,
    handledPreviewKey: browserPreviewKey(preview, sessionID),
  })
}
