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

export type TraceBundleAttempt = {
	number: number
	status?: string
	lifecycle: 'complete' | 'in-progress' | 'missing-start'
	startedAt: string
	completedAt?: string
	durationMs?: number
	httpStatus?: number
	errorCode?: string
	rawEvents: DiagnosticEvent[]
}

export type TraceBundleCheckpoint = {
	status?: string
	startedAt: string
	completedAt: string
	durationMs?: number
	errorCode?: string
}

export type TraceBundleTool = {
	id: string
	name?: string
	status?: string
	errorCode?: string
	lifecycle: 'complete' | 'in-progress' | 'missing-start'
	startedAt: string
	completedAt?: string
	durationMs?: number
	approvalDurationMs?: number
	executionDurationMs?: number
	arguments?: Record<string, unknown>
	result?: RequestSnapshotMessage
	rawEvents: DiagnosticEvent[]
}

export type TraceBundleRequest = {
	id: string
	number: number
	turnId?: string
	status?: string
	errorCode?: string
	lifecycle: 'complete' | 'in-progress' | 'missing-start'
	startedAt: string
	completedAt?: string
	durationMs?: number
	timeToFirstOutputMs?: number
	checkpointDurationMs?: number
	provider?: string
	model?: string
	inputTokens?: number
	inputUnknown?: boolean
	outputTokens?: number
	cacheReadTokens?: number
	cacheWriteTokens?: number
	totalTokens?: number
	costTotalUsd?: number
	attempts: TraceBundleAttempt[]
	checkpoints: TraceBundleCheckpoint[]
	tools: TraceBundleTool[]
	snapshotState: 'available' | 'missing' | 'error'
	capturedAt?: string
	input?: RequestSnapshot['input']
	output?: RequestSnapshot['output']
	attachments?: RequestSnapshotAttachment[]
	rawEvents: DiagnosticEvent[]
}

export type TraceBundleTask = {
	id: string
	status: string
	errorCode?: string
	prompt?: string
	startedAt: string
	updatedAt: string
	durationMs?: number
	timeToFirstOutputMs?: number
	checkpointDurationMs?: number
	toolDurationMs?: number
	approvalDurationMs?: number
	retries: number
	contextRecoveries: number
	inputTokens?: number
	outputTokens?: number
	cacheReadTokens?: number
	cacheWriteTokens?: number
	totalTokens?: number
	costTotalUsd?: number
	requests: TraceBundleRequest[]
	rawEvents: DiagnosticEvent[]
	omittedEvents?: number
}

export type TraceBundle = {
	version: number
	generatedAt: string
	sessionId: string
	selectedTaskId: string
	tasks: TraceBundleTask[]
}

type DiagnosticRequest = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>

export async function fetchDiagnosticTrace(
	sessionID: string,
	runID?: string,
	signal?: AbortSignal,
	request: DiagnosticRequest = fetch,
): Promise<TraceBundle> {
	const query = new URLSearchParams({ sessionId: sessionID })
	if (runID) query.set('runId', runID)
	const response = await request(apiURL(`/diagnostics/trace?${query}`), {
		cache: 'no-store',
		signal,
	})
	if (!response.ok) throw new Error(`HTTP ${response.status}`)
	return response.json() as Promise<TraceBundle>
}
