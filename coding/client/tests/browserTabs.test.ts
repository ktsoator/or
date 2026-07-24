import { describe, expect, test } from 'bun:test'
import {
  browserTabsReducer,
  browserTabNavigationURL,
  createBrowserTab,
  type BrowserNavigationTarget,
  type BrowserTab,
} from '../src/browserTabs'

const webTarget = (requestedURL: string): BrowserNavigationTarget => ({
  requestedURL,
  addressDraft: requestedURL,
  kind: 'web',
})

function agentTab(): BrowserTab {
  return createBrowserTab({
    id: 'preview:session-1',
    owner: 'agent',
    sessionID: 'session-1',
  })
}

function navigate(tabs: BrowserTab[], url: string): BrowserTab[] {
  return browserTabsReducer(tabs, {
    t: 'submit_navigation',
    tabID: 'preview:session-1',
    source: 'agent',
    target: webTarget(url),
  })
}

function nativeState(
  tabs: BrowserTab[],
  revision: number,
  url: string,
  title: string,
): BrowserTab[] {
  return browserTabsReducer(tabs, {
    t: 'native_state_received',
    tabID: 'preview:session-1',
    appliedRevision: revision,
    committedURL: url,
    title,
    status: 'ready',
    canGoBack: revision > 1,
    canGoForward: false,
  })
}

describe('browser tabs reducer', () => {
  test('commits the first agent navigation', () => {
    let tabs = navigate([agentTab()], 'https://github.com/')
    expect(tabs[0]?.desired?.revision).toBe(1)

    tabs = nativeState(tabs, 1, 'https://github.com/', 'GitHub')
    expect(tabs[0]?.observed).toMatchObject({
      appliedRevision: 1,
      committedURL: 'https://github.com/',
      title: 'GitHub',
      status: 'ready',
    })
    expect(tabs[0]?.addressDraft).toBe('https://github.com/')
  })

  test('ignores a late GitHub state after Bilibili revision 2 is desired', () => {
    let tabs = navigate([agentTab()], 'https://github.com/')
    tabs = nativeState(tabs, 1, 'https://github.com/', 'GitHub')
    tabs = navigate(tabs, 'https://www.bilibili.com/')

    expect(tabs[0]?.desired).toMatchObject({
      revision: 2,
      requestedURL: 'https://www.bilibili.com/',
    })
    expect(browserTabNavigationURL(tabs[0]!)).toBe('https://www.bilibili.com/')
    const stale = nativeState(tabs, 1, 'https://github.com/', 'GitHub')
    expect(stale).toBe(tabs)
    expect(stale[0]?.addressDraft).toBe('https://www.bilibili.com/')

    tabs = nativeState(stale, 2, 'https://www.bilibili.com/', 'Bilibili')
    expect(tabs[0]?.observed.committedURL).toBe('https://www.bilibili.com/')
  })

  test('makes only the latest of three rapid commands authoritative', () => {
    let tabs = navigate([agentTab()], 'https://github.com/')
    tabs = navigate(tabs, 'https://www.bilibili.com/')
    tabs = navigate(tabs, 'https://www.google.com/')

    expect(tabs[0]?.desired).toMatchObject({
      revision: 3,
      requestedURL: 'https://www.google.com/',
    })
    expect(nativeState(tabs, 1, 'https://github.com/', 'GitHub')).toBe(tabs)
    expect(nativeState(tabs, 2, 'https://www.bilibili.com/', 'Bilibili')).toBe(tabs)
  })

  test('stores requested and redirected committed URLs separately', () => {
    let tabs = navigate([agentTab()], 'https://example.com/start')
    tabs = nativeState(tabs, 1, 'https://example.com/final', 'Final')

    expect(tabs[0]?.desired?.requestedURL).toBe('https://example.com/start')
    expect(tabs[0]?.observed.committedURL).toBe('https://example.com/final')
    expect(tabs[0]?.addressDraft).toBe('https://example.com/final')
    expect(browserTabNavigationURL(tabs[0]!)).toBe('https://example.com/final')
  })

  test('drops native events after a tab closes', () => {
    let tabs = navigate([agentTab()], 'https://github.com/')
    tabs = browserTabsReducer(tabs, {
      t: 'close_tab',
      tabID: 'preview:session-1',
    })
    expect(tabs).toEqual([])
    expect(nativeState(tabs, 1, 'https://github.com/', 'GitHub')).toBe(tabs)
  })

  test('never lets an agent command replace a user-owned tab', () => {
    const userTab = createBrowserTab({ id: 'preview:session-1', owner: 'user' })
    const tabs = browserTabsReducer([userTab], {
      t: 'agent_navigate',
      tabID: 'preview:session-1',
      sessionID: 'session-1',
      target: webTarget('https://github.com/'),
    })

    expect(tabs).toEqual([userTab])
  })
})
