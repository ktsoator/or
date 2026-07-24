import { describe, expect, test } from 'bun:test'
import {
  clampSidebarWidth,
  groupSidebarSessions,
  keyboardSidebarWidth,
  parsePinnedSessionIDs,
  resizedSidebarWidth,
} from '../src/sidebarLayout'
import type { SessionSummary, WorkspaceSummary } from '../src/types'

function session(
  id: string,
  scope: SessionSummary['scope'],
  workspacePath = '',
): SessionSummary {
  return {
    id,
    title: id,
    workspacePath,
    workspaceName: workspacePath.split('/').at(-1) ?? '',
    scope,
    workspaceKind: scope === 'project' ? 'folder' : 'scratch',
    createdAt: '2026-07-23T11:00:00.000Z',
    updatedAt: '2026-07-23T12:00:00.000Z',
    running: false,
    hasApproval: false,
    modelProvider: 'openai',
    modelId: 'test-model',
    modelName: 'Test model',
    thinkingLevel: 'medium',
    permissionMode: 'ask',
  }
}

describe('sidebar layout', () => {
  test('parses pinned session storage defensively', () => {
    expect(parsePinnedSessionIDs(null)).toEqual([])
    expect(parsePinnedSessionIDs('{invalid')).toEqual([])
    expect(parsePinnedSessionIDs('{"id":"session-1"}')).toEqual([])
    expect(parsePinnedSessionIDs('["session-1",4,null,"session-2"]')).toEqual([
      'session-1',
      'session-2',
    ])
  })

  test('groups sessions by workspace and moves pinned sessions first', () => {
    const workspaces: WorkspaceSummary[] = [
      { path: '/work/a', name: 'a', addedAt: '2026-07-23T10:00:00.000Z' },
      { path: '/work/b', name: 'b', addedAt: '2026-07-23T10:00:00.000Z' },
    ]
    const grouped = groupSidebarSessions(
      [
        session('chat-1', 'chat'),
        session('project-a-1', 'project', '/work/a'),
        session('chat-2', 'chat'),
        session('project-a-2', 'project', '/work/a'),
        session('orphan', 'project', '/work/missing'),
      ],
      workspaces,
      new Set(['chat-2', 'project-a-2']),
    )

    expect(grouped.chatSessions.map(({ id }) => id)).toEqual(['chat-2', 'chat-1'])
    expect(grouped.workspaceGroups.map(({ path }) => path)).toEqual(['/work/a', '/work/b'])
    expect(grouped.workspaceGroups[0]?.sessions.map(({ id }) => id)).toEqual([
      'project-a-2',
      'project-a-1',
    ])
    expect(grouped.workspaceGroups[1]?.sessions).toEqual([])
  })

  test('clamps pointer resizing to the sidebar width range', () => {
    expect(clampSidebarWidth(100)).toBe(206)
    expect(clampSidebarWidth(400)).toBe(338)
    expect(resizedSidebarWidth(240, 100, 150)).toBe(290)
    expect(resizedSidebarWidth(240, 100, 0)).toBe(206)
    expect(resizedSidebarWidth(240, 100, 300)).toBe(338)
  })

  test('maps keyboard controls to bounded sidebar widths', () => {
    expect(keyboardSidebarWidth('ArrowLeft', 240)).toBe(232)
    expect(keyboardSidebarWidth('ArrowRight', 240)).toBe(248)
    expect(keyboardSidebarWidth('Home', 240)).toBe(206)
    expect(keyboardSidebarWidth('End', 240)).toBe(338)
    expect(keyboardSidebarWidth('Escape', 240)).toBeUndefined()
  })
})
