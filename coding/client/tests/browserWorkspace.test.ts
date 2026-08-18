import { describe, expect, test } from 'bun:test'
import { createBrowserTab, type BrowserNavigationTarget } from '../src/features/browser/tabs'
import { browserRuntimeTabID } from '../src/features/browser/runtimeID'
import {
  browserWorkspaceCommandTabID,
  browserWorkspaceContext,
  browserWorkspaceHasControl,
  browserWorkspaceInspectionTabID,
  browserWorkspaceRegistryReducer,
  browserWorkspaceReducer,
  createBrowserWorkspaceRegistryState,
  createBrowserWorkspaceState,
  selectedBrowserTab,
} from '../src/features/browser/workspace'

const webTarget = (
  requestedURL: string,
  commandID?: string,
): BrowserNavigationTarget => ({
  requestedURL,
  addressDraft: requestedURL,
  kind: 'web',
  commandID,
})

describe('browser workspace reducer', () => {
  test('namespaces runtime tab IDs without ambiguous concatenation', () => {
    expect(browserRuntimeTabID('session/1', 'tab:1')).toBe(
      'workspace:session%2F1:tab:tab%3A1',
    )
    expect(browserRuntimeTabID('session/1', 'tab:1')).not.toBe(
      browserRuntimeTabID('session', '1:tab:1'),
    )
  })

  test('creates and selects browser tabs without an external sequence ref', () => {
    let state = createBrowserWorkspaceState({
      initialTab: createBrowserTab({ id: 'tab-1' }),
    })
    state = browserWorkspaceReducer(state, { t: 'create_user_tab' })

    expect(state.tabs.map((tab) => tab.id)).toEqual(['tab-1', 'tab-2'])
    expect(state.activeItemID).toBe('tab-2')
    expect(state.nextUserTabSequence).toBe(2)
  })

  test('opens and selects a user web tab for a guest popup', () => {
    let state = createBrowserWorkspaceState({
      initialTab: createBrowserTab({ id: 'tab-1' }),
    })
    state = browserWorkspaceReducer(state, {
      t: 'open_user_tab',
      target: webTarget('https://example.com/popup'),
    })

    expect(state.tabs.map((tab) => tab.id)).toEqual(['tab-1', 'tab-2'])
    expect(state.activeItemID).toBe('tab-2')
    expect(state.tabs[1]?.desired).toMatchObject({
      requestedURL: 'https://example.com/popup',
      kind: 'web',
      source: 'address',
    })
  })

  test('closes and selects the neighboring tab atomically', () => {
    let state = createBrowserWorkspaceState({
      initialTab: createBrowserTab({ id: 'tab-1' }),
    })
    state = browserWorkspaceReducer(state, { t: 'create_user_tab' })
    state = browserWorkspaceReducer(state, { t: 'create_user_tab' })
    state = browserWorkspaceReducer(state, { t: 'select_item', itemID: 'tab-2' })
    state = browserWorkspaceReducer(state, { t: 'close_tab', tabID: 'tab-2' })

    expect(state.tabs.map((tab) => tab.id)).toEqual(['tab-1', 'tab-3'])
    expect(state.activeItemID).toBe('tab-3')
  })

  test('selects a new conversation and restores the first browser tab when it closes', () => {
    let state = createBrowserWorkspaceState({
      initialTab: createBrowserTab({ id: 'tab-1' }),
    })
    state = browserWorkspaceReducer(state, {
      t: 'sync_conversation',
      conversationTabID: 'conversation:session-1',
    })
    expect(state.activeItemID).toBe('conversation:session-1')
    expect(selectedBrowserTab(state)).toBeUndefined()

    state = browserWorkspaceReducer(state, {
      t: 'sync_conversation',
      conversationTabID: undefined,
    })
    expect(state.activeItemID).toBe('tab-1')
  })

  test('opens one task view, switches tasks, and closes without stopping task state', () => {
    let state = createBrowserWorkspaceState({
      initialTab: createBrowserTab({ id: 'tab-1' }),
      conversationTabID: 'conversation:session-1',
      activeItemID: 'conversation:session-1',
    })
    state = browserWorkspaceReducer(state, {
      t: 'open_tasks',
      taskTabID: 'tasks:session-1',
      taskID: 'task-1',
    })
    expect(state).toMatchObject({
      taskTabID: 'tasks:session-1',
      selectedTaskID: 'task-1',
      activeItemID: 'tasks:session-1',
    })
    expect(selectedBrowserTab(state)).toBeUndefined()

    state = browserWorkspaceReducer(state, { t: 'select_task', taskID: 'task-2' })
    expect(state.selectedTaskID).toBe('task-2')
    expect(state.taskTabID).toBe('tasks:session-1')

    state = browserWorkspaceReducer(state, { t: 'close_tasks' })
    expect(state.taskTabID).toBeUndefined()
    expect(state.selectedTaskID).toBeUndefined()
    expect(state.activeItemID).toBe('conversation:session-1')
  })

  test('keeps the task view available when the last browser tab closes', () => {
    let state = createBrowserWorkspaceState({
      initialTab: createBrowserTab({ id: 'tab-1' }),
    })
    state = browserWorkspaceReducer(state, {
      t: 'open_tasks',
      taskTabID: 'tasks:session-1',
    })
    state = browserWorkspaceReducer(state, { t: 'select_item', itemID: 'tab-1' })
    state = browserWorkspaceReducer(state, { t: 'close_tab', tabID: 'tab-1' })

    expect(state.tabs).toEqual([])
    expect(state.activeItemID).toBe('tasks:session-1')
  })

  test('keeps UI selection separate from the Agent navigation target', () => {
    let state = createBrowserWorkspaceState()
    const firstTabID = browserWorkspaceCommandTabID(
      state,
      'session-1',
      'command-1',
      'reuse_agent_tab',
    )
    state = browserWorkspaceReducer(state, {
      t: 'agent_command_received',
      commandKey: 'session-1:command-1',
      tabID: firstTabID,
      sessionID: 'session-1',
      target: webTarget('https://github.com/', 'command-1'),
      activate: true,
      selectForAgent: true,
    })
    state = browserWorkspaceReducer(state, { t: 'create_user_tab' })

    expect(state.activeItemID).toBe('tab-1')
    expect(state.agentSelectedTabID).toBe('preview:session-1')
    expect(
      browserWorkspaceCommandTabID(
        state,
        'session-1',
        'command-2',
        'reuse_agent_tab',
      ),
    ).toBe('preview:session-1')
  })

  test('does not overwrite a tab after manual navigation releases Agent control', () => {
    let state = createBrowserWorkspaceState()
    state = browserWorkspaceReducer(state, {
      t: 'agent_command_received',
      commandKey: 'session-1:command-1',
      tabID: 'preview:session-1',
      sessionID: 'session-1',
      target: webTarget('https://github.com/', 'command-1'),
      activate: true,
      selectForAgent: true,
    })
    state = browserWorkspaceReducer(state, {
      t: 'tab_action',
      action: {
        t: 'submit_navigation',
        tabID: 'preview:session-1',
        source: 'address',
        target: webTarget('https://www.google.com/'),
      },
    })

    expect(state.agentSelectedTabID).toBeUndefined()
    expect(state.controlLeases).toEqual({})
    expect(
      browserWorkspaceCommandTabID(
        state,
        'session-1',
        'command-2',
        'reuse_agent_tab',
      ),
    ).toBe('preview:session-1:command:command-2')
  })

  test('keeps Agent selection on the foreground tab when opening a background tab', () => {
    let state = createBrowserWorkspaceState()
    state = browserWorkspaceReducer(state, {
      t: 'agent_command_received',
      commandKey: 'session-1:foreground',
      tabID: 'preview:session-1',
      sessionID: 'session-1',
      target: webTarget('https://github.com/', 'foreground'),
      activate: true,
      selectForAgent: true,
    })
    state = browserWorkspaceReducer(state, {
      t: 'agent_command_received',
      commandKey: 'session-1:background',
      tabID: 'preview:session-1:command:background',
      sessionID: 'session-1',
      target: webTarget('https://www.google.com/', 'background'),
      activate: false,
      selectForAgent: false,
    })

    expect(state.agentSelectedTabID).toBe('preview:session-1')
    expect(
      browserWorkspaceCommandTabID(
        state,
        'session-1',
        'next-command',
        'reuse_agent_tab',
      ),
    ).toBe('preview:session-1')
  })

  test('binds a command target once even if the UI selection later changes', () => {
    let state = createBrowserWorkspaceState()
    state = browserWorkspaceReducer(state, {
      t: 'agent_command_received',
      commandKey: 'session-1:command-2',
      tabID: 'preview:session-1:command:command-2',
      sessionID: 'session-1',
      target: webTarget('https://github.com/', 'command-2'),
      activate: true,
      selectForAgent: true,
    })
    state = browserWorkspaceReducer(state, { t: 'create_user_tab' })

    expect(state.commandTargets['session-1:command-2']).toBe(
      'preview:session-1:command:command-2',
    )
    expect(state.activeItemID).toBe('tab-1')
  })

  test('keeps tabs, selections, leases, and command targets isolated by session', () => {
    const first = createBrowserWorkspaceState()
    let registry = createBrowserWorkspaceRegistryState('session-1', first)
    registry = browserWorkspaceRegistryReducer(registry, {
      t: 'workspace_action',
      workspaceID: 'session-1',
      initialState: first,
      action: { t: 'create_user_tab' },
    })

    const second = createBrowserWorkspaceState()
    registry = browserWorkspaceRegistryReducer(registry, {
      t: 'workspace_action',
      workspaceID: 'session-2',
      initialState: second,
      action: {
        t: 'agent_command_received',
        commandKey: 'session-2:command-1',
        tabID: 'preview:session-2',
        sessionID: 'session-2',
        target: webTarget('https://github.com/', 'command-1'),
        activate: true,
        selectForAgent: true,
      },
    })

    expect(registry.workspaces['session-1']).toMatchObject({
      activeItemID: 'tab-1',
      agentSelectedTabID: undefined,
      controlLeases: {},
      commandTargets: {},
    })
    expect(registry.workspaces['session-2']).toMatchObject({
      activeItemID: 'preview:session-2',
      agentSelectedTabID: 'preview:session-2',
      commandTargets: {
        'session-2:command-1': 'preview:session-2',
      },
    })
    expect(
      browserWorkspaceHasControl(
        registry.workspaces['session-2']!,
        'preview:session-2',
        'navigate',
      ),
    ).toBe(true)
  })

  test('projects shared open tabs and temporary control independently', () => {
    const controlledTab = createBrowserTab({
      id: 'stable-tab',
      sessionID: 'session-1',
      target: webTarget('https://example.com/', 'command-1'),
      source: 'agent',
    })
    let state = createBrowserWorkspaceState({ initialTab: controlledTab })
    state = browserWorkspaceReducer(state, {
      t: 'tab_action',
      action: {
        t: 'runtime_state_received',
        tabID: controlledTab.id,
        appliedRevision: 0,
        committedURL: 'https://example.com/',
        title: 'Final title',
        status: 'ready',
        canGoBack: false,
        canGoForward: false,
      },
    })
    state = browserWorkspaceReducer(state, { t: 'create_user_tab' })

    expect(browserWorkspaceContext(state)).toEqual({
      openTabs: [
        {
          tabID: 'stable-tab',
          url: 'https://example.com/',
          title: 'Final title',
          status: 'ready',
        },
        {
          tabID: 'tab-1',
          url: undefined,
          title: undefined,
          status: 'idle',
        },
      ],
      controlledTabs: [
        { tabID: 'stable-tab', capabilities: ['read', 'navigate'] },
      ],
      selected: 'stable-tab',
    })
    expect(browserWorkspaceInspectionTabID(state, 'session-1', 'tab-1')).toBe(
      'tab-1',
    )

    state = browserWorkspaceReducer(state, {
      t: 'attach_control',
      leaseID: 'inspection:1',
      tabID: 'tab-1',
      capabilities: ['read'],
    })
    expect(browserWorkspaceContext(state).controlledTabs).toEqual([
      { tabID: 'stable-tab', capabilities: ['read', 'navigate'] },
      { tabID: 'tab-1', capabilities: ['read'] },
    ])
    state = browserWorkspaceReducer(state, {
      t: 'release_control',
      leaseID: 'inspection:1',
    })
    expect(browserWorkspaceContext(state).controlledTabs).toEqual([
      { tabID: 'stable-tab', capabilities: ['read', 'navigate'] },
    ])
    expect(() => browserWorkspaceInspectionTabID(state, 'session-1', 'missing')).toThrow(
      'not open in this session',
    )
  })

  test('closing a tab removes control but retains the command tombstone', () => {
    let state = createBrowserWorkspaceState()
    state = browserWorkspaceReducer(state, {
      t: 'agent_command_received',
      commandKey: 'session-1:command-1',
      tabID: 'preview:session-1',
      sessionID: 'session-1',
      target: webTarget('https://example.com/', 'command-1'),
      activate: true,
      selectForAgent: true,
    })
    state = browserWorkspaceReducer(state, {
      t: 'close_tab',
      tabID: 'preview:session-1',
    })

    expect(state.tabs).toEqual([])
    expect(state.controlLeases).toEqual({})
    expect(state.agentSelectedTabID).toBeUndefined()
    expect(state.commandTargets).toEqual({
      'session-1:command-1': 'preview:session-1',
    })
  })
})
