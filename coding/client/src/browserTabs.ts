type BrowserTargetKind = 'web' | 'workspace-preview'

type BrowserNavigationSource = 'agent' | 'address' | 'reload'

export type BrowserNavigationTarget = {
  requestedURL: string
  addressDraft: string
  kind: BrowserTargetKind
  title?: string
  workspacePath?: string
  commandID?: string
}

type DesiredNavigation = BrowserNavigationTarget & {
  revision: number
  source: BrowserNavigationSource
}

export type ObservedNavigation = {
  appliedRevision: number
  committedURL: string
  title: string
  status: 'idle' | 'navigating' | 'ready' | 'failed'
  canGoBack: boolean
  canGoForward: boolean
  error?: string
}

export type BrowserTab = {
  // Stable session-local identity exposed to the UI and Agent tools. Runtime
  // registry keys add the workspace namespace separately.
  id: string
  sessionID?: string
  addressDraft: string
  desired?: DesiredNavigation
  observed: ObservedNavigation
  invalidAddress: boolean
}

export type BrowserTabsAction =
  | { t: 'create_tab'; tabID: string; sessionID?: string }
  | {
      t: 'agent_navigate'
      tabID: string
      sessionID: string
      target: BrowserNavigationTarget
    }
  | {
      t: 'submit_navigation'
      tabID: string
      source: 'agent' | 'address'
      target: BrowserNavigationTarget
    }
  | { t: 'reload'; tabID: string }
  | { t: 'edit_address'; tabID: string; address: string }
  | { t: 'reject_address'; tabID: string }
  | {
      t: 'resolve_navigation'
      tabID: string
      revision: number
      url: string
    }
  | {
      t: 'runtime_state_received'
      tabID: string
      appliedRevision: number
      committedURL: string
      title: string
      status: 'navigating' | 'ready' | 'failed'
      canGoBack: boolean
      canGoForward: boolean
      error?: string
    }
  | { t: 'close_tab'; tabID: string }

export function agentBrowserTabID(sessionID?: string): string {
  return `preview:${sessionID ?? 'unknown'}`
}

export function agentBrowserCommandTabID(
  sessionID: string | undefined,
  commandID: string,
): string {
  return `${agentBrowserTabID(sessionID)}:command:${commandID}`
}

export function browserTabNavigationURL(tab: BrowserTab): string {
  if (!tab.desired) return ''
  if (
    tab.observed.appliedRevision >= tab.desired.revision &&
    tab.observed.status === 'ready' &&
    tab.observed.committedURL
  ) {
    return tab.observed.committedURL
  }
  return tab.desired.requestedURL
}

const emptyObservedNavigation = (): ObservedNavigation => ({
  appliedRevision: -1,
  committedURL: '',
  title: '',
  status: 'idle',
  canGoBack: false,
  canGoForward: false,
})

export function createBrowserTab({
  id,
  sessionID,
  target,
  source = 'address',
  initialRevision = 0,
}: {
  id: string
  sessionID?: string
  target?: BrowserNavigationTarget
  source?: BrowserNavigationSource
  initialRevision?: number
}): BrowserTab {
  return {
    id,
    sessionID,
    addressDraft: target?.addressDraft ?? '',
    desired: target
      ? {
          ...target,
          revision: initialRevision,
          source,
        }
      : undefined,
    observed: target
      ? { ...emptyObservedNavigation(), status: 'navigating' }
      : emptyObservedNavigation(),
    invalidAddress: false,
  }
}

function replaceTab(
  tabs: BrowserTab[],
  tabID: string,
  update: (tab: BrowserTab) => BrowserTab,
): BrowserTab[] {
  const index = tabs.findIndex((tab) => tab.id === tabID)
  if (index < 0) return tabs
  const current = tabs[index]
  if (!current) return tabs
  const next = update(current)
  if (next === current) return tabs
  const copy = tabs.slice()
  copy[index] = next
  return copy
}

function nextRevision(tab: BrowserTab): number {
  return Math.max(tab.desired?.revision ?? 0, tab.observed.appliedRevision) + 1
}

function submitNavigation(
  tab: BrowserTab,
  target: BrowserNavigationTarget,
  source: BrowserNavigationSource,
): BrowserTab {
  return {
    ...tab,
    addressDraft: target.addressDraft,
    desired: {
      ...target,
      revision: nextRevision(tab),
      source,
    },
    observed: {
      ...tab.observed,
      title: source === 'reload' ? tab.observed.title : '',
      status: 'navigating',
      error: undefined,
    },
    invalidAddress: false,
  }
}

export function browserTabsReducer(
  tabs: BrowserTab[],
  action: BrowserTabsAction,
): BrowserTab[] {
  switch (action.t) {
    case 'create_tab':
      if (tabs.some((tab) => tab.id === action.tabID)) return tabs
      return [
        ...tabs,
        createBrowserTab({ id: action.tabID, sessionID: action.sessionID }),
      ]
    case 'agent_navigate': {
      const existing = tabs.find((tab) => tab.id === action.tabID)
      if (!existing) {
        return [
          ...tabs,
          createBrowserTab({
            id: action.tabID,
            sessionID: action.sessionID,
            target: action.target,
            source: 'agent',
          }),
        ]
      }
      return replaceTab(tabs, action.tabID, (tab) =>
        ({
          ...submitNavigation(tab, action.target, 'agent'),
          sessionID: action.sessionID,
        }),
      )
    }
    case 'submit_navigation':
      return replaceTab(tabs, action.tabID, (tab) =>
        submitNavigation(tab, action.target, action.source),
      )
    case 'reload':
      return replaceTab(tabs, action.tabID, (tab) => {
        const desired = tab.desired
        const requestedURL = tab.observed.committedURL || desired?.requestedURL
        if (!desired || !requestedURL) return tab
        return submitNavigation(
          tab,
          {
            requestedURL,
            addressDraft: tab.addressDraft,
            kind: desired.kind,
            title: desired.title,
            workspacePath: desired.workspacePath,
            commandID: desired.commandID,
          },
          'reload',
        )
      })
    case 'edit_address':
      return replaceTab(tabs, action.tabID, (tab) => ({
        ...tab,
        addressDraft: action.address,
        invalidAddress: false,
      }))
    case 'reject_address':
      return replaceTab(tabs, action.tabID, (tab) => ({
        ...tab,
        invalidAddress: true,
      }))
    case 'resolve_navigation':
      return replaceTab(tabs, action.tabID, (tab) => {
        if (!tab.desired || tab.desired.revision !== action.revision) return tab
        return {
          ...tab,
          addressDraft: action.url,
          desired: { ...tab.desired, requestedURL: action.url },
        }
      })
    case 'runtime_state_received':
      return replaceTab(tabs, action.tabID, (tab) => {
        const desiredRevision = tab.desired?.revision ?? -1
        if (
          action.appliedRevision < desiredRevision ||
          action.appliedRevision < tab.observed.appliedRevision
        ) {
          return tab
        }
        const committedURL = action.committedURL || tab.observed.committedURL
        return {
          ...tab,
          addressDraft:
            action.status !== 'ready' ||
              tab.desired?.kind === 'workspace-preview' ||
              !action.committedURL
              ? tab.addressDraft
              : action.committedURL,
          observed: {
            appliedRevision: action.appliedRevision,
            committedURL,
            title: action.title,
            status: action.status,
            canGoBack: action.canGoBack,
            canGoForward: action.canGoForward,
            error: action.error,
          },
        }
      })
    case 'close_tab':
      return tabs.filter((tab) => tab.id !== action.tabID)
  }
}
