import type {
  DiagnosticEvent,
  DiagnosticRun,
  RequestSnapshot,
  RequestSnapshotAttachment,
  RequestSnapshotContent,
} from './catalog'

export type TraceLifecycle = 'complete' | 'in-progress' | 'missing-start'

type TraceOperationBase = {
  id: string
  parentId?: string
  turnId?: string
  providerRequestId?: string
  startedAt: string
  completedAt?: string
  status?: string
  durationMs?: number
  errorCode?: string
  lifecycle: TraceLifecycle
  events: DiagnosticEvent[]
}

export type TraceAttempt = {
  id: string
  parentId: string
  attempt: number
  startedAt: string
  completedAt?: string
  status?: string
  durationMs?: number
  httpStatus?: number
  errorCode?: string
  lifecycle: TraceLifecycle
  events: DiagnosticEvent[]
}

export type TraceProviderRequest = TraceOperationBase & {
  kind: 'provider'
  providerRequestId: string
  provider?: string
  model?: string
  timeToFirstOutputMs?: number
  inputTokens?: number
  inputUnknown?: boolean
  outputTokens?: number
  cacheReadTokens?: number
  cacheWriteTokens?: number
  totalTokens?: number
  costTotalUsd?: number
  attempts: TraceAttempt[]
}

export type TraceCheckpoint = TraceOperationBase & {
  kind: 'checkpoint'
}

export type TraceApproval = TraceOperationBase & {
  kind: 'approval'
  toolCallId: string
  toolName?: string
}

export type TraceToolCall = TraceOperationBase & {
  kind: 'tool'
  toolCallId: string
  toolName?: string
  approvalDurationMs?: number
  executionDurationMs?: number
  approvals: TraceApproval[]
}

export type TraceOperation = TraceProviderRequest | TraceCheckpoint | TraceToolCall | TraceApproval

export type TraceTurn = {
  id: string
  startedAt: string
  completedAt?: string
  status?: string
  durationMs?: number
  errorCode?: string
  lifecycle: TraceLifecycle
  operations: TraceOperation[]
  events: DiagnosticEvent[]
}

export type TraceTimelineSpan = {
  id: string
  operationId: string
  kind: 'input' | 'model' | 'tool'
  startedAt: string
  durationMs?: number
  label: string
  providerRequestId?: string
  turnId?: string
  status?: string
}

export type TraceRun = {
  id: string
  sessionId: string
  status: string
  startedAt: string
  updatedAt: string
  durationMs?: number
  turns: TraceTurn[]
  operations: TraceOperation[]
  providerRequests: TraceProviderRequest[]
  timeline: TraceTimelineSpan[]
  providerDurationMs: number
}

export type TraceContentKind =
  | 'system'
  | 'user'
  | 'assistant'
  | 'context'
  | 'skill'
  | 'toolCall'
  | 'toolResult'
  | 'thinking'
  | 'image'
  | 'tool'
  | 'toolSchema'

export type TraceContentItem = {
  id: string
  kind: TraceContentKind
  role?: string
  providerRequestId?: string
  toolCallId?: string
  toolName?: string
  attachmentKind?: string
  preview: string
  raw: unknown
  thinkingPreview?: string
  thinkingRaw?: unknown
  toolCalls?: Array<{
    toolCallId?: string
    toolName?: string
    arguments?: Record<string, unknown>
  }>
  resultPreview?: string
  resultRaw?: unknown
  isError?: boolean
  source?: string
  turn?: number
  image?: { mimeType: string; encodedBytes?: number }
}

export type TraceRequestView = {
  request: TraceProviderRequest
  snapshot: RequestSnapshot
  trajectoryItems: TraceContentItem[]
  toolItems: TraceContentItem[]
}

export type TraceProviderRequestReference = {
  runId: string
  requestNumber: number
  request: TraceProviderRequest
}

type LifecycleGroup = {
  started?: DiagnosticEvent
  terminal?: DiagnosticEvent
  events: DiagnosticEvent[]
}

type ProviderGroup = LifecycleGroup & {
  attempts: Map<number, LifecycleGroup>
}

type ToolGroup = LifecycleGroup & {
  approvals: LifecycleGroup[]
}

