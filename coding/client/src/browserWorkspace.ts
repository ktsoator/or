import type {
  BrowserControlCapability,
  BrowserControlledTab,
  BrowserDisposition,
  BrowserOpenTab,
} from './generated/wire'
import {
  agentBrowserCommandTabID,
  agentBrowserTabID,
  browserTabsReducer,
  type BrowserNavigationTarget,
  type BrowserTab,
  type BrowserTabsAction,
} from './browserTabs'

type BrowserControlLease = {
  tabID: string
  capabilities: BrowserControlCapability[]
}

type BrowserWorkspaceContext = {
  openTabs: BrowserOpenTab[]
  controlledTabs: BrowserControlledTab[]
  selected?: string
}

export type BrowserWorkspaceState = {
  tabs: BrowserTab[]
  activeItemID: string
  conversationTabID?: string
  taskTabID?: string
  selectedTaskID?: string
  agentSelectedTabID?: string
  nextUserTabSequence: number
  controlLeases: Record<string, BrowserControlLease>
  commandTargets: Record<string, string>
  handledPreviewKey?: string
}

type BrowserWorkspaceRegistryState = {
  workspaces: Record<string, BrowserWorkspaceState>
}

export type BrowserWorkspaceAction =
  | { t: 'select_item'; itemID: string }
  | { t: 'sync_conversation'; conversationTabID?: string }
  | { t: 'open_tasks'; taskTabID: string; taskID?: string }
  | { t: 'select_task'; taskID: string }
  | { t: 'close_tasks' }
  | { t: 'create_user_tab' }
  | { t: 'close_tab'; tabID: string }
  | { t: 'tab_action'; action: BrowserTabsAction }
  | {
      t: 'attach_control'
      leaseID: string
      tabID: string
      capabilities: BrowserControlCapability[]
      select?: boolean
    }
  | { t: 'release_control'; leaseID: string }
  | { t: 'release_tab_control'; tabID: string }
  | {
      t: 'agent_command_received'
      commandKey: string
      tabID: string
      sessionID: string
      target: BrowserNavigationTarget
      activate: boolean
      selectForAgent: boolean
    }
  | {
      t: 'legacy_preview_received'
      previewKey: string
      tabID: string
      sessionID: string
      target: BrowserNavigationTarget
      activate: boolean
    }

type BrowserWorkspaceRegistryAction = {
  t: 'workspace_action'
  workspaceID: string
  initialState: BrowserWorkspaceState
  action: BrowserWorkspaceAction
}

export function createBrowserWorkspaceState({
  initialTab,
  activeItemID,
  conversationTabID,
  handledPreviewKey,
}: {
  initialTab?: BrowserTab
  activeItemID?: string
  conversationTabID?: string
  handledPreviewKey?: string
} = {}): BrowserWorkspaceState {
  const initiallyControlled = initialTab?.desired?.source === 'agent'
  return {
    tabs: initialTab ? [initialTab] : [],
    activeItemID: activeItemID ?? initialTab?.id ?? '',
    conversationTabID,
    agentSelectedTabID: initiallyControlled ? initialTab.id : undefined,
    nextUserTabSequence: initialTab?.id === 'tab-1' ? 1 : 0,
    controlLeases: initiallyControlled
      ? {
          [browserNavigationLeaseID(initialTab.id)]: {
            tabID: initialTab.id,
            capabilities: ['read', 'navigate'],
          },
        }
      : {},
    commandTargets: {},
    handledPreviewKey,
  }
}

export function createBrowserWorkspaceRegistryState(
  workspaceID?: string,
  workspace?: BrowserWorkspaceState,
): BrowserWorkspaceRegistryState {
  return {
    workspaces: workspaceID && workspace ? { [workspaceID]: workspace } : {},
  }
}

export function browserWorkspaceRegistryReducer(
  state: BrowserWorkspaceRegistryState,
  action: BrowserWorkspaceRegistryAction,
): BrowserWorkspaceRegistryState {
  const current = state.workspaces[action.workspaceID] ?? action.initialState
  const workspace = browserWorkspaceReducer(current, action.action)
  if (state.workspaces[action.workspaceID] === workspace) return state
  return {
    workspaces: {
      ...state.workspaces,
      [action.workspaceID]: workspace,
    },
  }
}

