import { describe, expect, test } from 'bun:test'
import {
  threadsReducer,
  type ThreadAction,
  type ThreadsState,
} from '../src/sessionReducer'

const sessionID = 'session-1'
const startedAt = '2026-07-23T12:00:00.000Z'

function reduce(actions: ThreadAction[], initial: ThreadsState = {}): ThreadsState {
  return actions.reduce(threadsReducer, initial)
}

function thread(state: ThreadsState) {
  const value = state[sessionID]
  if (!value) throw new Error(`missing thread state for ${sessionID}`)
  return value
}

describe('threadsReducer event sequences', () => {
  test('reconciles optimistic queue messages with acknowledgements and consumption', () => {
    let state = reduce([
      {
        t: 'sendUser',
        sessionID,
        id: 'queued-1',
        text: 'first follow-up',
        images: [],
        startedAt,
        delivery: 'followup',
      },
      {
        t: 'wire',
        sessionID,
        ev: {
          type: 'user_message',
          id: 'queued-1',
          text: 'first follow-up',
          images: [],
          delivery: 'followup',
          queued: true,
        },
      },
    ])

    expect(thread(state).queue).toEqual([
      {
        id: 'queued-1',
        text: 'first follow-up',
        images: [],
        delivery: 'followup',
        status: 'queued',
      },
    ])

    state = reduce(
      [
        {
          t: 'wire',
          sessionID,
          ev: {
            type: 'user_message',
            id: 'queued-1',
            text: 'first follow-up',
            images: [],
            delivery: 'followup',
          },
        },
      ],
      state,
    )

    expect(thread(state).queue).toHaveLength(0)
    expect(thread(state).items).toContainEqual({
      kind: 'user',
      id: 'queued-1',
      text: 'first follow-up',
      images: [],
      sentAt: undefined,
    })
  })

  test('tracks queue cancellation and removal events', () => {
    let state = reduce([
      {
        t: 'sendUser',
        sessionID,
        id: 'queued-2',
        text: 'cancel me',
        images: [],
        startedAt,
        delivery: 'steer',
      },
      {
        t: 'wire',
        sessionID,
        ev: { type: 'queue_cancelled', id: 'queued-2' },
      },
    ])

    expect(thread(state).queue).toEqual([
      expect.objectContaining({ id: 'queued-2', status: 'failed' }),
    ])

    state = reduce(
      [
        {
          t: 'wire',
          sessionID,
          ev: { type: 'queue_removed', id: 'queued-2' },
        },
      ],
      state,
    )

    expect(thread(state).queue).toHaveLength(0)
  })

  test('discards a partial attempt and retains the retry response', () => {
    let state = reduce([
      {
        t: 'wire',
        sessionID,
        ev: { type: 'run_start', id: 'run-1', startedAt },
      },
      {
        t: 'wire',
        sessionID,
        ev: { type: 'delta', kind: 'thinking', delta: 'old thinking' },
      },
      {
        t: 'wire',
        sessionID,
        ev: { type: 'delta', kind: 'text', delta: 'partial answer' },
      },
      { t: 'wire', sessionID, ev: { type: 'turn_discard' } },
    ])

    expect(thread(state).items.map((item) => item.kind)).toEqual(['run'])

    state = reduce(
      [
        {
          t: 'wire',
          sessionID,
          ev: { type: 'delta', kind: 'thinking', delta: 'retry thinking' },
        },
        {
          t: 'wire',
          sessionID,
          ev: { type: 'delta', kind: 'text', delta: 'retry response' },
        },
        {
          t: 'wire',
          sessionID,
          ev: { type: 'message_end', text: 'retry response', finalResponse: true },
        },
      ],
      state,
    )

    expect(thread(state).items).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          kind: 'thinking',
          text: 'retry thinking',
          streaming: false,
        }),
        expect.objectContaining({
          kind: 'assistant',
          markdown: 'retry response',
          open: false,
          complete: true,
        }),
      ]),
    )
    expect(JSON.stringify(thread(state).items)).not.toContain('old thinking')
    expect(JSON.stringify(thread(state).items)).not.toContain('partial answer')
  })

  test('adds, resolves, and cancels approvals without ending the run', () => {
    let state = reduce([
      {
        t: 'wire',
        sessionID,
        ev: {
          type: 'approval_request',
          id: 'approval-1',
          summary: 'Run command',
          reason: 'Needs workspace access',
        },
      },
    ])

    expect(thread(state).running).toBe(true)
    expect(thread(state).items).toContainEqual({
      kind: 'approval',
      id: 'approval-1',
      summary: 'Run command',
      reason: 'Needs workspace access',
    })

    state = reduce(
      [
        {
          t: 'wire',
          sessionID,
          ev: { type: 'approval_resolved', id: 'approval-1' },
        },
        {
          t: 'wire',
          sessionID,
          ev: { type: 'approval_request', id: 'approval-2' },
        },
        {
          t: 'wire',
          sessionID,
          ev: { type: 'approval_cancelled', id: 'approval-2' },
        },
      ],
      state,
    )

    expect(thread(state).items.filter((item) => item.kind === 'approval')).toHaveLength(0)
    expect(thread(state).running).toBe(true)
  })

  test('opens a pending browser command and keeps tool completion navigation-idempotent', () => {
    const state = reduce([
      {
        t: 'wire',
        sessionID,
        ev: {
          type: 'browser_request',
          id: 'browser-1',
          disposition: 'reuse_agent_tab',
          preview: { url: 'https://example.com/start', title: 'Example' },
        },
      },
      {
        t: 'wire',
        sessionID,
        ev: {
          type: 'tool_end',
          id: 'preview-call',
          tool: 'open_preview',
          result: 'Opened preview at https://example.com/final',
          preview: { url: 'https://example.com/start', title: 'Example' },
        },
      },
    ])

    expect(thread(state).preview).toEqual({
      url: 'https://example.com/start',
      title: 'Example',
      commandID: 'browser-1',
      disposition: 'reuse_agent_tab',
      revision: 1,
    })
    expect(thread(state).browserCommands).toEqual([thread(state).preview])
    expect(thread(state).previewOpen).toBe(true)
  })

  test('restores a pending browser request as an active preview after reconnect', () => {
    const state = reduce([
      {
        t: 'reset',
        sessionID,
        history: {
          running: true,
          events: [
            {
              type: 'browser_request',
              id: 'browser-pending',
              disposition: 'reuse_agent_tab',
              preview: { url: 'https://example.com/' },
            },
          ],
        },
      },
    ])

    expect(thread(state).preview).toEqual({
      url: 'https://example.com/',
      commandID: 'browser-pending',
      disposition: 'reuse_agent_tab',
      revision: 1,
    })
    expect(thread(state).browserCommands).toEqual([thread(state).preview])
    expect(thread(state).previewOpen).toBe(true)
  })

  test('restores multiple pending tab commands and keeps a background request hidden', () => {
    let state = reduce([
      {
        t: 'reset',
        sessionID,
        history: {
          running: true,
          events: [
            {
              type: 'browser_request',
              id: 'browser-foreground',
              disposition: 'new_foreground_tab',
              preview: { url: 'https://github.com/' },
            },
            {
              type: 'browser_request',
              id: 'browser-background',
              disposition: 'new_background_tab',
              preview: { url: 'https://www.google.com/' },
            },
          ],
        },
      },
    ])

    expect(thread(state).browserCommands.map((command) => command.commandID)).toEqual([
      'browser-foreground',
      'browser-background',
    ])
    expect(thread(state).previewOpen).toBe(false)

    state = reduce(
      [{ t: 'browserCommandHandled', sessionID, id: 'browser-foreground' }],
      state,
    )
    expect(thread(state).browserCommands.map((command) => command.commandID)).toEqual([
      'browser-background',
    ])
  })

  test('restores, deduplicates, and handles pending browser inspections', () => {
    let state = reduce([
      {
        t: 'reset',
        sessionID,
        history: {
          running: true,
          events: [
            { type: 'browser_inspect_request', id: 'inspection-1' },
            { type: 'browser_inspect_request', id: 'inspection-1' },
          ],
        },
      },
      {
        t: 'wire',
        sessionID,
        ev: { type: 'browser_inspect_request', id: 'inspection-2' },
      },
    ])

    expect(thread(state).browserInspections).toEqual([
      { commandID: 'inspection-1' },
      { commandID: 'inspection-2' },
    ])

    state = reduce(
      [{ t: 'browserInspectionHandled', sessionID, id: 'inspection-1' }],
      state,
    )
    expect(thread(state).browserInspections).toEqual([{ commandID: 'inspection-2' }])
  })

  test('rebuilds history after disconnect and finalizes an idle open run', () => {
    const state = reduce([
      { t: 'status', sessionID, status: 'disconnected' },
      {
        t: 'wire',
        sessionID,
        ev: { type: 'error', text: 'stale live error' },
      },
      {
        t: 'reset',
        sessionID,
        history: {
          running: false,
          events: [
            { type: 'user_message', id: 'user-1', text: 'hello', images: [] },
            { type: 'run_start', id: 'run-1', startedAt },
            { type: 'delta', kind: 'text', delta: 'restored response' },
          ],
        },
      },
    ])

    const restored = thread(state)
    expect(restored.status).toBe('disconnected')
    expect(restored.loaded).toBe(true)
    expect(restored.running).toBe(false)
    expect(JSON.stringify(restored.items)).not.toContain('stale live error')
    expect(restored.items).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ kind: 'user', id: 'user-1', text: 'hello' }),
        expect.objectContaining({ kind: 'assistant', markdown: 'restored response' }),
      ]),
    )

    const run = restored.items.find((item) => item.kind === 'run')
    expect(run?.kind).toBe('run')
    if (run?.kind === 'run') {
      expect(run.durationMs).toBeNumber()
      expect(run.durationMs).toBeGreaterThanOrEqual(0)
    }
  })

  test('invalidates measured context after successful compaction only', () => {
    const usage = {
      input: 40,
      output: 2,
      cacheRead: 0,
      cacheWrite: 0,
      totalTokens: 42,
      cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 },
    }
    let state = reduce([
      {
        t: 'contextInvalidate',
        sessionID,
        provider: 'openai',
        model: 'test-model',
        contextWindow: 128_000,
      },
      {
        t: 'wire',
        sessionID,
        ev: { type: 'message_end', usage },
      },
      { t: 'wire', sessionID, ev: { type: 'compaction_start' } },
    ])

    expect(thread(state).autoCompacting).toBe(true)
    expect(thread(state).contextUsage).toEqual({
      provider: 'openai',
      model: 'test-model',
      usedTokens: 42,
      contextWindow: 128_000,
      measured: true,
    })

    state = reduce(
      [{ t: 'wire', sessionID, ev: { type: 'compaction_end', isError: true } }],
      state,
    )
    expect(thread(state).autoCompacting).toBe(false)
    expect(thread(state).contextUsage?.usedTokens).toBe(42)
    expect(thread(state).contextUsage?.measured).toBe(true)

    state = reduce(
      [
        { t: 'wire', sessionID, ev: { type: 'compaction_start' } },
        { t: 'wire', sessionID, ev: { type: 'compaction_end', isError: false } },
      ],
      state,
    )
    expect(thread(state).autoCompacting).toBe(false)
    expect(thread(state).contextUsage?.usedTokens).toBe(0)
    expect(thread(state).contextUsage?.measured).toBe(false)
  })
})
