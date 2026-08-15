import { describe, expect, test } from 'bun:test'
import {
  DiagnosticTraceError,
  fetchDiagnosticTrace,
  mergeDiagnosticTracePage,
  mergeDiagnosticTraceRun,
  mergeLatestDiagnosticTrace,
} from '../src/features/diagnostics/catalog'
import type { TraceBundle } from '../src/features/diagnostics/catalog'
import {
  liveTraceRefreshKey,
  mergeLiveTraceBundle,
} from '../src/features/diagnostics/liveTrace'
import type { Item } from '../src/types'

describe('diagnostic trace catalog', () => {
  test('loads one assembled conversation bundle scoped to session and run', async () => {
    let requestedURL = ''
    let requestInit: RequestInit | undefined
    const controller = new AbortController()
    const bundle = await fetchDiagnosticTrace(
      'session/one',
      {
        runID: 'run one',
        signal: controller.signal,
        request: async (url, init) => {
          requestedURL = String(url)
          requestInit = init
          return Response.json({
            version: 1,
            generatedAt: '2026-08-14T12:00:00Z',
            sessionId: 'session/one',
            selectedTaskId: 'run one',
            tasks: [{ id: 'run one', requests: [] }],
          })
        },
      },
    )

    expect(requestedURL).toBe('/api/diagnostics/trace?sessionId=session%2Fone&runId=run+one')
    expect(requestInit?.cache).toBe('no-store')
    expect(requestInit?.signal).toBe(controller.signal)
    expect(bundle.selectedTaskId).toBe('run one')
    expect(bundle.tasks[0]?.id).toBe('run one')
  })

  test('loads the conversation with its latest task selected when no run is specified', async () => {
    let requestedURL = ''
    await fetchDiagnosticTrace('session', {
      request: async (url) => {
        requestedURL = String(url)
        return Response.json({ selectedTaskId: 'latest', tasks: [] })
      },
    })
    expect(requestedURL).toBe('/api/diagnostics/trace?sessionId=session&limit=12')
  })

  test('loads an older page with an opaque cursor', async () => {
    let requestedURL = ''
    await fetchDiagnosticTrace('session', {
      beforeCursor: 'opaque cursor',
      limit: 8,
      request: async (url) => {
        requestedURL = String(url)
        return Response.json({ selectedTaskId: 'older', tasks: [], page: { hasMore: false } })
      },
    })
    expect(requestedURL).toBe('/api/diagnostics/trace?sessionId=session&limit=8&before=opaque+cursor')
  })

  test('normalizes incomplete interrupted bundles', async () => {
    const bundle = await fetchDiagnosticTrace('session', { request: async () => Response.json({
      version: 1,
      generatedAt: '2026-08-14T12:00:00Z',
      sessionId: 'session',
      selectedTaskId: 'run-1',
      tasks: [{
        id: 'run-1',
        rawEvents: null,
        requests: [{
          id: 'request-1',
          rawEvents: null,
          attempts: [{ number: 1, rawEvents: null }],
          checkpoints: null,
          attachments: null,
          input: { messages: [{ role: 'user', content: null }], tools: null },
          output: { message: { role: 'assistant', content: null } },
          tools: [{ id: 'call-1', rawEvents: null, result: { role: 'toolResult', content: null } }],
        }],
      }],
    }) })

    expect(bundle.tasks[0]?.rawEvents).toEqual([])
    const request = bundle.tasks[0]?.requests[0]
    expect(request?.rawEvents).toEqual([])
    expect(request?.attempts[0]?.rawEvents).toEqual([])
    expect(request?.checkpoints).toEqual([])
    expect(request?.attachments).toEqual([])
    expect(request?.input?.messages[0]?.content).toEqual([])
    expect(request?.input?.tools).toEqual([])
    expect(request?.output?.message.content).toEqual([])
    expect(request?.tools[0]?.rawEvents).toEqual([])
    expect(request?.tools[0]?.result?.content).toEqual([])
    expect(bundle.page).toEqual({ hasMore: false })
  })

  test('rejects failed bundle responses', async () => {
    expect(
      fetchDiagnosticTrace('session', { request: async () => new Response('', { status: 503 }) }),
    ).rejects.toThrow('HTTP 503')
  })

  test('preserves the HTTP status for an unavailable trace', async () => {
    try {
      await fetchDiagnosticTrace(
        'new-session',
        { request: async () => new Response('', { status: 404 }) },
      )
      throw new Error('expected the trace request to fail')
    } catch (cause) {
      expect(cause).toBeInstanceOf(DiagnosticTraceError)
      expect((cause as DiagnosticTraceError).status).toBe(404)
    }
  })

  test('rejects a run and page cursor in the same request', async () => {
    expect(fetchDiagnosticTrace('session', {
      runID: 'run-1',
      beforeCursor: 'cursor',
    })).rejects.toThrow('mutually exclusive')
  })
})

