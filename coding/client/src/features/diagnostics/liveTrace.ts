import type { Item } from '@/types'
import type {
  RequestSnapshotMessage,
  TraceBundle,
  TraceBundleRequest,
  TraceBundleTask,
  TraceBundleTool,
} from './catalog'

type RunItem = Extract<Item, { kind: 'run' }>
type AssistantItem = Extract<Item, { kind: 'assistant' }>
type ToolItem = Extract<Item, { kind: 'tool' }>

type LiveRequestSegment = {
  providerRequestId?: string
  thinking: string[]
  assistant?: AssistantItem
  tools: ToolItem[]
}

type LiveRun = {
  run: RunItem
  prompt: string
  segments: LiveRequestSegment[]
}

export function mergeLiveTraceBundle(
  bundle: TraceBundle | undefined,
  sessionID: string | undefined,
  items: Item[] | undefined,
  running: boolean,
): TraceBundle | undefined {
  if (!sessionID || !items?.length) return bundle
  const live = latestLiveRun(items)
  if (!live) return bundle
  const liveActive = running || live.run.durationMs === undefined
  if (!liveActive) return bundle

  const runID = live.run.runId || live.run.id
  const tasks = [...(bundle?.tasks ?? [])]
  let taskIndex = tasks.findIndex((task) => task.id === runID)
  if (taskIndex < 0 && !live.run.runId) {
    taskIndex = matchingTaskIndex(tasks, live.run.startedAt)
  }
  const previous = taskIndex >= 0 ? tasks[taskIndex] : undefined
  const updatedAt = liveRunUpdatedAt(live, previous?.updatedAt)
  const task: TraceBundleTask = previous
    ? { ...previous, requests: [...previous.requests] }
    : {
        id: runID,
        status: 'running',
        prompt: live.prompt,
        startedAt: live.run.startedAt,
        updatedAt,
        durationMs: live.run.durationMs,
        retries: 0,
        contextRecoveries: 0,
        requests: [],
        rawEvents: [],
      }

  const highestRequestNumber = Math.max(
    0,
    ...tasks.flatMap((candidate) => candidate.requests.map((request) => request.number)),
  )
  let nextRequestNumber = highestRequestNumber + 1
  for (const [index, segment] of live.segments.entries()) {
    let requestIndex = segment.providerRequestId
      ? task.requests.findIndex((request) => request.id === segment.providerRequestId)
      : -1
    if (requestIndex < 0 && !segment.providerRequestId && index < task.requests.length) {
      requestIndex = index
    }
    const existing = requestIndex >= 0 ? task.requests[requestIndex] : undefined
    const request = mergeLiveRequest(
      existing,
      segment,
      runID,
      live.run.startedAt,
      updatedAt,
      existing?.number ?? nextRequestNumber++,
      liveActive && index === live.segments.length - 1,
    )
    if (requestIndex >= 0) task.requests[requestIndex] = request
    else task.requests.push(request)
  }
  const activeSegment = live.segments.at(-1)
  const activeRequestIndex = activeSegment?.providerRequestId
    ? task.requests.findIndex((request) => request.id === activeSegment.providerRequestId)
    : Math.min(live.segments.length - 1, task.requests.length - 1)
  if (activeRequestIndex > 0) {
    task.requests = task.requests.map((request, index) => {
      if (index >= activeRequestIndex || request.status !== 'running') return request
      return {
        ...request,
        status: 'completed',
        lifecycle: request.lifecycle === 'missing-start' ? 'missing-start' : 'complete',
      }
    })
  }
  task.status = 'running'
  task.prompt = task.prompt || live.prompt
  task.updatedAt = updatedAt
  task.durationMs = live.run.durationMs ?? task.durationMs

  if (taskIndex >= 0) tasks[taskIndex] = task
  else tasks.push(task)

  return {
    version: bundle?.version ?? 1,
    generatedAt: bundle?.generatedAt ?? updatedAt,
    sessionId: sessionID,
    selectedTaskId: task.id,
    tasks,
    page: bundle?.page ?? { hasMore: false },
  }
}

