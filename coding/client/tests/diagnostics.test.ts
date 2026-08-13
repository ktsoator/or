import { describe, expect, test } from 'bun:test'
import { fetchDiagnosticRuns } from '../src/features/diagnostics/catalog'
import { buildDiagnosticTurns } from '../src/features/diagnostics/viewModel'

describe('diagnostic catalog', () => {
  test('scopes the report to the selected session', async () => {
    let requestedURL = ''
    let requestInit: RequestInit | undefined
    const report = await fetchDiagnosticRuns(
      'session/one',
      undefined,
      async (url, init) => {
        requestedURL = String(url)
        requestInit = init
        return Response.json({ runs: [], generatedAt: '2026-08-13T12:00:00Z' })
      },
    )

    expect(requestedURL).toBe('/api/diagnostics/runs?limit=50&sessionId=session%2Fone')
    expect(requestInit?.cache).toBe('no-store')
    expect(report.runs).toEqual([])
  })

  test('rejects failed responses', async () => {
    expect(
      fetchDiagnosticRuns(undefined, undefined, async () => new Response('', { status: 503 })),
    ).rejects.toThrow('HTTP 503')
  })
})

describe('diagnostic view model', () => {
  test('groups lifecycle events into readable turn steps', () => {
    const turns = buildDiagnosticTurns([
      { name: 'turn.started', timestamp: '2026-08-13T12:00:00.000Z', turnId: 'turn-1', status: 'running' },
      { name: 'provider.request.started', timestamp: '2026-08-13T12:00:00.010Z', turnId: 'turn-1', providerRequestId: 'request-1', provider: 'openai', model: 'gpt-5' },
      { name: 'provider.http_attempt.started', timestamp: '2026-08-13T12:00:00.020Z', turnId: 'turn-1', providerRequestId: 'request-1', attempt: 1 },
      { name: 'provider.http_attempt.response', timestamp: '2026-08-13T12:00:00.300Z', turnId: 'turn-1', providerRequestId: 'request-1', attempt: 1, httpStatus: 429 },
      { name: 'provider.http_attempt.started', timestamp: '2026-08-13T12:00:00.400Z', turnId: 'turn-1', providerRequestId: 'request-1', attempt: 2 },
      { name: 'provider.request.completed', timestamp: '2026-08-13T12:00:04.000Z', turnId: 'turn-1', providerRequestId: 'request-1', provider: 'openai', model: 'gpt-5', status: 'completed', durationMs: 3990, timeToFirstOutputMs: 2200, totalTokens: 800 },
      { name: 'tool.call.started', timestamp: '2026-08-13T12:00:04.010Z', turnId: 'turn-1', providerRequestId: 'request-1', toolCallId: 'call-1', toolName: 'write', status: 'running' },
      { name: 'tool.approval.started', timestamp: '2026-08-13T12:00:04.020Z', turnId: 'turn-1', providerRequestId: 'request-1', toolCallId: 'call-1', toolName: 'write', status: 'waiting' },
      { name: 'tool.approval.completed', timestamp: '2026-08-13T12:00:05.000Z', turnId: 'turn-1', providerRequestId: 'request-1', toolCallId: 'call-1', toolName: 'write', status: 'allowed', durationMs: 980 },
      { name: 'tool.call.completed', timestamp: '2026-08-13T12:00:05.100Z', turnId: 'turn-1', providerRequestId: 'request-1', toolCallId: 'call-1', toolName: 'write', status: 'success', durationMs: 1090 },
      { name: 'turn.completed', timestamp: '2026-08-13T12:00:05.110Z', turnId: 'turn-1', providerRequestId: 'request-1', status: 'completed', durationMs: 5110 },
    ])

    expect(turns).toHaveLength(1)
    expect(turns[0]).toMatchObject({ id: 'turn-1', status: 'completed', durationMs: 5110 })
    expect(turns[0]?.steps).toEqual([
      expect.objectContaining({ kind: 'provider', durationMs: 3990, timeToFirstOutputMs: 2200, attempts: 2, totalTokens: 800 }),
      expect.objectContaining({ kind: 'tool', toolName: 'write', durationMs: 1090, approvalDurationMs: 980, status: 'success' }),
    ])
  })

  test('keeps in-progress operations visible', () => {
    const turns = buildDiagnosticTurns([
      { name: 'turn.started', timestamp: '2026-08-13T12:00:00.000Z', turnId: 'turn-1', status: 'running' },
      { name: 'provider.request.started', timestamp: '2026-08-13T12:00:00.010Z', turnId: 'turn-1', providerRequestId: 'request-1', provider: 'openai', model: 'gpt-5', status: 'running' },
    ])

    expect(turns[0]).toMatchObject({ status: 'running' })
    expect(turns[0]?.steps[0]).toMatchObject({ kind: 'provider', status: 'running' })
  })
})