describe('diagnostic trace paging', () => {
  test('prepends older tasks and assigns request numbers across the loaded window', () => {
    const current = traceBundle()
    current.page = { hasMore: true, beforeCursor: 'current-cursor' }
    const older = traceBundle()
    older.selectedTaskId = 'run-old'
    older.tasks[0] = {
      ...older.tasks[0]!,
      id: 'run-old',
      startedAt: '2026-08-14T11:00:00Z',
      requests: [{ ...older.tasks[0]!.requests[0]!, id: 'request-old', number: 1 }],
    }
    older.page = { hasMore: false }

    const merged = mergeDiagnosticTracePage(current, older)

    expect(merged.tasks.map((task) => task.id)).toEqual(['run-old', 'run-1'])
    expect(merged.tasks.flatMap((task) => task.requests.map((request) => request.number))).toEqual([1, 2])
    expect(merged.selectedTaskId).toBe('run-1')
    expect(merged.page).toEqual({ hasMore: false })
  })

  test('refreshes the latest page without discarding already loaded history', () => {
    const older = traceBundle().tasks[0]!
    older.id = 'run-old'
    older.startedAt = '2026-08-14T11:00:00Z'
    older.requests = [{ ...older.requests[0]!, id: 'request-old' }]
    const current = traceBundle()
    current.tasks = [older, current.tasks[0]!]
    current.page = { hasMore: false }
    const latest = traceBundle()
    latest.tasks[0] = { ...latest.tasks[0]!, status: 'completed' }
    latest.page = { hasMore: true, beforeCursor: 'latest-cursor' }

    const merged = mergeLatestDiagnosticTrace(current, latest)

    expect(merged.tasks.map((task) => task.id)).toEqual(['run-old', 'run-1'])
    expect(merged.tasks[1]?.status).toBe('completed')
    expect(merged.page).toEqual({ hasMore: false })
  })

  test('uses the latest cursor when the only unmatched task is newer provisional data', () => {
    const current = traceBundle()
    current.tasks.push({
      ...current.tasks[0]!,
      id: 'run-live',
      startedAt: '2026-08-14T13:00:00Z',
      requests: [{ ...current.tasks[0]!.requests[0]!, id: 'request-live' }],
    })
    current.page = { hasMore: false }
    const latest = traceBundle()
    latest.page = { hasMore: true, beforeCursor: 'latest-cursor' }

    const merged = mergeLatestDiagnosticTrace(current, latest)

    expect(merged.tasks.map((task) => task.id)).toEqual(['run-1', 'run-live'])
    expect(merged.page).toEqual({ hasMore: true, beforeCursor: 'latest-cursor' })
  })

  test('does not let an older full-page response overwrite a newer run refresh', () => {
    const current = traceBundle()
    current.generatedAt = '2026-08-14T12:00:03Z'
    current.tasks[0] = { ...current.tasks[0]!, status: 'completed', totalTokens: 42 }
    const stalePage = traceBundle()
    stalePage.generatedAt = '2026-08-14T12:00:02Z'
    stalePage.tasks[0] = { ...stalePage.tasks[0]!, status: 'running', totalTokens: 12 }

    const merged = mergeLatestDiagnosticTrace(current, stalePage)

    expect(merged.tasks[0]).toMatchObject({ status: 'completed', totalTokens: 42 })
    expect(merged.generatedAt).toBe('2026-08-14T12:00:03Z')
  })

  test('replaces only the refreshed run and keeps the history cursor', () => {
    const current = traceBundle()
    current.page = { hasMore: true, beforeCursor: 'history-cursor' }
    const run = traceBundle()
    run.tasks[0] = { ...run.tasks[0]!, status: 'completed', totalTokens: 42 }
    run.page = { hasMore: false }

    const merged = mergeDiagnosticTraceRun(current, run)

    expect(merged.tasks).toHaveLength(1)
    expect(merged.tasks[0]).toMatchObject({ status: 'completed', totalTokens: 42 })
    expect(merged.page).toEqual({ hasMore: true, beforeCursor: 'history-cursor' })
  })
})