export function buildTraceRun(run: DiagnosticRun): TraceRun {
  const operations = buildTraceOperations(run.events)
  const providerRequests = operations.filter(
    (operation): operation is TraceProviderRequest => operation.kind === 'provider',
  )
  return {
    id: run.id,
    sessionId: run.sessionId,
    status: run.status,
    startedAt: run.startedAt,
    updatedAt: run.updatedAt,
    durationMs: run.durationMs,
    turns: buildTraceTurns(run.events, operations),
    operations,
    providerRequests,
    timeline: buildTraceTimeline(operations),
    providerDurationMs: providerRequests.reduce(
      (total, request) => total + (request.durationMs ?? 0),
      0,
    ),
  }
}

export function buildTraceRequestCatalog(
  runs: DiagnosticRun[],
): TraceProviderRequestReference[] {
  const sessions = new Map<string, Array<Omit<TraceProviderRequestReference, 'requestNumber'>>>()
  for (const run of runs) {
    const references = sessions.get(run.sessionId) ?? []
    references.push(...buildTraceRun(run).providerRequests.map(
      (request) => ({ runId: run.id, request }),
    ))
    sessions.set(run.sessionId, references)
  }
  return [...sessions.values()].flatMap((references) =>
    references
      .sort((left, right) => compareStartedAt(left.request, right.request))
      .map((reference, index) => ({ ...reference, requestNumber: index + 1 })),
  )
}

export function findTraceProviderRequest(
  catalog: TraceProviderRequestReference[],
  providerRequestId?: string,
): TraceProviderRequestReference | undefined {
  if (!providerRequestId) return undefined
  return catalog.find(
    (reference) => reference.request.providerRequestId === providerRequestId,
  )
}

export function buildTraceRequestView(
  request: TraceProviderRequest,
  snapshot: RequestSnapshot,
): TraceRequestView {
  return {
    request,
    snapshot,
    trajectoryItems: buildTrajectoryItems(snapshot),
    toolItems: (snapshot.input.tools ?? []).map((tool, index) => ({
      id: `${snapshot.providerRequestId}:tool-definition:${index}`,
      kind: 'toolSchema' as const,
      toolName: tool.name,
      preview: [tool.description, '', JSON.stringify(tool.parameters, null, 2)]
        .filter(Boolean)
        .join('\n'),
      raw: tool,
    })),
  }
}

function buildTraceOperations(events: DiagnosticEvent[]): TraceOperation[] {
  const providers = new Map<string, ProviderGroup>()
  const tools = new Map<string, ToolGroup>()
  const checkpoints: TraceCheckpoint[] = []
  const providerFallbacks = new Map<string, string>()
  const toolFallbacks = new Map<string, string>()
  const counters = new Map<string, number>()

  for (const event of events) {
    switch (event.name) {
      case 'provider.request.started': {
        const key = correlationKey('provider', event, providerFallbacks, counters, true)
        const group = getProviderGroup(providers, key)
        group.started = event
        group.events.push(event)
        break
      }
      case 'provider.request.completed':
      case 'provider.request.failed': {
        const key = correlationKey('provider', event, providerFallbacks, counters, false)
        const group = getProviderGroup(providers, key)
        group.terminal = event
        group.events.push(event)
        break
      }
      case 'provider.http_attempt.started':
      case 'provider.http_attempt.response': {
        const key = correlationKey('provider', event, providerFallbacks, counters, false)
        const group = getProviderGroup(providers, key)
        const attemptNumber = event.attempt ?? nextCounter(counters, `attempt:${key}`)
        const attempt = getLifecycleGroup(group.attempts, attemptNumber)
        if (event.name === 'provider.http_attempt.started') attempt.started = event
        else attempt.terminal = event
        attempt.events.push(event)
        group.events.push(event)
        break
      }
      case 'checkpoint.completed':
      case 'checkpoint.failed': {
        const correlation = event.providerRequestId ?? event.turnId ?? 'run'
        const ordinal = nextCounter(counters, `checkpoint:${correlation}`)
        const durationMs = event.durationMs
        checkpoints.push({
          id: `checkpoint:${correlation}:${ordinal}`,
          parentId: event.providerRequestId
            ? `provider:${event.providerRequestId}`
            : turnParentId(event.turnId),
          kind: 'checkpoint',
          turnId: event.turnId,
          providerRequestId: event.providerRequestId,
          startedAt: subtractDuration(event.timestamp, durationMs),
          completedAt: event.timestamp,
          status: event.status,
          durationMs,
          errorCode: event.errorCode,
          lifecycle: 'complete',
          events: [event],
        })
        break
      }
      case 'tool.call.started': {
        const key = correlationKey('tool', event, toolFallbacks, counters, true)
        const group = getToolGroup(tools, key)
        group.started = event
        group.events.push(event)
        break
      }
      case 'tool.call.completed':
      case 'tool.call.failed': {
        const key = correlationKey('tool', event, toolFallbacks, counters, false)
        const group = getToolGroup(tools, key)
        group.terminal = event
        group.events.push(event)
        break
      }
      case 'tool.approval.started': {
        const key = correlationKey('tool', event, toolFallbacks, counters, false)
        const group = getToolGroup(tools, key)
        const approval = newLifecycleGroup()
        approval.started = event
        approval.events.push(event)
        group.approvals.push(approval)
        group.events.push(event)
        break
      }
      case 'tool.approval.completed':
      case 'tool.approval.failed': {
        const key = correlationKey('tool', event, toolFallbacks, counters, false)
        const group = getToolGroup(tools, key)
        const approval = group.approvals.findLast((candidate) => !candidate.terminal)
          ?? newLifecycleGroup()
        if (!group.approvals.includes(approval)) group.approvals.push(approval)
        approval.terminal = event
        approval.events.push(event)
        group.events.push(event)
        break
      }
    }
  }

  const providerOperations = [...providers.entries()].map(([key, group]) =>
    buildProviderOperation(key, group),
  )
  const toolOperations: TraceOperation[] = []
  for (const [key, group] of tools) {
    const tool = buildToolOperation(key, group)
    toolOperations.push(tool, ...tool.approvals)
  }
  return [...providerOperations, ...checkpoints, ...toolOperations].sort(compareStartedAt)
}