export function browserWorkspaceReducer(
  state: BrowserWorkspaceState,
  action: BrowserWorkspaceAction,
): BrowserWorkspaceState {
  switch (action.t) {
    case 'select_item':
      return action.itemID === state.activeItemID
        ? state
        : { ...state, activeItemID: action.itemID }

    case 'sync_conversation': {
      const previous = state.conversationTabID
      if (previous === action.conversationTabID) return state
      if (action.conversationTabID) {
        return {
          ...state,
          conversationTabID: action.conversationTabID,
          activeItemID: action.conversationTabID,
        }
      }
      return {
        ...state,
        conversationTabID: undefined,
        activeItemID:
          previous && state.activeItemID === previous
            ? state.taskTabID ?? state.tabs[0]?.id ?? ''
            : state.activeItemID,
      }
    }

    case 'open_tasks':
      return {
        ...state,
        taskTabID: action.taskTabID,
        selectedTaskID: action.taskID ?? state.selectedTaskID,
        activeItemID: action.taskTabID,
      }

    case 'select_task':
      return action.taskID === state.selectedTaskID
        ? state
        : { ...state, selectedTaskID: action.taskID }

    case 'close_tasks': {
      if (!state.taskTabID) return state
      return {
        ...state,
        taskTabID: undefined,
        selectedTaskID: undefined,
        activeItemID:
          state.activeItemID === state.taskTabID
            ? state.conversationTabID ?? state.tabs.at(-1)?.id ?? ''
            : state.activeItemID,
      }
    }

    case 'create_user_tab': {
      const nextUserTabSequence = state.nextUserTabSequence + 1
      const tabID = `tab-${nextUserTabSequence}`
      return {
        ...state,
        tabs: browserTabsReducer(state.tabs, { t: 'create_tab', tabID }),
        activeItemID: tabID,
        nextUserTabSequence,
      }
    }

    case 'close_tab': {
      const closingIndex = state.tabs.findIndex((tab) => tab.id === action.tabID)
      if (closingIndex < 0) return state
      const tabs = browserTabsReducer(state.tabs, {
        t: 'close_tab',
        tabID: action.tabID,
      })
      const next = tabs[Math.min(closingIndex, tabs.length - 1)]
      const controlLeases = withoutTabControl(state.controlLeases, action.tabID)
      return {
        ...state,
        tabs,
        controlLeases,
        agentSelectedTabID:
          state.agentSelectedTabID === action.tabID
            ? undefined
            : state.agentSelectedTabID,
        activeItemID:
          state.activeItemID === action.tabID
            ? next?.id ?? state.conversationTabID ?? state.taskTabID ?? ''
            : state.activeItemID,
      }
    }

    case 'tab_action': {
      const tabs = browserTabsReducer(state.tabs, action.action)
      if (tabs === state.tabs) return state
      const next = { ...state, tabs }
      return isManualNavigation(action.action)
        ? releaseTabControl(next, action.action.tabID)
        : next
    }

    case 'attach_control': {
      if (!state.tabs.some((tab) => tab.id === action.tabID)) return state
      const capabilities = uniqueCapabilities(action.capabilities)
      if (capabilities.length === 0) return state
      const lease: BrowserControlLease = {
        tabID: action.tabID,
        capabilities,
      }
      const current = state.controlLeases[action.leaseID]
      if (
        current &&
        current.tabID === lease.tabID &&
        sameCapabilities(current.capabilities, lease.capabilities) &&
        (!action.select || state.agentSelectedTabID === action.tabID)
      ) {
        return state
      }
      return {
        ...state,
        controlLeases: { ...state.controlLeases, [action.leaseID]: lease },
        agentSelectedTabID:
          action.select && capabilities.includes('navigate')
            ? action.tabID
            : state.agentSelectedTabID,
      }
    }

    case 'release_control': {
      if (!state.controlLeases[action.leaseID]) return state
      const controlLeases = { ...state.controlLeases }
      delete controlLeases[action.leaseID]
      return normalizeAgentSelection({ ...state, controlLeases })
    }

    case 'release_tab_control':
      return releaseTabControl(state, action.tabID)

    case 'agent_command_received':
      if (state.commandTargets[action.commandKey]) return state
      return {
        ...state,
        tabs: browserTabsReducer(state.tabs, {
          t: 'agent_navigate',
          tabID: action.tabID,
          sessionID: action.sessionID,
          target: action.target,
        }),
        activeItemID: action.activate ? action.tabID : state.activeItemID,
        agentSelectedTabID: action.selectForAgent
          ? action.tabID
          : state.agentSelectedTabID,
        controlLeases: {
          ...state.controlLeases,
          [browserNavigationLeaseID(action.tabID)]: {
            tabID: action.tabID,
            capabilities: ['read', 'navigate'],
          },
        },
        commandTargets: {
          ...state.commandTargets,
          [action.commandKey]: action.tabID,
        },
      }

    case 'legacy_preview_received':
      if (state.handledPreviewKey === action.previewKey) return state
      return {
        ...state,
        tabs: browserTabsReducer(state.tabs, {
          t: 'agent_navigate',
          tabID: action.tabID,
          sessionID: action.sessionID,
          target: action.target,
        }),
        activeItemID: action.activate ? action.tabID : state.activeItemID,
        agentSelectedTabID: action.tabID,
        controlLeases: {
          ...state.controlLeases,
          [browserNavigationLeaseID(action.tabID)]: {
            tabID: action.tabID,
            capabilities: ['read', 'navigate'],
          },
        },
        handledPreviewKey: action.previewKey,
      }
  }
}

export function selectedBrowserTab(
  state: BrowserWorkspaceState,
): BrowserTab | undefined {
  if (
    state.activeItemID === state.conversationTabID ||
    state.activeItemID === state.taskTabID
  ) {
    return undefined
  }
  return state.tabs.find((tab) => tab.id === state.activeItemID) ?? state.tabs[0]
}

