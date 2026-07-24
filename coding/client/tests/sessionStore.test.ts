import { describe, expect, test } from 'bun:test'
import {
  createSessionDraft,
  createSessionStoreState,
  sessionStoreReducer,
  type SessionStoreAction,
  type SessionStoreState,
} from '../src/sessionStore'
import type { SessionSummary } from '../src/types'

function session(id: string, updatedAt = '2026-07-23T12:00:00.000Z'): SessionSummary {
  return {
    id,
    title: 'New session',
    workspacePath: `/tmp/${id}`,
    workspaceName: id,
    scope: 'chat',
    workspaceKind: 'scratch',
    createdAt: '2026-07-23T11:00:00.000Z',
    updatedAt,
    running: false,
    hasApproval: false,
    modelProvider: 'openai',
    modelId: 'test-model',
    modelName: 'Test model',
    thinkingLevel: 'medium',
    permissionMode: 'ask',
  }
}

function reduce(actions: SessionStoreAction[], initial = createSessionStoreState()) {
  return actions.reduce(sessionStoreReducer, initial)
}

describe('sessionStoreReducer', () => {
  test('opens a draft for an empty catalog and keeps draft selection exclusive', () => {
    const emptyDraft = createSessionDraft(undefined, false, undefined, [], undefined, 'draft-1')
    let state = reduce([
      { t: 'sessionsLoaded', sessions: [], emptyDraft },
    ])

    expect(state.draft).toEqual(emptyDraft)
    expect(state.activeSessionID).toBeUndefined()

    state = sessionStoreReducer(state, {
      t: 'sessionsLoaded',
      sessions: [session('session-1')],
      storedSessionID: 'session-1',
      emptyDraft,
    })
    expect(state.activeSessionID).toBeUndefined()

    state = sessionStoreReducer(state, { t: 'sessionSelected', sessionID: 'session-1' })
    expect(state.draft).toBeUndefined()
    expect(state.activeSessionID).toBe('session-1')
  })

  test('merges newer local summaries and does not resurrect deleted sessions', () => {
    const remote = session('session-1', '2026-07-23T12:00:00.000Z')
    const local = { ...remote, title: 'Local title', updatedAt: '2026-07-23T13:00:00.000Z' }
    const emptyDraft = createSessionDraft(undefined, false, undefined, [], undefined, 'draft-1')
    const initial: SessionStoreState = {
      ...createSessionStoreState(),
      sessions: [local, session('session-2')],
      activeSessionID: 'session-1',
    }

    const state = reduce(
      [
        { t: 'sessionDeleted', sessionID: 'session-2' },
        {
          t: 'sessionsLoaded',
          sessions: [remote, session('session-2')],
          emptyDraft,
        },
      ],
      initial,
    )

    expect(state.sessions).toEqual([local])
    expect(state.deletedSessionIDs['session-2']).toBe(true)
    expect(state.activeSessionID).toBe('session-1')
  })

  test('derives session summary flags from wire events and snapshots', () => {
    let state: SessionStoreState = {
      ...createSessionStoreState(),
      sessions: [session('session-1')],
    }
    state = reduce(
      [
        {
          t: 'sessionWire',
          sessionID: 'session-1',
          event: { type: 'approval_request', id: 'approval-1' },
        },
        {
          t: 'sessionSnapshot',
          sessionID: 'session-1',
          history: {
            running: false,
            events: [
              { type: 'approval_request', id: 'approval-1' },
              { type: 'approval_resolved', id: 'approval-1' },
            ],
          },
        },
      ],
      state,
    )

    expect(state.sessions[0]).toMatchObject({ running: false, hasApproval: false })
  })

  test('updates draft settings and transfers first send to the created session', () => {
    const draft = createSessionDraft(undefined, false, undefined, [], undefined, 'draft-1')
    const state = reduce([
      { t: 'draftStarted', draft },
      {
        t: 'draftModelUpdated',
        provider: 'openai',
        model: 'test-model',
        thinkingLevel: 'high',
      },
      { t: 'draftPermissionUpdated', permissionMode: 'read_only' },
      {
        t: 'draftSendQueued',
        submission: { sessionID: 'session-1', text: 'hello', images: [] },
      },
    ])

    expect(state.draft).toBeUndefined()
    expect(state.activeSessionID).toBe('session-1')
    expect(state.pendingDraftSend).toEqual({
      sessionID: 'session-1',
      text: 'hello',
      images: [],
    })
  })

  test('adds a created project session and its workspace atomically', () => {
    const created = {
      ...session('session-1'),
      scope: 'project' as const,
      workspaceKind: 'folder' as const,
      workspacePath: '/work/project',
      workspaceName: 'project',
    }
    const initial = {
      ...createSessionStoreState(),
      draft: createSessionDraft(undefined, false, undefined, [], undefined, 'draft-1'),
    }
    const state = reduce(
      [{ t: 'sessionCreated', session: created, select: true }],
      initial,
    )

    expect(state.sessions).toEqual([created])
    expect(state.workspaces).toEqual([
      { path: '/work/project', name: 'project', addedAt: created.createdAt },
    ])
    expect(state.draft).toBeUndefined()
    expect(state.activeSessionID).toBe('session-1')
  })
})