function buildProviderOperation(key: string, group: ProviderGroup): TraceProviderRequest {
  const event = group.terminal ?? group.started ?? group.events[0]
  const providerRequestId = event?.providerRequestId ?? key
  const id = `provider:${providerRequestId}`
  return {
    ...lifecycleFields(group),
    id,
    parentId: turnParentId(event?.turnId),
    kind: 'provider',
    turnId: event?.turnId,
    providerRequestId,
    provider: group.terminal?.provider ?? group.started?.provider,
    model: group.terminal?.model ?? group.started?.model,
    timeToFirstOutputMs: group.terminal?.timeToFirstOutputMs,
    inputTokens: group.terminal?.inputTokens,
    inputUnknown: group.terminal?.inputUnknown,
    outputTokens: group.terminal?.outputTokens,
    cacheReadTokens: group.terminal?.cacheReadTokens,
    cacheWriteTokens: group.terminal?.cacheWriteTokens,
    totalTokens: group.terminal?.totalTokens,
    costTotalUsd: group.terminal?.costTotalUsd,
    attempts: [...group.attempts.entries()]
      .map(([attempt, attemptGroup]) => buildAttempt(id, attempt, attemptGroup))
      .sort((left, right) => left.attempt - right.attempt),
  }
}

function buildAttempt(parentId: string, attempt: number, group: LifecycleGroup): TraceAttempt {
  return {
    ...lifecycleFields(group),
    id: `attempt:${parentId.slice('provider:'.length)}:${attempt}`,
    parentId,
    attempt,
    httpStatus: group.terminal?.httpStatus,
  }
}

function buildToolOperation(key: string, group: ToolGroup): TraceToolCall {
  const event = group.terminal ?? group.started ?? group.events[0]
  const toolCallId = event?.toolCallId ?? key
  const id = `tool:${toolCallId}`
  const approvals = group.approvals.map((approval, index) => ({
    ...lifecycleFields(approval),
    id: `approval:${toolCallId}:${index + 1}`,
    parentId: id,
    kind: 'approval' as const,
    turnId: event?.turnId,
    providerRequestId: event?.providerRequestId,
    toolCallId,
    toolName: event?.toolName,
  }))
  const lifecycle = lifecycleFields(group)
  const approvalDurationMs = approvals.reduce(
    (total, approval) => total + (approval.durationMs ?? 0),
    0,
  )
  return {
    ...lifecycle,
    id,
    parentId: event?.providerRequestId
      ? `provider:${event.providerRequestId}`
      : turnParentId(event?.turnId),
    kind: 'tool',
    turnId: event?.turnId,
    providerRequestId: event?.providerRequestId,
    toolCallId,
    toolName: group.terminal?.toolName ?? group.started?.toolName,
    approvalDurationMs: approvalDurationMs || undefined,
    executionDurationMs: lifecycle.durationMs === undefined
      ? undefined
      : Math.max(0, lifecycle.durationMs - approvalDurationMs),
    approvals,
  }
}

