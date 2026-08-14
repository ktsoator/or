import { describe, expect, test } from 'bun:test'
import {
  fetchDiagnosticRequest,
  fetchDiagnosticRuns,
  type DiagnosticEvent,
  type DiagnosticRun,
  type RequestSnapshot,
} from '../src/features/diagnostics/catalog'
import {
  buildTraceRequestCatalog,
  buildTraceRequestView,
  buildTraceRun,
  findTraceProviderRequest,
} from '../src/features/diagnostics/viewModel'

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

	test('loads one request snapshot with run correlation', async () => {
		let requestedURL = ''
		const snapshot = await fetchDiagnosticRequest(
			'request/one',
			'session one',
			'run one',
			undefined,
			async (url) => {
				requestedURL = String(url)
				return Response.json({ providerRequestId: 'request/one', input: { messages: [] } })
			},
		)
		expect(requestedURL).toBe('/api/diagnostics/requests/request%2Fone?sessionId=session+one&runId=run+one')
		expect(snapshot?.providerRequestId).toBe('request/one')
	})

	test('returns undefined when a historical request has no snapshot', async () => {
		const snapshot = await fetchDiagnosticRequest(
			'request-old', 'session', 'run', undefined,
			async () => new Response('', { status: 404 }),
		)
		expect(snapshot).toBeUndefined()
	})
})

