import { apiURL } from '@/api'

export type DiagnosticEvent = {
  name: string
  timestamp: string
  turnId?: string
  stepId?: string
  providerRequestId?: string
  attemptId?: string
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
	stepId?: string
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
	id: string
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
	stepId?: string
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

export type TraceBundlePage = {
	hasMore: boolean
	beforeCursor?: string
}

export type TraceBundle = {
	version: number
	generatedAt: string
	sessionId: string
	selectedTaskId: string
	tasks: TraceBundleTask[]
	page: TraceBundlePage
}

type DiagnosticRequest = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>

export type DiagnosticTraceOptions = {
	runID?: string
	beforeCursor?: string
	limit?: number
	signal?: AbortSignal
	request?: DiagnosticRequest
}

export class DiagnosticTraceError extends Error {
	readonly status: number

	constructor(status: number) {
		super(`HTTP ${status}`)
		this.name = 'DiagnosticTraceError'
		this.status = status
	}
}

function normalizeMessage(message: RequestSnapshotMessage): RequestSnapshotMessage {
	return { ...message, content: message.content ?? [] }
}

function normalizeTraceBundle(bundle: TraceBundle): TraceBundle {
	return {
		...bundle,
		page: {
			hasMore: bundle.page?.hasMore ?? false,
			...(bundle.page?.beforeCursor ? { beforeCursor: bundle.page.beforeCursor } : {}),
		},
		tasks: (bundle.tasks ?? []).map((task) => ({
			...task,
			rawEvents: task.rawEvents ?? [],
			requests: (task.requests ?? []).map((traceRequest) => ({
				...traceRequest,
				rawEvents: traceRequest.rawEvents ?? [],
				attempts: (traceRequest.attempts ?? []).map((attempt) => ({
					...attempt,
					rawEvents: attempt.rawEvents ?? [],
				})),
				checkpoints: traceRequest.checkpoints ?? [],
				tools: (traceRequest.tools ?? []).map((tool) => ({
					...tool,
					rawEvents: tool.rawEvents ?? [],
					result: tool.result ? normalizeMessage(tool.result) : undefined,
				})),
				attachments: traceRequest.attachments ?? [],
				input: traceRequest.input
					? {
						...traceRequest.input,
						messages: (traceRequest.input.messages ?? []).map(normalizeMessage),
						tools: traceRequest.input.tools ?? [],
					}
					: undefined,
				output: traceRequest.output
					? {
						...traceRequest.output,
						message: normalizeMessage(traceRequest.output.message),
					}
					: undefined,
			})),
		})),
	}
}

export async function fetchDiagnosticTrace(
	sessionID: string,
	options: DiagnosticTraceOptions = {},
): Promise<TraceBundle> {
	const { runID, beforeCursor, limit, signal, request = fetch } = options
	if (runID && beforeCursor) {
		throw new Error('runID and beforeCursor are mutually exclusive')
	}
	const query = new URLSearchParams({ sessionId: sessionID })
	if (runID) query.set('runId', runID)
	else {
		query.set('limit', String(limit ?? 12))
		if (beforeCursor) query.set('before', beforeCursor)
	}
	const response = await request(apiURL(`/diagnostics/trace?${query}`), {
		cache: 'no-store',
		signal,
	})
	if (!response.ok) throw new DiagnosticTraceError(response.status)
	return normalizeTraceBundle(await response.json() as TraceBundle)
}

export function mergeDiagnosticTracePage(
	current: TraceBundle,
	olderPage: TraceBundle,
): TraceBundle {
	return withSequentialRequestNumbers({
		...current,
		generatedAt: newerGeneratedAt(current.generatedAt, olderPage.generatedAt),
		tasks: mergeTasks(olderPage.tasks, current.tasks),
		page: olderPage.page,
	})
}

export function mergeLatestDiagnosticTrace(
	current: TraceBundle | undefined,
	latestPage: TraceBundle,
): TraceBundle {
	if (!current) return withSequentialRequestNumbers(latestPage)
	const latestIDs = new Set(latestPage.tasks.map((task) => task.id))
	const oldestLatestTask = latestPage.tasks[0]
	const retainedEarlier = Boolean(oldestLatestTask && current.tasks.some((task) =>
		!latestIDs.has(task.id) && compareTasks(task, oldestLatestTask) < 0,
	))
	return withSequentialRequestNumbers({
		...latestPage,
		generatedAt: newerGeneratedAt(current.generatedAt, latestPage.generatedAt),
		tasks: mergeFreshTasks(
			current.tasks,
			latestPage.tasks,
			current.generatedAt,
			latestPage.generatedAt,
		),
		page: retainedEarlier ? current.page : latestPage.page,
	})
}

export function mergeDiagnosticTraceRun(
	current: TraceBundle | undefined,
	runBundle: TraceBundle,
): TraceBundle {
	if (!current) return withSequentialRequestNumbers(runBundle)
	return withSequentialRequestNumbers({
		...current,
		generatedAt: newerGeneratedAt(current.generatedAt, runBundle.generatedAt),
		selectedTaskId: runBundle.selectedTaskId || current.selectedTaskId,
		tasks: mergeFreshTasks(
			current.tasks,
			runBundle.tasks,
			current.generatedAt,
			runBundle.generatedAt,
		),
	})
}

function mergeTasks(
	base: TraceBundleTask[],
	preferred: TraceBundleTask[],
): TraceBundleTask[] {
	const tasks = new Map(base.map((task) => [task.id, task]))
	for (const task of preferred) tasks.set(task.id, task)
	return [...tasks.values()].sort(compareTasks)
}

function mergeFreshTasks(
	current: TraceBundleTask[],
	incoming: TraceBundleTask[],
	currentGeneratedAt: string,
	incomingGeneratedAt: string,
): TraceBundleTask[] {
	const tasks = new Map(current.map((task) => [task.id, task]))
	for (const candidate of incoming) {
		const existing = tasks.get(candidate.id)
		if (!existing || compareTaskFreshness(
			existing,
			candidate,
			currentGeneratedAt,
			incomingGeneratedAt,
		) <= 0) {
			tasks.set(candidate.id, candidate)
		}
	}
	return [...tasks.values()].sort(compareTasks)
}

function compareTaskFreshness(
	current: TraceBundleTask,
	incoming: TraceBundleTask,
	currentGeneratedAt: string,
	incomingGeneratedAt: string,
): number {
	const currentUpdated = Date.parse(current.updatedAt)
	const incomingUpdated = Date.parse(incoming.updatedAt)
	if (Number.isFinite(currentUpdated) && Number.isFinite(incomingUpdated) && currentUpdated !== incomingUpdated) {
		return currentUpdated - incomingUpdated
	}
	return compareTimestamps(currentGeneratedAt, incomingGeneratedAt)
}

function compareTasks(left: TraceBundleTask, right: TraceBundleTask): number {
	const leftStarted = Date.parse(left.startedAt)
	const rightStarted = Date.parse(right.startedAt)
	if (Number.isFinite(leftStarted) && Number.isFinite(rightStarted) && leftStarted !== rightStarted) {
		return leftStarted - rightStarted
	}
	if (left.id === right.id) return 0
	return left.id > right.id ? -1 : 1
}

function withSequentialRequestNumbers(bundle: TraceBundle): TraceBundle {
	let number = 0
	return {
		...bundle,
		tasks: bundle.tasks.map((task) => ({
			...task,
			requests: task.requests.map((traceRequest) => ({
				...traceRequest,
				number: ++number,
			})),
		})),
	}
}

function newerGeneratedAt(left: string, right: string): string {
	return compareTimestamps(left, right) < 0 ? right : left
}

function compareTimestamps(left: string, right: string): number {
	const leftTime = Date.parse(left)
	const rightTime = Date.parse(right)
	if (!Number.isFinite(leftTime)) return Number.isFinite(rightTime) ? -1 : 0
	if (!Number.isFinite(rightTime)) return 1
	return leftTime - rightTime
}