function buildTraceTurns(events: DiagnosticEvent[], operations: TraceOperation[]): TraceTurn[] {
  const groups = new Map<string, LifecycleGroup>()
  for (const event of events) {
    if (!event.turnId) continue
    const group = getLifecycleGroup(groups, event.turnId)
    group.events.push(event)
    if (event.name === 'turn.started') group.started = event
    else if (event.name === 'turn.completed' || event.name === 'turn.discarded') {
      group.terminal = event
    }
  }
  for (const operation of operations) {
    if (!operation.turnId || groups.has(operation.turnId)) continue
    groups.set(operation.turnId, newLifecycleGroup())
  }
  return [...groups.entries()]
    .map(([id, group]) => {
      const lifecycle = lifecycleFields(group)
      const turnOperations = operations
        .filter((operation) => operation.turnId === id)
        .sort(compareStartedAt)
      const startedAt = lifecycle.startedAt || turnOperations[0]?.startedAt || ''
      return {
        id,
        startedAt,
        completedAt: lifecycle.completedAt,
        status: lifecycle.status ?? (turnOperations.some(isInProgress) ? 'running' : undefined),
        durationMs: lifecycle.durationMs,
        errorCode: lifecycle.errorCode,
        lifecycle: group.started || group.terminal ? lifecycle.lifecycle : 'missing-start',
        operations: turnOperations,
        events: group.events,
      }
    })
    .sort(compareStartedAt)
}

function buildTraceTimeline(operations: TraceOperation[]): TraceTimelineSpan[] {
  return operations.flatMap((operation): TraceTimelineSpan[] => {
    if (operation.kind === 'approval') return []
    const kind = operation.kind === 'checkpoint'
      ? 'input'
      : operation.kind === 'provider'
        ? 'model'
        : 'tool'
    const label = operation.kind === 'checkpoint'
      ? 'Checkpoint'
      : operation.kind === 'provider'
        ? (operation.model ?? operation.provider ?? operation.providerRequestId)
        : (operation.toolName ?? operation.toolCallId)
    return [{
      id: `span:${operation.id}`,
      operationId: operation.id,
      kind,
      startedAt: operation.startedAt,
      durationMs: operation.durationMs,
      label,
      providerRequestId: operation.providerRequestId,
      turnId: operation.turnId,
      status: operation.status,
    }]
  }).sort(compareStartedAt)
}

function buildTrajectoryItems(snapshot: RequestSnapshot): TraceContentItem[] {
  const attachments = new Map(
    (snapshot.attachments ?? []).map((attachment) => [attachment.messageIndex, attachment]),
  )
  const items: TraceContentItem[] = []
  if (snapshot.input.systemPrompt) {
    items.push({
      id: `${snapshot.providerRequestId}:system`,
      kind: 'system',
      preview: snapshot.input.systemPrompt,
      raw: { systemPrompt: snapshot.input.systemPrompt },
    })
  }
  let turn = 0
  snapshot.input.messages.forEach((message, index) => {
    const attachment = attachments.get(index)
    if (message.role === 'user' && !attachment) turn += 1
    items.push(...buildMessageItems(
      snapshot.providerRequestId,
      message.role,
      message.content,
      message.toolName,
      message,
      index,
      attachment,
      Math.max(1, turn),
    ))
  })
  if (snapshot.output) {
    items.push(...buildMessageItems(
      snapshot.providerRequestId,
      snapshot.output.message.role,
      snapshot.output.message.content,
      snapshot.output.message.toolName,
      snapshot.output,
      'output',
      undefined,
      Math.max(1, turn),
    ))
  }
  return pairToolResults(items)
}