describe('live diagnostic trace projection', () => {
  test('overlays streaming reasoning, response text, and tools onto the current request', () => {
    const bundle = traceBundle()
    const liveItems: Item[] = [
      { kind: 'user', id: 'user-1', text: 'Inspect the trace', images: [] },
      { kind: 'run', id: 'run-item-1', runId: 'run-1', startedAt: '2026-08-14T12:00:00Z' },
      { kind: 'thinking', id: 'thinking-1', text: 'Reading the source', streaming: true },
      { kind: 'assistant', id: 'assistant-1', markdown: 'The trace is updating', open: true, complete: false },
      { kind: 'tool', id: 'call-1', name: 'read', args: { path: 'trace.go' }, status: 'running' },
    ]

    const merged = mergeLiveTraceBundle(bundle, 'session-1', liveItems, true)
    const request = merged?.tasks[0]?.requests[0]

    expect(merged?.selectedTaskId).toBe('run-1')
    expect(request?.lifecycle).toBe('in-progress')
    expect(request?.output?.message.content).toEqual([
      { type: 'thinking', thinking: 'Reading the source' },
      { type: 'text', text: 'The trace is updating' },
    ])
    expect(request?.tools[0]).toMatchObject({
      id: 'call-1',
      name: 'read',
      status: 'running',
      lifecycle: 'in-progress',
      arguments: { path: 'trace.go' },
    })
  })

  test('does not invalidate the trace bundle for every streamed text delta', () => {
    const before: Item[] = [
      { kind: 'run', id: 'run-1', runId: 'run-1', startedAt: '2026-08-14T12:00:00Z' },
      { kind: 'thinking', id: 'thinking-1', text: 'a', streaming: true },
      { kind: 'assistant', id: 'assistant-1', markdown: 'a', open: true, complete: false },
    ]
    const after: Item[] = [
      before[0]!,
      { ...(before[1] as Extract<Item, { kind: 'thinking' }>), text: 'a longer thought' },
      { ...(before[2] as Extract<Item, { kind: 'assistant' }>), markdown: 'a longer response' },
    ]

    expect(liveTraceRefreshKey(before, true)).toBe(liveTraceRefreshKey(after, true))
  })

  test('creates a provisional task before the first trace bundle is readable', () => {
    const merged = mergeLiveTraceBundle(undefined, 'session-1', [
      { kind: 'user', id: 'user-1', text: 'Start now', images: [] },
      { kind: 'run', id: 'run-item-1', runId: 'run-live', startedAt: '2026-08-14T12:00:00Z' },
      { kind: 'thinking', id: 'thinking-1', text: 'Starting', streaming: true },
    ], true)

    expect(merged?.tasks[0]).toMatchObject({
      id: 'run-live',
      prompt: 'Start now',
      status: 'running',
    })
    expect(merged?.tasks[0]?.requests[0]?.output?.message.content).toEqual([
      { type: 'thinking', thinking: 'Starting' },
    ])
  })

  test('does not overwrite a historical task when a different live run shares its timestamp', () => {
    const bundle = traceBundle()
    bundle.tasks[0]!.requests[0]!.output = {
      capturedAt: '2026-08-14T12:00:01Z',
      message: { role: 'assistant', content: [{ type: 'text', text: 'Historical output' }] },
    }
    const merged = mergeLiveTraceBundle(bundle, 'session-1', [
      { kind: 'run', id: 'run-item-2', runId: 'run-2', startedAt: '2026-08-14T12:00:00Z' },
      { kind: 'assistant', id: 'assistant-2', markdown: 'Live output', open: true, complete: false },
    ], true)

    expect(merged?.tasks).toHaveLength(2)
    expect(merged?.tasks[0]?.requests[0]?.output?.message.content[0]?.text).toBe('Historical output')
    expect(merged?.tasks[1]?.requests[0]?.output?.message.content[0]?.text).toBe('Live output')
  })

  test('maps live output to its exact provider request instead of its list position', () => {
    const bundle = traceBundle()
    bundle.tasks[0]!.requests[0]!.output = {
      capturedAt: '2026-08-14T12:00:01Z',
      message: {
        role: 'assistant',
        providerRequestId: 'request-1',
        content: [{ type: 'text', text: 'First request output' }],
      },
    }
    bundle.tasks[0]!.requests.push({
      id: 'request-2',
      number: 2,
      status: 'running',
      lifecycle: 'in-progress',
      startedAt: '2026-08-14T12:00:02Z',
      attempts: [],
      checkpoints: [],
      tools: [],
      snapshotState: 'available',
      rawEvents: [],
    })

    const merged = mergeLiveTraceBundle(bundle, 'session-1', [
      { kind: 'run', id: 'run-item-1', runId: 'run-1', startedAt: '2026-08-14T12:00:00Z' },
      {
        kind: 'thinking',
        id: 'thinking-2',
        providerRequestId: 'request-2',
        text: 'Second request reasoning',
        streaming: true,
      },
    ], true)

    expect(merged?.tasks[0]?.requests[0]?.output?.message.content[0]?.text).toBe(
      'First request output',
    )
    expect(merged?.tasks[0]?.requests[0]).toMatchObject({
      status: 'completed',
      lifecycle: 'complete',
    })
    expect(merged?.tasks[0]?.requests[1]?.output?.message.content).toEqual([
      { type: 'thinking', thinking: 'Second request reasoning' },
    ])
  })

  test('keeps the final trace bundle authoritative after a run completes', () => {
    const bundle = traceBundle()
    bundle.tasks[0]!.requests[0]!.output = {
      capturedAt: '2026-08-14T12:00:01Z',
      message: { role: 'assistant', content: [{ type: 'text', text: 'Final snapshot' }] },
    }
    const merged = mergeLiveTraceBundle(bundle, 'session-1', [
      { kind: 'run', id: 'run-item-1', runId: 'run-1', startedAt: '2026-08-14T12:00:00Z', durationMs: 1000 },
      { kind: 'assistant', id: 'assistant-1', markdown: 'Replayed transcript', open: false, complete: true },
    ], false)

    expect(merged).toBe(bundle)
    expect(merged?.tasks[0]?.requests[0]?.output?.message.content[0]?.text).toBe('Final snapshot')
  })
})

function traceBundle(): TraceBundle {
  return {
    version: 1,
    generatedAt: '2026-08-14T12:00:00Z',
    sessionId: 'session-1',
    selectedTaskId: 'run-1',
    page: { hasMore: false },
    tasks: [{
      id: 'run-1',
      status: 'running',
      prompt: 'Inspect the trace',
      startedAt: '2026-08-14T12:00:00Z',
      updatedAt: '2026-08-14T12:00:00Z',
      retries: 0,
      contextRecoveries: 0,
      rawEvents: [],
      requests: [{
        id: 'request-1',
        number: 1,
        status: 'running',
        lifecycle: 'in-progress',
        startedAt: '2026-08-14T12:00:00Z',
        attempts: [],
        checkpoints: [],
        tools: [],
        snapshotState: 'available',
        rawEvents: [],
      }],
    }],
  }
}
