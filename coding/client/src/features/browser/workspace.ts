import type {
  BrowserControlCapability,
  BrowserControlledTab,
  BrowserDisposition,
  BrowserOpenTab,
} from '@/generated/wire'
import {
  agentBrowserCommandTabID,
  agentBrowserTabID,
  browserTabsReducer,
  createBrowserTab,
  type BrowserNavigationTarget,
  type BrowserTab,
  type BrowserTabsAction,
} from './tabs'

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
  selectedBrowserTabID?: string
  conversationTabIDs: string[]
  activeConversationTabID?: string
  taskTabID?: string
  selectedTaskID?: string
  agentSelectedTabIDs: Record<string, string>
  nextUserTabSequence: number
  controlLeases: Record<string, BrowserControlLease>
  commandTargets: Record<string, string>
  handledPreviewKeys: Record<string, string>
}

export type BrowserWorkspaceAction =
  | { t: 'select_item'; itemID: string }
  | {
      t: 'sync_conversations'
      conversationTabIDs: string[]
      activeConversationTabID?: string
    }
  | { t: 'open_tasks'; taskTabID: string; taskID?: string }
  | { t: 'select_task'; taskID: string }
  | { t: 'close_tasks' }
  | { t: 'create_user_tab'; sessionID?: string }
  | { t: 'open_user_tab'; sessionID?: string; target: BrowserNavigationTarget }
  | { t: 'close_tab'; tabID: string }
  | { t: 'tab_action'; action: BrowserTabsAction }
  | {
      t: 'attach_control'
      leaseID: string
      sessionID: string
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
      t: 'restored_preview_received'
      previewKey: string
      tabID: string
      sessionID: string
      target: BrowserNavigationTarget
      activate: boolean
    }

export function createBrowserWorkspaceState({
  initialTab,
  activeItemID,
  conversationTabIDs = [],
  activeConversationTabID,
  handledPreviewKey,
}: {
  initialTab?: BrowserTab
  activeItemID?: string
  conversationTabIDs?: string[]
  activeConversationTabID?: string
  handledPreviewKey?: string
} = {}): BrowserWorkspaceState {
  const initiallyControlled = initialTab?.desired?.source === 'agent'
  const initialSessionID = initiallyControlled ? initialTab.sessionID : undefined
  return {
    tabs: initialTab ? [initialTab] : [],
    activeItemID: activeItemID ?? initialTab?.id ?? '',
    selectedBrowserTabID: initialTab?.id,
    conversationTabIDs,
    activeConversationTabID,
    agentSelectedTabIDs:
      initialSessionID && initialTab
        ? { [initialSessionID]: initialTab.id }
        : {},
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
    handledPreviewKeys:
      initialSessionID && handledPreviewKey
        ? { [initialSessionID]: handledPreviewKey }
        : {},
  }
}