function buildMessageItems(
  providerRequestId: string,
  role: string,
  content: RequestSnapshotContent[],
  messageToolName: string | undefined,
  raw: unknown,
  index: number | 'output',
  attachment: RequestSnapshotAttachment | undefined,
  turn: number,
): TraceContentItem[] {
  const sourceProviderRequestId = index === 'output'
    ? providerRequestId
    : isSnapshotMessage(raw) ? raw.providerRequestId : undefined
  if (attachment) {
    return [{
      id: `${providerRequestId}:attachment:${attachment.id || index}`,
      kind: attachment.kind.includes('skill') ? 'skill' : 'context',
      attachmentKind: attachment.kind,
      providerRequestId: sourceProviderRequestId,
      preview: contentPreview(content),
      raw: { attachment, message: raw },
      source: attachment.path,
      turn,
    }]
  }
  if (role === 'user') {
    return [{
      id: `${providerRequestId}:message:${index}`,
      kind: content.length > 0 && content.every((item) => item.type === 'image') ? 'image' : 'user',
      role, providerRequestId: sourceProviderRequestId, preview: contentPreview(content), raw, turn,
      image: content.length === 1 ? content[0]?.image : undefined,
    }]
  }
  if (role === 'toolResult') {
    return [{
      id: `${providerRequestId}:message:${index}`,
      kind: 'toolResult', role, toolCallId: (raw as { toolCallId?: string }).toolCallId,
      providerRequestId: sourceProviderRequestId,
      toolName: messageToolName, preview: contentPreview(content), raw, turn,
      isError: Boolean((raw as { isError?: boolean }).isError),
    }]
  }
  if (content.length === 0) {
    return [{
      id: `${providerRequestId}:message:${index}`,
      kind: 'assistant', role, providerRequestId: sourceProviderRequestId,
      preview: '', raw, turn,
    }]
  }
  const thinkingBlocks = content
    .map((block, blockIndex) => ({ block, blockIndex }))
    .filter(({ block }) => block.type === 'thinking')
  const responseBlocks = content
    .map((block, blockIndex) => ({ block, blockIndex }))
    .filter(({ block }) => block.type !== 'thinking' && block.type !== 'toolCall')
  const thinkingPreview = contentPreview(thinkingBlocks.map(({ block }) => block))
  const thinkingRaw = thinkingBlocks.map(({ block }) => block)
  const toolCalls = content
    .filter((block) => block.type === 'toolCall')
    .map((block) => ({
      toolCallId: block.toolCallId,
      toolName: block.toolName ?? messageToolName,
      arguments: block.arguments,
    }))
  const candidates: Array<{ blockIndex: number; item: TraceContentItem }> = []

  if (responseBlocks.length > 0 || thinkingBlocks.length > 0) {
    const blocks = responseBlocks.map(({ block }) => block)
    const blockIndex = Math.min(
      responseBlocks[0]?.blockIndex ?? Number.POSITIVE_INFINITY,
      thinkingBlocks[0]?.blockIndex ?? Number.POSITIVE_INFINITY,
    )
    candidates.push({
      blockIndex: Number.isFinite(blockIndex) ? blockIndex : 0,
      item: {
        id: `${providerRequestId}:message:${index}:content:${Number.isFinite(blockIndex) ? blockIndex : 0}`,
        kind: blocks.length > 0 && blocks.every((block) => block.type === 'image') ? 'image' : 'assistant',
        role,
        providerRequestId: sourceProviderRequestId,
        preview: contentPreview(blocks),
        raw,
        thinkingPreview: thinkingPreview || undefined,
        thinkingRaw: thinkingRaw.length > 0 ? thinkingRaw : undefined,
        toolCalls: toolCalls.length > 0 ? toolCalls : undefined,
        turn,
        image: blocks.length === 1 ? blocks[0]?.image : undefined,
      },
    })
  }

  content.forEach((block, blockIndex) => {
    if (block.type !== 'toolCall') return
    candidates.push({
      blockIndex,
      item: {
        id: `${providerRequestId}:message:${index}:content:${blockIndex}`,
        kind: 'toolCall',
        role,
        providerRequestId: sourceProviderRequestId,
        toolCallId: block.toolCallId,
        toolName: block.toolName ?? messageToolName,
        preview: contentPreview([block]),
        raw: block,
        turn,
      },
    })
  })

  return candidates.sort((left, right) => left.blockIndex - right.blockIndex).map(({ item }) => item)
}

function isSnapshotMessage(value: unknown): value is RequestSnapshot['input']['messages'][number] {
  return value !== null && typeof value === 'object' && 'role' in value
}

function pairToolResults(items: TraceContentItem[]): TraceContentItem[] {
  const result: TraceContentItem[] = []
  const calls = new Map<string, number>()
  for (const item of items) {
    if (item.kind === 'toolCall' && item.toolCallId) {
      calls.set(item.toolCallId, result.length)
      result.push(item)
      continue
    }
    if (item.kind === 'toolResult' && item.toolCallId) {
      const callIndex = calls.get(item.toolCallId)
      const call = callIndex === undefined ? undefined : result[callIndex]
      if (callIndex !== undefined && call) {
        result[callIndex] = {
          ...call,
          kind: 'tool',
          resultPreview: item.preview,
          resultRaw: item.raw,
          isError: item.isError,
        }
        continue
      }
      result.push({ ...item, kind: 'tool' })
      continue
    }
    result.push(item)
  }
  return result
}