export function liveTraceRefreshKey(items: Item[] | undefined, running: boolean): string {
  if (!items?.length) return String(running)
  const runIndex = items.findLastIndex((item) => item.kind === 'run')
  const tail = runIndex < 0 ? items : items.slice(runIndex)
  return [
    running ? 'running' : 'idle',
    ...tail.flatMap((item) => {
      if (item.kind === 'run') {
        return [`run:${item.runId ?? item.id}:${item.durationMs === undefined ? 'open' : 'done'}`]
      }
      if (item.kind === 'thinking') {
        return [`thinking:${item.providerRequestId ?? ''}:${item.id}:${item.streaming ? 'open' : 'done'}`]
      }
      if (item.kind === 'assistant') {
        return [`assistant:${item.providerRequestId ?? ''}:${item.id}:${item.open ? 'open' : 'closed'}:${item.complete ? 'final' : 'partial'}`]
      }
      if (item.kind === 'tool') {
        return [`tool:${item.providerRequestId ?? ''}:${item.id}:${item.status}`]
      }
      if (item.kind === 'approval') return [`approval:${item.id}`]
      return []
    }),
  ].join('|')
}

function latestLiveRun(items: Item[]): LiveRun | undefined {
  const runIndex = items.findLastIndex((item) => item.kind === 'run')
  const run = runIndex < 0 ? undefined : items[runIndex]
  if (!run || run.kind !== 'run') return undefined

  const promptParts: string[] = []
  for (let index = runIndex - 1; index >= 0; index--) {
    const item = items[index]
    if (item?.kind !== 'user') break
    promptParts.unshift(item.text)
  }

  const segments: LiveRequestSegment[] = []
  let current: LiveRequestSegment | undefined
  const completeCurrent = () => {
    if (!current) return
    if (current.thinking.length || current.assistant || current.tools.length) segments.push(current)
    current = undefined
  }
  const ensureCurrent = (providerRequestId?: string) => {
    if (
      current?.providerRequestId &&
      providerRequestId &&
      current.providerRequestId !== providerRequestId
    ) {
      completeCurrent()
    }
    current ??= { providerRequestId, thinking: [], tools: [] }
    if (providerRequestId && !current.providerRequestId) current.providerRequestId = providerRequestId
    return current
  }

  for (const item of items.slice(runIndex + 1)) {
    if (item.kind === 'run' || item.kind === 'user') break
    if (item.kind === 'thinking') {
      const segment = ensureCurrent(item.providerRequestId)
      if (!item.providerRequestId && (segment.assistant || segment.tools.length)) {
        completeCurrent()
      }
      if (item.text) ensureCurrent(item.providerRequestId).thinking.push(item.text)
      continue
    }
    if (item.kind === 'assistant') {
      const segment = ensureCurrent(item.providerRequestId)
      if (segment.assistant || (!item.providerRequestId && segment.tools.length)) {
        completeCurrent()
      }
      ensureCurrent(item.providerRequestId).assistant = item
      continue
    }
    if (item.kind === 'tool') ensureCurrent(item.providerRequestId).tools.push(item)
  }
  completeCurrent()
  return { run, prompt: promptParts.join('\n\n'), segments }
}

function matchingTaskIndex(tasks: TraceBundleTask[], startedAt: string): number {
  const started = Date.parse(startedAt)
  if (!Number.isFinite(started)) return -1
  return tasks.findLastIndex((task) => {
    const taskStarted = Date.parse(task.startedAt)
    return Number.isFinite(taskStarted) && Math.abs(taskStarted - started) < 2_000
  })
}

function liveRunUpdatedAt(live: LiveRun, previous?: string): string {
  return live.segments.findLast((segment) => segment.assistant?.completedAt)?.assistant?.completedAt
    ?? previous
    ?? live.run.startedAt
}