export function browserWorkspaceContext(
  state: BrowserWorkspaceState | undefined,
): BrowserWorkspaceContext {
  if (!state) return { openTabs: [], controlledTabs: [] }
  const controlledTabs = state.tabs.flatMap((tab) => {
    const capabilities = browserWorkspaceControlCapabilities(state, tab.id)
    return capabilities.length > 0 ? [{ tabID: tab.id, capabilities }] : []
  })
  const controlledIDs = new Set(controlledTabs.map((tab) => tab.tabID))
  return {
    openTabs: state.tabs.map((tab) => ({
      tabID: tab.id,
      url: browserContextURL(tab),
      title: tab.observed.title || tab.desired?.title || undefined,
      status: tab.observed.status,
    })),
    controlledTabs,
    selected:
      state.agentSelectedTabID && controlledIDs.has(state.agentSelectedTabID)
        ? state.agentSelectedTabID
        : undefined,
  }
}

export function browserWorkspaceInspectionTabID(
  state: BrowserWorkspaceState | undefined,
  sessionID: string,
  requestedTabID?: string,
): string {
  const tab = requestedTabID
    ? state?.tabs.find((candidate) => candidate.id === requestedTabID)
    : state?.agentSelectedTabID
      ? state.tabs.find((candidate) => candidate.id === state.agentSelectedTabID)
      : undefined
  if (requestedTabID && !tab) {
    throw new Error('Browser tab is not open in this session')
  }
  if (!tab) return agentBrowserTabID(sessionID)
  return tab.id
}

function browserContextURL(tab: BrowserTab): string | undefined {
  const value = tab.observed.committedURL || tab.desired?.requestedURL
  if (!value) return undefined
  try {
    const url = new URL(value)
    return url.protocol === 'http:' || url.protocol === 'https:'
      ? url.href
      : undefined
  } catch {
    return undefined
  }
}

export function browserWorkspaceCommandTabID(
  state: BrowserWorkspaceState,
  sessionID: string | undefined,
  commandID: string,
  disposition: BrowserDisposition,
): string {
  if (disposition !== 'reuse_agent_tab') {
    return agentBrowserCommandTabID(sessionID, commandID)
  }
  const selected = state.agentSelectedTabID
  if (
    selected &&
    state.tabs.some((tab) => tab.id === selected) &&
    browserWorkspaceHasControl(state, selected, 'navigate')
  ) {
    return selected
  }
  const stableTabID = agentBrowserTabID(sessionID)
  if (!state.tabs.some((tab) => tab.id === stableTabID)) return stableTabID
  if (browserWorkspaceHasControl(state, stableTabID, 'navigate')) return stableTabID
  return agentBrowserCommandTabID(sessionID, commandID)
}

export function browserWorkspaceHasControl(
  state: BrowserWorkspaceState,
  tabID: string,
  capability: BrowserControlCapability,
): boolean {
  return Object.values(state.controlLeases).some(
    (lease) => lease.tabID === tabID && lease.capabilities.includes(capability),
  )
}

export function browserWorkspaceControlCapabilities(
  state: BrowserWorkspaceState,
  tabID: string,
): BrowserControlCapability[] {
  const capabilities = new Set<BrowserControlCapability>()
  for (const lease of Object.values(state.controlLeases)) {
    if (lease.tabID !== tabID) continue
    for (const capability of lease.capabilities) capabilities.add(capability)
  }
  return browserCapabilityOrder.filter((capability) => capabilities.has(capability))
}

const browserCapabilityOrder: BrowserControlCapability[] = [
  'read',
  'navigate',
  'interact',
]

function browserNavigationLeaseID(tabID: string): string {
  return `navigation:${tabID}`
}

function uniqueCapabilities(
  capabilities: BrowserControlCapability[],
): BrowserControlCapability[] {
  const values = new Set(capabilities)
  return browserCapabilityOrder.filter((capability) => values.has(capability))
}

function sameCapabilities(
  left: BrowserControlCapability[],
  right: BrowserControlCapability[],
): boolean {
  return left.length === right.length &&
    left.every((value, index) => value === right[index])
}

function isManualNavigation(action: BrowserTabsAction): boolean {
  return action.t === 'reload' ||
    (action.t === 'submit_navigation' && action.source === 'address')
}

function withoutTabControl(
  leases: Record<string, BrowserControlLease>,
  tabID: string,
): Record<string, BrowserControlLease> {
  return Object.fromEntries(
    Object.entries(leases).filter(([, lease]) => lease.tabID !== tabID),
  )
}

function releaseTabControl(
  state: BrowserWorkspaceState,
  tabID: string,
): BrowserWorkspaceState {
  const controlLeases = withoutTabControl(state.controlLeases, tabID)
  if (Object.keys(controlLeases).length === Object.keys(state.controlLeases).length) {
    return state
  }
  return normalizeAgentSelection({ ...state, controlLeases })
}

function normalizeAgentSelection(
  state: BrowserWorkspaceState,
): BrowserWorkspaceState {
  const selected = state.agentSelectedTabID
  if (!selected || browserWorkspaceHasControl(state, selected, 'navigate')) return state
  return { ...state, agentSelectedTabID: undefined }
}
