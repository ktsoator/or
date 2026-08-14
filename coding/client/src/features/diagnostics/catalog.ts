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
  inputTokens?: number
  inputUnknown?: boolean
  outputTokens?: number
  cacheReadTokens?: number
  cacheWriteTokens?: number
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

export type RequestSnapshotAttachment = {
	id: string
	kind: string
	placement: string
	path?: string
	revision?: string
	messageIndex: number
}

export type RequestSnapshotImage = {
	mimeType: string
	encodedBytes?: number
}

export type RequestSnapshotContent = {
	type: string
	text?: string
	thinking?: string
	redacted?: boolean
	image?: RequestSnapshotImage
	toolCallId?: string
	toolName?: string
	arguments?: Record<string, unknown>
}

export type RequestSnapshotMessage = {
	role: string
	content: RequestSnapshotContent[]
	providerRequestId?: string
	toolCallId?: string
	toolName?: string
	isError?: boolean
}

export type RequestSnapshotTool = {
	name: string
	description: string
	parameters: unknown
	strict?: boolean
}

export type RequestSnapshot = {
	version: number
	capturedAt: string
	sessionId: string
	runId: string
	turnId: string
	providerRequestId: string
	provider: string
	model: string
	input: {
		systemPrompt?: string
		messages: RequestSnapshotMessage[]
		tools?: RequestSnapshotTool[]
	}
	output?: {
		capturedAt: string
		message: RequestSnapshotMessage
		stopReason?: string
		errorMessage?: string
	}
	attachments?: RequestSnapshotAttachment[]
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

export async function fetchDiagnosticRequest(
	providerRequestID: string,
	sessionID: string,
	runID: string,
	signal?: AbortSignal,
	request: DiagnosticRequest = fetch,
): Promise<RequestSnapshot | undefined> {
	const query = new URLSearchParams({ sessionId: sessionID, runId: runID })
	const response = await request(
		apiURL(`/diagnostics/requests/${encodeURIComponent(providerRequestID)}?${query}`),
		{ cache: 'no-store', signal },
	)
	if (response.status === 404) return undefined
	if (!response.ok) throw new Error(`HTTP ${response.status}`)
	return response.json() as Promise<RequestSnapshot>
}