describe('diagnostic view model', () => {
  test('projects lifecycle events into correlated operations', () => {
    const trace = buildTraceRun(diagnosticRun([
      { name: 'turn.started', timestamp: '2026-08-13T12:00:00.000Z', turnId: 'turn-1', status: 'running' },
      { name: 'provider.request.started', timestamp: '2026-08-13T12:00:00.010Z', turnId: 'turn-1', providerRequestId: 'request-1', provider: 'openai', model: 'gpt-5' },
      { name: 'provider.http_attempt.started', timestamp: '2026-08-13T12:00:00.020Z', turnId: 'turn-1', providerRequestId: 'request-1', attempt: 1 },
      { name: 'provider.http_attempt.response', timestamp: '2026-08-13T12:00:00.300Z', turnId: 'turn-1', providerRequestId: 'request-1', attempt: 1, httpStatus: 429 },
      { name: 'provider.http_attempt.started', timestamp: '2026-08-13T12:00:00.400Z', turnId: 'turn-1', providerRequestId: 'request-1', attempt: 2 },
      { name: 'provider.request.completed', timestamp: '2026-08-13T12:00:04.000Z', turnId: 'turn-1', providerRequestId: 'request-1', provider: 'openai', model: 'gpt-5', status: 'completed', durationMs: 3990, timeToFirstOutputMs: 2200, inputTokens: 620, outputTokens: 150, cacheReadTokens: 30, totalTokens: 800 },
      { name: 'tool.call.started', timestamp: '2026-08-13T12:00:04.010Z', turnId: 'turn-1', providerRequestId: 'request-1', toolCallId: 'call-1', toolName: 'write', status: 'running' },
      { name: 'tool.approval.started', timestamp: '2026-08-13T12:00:04.020Z', turnId: 'turn-1', providerRequestId: 'request-1', toolCallId: 'call-1', toolName: 'write', status: 'waiting' },
      { name: 'tool.approval.completed', timestamp: '2026-08-13T12:00:05.000Z', turnId: 'turn-1', providerRequestId: 'request-1', toolCallId: 'call-1', toolName: 'write', status: 'allowed', durationMs: 980 },
      { name: 'tool.call.completed', timestamp: '2026-08-13T12:00:05.100Z', turnId: 'turn-1', providerRequestId: 'request-1', toolCallId: 'call-1', toolName: 'write', status: 'success', durationMs: 1090 },
      { name: 'turn.completed', timestamp: '2026-08-13T12:00:05.110Z', turnId: 'turn-1', providerRequestId: 'request-1', status: 'completed', durationMs: 5110 },
    ]))

    expect(trace.turns).toHaveLength(1)
    expect(trace.turns[0]).toMatchObject({ id: 'turn-1', status: 'completed', durationMs: 5110 })
    expect(trace.providerRequests[0]).toMatchObject({
      id: 'provider:request-1',
      parentId: 'turn:turn-1',
      durationMs: 3990,
      timeToFirstOutputMs: 2200,
      inputTokens: 620,
      outputTokens: 150,
      cacheReadTokens: 30,
      totalTokens: 800,
      lifecycle: 'complete',
    })
    expect(trace.providerRequests[0]?.attempts).toEqual([
      expect.objectContaining({ id: 'attempt:request-1:1', attempt: 1, httpStatus: 429 }),
      expect.objectContaining({ id: 'attempt:request-1:2', attempt: 2, lifecycle: 'in-progress' }),
    ])
    const tool = trace.operations.find((operation) => operation.kind === 'tool')
    expect(tool).toMatchObject({
      id: 'tool:call-1',
      parentId: 'provider:request-1',
      toolName: 'write',
      durationMs: 1090,
      approvalDurationMs: 980,
      executionDurationMs: 110,
      status: 'success',
    })
    expect(tool?.kind === 'tool' ? tool.approvals : []).toEqual([
      expect.objectContaining({ id: 'approval:call-1:1', parentId: 'tool:call-1', status: 'allowed' }),
    ])
  })

  test('keeps in-progress operations visible', () => {
    const trace = buildTraceRun(diagnosticRun([
      { name: 'turn.started', timestamp: '2026-08-13T12:00:00.000Z', turnId: 'turn-1', status: 'running' },
      { name: 'provider.request.started', timestamp: '2026-08-13T12:00:00.010Z', turnId: 'turn-1', providerRequestId: 'request-1', provider: 'openai', model: 'gpt-5', status: 'running' },
    ]))

    expect(trace.turns[0]).toMatchObject({ status: 'running', lifecycle: 'in-progress' })
    expect(trace.providerRequests[0]).toMatchObject({ status: 'running', lifecycle: 'in-progress' })
  })

  test('builds stable input, model, and tool timeline spans', () => {
    const trace = buildTraceRun(diagnosticRun([
      { name: 'checkpoint.completed', timestamp: '2026-08-13T12:00:00.100Z', turnId: 'turn-1', providerRequestId: 'request-1', status: 'completed', durationMs: 100 },
      { name: 'provider.request.started', timestamp: '2026-08-13T12:00:00.100Z', turnId: 'turn-1', providerRequestId: 'request-1', provider: 'openai', model: 'gpt-5', status: 'running' },
      { name: 'provider.request.completed', timestamp: '2026-08-13T12:00:02.100Z', turnId: 'turn-1', providerRequestId: 'request-1', provider: 'openai', model: 'gpt-5', status: 'completed', durationMs: 2000 },
      { name: 'tool.call.started', timestamp: '2026-08-13T12:00:02.200Z', turnId: 'turn-1', providerRequestId: 'request-1', toolCallId: 'call-1', toolName: 'exec_command', status: 'running' },
      { name: 'tool.call.completed', timestamp: '2026-08-13T12:00:02.700Z', turnId: 'turn-1', providerRequestId: 'request-1', toolCallId: 'call-1', toolName: 'exec_command', status: 'success', durationMs: 500 },
    ]))

    expect(trace.timeline.map((span) => span.kind)).toEqual(['input', 'model', 'tool'])
    expect(trace.timeline[0]).toMatchObject({
      id: 'span:checkpoint:request-1:1',
      operationId: 'checkpoint:request-1:1',
      startedAt: '2026-08-13T12:00:00.000Z',
      durationMs: 100,
    })
    expect(trace.timeline[2]).toMatchObject({
      id: 'span:tool:call-1',
      label: 'exec_command',
      durationMs: 500,
    })
    expect(trace.turns[0]?.operations.some((operation) => operation.kind === 'checkpoint')).toBe(true)
  })

  test('retains a terminal operation when earlier events were truncated', () => {
    const trace = buildTraceRun(diagnosticRun([
      { name: 'provider.request.completed', timestamp: '2026-08-13T12:00:04.000Z', turnId: 'turn-1', providerRequestId: 'request-1', status: 'completed', durationMs: 2000 },
    ]))

    expect(trace.providerRequests[0]).toMatchObject({
      id: 'provider:request-1',
      lifecycle: 'missing-start',
      startedAt: '2026-08-13T12:00:02.000Z',
      completedAt: '2026-08-13T12:00:04.000Z',
    })
    expect(trace.turns[0]).toMatchObject({ id: 'turn-1', lifecycle: 'missing-start' })
  })

  test('projects request input into stable content records', () => {
    const trace = buildTraceRun(diagnosticRun([
      { name: 'provider.request.started', timestamp: '2026-08-13T12:00:00.000Z', turnId: 'turn-1', providerRequestId: 'request-1', provider: 'openai', model: 'gpt-5' },
    ]))
    const requestView = buildTraceRequestView(trace.providerRequests[0]!, requestSnapshot())

    expect(requestView.trajectoryItems.map((item) => [item.id, item.kind])).toEqual([
      ['request-1:system', 'system'],
      ['request-1:message:0', 'user'],
      ['request-1:attachment:skill-1', 'skill'],
      ['request-1:message:2:content:0', 'assistant'],
      ['request-1:message:2:content:1', 'tool'],
      ['request-1:message:output:content:0', 'assistant'],
    ])
    expect(requestView.trajectoryItems[1]).toMatchObject({ turn: 1, preview: 'Fix the trace' })
    expect(requestView.trajectoryItems[2]).toMatchObject({
      attachmentKind: 'activated_skill',
      source: '/skills/trace/SKILL.md',
    })
    expect(requestView.trajectoryItems[3]).toMatchObject({
      preview: '',
      thinkingPreview: 'Read the trace before changing it',
      providerRequestId: 'request-previous',
      toolCalls: [{
        toolCallId: 'call-1',
        toolName: 'read_file',
        arguments: { path: 'trace.ts' },
      }],
    })
    expect(requestView.trajectoryItems[4]).toMatchObject({
      toolCallId: 'call-1',
      resultPreview: 'Loaded trace.ts',
      providerRequestId: 'request-previous',
    })
    expect(requestView.trajectoryItems[5]).toMatchObject({
      kind: 'assistant',
      preview: 'Done',
      thinkingPreview: 'Plan the trace update',
      providerRequestId: 'request-1',
    })
    expect(requestView.trajectoryItems.some((item) => item.kind === 'thinking')).toBe(false)
    expect(requestView.toolItems[0]).toMatchObject({
      id: 'request-1:tool-definition:0',
      kind: 'toolSchema',
      toolName: 'read_file',
    })
  })

  test('resolves historical assistant metrics and numbers requests across session runs', () => {
    const previousRun = diagnosticRun([
      { name: 'provider.request.started', timestamp: '2026-08-13T11:00:00.000Z', turnId: 'turn-previous', providerRequestId: 'request-previous' },
      { name: 'provider.request.completed', timestamp: '2026-08-13T11:00:01.000Z', turnId: 'turn-previous', providerRequestId: 'request-previous', durationMs: 1000, totalTokens: 321 },
    ], 'run-previous')
    const currentRun = diagnosticRun([
      { name: 'provider.request.started', timestamp: '2026-08-13T12:00:00.000Z', turnId: 'turn-1', providerRequestId: 'request-1' },
      { name: 'provider.request.completed', timestamp: '2026-08-13T12:00:02.000Z', turnId: 'turn-1', providerRequestId: 'request-1', durationMs: 2000, totalTokens: 654 },
    ], 'run-1')
    const otherSessionRun = diagnosticRun([
      { name: 'provider.request.started', timestamp: '2026-08-13T13:00:00.000Z', turnId: 'turn-other', providerRequestId: 'request-other' },
      { name: 'provider.request.completed', timestamp: '2026-08-13T13:00:01.000Z', turnId: 'turn-other', providerRequestId: 'request-other', durationMs: 1000 },
    ], 'run-other')
    otherSessionRun.sessionId = 'session-2'
    const catalog = buildTraceRequestCatalog([currentRun, otherSessionRun, previousRun])
    const requestView = buildTraceRequestView(
      buildTraceRun(currentRun).providerRequests[0]!,
      requestSnapshot(),
    )
    const historicalAssistant = requestView.trajectoryItems.find(
      (item) => item.providerRequestId === 'request-previous',
    )
    const currentAssistant = requestView.trajectoryItems.find(
      (item) => item.providerRequestId === 'request-1',
    )

    expect(findTraceProviderRequest(catalog, historicalAssistant?.providerRequestId)).toMatchObject({
      runId: 'run-previous',
      requestNumber: 1,
      request: { totalTokens: 321 },
    })
    expect(findTraceProviderRequest(catalog, currentAssistant?.providerRequestId)).toMatchObject({
      runId: 'run-1',
      requestNumber: 2,
      request: { totalTokens: 654 },
    })
    expect(findTraceProviderRequest(catalog, 'request-other')).toMatchObject({
      runId: 'run-other',
      requestNumber: 1,
    })
  })
})