function contentPreview(content: RequestSnapshotContent[]): string {
  return content.map((item) => {
    if (item.type === 'text') return item.text ?? ''
    if (item.type === 'thinking') return item.thinking ?? ''
    if (item.type === 'toolCall') {
      return JSON.stringify(item.arguments ?? {}, null, 2)
    }
    if (item.type === 'image') return item.image?.mimeType ?? 'image'
    return item.type
  }).filter(Boolean).join('\n\n')
}

function lifecycleFields(group: LifecycleGroup): Omit<TraceOperationBase, 'id' | 'kind'> {
  const event = group.terminal ?? group.started ?? group.events[0]
  const startedAt = group.started?.timestamp
    ?? subtractDuration(group.terminal?.timestamp ?? '', group.terminal?.durationMs)
  return {
    turnId: event?.turnId,
    providerRequestId: event?.providerRequestId,
    startedAt,
    completedAt: group.terminal?.timestamp,
    status: group.terminal?.status ?? group.started?.status,
    durationMs: group.terminal?.durationMs ?? elapsedMs(group.started?.timestamp, group.terminal?.timestamp),
    errorCode: group.terminal?.errorCode,
    lifecycle: group.terminal ? (group.started ? 'complete' : 'missing-start') : 'in-progress',
    events: group.events,
  }
}

function correlationKey(
  kind: 'provider' | 'tool',
  event: DiagnosticEvent,
  fallbacks: Map<string, string>,
  counters: Map<string, number>,
  start: boolean,
): string {
  const explicit = kind === 'provider' ? event.providerRequestId : event.toolCallId
  if (explicit) return explicit
  const scope = event.turnId ?? 'run'
  if (!start) {
    const existing = fallbacks.get(scope)
    if (existing) return existing
  }
  const key = `missing-${scope}-${nextCounter(counters, `${kind}:${scope}`)}`
  fallbacks.set(scope, key)
  return key
}

function turnParentId(turnId?: string): string | undefined {
  return turnId ? `turn:${turnId}` : undefined
}

function newLifecycleGroup(): LifecycleGroup {
  return { events: [] }
}

function getLifecycleGroup<K>(groups: Map<K, LifecycleGroup>, key: K): LifecycleGroup {
  const existing = groups.get(key)
  if (existing) return existing
  const group = newLifecycleGroup()
  groups.set(key, group)
  return group
}

function getProviderGroup(groups: Map<string, ProviderGroup>, key: string): ProviderGroup {
  const existing = groups.get(key)
  if (existing) return existing
  const group: ProviderGroup = { events: [], attempts: new Map() }
  groups.set(key, group)
  return group
}

function getToolGroup(groups: Map<string, ToolGroup>, key: string): ToolGroup {
  const existing = groups.get(key)
  if (existing) return existing
  const group: ToolGroup = { events: [], approvals: [] }
  groups.set(key, group)
  return group
}

function nextCounter(counters: Map<string, number>, key: string): number {
  const next = (counters.get(key) ?? 0) + 1
  counters.set(key, next)
  return next
}

function subtractDuration(timestamp: string, durationMs?: number): string {
  const value = Date.parse(timestamp)
  if (!Number.isFinite(value)) return timestamp
  return new Date(value - (durationMs ?? 0)).toISOString()
}

function elapsedMs(startedAt?: string, completedAt?: string): number | undefined {
  const start = Date.parse(startedAt ?? '')
  const end = Date.parse(completedAt ?? '')
  if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) return undefined
  return end - start
}

function compareStartedAt(
  left: { startedAt: string },
  right: { startedAt: string },
): number {
  const leftTime = Date.parse(left.startedAt)
  const rightTime = Date.parse(right.startedAt)
  if (!Number.isFinite(leftTime)) return Number.isFinite(rightTime) ? 1 : 0
  if (!Number.isFinite(rightTime)) return -1
  return leftTime - rightTime
}

function isInProgress(operation: TraceOperation): boolean {
  return operation.lifecycle === 'in-progress'
}
