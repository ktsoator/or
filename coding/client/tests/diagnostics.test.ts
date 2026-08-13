import { describe, expect, test } from 'bun:test'
import { fetchDiagnosticRuns } from '../src/features/diagnostics/catalog'

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
