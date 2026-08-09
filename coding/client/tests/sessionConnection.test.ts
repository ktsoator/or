import { describe, expect, test } from 'bun:test'
import {
  startSessionConnection,
  type SessionConnectionDependencies,
  type SessionConnectionHandlers,
  type SessionEventSource,
} from '../src/features/session/connection'
import {
  createSessionStoreState,
  sessionStoreReducer,
  type SessionStoreState,
} from '../src/features/session/store'
import {
  threadsReducer,
  type ThreadsState,
} from '../src/features/session/reducer'
import type {
  ConnectionStatus,
  HistoryResponse,
  SessionSummary,
  ThreadSnapshot,
  WireEvent,
} from '../src/types'

class TestEventSource implements SessionEventSource {
  onopen: ((event: Event) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  onmessage: ((event: MessageEvent<string>) => void) | null = null
  closed = false

  constructor(readonly url: string) {}

  close() {
    this.closed = true
  }

  open() {
    this.onopen?.({} as Event)
  }

  fail() {
    this.onerror?.({} as Event)
  }

  emit(payload: unknown, eventSeq?: number) {
    this.emitRaw(JSON.stringify(payload), eventSeq)
  }

  emitRaw(data: string, eventSeq?: number) {
    this.onmessage?.({
      data,
      lastEventId: eventSeq === undefined ? '' : String(eventSeq),
    } as MessageEvent<string>)
  }
}

type Records = {
  statuses: ConnectionStatus[]
  snapshots: ThreadSnapshot[]
  wires: WireEvent[]
  eventSeqs: Array<number | undefined>
  handlers: SessionConnectionHandlers
}

function records(): Records {
  const statuses: ConnectionStatus[] = []
  const snapshots: ThreadSnapshot[] = []
  const wires: WireEvent[] = []
  const eventSeqs: Array<number | undefined> = []
  return {
    statuses,
    snapshots,
    wires,
    eventSeqs,
    handlers: {
      onStatus: (_sessionID, status) => statuses.push(status),
      onSnapshot: (_sessionID, history) => snapshots.push(history),
      onWire: (_sessionID, event, eventSeq) => {
        wires.push(event)
        eventSeqs.push(eventSeq)
      },
    },
  }
}

function history(overrides: Partial<HistoryResponse> = {}): HistoryResponse {
  return {
    events: [],
    queue: [],
    context: {
      provider: 'test',
      model: 'test',
      usedTokens: 0,
      contextWindow: 0,
      measured: false,
    },
    running: false,
    eventSeq: 0,
    title: 'New session',
    ...overrides,
  }
}

function session(id: string): SessionSummary {
  return {
    id,
    title: 'Original prompt title',
    workspacePath: `/tmp/${id}`,
    workspaceName: id,
    scope: 'chat',
    workspaceKind: 'scratch',
    createdAt: '2026-07-23T11:00:00.000Z',
    updatedAt: '2026-07-23T12:00:00.000Z',
    running: false,
    hasApproval: false,
    hasQuestion: false,
    modelProvider: 'openai',
    modelId: 'test-model',
    modelName: 'Test model',
    thinkingLevel: 'medium',
    permissionMode: 'ask',
  }
}

async function waitFor(check: () => boolean, message: string) {
  for (let attempt = 0; attempt < 50; attempt++) {
    if (check()) return
    await new Promise((resolve) => setTimeout(resolve, 0))
  }
  throw new Error(message)
}

describe('startSessionConnection', () => {
  test('restores a title completed before the SSE stream opens', async () => {
    const sessionID = 'session-1'
    const sources: TestEventSource[] = []
    let state: SessionStoreState = {
      ...createSessionStoreState(),
      sessions: [session(sessionID)],
    }
    let snapshotApplied = false
    const restored = history({
      title: 'Inspect parser behavior',
      eventSeq: 8,
    })
    const handlers: SessionConnectionHandlers = {
      onStatus: () => undefined,
      onWire: (id, event) => {
        state = sessionStoreReducer(state, { t: 'sessionWire', sessionID: id, event })
      },
      onSnapshot: (id, snapshot) => {
        state = sessionStoreReducer(state, { t: 'sessionSnapshot', sessionID: id, history: snapshot })
        snapshotApplied = true
      },
    }
    const dependencies: SessionConnectionDependencies = {
      request: async () => Response.json(restored),
      openEvents: (url) => {
        expect(snapshotApplied).toBe(true)
        const source = new TestEventSource(url)
        sources.push(source)
        return source
      },
    }

    const stop = startSessionConnection(sessionID, handlers, dependencies)
    await waitFor(() => sources.length === 1, 'event stream was not opened')

    expect(state.sessions[0]?.title).toBe('Inspect parser behavior')
    expect(sources[0]?.url).toBe('/api/sessions/session-1/events?after=8')

    stop()
  })

  test('restores history before connecting from the snapshot sequence', async () => {
    const recorded = records()
    const sources: TestEventSource[] = []
    const requestURLs: string[] = []
    let requestSignal: AbortSignal | undefined
    const restored = history({
      running: true,
      events: [{ type: 'user_message', text: 'restored' }],
      eventSeq: 17,
    })
    const dependencies: SessionConnectionDependencies = {
      request: async (url, init) => {
        requestURLs.push(url)
        requestSignal = init.signal as AbortSignal
        return Response.json(restored)
      },
      openEvents: (url) => {
        const source = new TestEventSource(url)
        sources.push(source)
        return source
      },
    }

    const stop = startSessionConnection('session-1', recorded.handlers, dependencies)
    await waitFor(() => sources.length === 1, 'event stream was not opened')

    expect(requestURLs).toEqual(['/api/sessions/session-1/history'])
    expect(recorded.statuses).toEqual(['connecting'])
    expect(recorded.snapshots).toEqual([restored])
    expect(sources[0]?.url).toBe('/api/sessions/session-1/events?after=17')

    sources[0]?.open()
    sources[0]?.emit({ type: 'delta', kind: 'text', delta: 'live' })
    expect(recorded.statuses).toEqual(['connecting', 'ready'])
    expect(recorded.wires).toEqual([{ type: 'delta', kind: 'text', delta: 'live' }])

    stop()
    expect(sources[0]?.closed).toBe(true)
    expect(requestSignal?.aborted).toBe(true)
  })

  test('resumes a loaded session from its event cursor without replacing history', () => {
    const recorded = records()
    const sources: TestEventSource[] = []
    const requestURLs: string[] = []
    const dependencies: SessionConnectionDependencies = {
      request: async (url) => {
        requestURLs.push(url)
        return Response.json(history())
      },
      openEvents: (url) => {
        const source = new TestEventSource(url)
        sources.push(source)
        return source
      },
    }

    const stop = startSessionConnection(
      'session-1',
      recorded.handlers,
      dependencies,
      { eventSeq: 17 },
    )

    expect(requestURLs).toEqual([])
    expect(recorded.snapshots).toEqual([])
    expect(recorded.statuses).toEqual(['connecting'])
    expect(sources[0]?.url).toBe('/api/sessions/session-1/events?after=17')

    sources[0]?.emit({ type: 'delta', kind: 'text', delta: 'still here' }, 18)
    expect(recorded.wires).toEqual([
      { type: 'delta', kind: 'text', delta: 'still here' },
    ])
    expect(recorded.eventSeqs).toEqual([18])

    stop()
  })

  test('falls back to history when a resumed cursor requires synchronization', async () => {
    const recorded = records()
    const sources: TestEventSource[] = []
    const restored = history({
      running: true,
      events: [{ type: 'user_message', text: 'restored after restart' }],
      eventSeq: 4,
    })
    let requests = 0
    const dependencies: SessionConnectionDependencies = {
      request: async () => {
        requests++
        return Response.json(restored)
      },
      openEvents: (url) => {
        const source = new TestEventSource(url)
        sources.push(source)
        return source
      },
    }

    const stop = startSessionConnection(
      'session-1',
      recorded.handlers,
      dependencies,
      { eventSeq: 99 },
    )
    sources[0]?.emit({ type: 'sync_required' })
    await waitFor(() => sources.length === 2, 'replacement event stream was not opened')

    expect(requests).toBe(1)
    expect(recorded.snapshots).toEqual([restored])
    expect(sources[0]?.closed).toBe(true)
    expect(sources[1]?.url).toBe('/api/sessions/session-1/events?after=4')

    stop()
  })

  test('keeps streamed text while replaying tool events after a session switch', () => {
    const sessionID = 'session-1'
    let threads: ThreadsState = threadsReducer({}, {
      t: 'reset',
      sessionID,
      history: { running: true, events: [], eventSeq: 8 },
    })
    const apply = (event: WireEvent, eventSeq: number) => {
      threads = threadsReducer(threads, {
        t: 'wire',
        sessionID,
        ev: event,
        serverEventSeq: eventSeq,
      })
    }
    apply({ type: 'run_start', startedAt: '2026-07-27T12:00:00Z' }, 9)
    apply({ type: 'delta', kind: 'text', delta: 'I will inspect the file.' }, 10)
    apply({ type: 'tool_input_start', tool: 'read', toolContentIndex: 0 }, 11)
    apply({ type: 'tool_input_delta', tool: 'read', toolContentIndex: 0, bytes: 12 }, 12)

    const beforeSwitch = threads[sessionID]
    expect(beforeSwitch?.serverEventSeq).toBe(12)
    expect(beforeSwitch?.items).toContainEqual(
      expect.objectContaining({ kind: 'assistant', markdown: 'I will inspect the file.' }),
    )

    const sources: TestEventSource[] = []
    let historyRequests = 0
    const stop = startSessionConnection(
      sessionID,
      {
        onStatus: () => undefined,
        onSnapshot: (id, snapshot) => {
          threads = threadsReducer(threads, { t: 'reset', sessionID: id, history: snapshot })
        },
        onWire: (id, event, eventSeq) => {
          threads = threadsReducer(threads, {
            t: 'wire',
            sessionID: id,
            ev: event,
            serverEventSeq: eventSeq,
          })
        },
      },
      {
        request: async () => {
          historyRequests++
          return Response.json(history())
        },
        openEvents: (url) => {
          const source = new TestEventSource(url)
          sources.push(source)
          return source
        },
      },
      { eventSeq: beforeSwitch?.serverEventSeq ?? 0 },
    )

    expect(historyRequests).toBe(0)
    expect(sources[0]?.url).toBe('/api/sessions/session-1/events?after=12')
    sources[0]?.emit(
      {
        type: 'tool_input_end',
        id: 'call-1',
        tool: 'read',
        args: { path: 'README.md' },
        toolContentIndex: 0,
      },
      13,
    )
    sources[0]?.emit(
      { type: 'message_end', text: 'I will inspect the file.' },
      14,
    )
    sources[0]?.emit(
      { type: 'tool_start', id: 'call-1', tool: 'read', args: { path: 'README.md' } },
      15,
    )
    sources[0]?.emit(
      {
        type: 'tool_end',
        id: 'call-1',
        tool: 'read',
        result: 'contents',
        outcome: { status: 'success' },
      },
      16,
    )

    const afterReplay = threads[sessionID]
    expect(afterReplay?.serverEventSeq).toBe(16)
    expect(
      afterReplay?.items.filter(
        (item) => item.kind === 'assistant' && item.markdown === 'I will inspect the file.',
      ),
    ).toHaveLength(1)
    expect(afterReplay?.items).toContainEqual(
      expect.objectContaining({ kind: 'tool', id: 'call-1', status: 'complete' }),
    )

    stop()
  })

  test('restores in-flight text and tool input after a renderer restart', async () => {
    const sessionID = 'session-1'
    let threads: ThreadsState = {}
    const sources: TestEventSource[] = []
    const restored = history({
      running: true,
      eventSeq: 44,
      events: [
        { type: 'user_message', text: 'Update the file' },
        { type: 'run_start', startedAt: '2026-07-27T12:00:00Z' },
        { type: 'delta', kind: 'text', delta: 'I am updating it.' },
        { type: 'tool_input_start', tool: 'write', toolContentIndex: 0 },
        {
          type: 'tool_input_delta',
          tool: 'write',
          toolContentIndex: 0,
          delta: '{"path":"README.md"',
          bytes: 128,
        },
      ],
    })
    const stop = startSessionConnection(
      sessionID,
      {
        onStatus: () => undefined,
        onSnapshot: (id, snapshot) => {
          threads = threadsReducer(threads, { t: 'reset', sessionID: id, history: snapshot })
        },
        onWire: (id, event, eventSeq) => {
          threads = threadsReducer(threads, {
            t: 'wire',
            sessionID: id,
            ev: event,
            serverEventSeq: eventSeq,
          })
        },
      },
      {
        request: async () => Response.json(restored),
        openEvents: (url) => {
          const source = new TestEventSource(url)
          sources.push(source)
          return source
        },
      },
    )
    await waitFor(() => sources.length === 1, 'event stream was not opened')

    expect(sources[0]?.url).toBe('/api/sessions/session-1/events?after=44')
    expect(threads[sessionID]?.items).toContainEqual(
      expect.objectContaining({
        kind: 'assistant',
        markdown: 'I am updating it.',
        open: false,
      }),
    )
    expect(threads[sessionID]?.items).toContainEqual(
      expect.objectContaining({
        kind: 'tool',
        name: 'write',
        status: 'preparing',
        args: '{"path":"README.md"',
        generatedBytes: 128,
      }),
    )

    sources[0]?.emit(
      {
        type: 'tool_input_end',
        id: 'write-1',
        tool: 'write',
        args: { path: 'README.md', content: 'updated' },
        toolContentIndex: 0,
      },
      45,
    )
    sources[0]?.emit({ type: 'message_end', text: 'I am updating it.' }, 46)
    sources[0]?.emit(
      {
        type: 'tool_start',
        id: 'write-1',
        tool: 'write',
        args: { path: 'README.md', content: 'updated' },
      },
      47,
    )
    sources[0]?.emit(
      {
        type: 'tool_end',
        id: 'write-1',
        tool: 'write',
        result: 'Updated README.md',
        outcome: { status: 'success' },
      },
      48,
    )

    expect(threads[sessionID]?.serverEventSeq).toBe(48)
    expect(
      threads[sessionID]?.items.filter(
        (item) => item.kind === 'assistant' && item.markdown === 'I am updating it.',
      ),
    ).toHaveLength(1)
    expect(threads[sessionID]?.items).toContainEqual(
      expect.objectContaining({ kind: 'tool', id: 'write-1', status: 'complete' }),
    )

    stop()
  })

  test('resynchronizes on demand and ignores events from the replaced source', async () => {
    const recorded = records()
    const sources: TestEventSource[] = []
    const histories: HistoryResponse[] = [
      history({ running: true, eventSeq: 3 }),
      history({
        running: true,
        events: [{ type: 'delta', kind: 'text', delta: 'replayed' }],
        eventSeq: 9,
      }),
    ]
    let requestIndex = 0
    const dependencies: SessionConnectionDependencies = {
      request: async () => Response.json(histories[requestIndex++] as HistoryResponse),
      openEvents: (url) => {
        const source = new TestEventSource(url)
        sources.push(source)
        return source
      },
    }

    const stop = startSessionConnection('session-1', recorded.handlers, dependencies)
    await waitFor(() => sources.length === 1, 'initial event stream was not opened')
    const replaced = sources[0]
    replaced?.emit({ type: 'sync_required' })
    await waitFor(() => sources.length === 2, 'replacement event stream was not opened')

    expect(replaced?.closed).toBe(true)
    expect(recorded.snapshots).toEqual(histories)
    expect(recorded.statuses).toEqual(['connecting', 'connecting'])
    expect(sources[1]?.url).toBe('/api/sessions/session-1/events?after=9')

    replaced?.emit({ type: 'delta', kind: 'text', delta: 'stale' })
    sources[1]?.emit({ type: 'delta', kind: 'text', delta: 'current' })
    expect(recorded.wires).toEqual([{ type: 'delta', kind: 'text', delta: 'current' }])

    stop()
  })

  test('reports initial history failure and continues with a fresh stream', async () => {
    const recorded = records()
    const sources: TestEventSource[] = []
    const dependencies: SessionConnectionDependencies = {
      request: async () => new Response(null, { status: 503 }),
      openEvents: (url) => {
        const source = new TestEventSource(url)
        sources.push(source)
        return source
      },
    }

    const stop = startSessionConnection('session-1', recorded.handlers, dependencies)
    await waitFor(() => sources.length === 1, 'fallback event stream was not opened')

    expect(recorded.statuses).toEqual(['connecting', 'disconnected'])
    expect(recorded.snapshots).toEqual([
      {
        running: false,
        events: [{ type: 'error', text: 'History could not be restored.' }],
      },
    ])
    expect(sources[0]?.url).toBe('/api/sessions/session-1/events')

    sources[0]?.fail()
    sources[0]?.emitRaw('{invalid json')
    expect(recorded.statuses.at(-1)).toBe('disconnected')
    expect(recorded.wires).toEqual([
      { type: 'error', text: 'Received an invalid server event.' },
    ])

    stop()
  })

  test('aborts pending history and suppresses late results when stopped', async () => {
    const recorded = records()
    const sources: TestEventSource[] = []
    let resolveRequest: ((response: Response) => void) | undefined
    let requestSignal: AbortSignal | undefined
    const dependencies: SessionConnectionDependencies = {
      request: (_url, init) => {
        requestSignal = init.signal as AbortSignal
        return new Promise((resolve) => {
          resolveRequest = resolve
        })
      },
      openEvents: (url) => {
        const source = new TestEventSource(url)
        sources.push(source)
        return source
      },
    }

    const stop = startSessionConnection('session-1', recorded.handlers, dependencies)
    stop()
    resolveRequest?.(Response.json({ running: false, events: [], eventSeq: 1 }))
    await new Promise((resolve) => setTimeout(resolve, 0))

    expect(requestSignal?.aborted).toBe(true)
    expect(recorded.snapshots).toHaveLength(0)
    expect(sources).toHaveLength(0)
    expect(recorded.statuses).toEqual(['connecting'])
  })
})
