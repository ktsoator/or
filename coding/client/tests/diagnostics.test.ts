import { describe, expect, test } from 'bun:test'
import { fetchDiagnosticTrace } from '../src/features/diagnostics/catalog'

describe('diagnostic trace catalog', () => {
  test('loads one assembled conversation bundle scoped to session and run', async () => {
    let requestedURL = ''
    let requestInit: RequestInit | undefined
    const bundle = await fetchDiagnosticTrace(
      'session/one',
      'run one',
      undefined,
      async (url, init) => {
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
    )

    expect(requestedURL).toBe('/api/diagnostics/trace?sessionId=session%2Fone&runId=run+one')
    expect(requestInit?.cache).toBe('no-store')
    expect(bundle.selectedTaskId).toBe('run one')
    expect(bundle.tasks[0]?.id).toBe('run one')
  })

  test('loads the conversation with its latest task selected when no run is specified', async () => {
    let requestedURL = ''
    await fetchDiagnosticTrace('session', undefined, undefined, async (url) => {
      requestedURL = String(url)
      return Response.json({ selectedTaskId: 'latest', tasks: [] })
    })
    expect(requestedURL).toBe('/api/diagnostics/trace?sessionId=session')
  })

  test('normalizes incomplete interrupted bundles', async () => {
    const bundle = await fetchDiagnosticTrace('session', undefined, undefined, async () => Response.json({
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
    }))

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
  })

  test('rejects failed bundle responses', async () => {
    expect(
      fetchDiagnosticTrace('session', undefined, undefined, async () => new Response('', { status: 503 })),
    ).rejects.toThrow('HTTP 503')
  })
})
