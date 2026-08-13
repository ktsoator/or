import { apiURL } from '@/api'

export type DiagnosticEvent = {
  name: string
  timestamp: string
  turnId?: string
  providerRequestId?: string
  toolCallId?: string
  toolName?: string
  status?: string
  errorCode?: string
  reason?: string
  durationMs?: number
  timeToFirstOutputMs?: number
  provider?: string
  model?: string
  attempt?: number
  httpStatus?: number
  totalTokens?: number
  costTotalUsd?: number
}

export type DiagnosticRun = {
  id: string
  sessionId: string
  status: string
  errorCode?: string
  startedAt: string
  updatedAt: string
  durationMs?: number
  timeToFirstOutputMs?: number
  checkpointDurationMs?: number
  toolDurationMs?: number
  approvalDurationMs?: number
  providerRequests: number
  toolCalls: number
  approvalRequests: number
  retries: number
  contextRecoveries: number
  inputTokens?: number
  outputTokens?: number
  cacheReadTokens?: number
  cacheWriteTokens?: number
  totalTokens?: number
  costTotalUsd?: number
  events: DiagnosticEvent[]
  omittedEvents?: number
}

export type DiagnosticReport = {
  runs: DiagnosticRun[]
  generatedAt: string
}

type DiagnosticRequest = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>

export async function fetchDiagnosticRuns(
  sessionID?: string,
  signal?: AbortSignal,
  request: DiagnosticRequest = fetch,
): Promise<DiagnosticReport> {
  const query = new URLSearchParams({ limit: '50' })
  if (sessionID) query.set('sessionId', sessionID)
  const response = await request(apiURL(`/diagnostics/runs?${query}`), {
    cache: 'no-store',
    signal,
  })
  if (!response.ok) throw new Error(`HTTP ${response.status}`)
  return response.json() as Promise<DiagnosticReport>
}
