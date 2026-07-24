import { describe, expect, test } from 'bun:test'
import {
  startSessionConnection,
  type SessionConnectionDependencies,
  type SessionConnectionHandlers,
  type SessionEventSource,
} from '../src/sessionConnection'
import {
  createSessionStoreState,
  sessionStoreReducer,
  type SessionStoreState,
} from '../src/sessionStore'
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

  emit(payload: unknown) {
    this.emitRaw(JSON.stringify(payload))
  }

  emitRaw(data: string) {
    this.onmessage?.({ data } as MessageEvent<string>)
  }
}

type Records = {
  statuses: ConnectionStatus[]
  snapshots: ThreadSnapshot[]
  wires: WireEvent[]
  handlers: SessionConnectionHandlers
}

function records(): Records {
  const statuses: ConnectionStatus[] = []
  const snapshots: ThreadSnapshot[] = []
  const wires: WireEvent[] = []
  return {
    statuses,
    snapshots,
    wires,
    handlers: {
      onStatus: (_sessionID, status) => statuses.push(status),
      onSnapshot: (_sessionID, history) => snapshots.push(history),
      onWire: (_sessionID, event) => wires.push(event),
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
  test('restores an AI title completed before the SSE stream opens', async () => {
    const sessionID = 'session-1'
    const sources: TestEventSource[] = []
    let state: SessionStoreState = {
      ...createSessionStoreState(),
      sessions: [session(sessionID)],
    }
    let snapshotApplied = false
    const restored = history({
      title: 'Inspect parser behavior',
      aiTitle: 'Inspect parser behavior',
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

    expect(state.sessions[0]).toMatchObject({
      title: 'Inspect parser behavior',
      aiTitle: 'Inspect parser behavior',
    })
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