function mergeLiveRequest(
  existing: TraceBundleRequest | undefined,
  segment: LiveRequestSegment,
  runID: string,
  startedAt: string,
  updatedAt: string,
  number: number,
  requestRunning: boolean,
): TraceBundleRequest {
  const requestID = existing?.id ?? segment.providerRequestId ?? `live:${runID}:${number}`
  const output = liveOutput(existing, segment, requestID, updatedAt)
  const completedStatus = !existing?.status || existing.status === 'running'
    ? 'completed'
    : existing.status
  const completedLifecycle = existing?.lifecycle === 'missing-start'
    ? 'missing-start'
    : 'complete'
  return {
    ...existing,
    id: requestID,
    number,
    status: requestRunning ? 'running' : completedStatus,
    lifecycle: requestRunning ? 'in-progress' : completedLifecycle,
    startedAt: existing?.startedAt ?? startedAt,
    ...(output ? { output } : {}),
    tools: mergeLiveTools(existing?.tools ?? [], segment.tools, startedAt),
    attempts: existing?.attempts ?? [],
    checkpoints: existing?.checkpoints ?? [],
    snapshotState: existing?.snapshotState ?? 'missing',
    rawEvents: existing?.rawEvents ?? [],
  }
}

function liveOutput(
  existing: TraceBundleRequest | undefined,
  segment: LiveRequestSegment,
  requestID: string,
  capturedAt: string,
): TraceBundleRequest['output'] {
  const content = [
    ...segment.thinking.filter(Boolean).map((thinking) => ({ type: 'thinking', thinking })),
    ...(segment.assistant?.markdown
      ? [{ type: 'text', text: segment.assistant.markdown }]
      : []),
  ]
  if (content.length === 0) return existing?.output
  return {
    ...existing?.output,
    capturedAt: segment.assistant?.completedAt ?? existing?.output?.capturedAt ?? capturedAt,
    message: {
      role: 'assistant',
      providerRequestId: requestID,
      content,
    },
  }
}

function mergeLiveTools(
  existing: TraceBundleTool[],
  live: ToolItem[],
  startedAt: string,
): TraceBundleTool[] {
  const merged = [...existing]
  for (const [index, tool] of live.entries()) {
    const existingIndex = merged.findIndex((candidate) => candidate.id === tool.id)
    const fallbackIndex = existingIndex < 0
      ? merged.findIndex((candidate, candidateIndex) =>
          candidateIndex >= index && candidate.name === tool.name)
      : existingIndex
    const previous = fallbackIndex >= 0 ? merged[fallbackIndex] : undefined
    const next = liveTool(previous, tool, startedAt)
    if (fallbackIndex >= 0) merged[fallbackIndex] = next
    else merged.push(next)
  }
  return merged
}

function liveTool(previous: TraceBundleTool | undefined, tool: ToolItem, startedAt: string): TraceBundleTool {
  const terminal = tool.status === 'complete' || tool.status === 'error'
  return {
    ...previous,
    id: tool.id,
    name: tool.name,
    status: liveToolStatus(tool.status),
    lifecycle: terminal ? 'complete' : 'in-progress',
    startedAt: previous?.startedAt ?? startedAt,
    arguments: recordValue(tool.args) ?? previous?.arguments,
    ...(tool.result === undefined
      ? {}
      : { result: toolResultMessage(tool) }),
    rawEvents: previous?.rawEvents ?? [],
  }
}

function liveToolStatus(status: ToolItem['status']): string {
  if (status === 'complete') return 'success'
  if (status === 'error') return 'failed'
  return 'running'
}

function toolResultMessage(tool: ToolItem): RequestSnapshotMessage {
  return {
    role: 'toolResult',
    toolCallId: tool.id,
    toolName: tool.name,
    isError: tool.status === 'error',
    content: [{ type: 'text', text: tool.result ?? '' }],
  }
}

function recordValue(value: unknown): Record<string, unknown> | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
  return value as Record<string, unknown>
}