export function browserWorkspaceReducer(
  state: BrowserWorkspaceState,
  action: BrowserWorkspaceAction,
): BrowserWorkspaceState {
  switch (action.t) {
    case 'select_item': {
      const conversationSelected = state.conversationTabIDs.includes(action.itemID)
      return action.itemID === state.activeItemID
        ? state
        : {
            ...state,
            activeItemID: action.itemID,
            activeConversationTabID: conversationSelected
              ? action.itemID
              : state.activeConversationTabID,
            selectedBrowserTabID: state.tabs.some(
              (tab) => tab.id === action.itemID,
            )
              ? action.itemID
              : state.selectedBrowserTabID,
          }
    }

    case 'sync_conversations': {
      const sameTabs =
        state.conversationTabIDs.length === action.conversationTabIDs.length &&
        state.conversationTabIDs.every(
          (tabID, index) => tabID === action.conversationTabIDs[index],
        )
      const requestedActive = action.activeConversationTabID &&
        action.conversationTabIDs.includes(action.activeConversationTabID)
          ? action.activeConversationTabID
          : undefined
      const requestedActiveChanged =
        requestedActive !== undefined &&
        requestedActive !== state.activeConversationTabID
      if (
        sameTabs &&
        (!requestedActive || requestedActive === state.activeConversationTabID)
      ) return state

      let activeConversationTabID = requestedActive ?? state.activeConversationTabID
      if (
        activeConversationTabID &&
        !action.conversationTabIDs.includes(activeConversationTabID)
      ) {
        const previousIndex = state.conversationTabIDs.indexOf(activeConversationTabID)
        activeConversationTabID = action.conversationTabIDs[
          Math.min(Math.max(previousIndex, 0), action.conversationTabIDs.length - 1)
        ]
      }
      activeConversationTabID ??= action.conversationTabIDs.at(-1)

      const activeConversationRemoved =
        state.conversationTabIDs.includes(state.activeItemID) &&
        !action.conversationTabIDs.includes(state.activeItemID)
      return {
        ...state,
        conversationTabIDs: action.conversationTabIDs,
        activeConversationTabID,
        activeItemID: requestedActiveChanged
          ? requestedActive
          : activeConversationRemoved
            ? activeConversationTabID ?? state.taskTabID ?? state.tabs[0]?.id ?? ''
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
            ? state.activeConversationTabID ?? state.tabs.at(-1)?.id ?? ''
            : state.activeItemID,
      }
    }

    case 'create_user_tab': {
      const nextUserTabSequence = state.nextUserTabSequence + 1
      const tabID = `tab-${nextUserTabSequence}`
      return {
        ...state,
        tabs: browserTabsReducer(state.tabs, {
          t: 'create_tab',
          tabID,
          sessionID: action.sessionID,
        }),
        activeItemID: tabID,
        selectedBrowserTabID: tabID,
        nextUserTabSequence,
      }
    }

    case 'open_user_tab': {
      const nextUserTabSequence = state.nextUserTabSequence + 1
      const tabID = `tab-${nextUserTabSequence}`
      return {
        ...state,
        tabs: [
          ...state.tabs,
          createBrowserTab({
            id: tabID,
            sessionID: action.sessionID,
            target: action.target,
            source: 'address',
          }),
        ],
        activeItemID: tabID,
        selectedBrowserTabID: tabID,
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
        agentSelectedTabIDs: withoutTabSelection(
          state.agentSelectedTabIDs,
          action.tabID,
        ),
        activeItemID:
          state.activeItemID === action.tabID
            ? next?.id ?? state.activeConversationTabID ?? state.taskTabID ?? ''
            : state.activeItemID,
        selectedBrowserTabID:
          state.selectedBrowserTabID === action.tabID
            ? next?.id
            : state.selectedBrowserTabID,
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
      if (!state.tabs.some(
        (tab) => tab.id === action.tabID && tab.sessionID === action.sessionID,
      )) return state
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
        (!action.select || state.agentSelectedTabIDs[action.sessionID] === action.tabID)
      ) {
        return state
      }
      return {
        ...state,
        controlLeases: { ...state.controlLeases, [action.leaseID]: lease },
        agentSelectedTabIDs:
          action.select && capabilities.includes('navigate')
            ? {
                ...state.agentSelectedTabIDs,
                [action.sessionID]: action.tabID,
              }
            : state.agentSelectedTabIDs,
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
        selectedBrowserTabID: action.activate
          ? action.tabID
          : state.selectedBrowserTabID,
        agentSelectedTabIDs: action.selectForAgent
          ? {
              ...state.agentSelectedTabIDs,
              [action.sessionID]: action.tabID,
            }
          : state.agentSelectedTabIDs,
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

    case 'restored_preview_received':
      if (state.handledPreviewKeys[action.sessionID] === action.previewKey) return state
      return {
        ...state,
        tabs: browserTabsReducer(state.tabs, {
          t: 'agent_navigate',
          tabID: action.tabID,
          sessionID: action.sessionID,
          target: action.target,
        }),
        activeItemID: action.activate ? action.tabID : state.activeItemID,
        selectedBrowserTabID: action.activate
          ? action.tabID
          : state.selectedBrowserTabID,
        agentSelectedTabIDs: {
          ...state.agentSelectedTabIDs,
          [action.sessionID]: action.tabID,
        },
        controlLeases: {
          ...state.controlLeases,
          [browserNavigationLeaseID(action.tabID)]: {
            tabID: action.tabID,
            capabilities: ['read', 'navigate'],
          },
        },
        handledPreviewKeys: {
          ...state.handledPreviewKeys,
          [action.sessionID]: action.previewKey,
        },
      }
  }
}

export function selectedBrowserTab(
  state: BrowserWorkspaceState,
  sessionID?: string,
): BrowserTab | undefined {
  const tabs = visibleBrowserTabs(state, sessionID)
  return tabs.find((tab) => tab.id === state.selectedBrowserTabID) ??
    tabs.find((tab) => tab.id === state.activeItemID) ??
    tabs[0]
}

export function visibleBrowserTabs(
  state: BrowserWorkspaceState,
  sessionID?: string,
): BrowserTab[] {
  if (sessionID === undefined) return state.tabs
  return state.tabs.filter(
    (tab) => tab.scope === 'workbench' || tab.sessionID === sessionID,
  )
}

export function browserWorkspaceContext(
  state: BrowserWorkspaceState | undefined,
  sessionID?: string,
): BrowserWorkspaceContext {
  if (!state) return { openTabs: [], controlledTabs: [] }
  const tabs = sessionID
    ? state.tabs.filter((tab) => tab.sessionID === sessionID)
    : state.tabs
  const controlledTabs = tabs.flatMap((tab) => {
    const capabilities = browserWorkspaceControlCapabilities(state, tab.id)
    return capabilities.length > 0 ? [{ tabID: tab.id, capabilities }] : []
  })
  const controlledIDs = new Set(controlledTabs.map((tab) => tab.tabID))
  const selectedTabID = sessionID
    ? state.agentSelectedTabIDs[sessionID]
    : Object.values(state.agentSelectedTabIDs).find((tabID) =>
        controlledIDs.has(tabID),
      )
  return {
    openTabs: tabs.map((tab) => ({
      tabID: tab.id,
      url: browserContextURL(tab),
      title: tab.observed.title || tab.desired?.title || undefined,
      status: tab.observed.status,
    })),
    controlledTabs,
    selected:
      selectedTabID && controlledIDs.has(selectedTabID)
        ? selectedTabID
        : undefined,
  }
}

export function browserWorkspaceInspectionTabID(
  state: BrowserWorkspaceState | undefined,
  sessionID: string,
  requestedTabID?: string,
): string {
  const tab = requestedTabID
    ? state?.tabs.find(
        (candidate) =>
          candidate.id === requestedTabID && candidate.sessionID === sessionID,
      )
    : state?.agentSelectedTabIDs[sessionID]
      ? state.tabs.find(
          (candidate) =>
            candidate.id === state.agentSelectedTabIDs[sessionID] &&
            candidate.sessionID === sessionID,
        )
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
  const selected = sessionID ? state.agentSelectedTabIDs[sessionID] : undefined
  if (
    selected &&
    state.tabs.some(
      (tab) => tab.id === selected && tab.sessionID === sessionID,
    ) &&
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
  const agentSelectedTabIDs = Object.fromEntries(
    Object.entries(state.agentSelectedTabIDs).filter(([, tabID]) =>
      browserWorkspaceHasControl(state, tabID, 'navigate'),
    ),
  )
  return Object.keys(agentSelectedTabIDs).length ===
    Object.keys(state.agentSelectedTabIDs).length
    ? state
    : { ...state, agentSelectedTabIDs }
}

function withoutTabSelection(
  selections: Record<string, string>,
  tabID: string,
): Record<string, string> {
  return Object.fromEntries(
    Object.entries(selections).filter(([, selectedTabID]) => selectedTabID !== tabID),
  )
}
