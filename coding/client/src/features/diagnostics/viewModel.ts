import type { DiagnosticEvent } from './catalog'

export type DiagnosticStep = {
  kind: 'provider' | 'tool' | 'checkpoint'
  timestamp: string
  status?: string
  durationMs?: number
  timeToFirstOutputMs?: number
  provider?: string
  model?: string
  toolName?: string
  approvalDurationMs?: number
  attempts?: number
  httpStatus?: number
  totalTokens?: number
  errorCode?: string
}

export type DiagnosticTurn = {
  id: string
  timestamp: string
  status?: string
  durationMs?: number
  steps: DiagnosticStep[]
}

type EventGroup = {
  started?: DiagnosticEvent
  terminal?: DiagnosticEvent
  approval?: DiagnosticEvent
  attempts: DiagnosticEvent[]
}

export function buildDiagnosticTurns(events: DiagnosticEvent[]): DiagnosticTurn[] {
  const turnEvents = new Map<string, DiagnosticEvent[]>()
  for (const event of events) {
    if (!event.turnId) continue
    const grouped = turnEvents.get(event.turnId) ?? []
    grouped.push(event)
    turnEvents.set(event.turnId, grouped)
  }

  return [...turnEvents.entries()]
    .map(([id, groupedEvents]) => buildTurn(id, groupedEvents))
    .sort((left, right) => Date.parse(left.timestamp) - Date.parse(right.timestamp))
}

function buildTurn(id: string, events: DiagnosticEvent[]): DiagnosticTurn {
  const sorted = [...events].sort((left, right) => Date.parse(left.timestamp) - Date.parse(right.timestamp))
  const started = sorted.find((event) => event.name === 'turn.started')
  const terminal = sorted.findLast((event) =>
    event.name === 'turn.completed' || event.name === 'turn.discarded',
  )
  const providers = new Map<string, EventGroup>()
  const tools = new Map<string, EventGroup>()

  for (const event of sorted) {
    if (event.name.startsWith('provider.request.')) {
      const group = getGroup(providers, event.providerRequestId ?? event.timestamp)
      if (event.name === 'provider.request.started') group.started = event
      else group.terminal = event
      continue
    }
    if (event.name.startsWith('provider.http_attempt.')) {
      const group = getGroup(providers, event.providerRequestId ?? event.timestamp)
      group.attempts.push(event)
      continue
    }
    if (event.name.startsWith('tool.call.')) {
      const group = getGroup(tools, event.toolCallId ?? event.timestamp)
      if (event.name === 'tool.call.started') group.started = event
      else group.terminal = event
      continue
    }
    if (event.name.startsWith('tool.approval.')) {
      const group = getGroup(tools, event.toolCallId ?? event.timestamp)
      if (event.name !== 'tool.approval.started') group.approval = event
      else if (!group.approval) group.approval = event
    }
  }

  const steps: DiagnosticStep[] = []
  for (const group of providers.values()) {
    const event = group.terminal ?? group.started
    if (!event) continue
    const attempts = Math.max(0, ...group.attempts.map((attempt) => attempt.attempt ?? 0))
    const response = group.attempts.findLast((attempt) => attempt.httpStatus)
    steps.push({
      kind: 'provider',
      timestamp: group.started?.timestamp ?? event.timestamp,
      status: event.status,
      durationMs: group.terminal?.durationMs,
      timeToFirstOutputMs: group.terminal?.timeToFirstOutputMs,
      provider: event.provider ?? group.started?.provider,
      model: event.model ?? group.started?.model,
      attempts: attempts || undefined,
      httpStatus: response?.httpStatus,
      totalTokens: group.terminal?.totalTokens,
      errorCode: group.terminal?.errorCode,
    })
  }
  for (const group of tools.values()) {
    const event = group.terminal ?? group.started ?? group.approval
    if (!event) continue
    steps.push({
      kind: 'tool',
      timestamp: group.started?.timestamp ?? event.timestamp,
      status: group.terminal?.status ?? group.approval?.status ?? group.started?.status,
      durationMs: group.terminal?.durationMs,
      toolName: event.toolName,
      approvalDurationMs: group.approval?.durationMs,
      errorCode: group.terminal?.errorCode ?? group.approval?.errorCode,
    })
  }
  for (const event of sorted) {
    if (event.name !== 'checkpoint.failed') continue
    steps.push({
      kind: 'checkpoint',
      timestamp: event.timestamp,
      status: event.status,
      durationMs: event.durationMs,
      errorCode: event.errorCode,
    })
  }

  return {
    id,
    timestamp: started?.timestamp ?? sorted[0]?.timestamp ?? '',
    status: terminal?.status ?? started?.status,
    durationMs: terminal?.durationMs,
    steps: steps.sort((left, right) => Date.parse(left.timestamp) - Date.parse(right.timestamp)),
  }
}

function getGroup(groups: Map<string, EventGroup>, key: string): EventGroup {
  const existing = groups.get(key)
  if (existing) return existing
  const group: EventGroup = { attempts: [] }
  groups.set(key, group)
  return group
}