function diagnosticRun(events: DiagnosticEvent[], id = 'run-1'): DiagnosticRun {
  return {
    id,
    sessionId: 'session-1',
    status: 'running',
    startedAt: '2026-08-13T12:00:00.000Z',
    updatedAt: events.at(-1)?.timestamp ?? '2026-08-13T12:00:00.000Z',
    providerRequests: 0,
    toolCalls: 0,
    approvalRequests: 0,
    retries: 0,
    contextRecoveries: 0,
    events,
  }
}

function requestSnapshot(): RequestSnapshot {
  return {
    version: 1,
    capturedAt: '2026-08-13T12:00:00.000Z',
    sessionId: 'session-1',
    runId: 'run-1',
    turnId: 'turn-1',
    providerRequestId: 'request-1',
    provider: 'openai',
    model: 'gpt-5',
    input: {
      systemPrompt: 'You are a coding agent.',
      messages: [
        { role: 'user', content: [{ type: 'text', text: 'Fix the trace' }] },
        { role: 'user', content: [{ type: 'text', text: 'Use the trace skill' }] },
        {
          role: 'assistant',
          providerRequestId: 'request-previous',
          content: [
            { type: 'thinking', thinking: 'Read the trace before changing it' },
            { type: 'toolCall', toolCallId: 'call-1', toolName: 'read_file', arguments: { path: 'trace.ts' } },
          ],
        },
        { role: 'toolResult', toolCallId: 'call-1', toolName: 'read_file', content: [{ type: 'text', text: 'Loaded trace.ts' }] },
      ],
      tools: [{ name: 'read_file', description: 'Read one file', parameters: { type: 'object' } }],
    },
    attachments: [{
      id: 'skill-1',
      kind: 'activated_skill',
      placement: 'message',
      path: '/skills/trace/SKILL.md',
      messageIndex: 1,
    }],
    output: {
      capturedAt: '2026-08-13T12:00:02.000Z',
      stopReason: 'stop',
      message: {
        role: 'assistant',
        content: [
          { type: 'thinking', thinking: 'Plan the trace update' },
          { type: 'text', text: 'Done' },
        ],
      },
    },
  }
}
