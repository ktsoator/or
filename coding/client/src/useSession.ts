import { useCallback, useEffect, useReducer, useRef, useState } from 'react'
import { APIError, apiURL } from './api'
import type {
  ApprovalChoice,
  ApprovalItem,
  CompactionResult,
  ConnectionStatus,
  ContextUsage,
  DeliveryMode,
  HistoryResponse,
  Item,
  ModelCatalogResponse,
  ModelOption,
  MessageImage,
  QueuedMessage,
  SessionSummary,
  WorkspaceSummary,
  ThinkingLevel,
  Usage,
  WireEvent,
} from './types'

type ThreadState = {
  items: Item[]
  queue: QueuedMessage[]
  responseUsage: Usage
  contextUsage?: ContextUsage
  running: boolean
  autoCompacting: boolean
  status: ConnectionStatus
  seq: number
  loaded: boolean
}

type ThreadsState = Record<string, ThreadState>

type Action =
  | { t: 'reset'; sessionID: string; history: HistoryResponse }
  | { t: 'wire'; sessionID: string; ev: WireEvent }
  | { t: 'status'; sessionID: string; status: ConnectionStatus }
  | { t: 'running'; sessionID: string; running: boolean }
  | {
      t: 'sendUser'
      sessionID: string
      id: string
      text: string
      images: MessageImage[]
      startedAt: string
      delivery?: DeliveryMode
    }
  | { t: 'queueFailed'; sessionID: string; id: string }
  | { t: 'queueStatus'; sessionID: string; id: string; status: 'queued' | 'removing' }
  | { t: 'queueRemove'; sessionID: string; id: string }
  | {
      t: 'contextInvalidate'
      sessionID: string
      provider: string
      model: string
      contextWindow: number
    }
  | { t: 'resolveApproval'; sessionID: string; id: string }
  | { t: 'forget'; sessionID: string }

const emptyUsage = (): Usage => ({
  input: 0,
  output: 0,
  cacheRead: 0,
  cacheWrite: 0,
  totalTokens: 0,
  cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 },
})

const emptyThread = (): ThreadState => ({
  items: [],
  queue: [],
  responseUsage: emptyUsage(),
  contextUsage: undefined,
  running: false,
  autoCompacting: false,
  status: 'connecting',
  seq: 0,
  loaded: false,
})

function lastIndex(items: Item[], pred: (it: Item) => boolean): number {
  for (let i = items.length - 1; i >= 0; i--) if (pred(items[i])) return i
  return -1
}

function replaceAt(items: Item[], index: number, next: Item): Item[] {
  const copy = items.slice()
  copy[index] = next
  return copy
}

function threadsReducer(state: ThreadsState, action: Action): ThreadsState {
  if (action.t === 'forget') {
    const next = { ...state }
    delete next[action.sessionID]
    return next
  }
  const current = state[action.sessionID] ?? emptyThread()
  let next = current

  switch (action.t) {
    case 'reset': {
      next = {
        ...emptyThread(),
        status: current.status,
        running: action.history.running,
        loaded: true,
      }
      for (const ev of action.history.events) next = reduceWire(next, ev)
      for (const ev of action.history.queue ?? []) next = reduceWire(next, ev)
      next = {
        ...next,
        contextUsage: action.history.context,
        running: action.history.running,
        items: action.history.running ? next.items : completeOpenRun(next.items),
      }
      break
    }
    case 'status':
      if (current.status === action.status) return state
      next = { ...current, status: action.status }
      break
    case 'running':
      if (current.running === action.running) return state
      next = {
        ...current,
        running: action.running,
        autoCompacting: action.running ? current.autoCompacting : false,
        items: action.running ? current.items : completeOpenRun(current.items),
      }
      break
    case 'sendUser':
      next = action.delivery
        ? {
            ...current,
            running: true,
            queue: [
              ...current.queue,
              {
                id: action.id,
                text: action.text,
                images: action.images,
                delivery: action.delivery,
                status: 'queued',
              },
            ],
          }
        : {
            ...current,
            seq: current.seq + 1,
            running: true,
            items: [
              ...current.items,
              {
                kind: 'user',
                id: action.id,
                text: action.text,
                images: action.images,
                deliveryStatus: 'sending',
              },
              {
                kind: 'run',
                id: `run-${action.id}`,
                startedAt: action.startedAt,
              },
            ],
          }
      break
    case 'queueFailed':
      next = {
        ...current,
        queue: current.queue.map((message) =>
          message.id === action.id ? { ...message, status: 'failed' } : message,
        ),
        items: current.items.map((item) =>
          item.kind === 'user' && item.id === action.id
            ? { ...item, deliveryStatus: 'failed' }
            : item,
        ),
      }
      break
    case 'queueStatus':
      next = {
        ...current,
        queue: current.queue.map((message) =>
          message.id === action.id ? { ...message, status: action.status } : message,
        ),
      }
      break
    case 'queueRemove':
      next = {
        ...current,
        queue: current.queue.filter((message) => message.id !== action.id),
      }
      break
    case 'contextInvalidate':
      next = {
        ...current,
        contextUsage: {
          provider: action.provider,
          model: action.model,
          usedTokens: 0,
          contextWindow: action.contextWindow,
          measured: false,
        },
      }
      break
    case 'resolveApproval':
      next = {
        ...current,
        items: current.items.filter(
          (item) => !(item.kind === 'approval' && item.id === action.id),
        ),
      }
      break
    case 'wire':
      next = reduceWire(current, action.ev)
      break
  }

  return { ...state, [action.sessionID]: next }
}

function reduceWire(state: ThreadState, ev: WireEvent): ThreadState {
  let items = state.items
  let queue = state.queue
  let responseUsage = state.responseUsage
  let contextUsage = state.contextUsage
  let running = state.running
  let autoCompacting = state.autoCompacting
  let seq = state.seq
  const nextId = () => `i-${seq++}`

  const closeAssistant = () => {
    items = items.map((it) => (it.kind === 'assistant' && it.open ? { ...it, open: false } : it))
  }
  const completeThinking = () => {
    items = items.map((it) =>
      it.kind === 'thinking' && it.streaming ? { ...it, streaming: false } : it,
    )
  }
  const completeRun = (durationMs?: number, startedAt?: string) => {
    let idx = startedAt
      ? lastIndex(items, (item) => item.kind === 'run' && item.startedAt === startedAt)
      : -1
    if (idx < 0) {
      idx = lastIndex(items, (item) => item.kind === 'run' && item.durationMs === undefined)
    }
    if (idx < 0) return
    const run = items[idx] as Extract<Item, { kind: 'run' }>
    if (durationMs === undefined && run.durationMs !== undefined) return
    items = replaceAt(items, idx, {
      ...run,
      durationMs:
        durationMs === undefined
          ? elapsedSince(run.startedAt)
          : Math.max(durationMs, elapsedSince(run.startedAt)),
    })
  }

  switch (ev.type) {
    case 'run_start': {
      const exactIndex = ev.startedAt
        ? lastIndex(items, (it) => it.kind === 'run' && it.startedAt === ev.startedAt)
        : -1
      const idx =
        exactIndex >= 0
          ? exactIndex
          : lastIndex(items, (it) => it.kind === 'run' && it.durationMs === undefined)
      if (idx >= 0) {
        const run = items[idx] as Extract<Item, { kind: 'run' }>
        items = replaceAt(items, idx, {
          ...run,
          startedAt: ev.startedAt ?? run.startedAt,
          durationMs: ev.durationMs ?? run.durationMs,
        })
      } else {
        const run = {
          kind: 'run' as const,
          id: ev.id ?? nextId(),
          startedAt: ev.startedAt ?? new Date().toISOString(),
          durationMs: ev.durationMs,
        }
        items = [...items, run]
      }
      const projectedRun = items[idx >= 0 ? idx : items.length - 1]
      if (projectedRun.kind === 'run' && projectedRun.durationMs === undefined) running = true
      break
    }

    case 'user_message':
      {
        const text = ev.text ?? ''
        const images = ev.images ?? []
        if (ev.queued && ev.delivery) {
          let queueIndex = ev.id ? queue.findIndex((message) => message.id === ev.id) : -1
          if (queueIndex < 0) {
            queueIndex = queue.findIndex((message) =>
              sameUserMessage(message.text, message.images, text, images),
            )
          }
          const message: QueuedMessage = {
            id: ev.id ?? `queued-${nextId()}`,
            text,
            images,
            delivery: ev.delivery,
            status: 'queued',
          }
          queue =
            queueIndex >= 0
              ? replaceQueueAt(queue, queueIndex, message)
              : [...queue, message]
          break
        }

        let queueIndex = ev.id ? queue.findIndex((message) => message.id === ev.id) : -1
        if (queueIndex < 0) {
          queueIndex = queue.findIndex((message) =>
            sameUserMessage(message.text, message.images, text, images),
          )
        }
        if (queueIndex >= 0) queue = queue.filter((_, index) => index !== queueIndex)

        let idx = ev.id
          ? items.findIndex((item) => item.kind === 'user' && item.id === ev.id)
          : -1
        if (idx < 0) {
          idx = items.findIndex(
            (item) =>
              item.kind === 'user' &&
              item.deliveryStatus === 'sending' &&
              sameUserMessage(item.text, item.images, text, images),
          )
        }
        if (idx < 0) {
          const runIndex = lastIndex(items, (item) => item.kind === 'run')
          const candidate = items[runIndex - 1]
          if (
            candidate?.kind === 'user' &&
            sameUserMessage(candidate.text, candidate.images, text, images)
          ) {
            idx = runIndex - 1
          }
        }
        const user = {
          kind: 'user' as const,
          id: ev.id ?? (idx >= 0 ? items[idx].id : nextId()),
          text,
          images,
        }
        if (idx >= 0) {
          items = replaceAt(items, idx, user)
        } else {
          const openRunIndex = lastIndex(
            items,
            (item) => item.kind === 'run' && item.durationMs === undefined,
          )
          items =
            openRunIndex >= 0 && !ev.delivery
              ? [...items.slice(0, openRunIndex), user, ...items.slice(openRunIndex)]
              : [...items, user]
        }
      }
      break

    case 'queue_cancelled':
      if (ev.id) {
        queue = queue.map((message) =>
          message.id === ev.id ? { ...message, status: 'failed' } : message,
        )
      }
      break

    case 'queue_removed':
      if (ev.id) queue = queue.filter((message) => message.id !== ev.id)
      break

    case 'delta':
      if (ev.kind === 'thinking') {
        const idx = lastIndex(items, (it) => it.kind === 'thinking' && it.streaming)
        if (idx >= 0) {
          const cur = items[idx] as Extract<Item, { kind: 'thinking' }>
          items = replaceAt(items, idx, { ...cur, text: cur.text + (ev.delta ?? '') })
        } else {
          items = [
            ...items,
            { kind: 'thinking', id: nextId(), text: ev.delta ?? '', streaming: true },
          ]
        }
      } else {
        completeThinking()
        const idx = lastIndex(items, (it) => it.kind === 'assistant' && it.open)
        if (idx >= 0) {
          const cur = items[idx] as Extract<Item, { kind: 'assistant' }>
          items = replaceAt(items, idx, {
            ...cur,
            markdown: cur.markdown + (ev.delta ?? ''),
          })
        } else {
          items = [
            ...items,
            {
              kind: 'assistant',
              id: nextId(),
              markdown: ev.delta ?? '',
              open: true,
              complete: false,
            },
          ]
        }
      }
      break

    case 'tool_start':
      closeAssistant()
      completeThinking()
      if (!ev.id || !items.some((item) => item.kind === 'tool' && item.id === ev.id)) {
        items = [
          ...items,
          {
            kind: 'tool',
            id: ev.id ?? nextId(),
            name: ev.tool ?? 'tool',
            args: ev.args,
            status: 'running',
          },
        ]
      }
      break

    case 'tool_end': {
      let idx = ev.id ? lastIndex(items, (it) => it.kind === 'tool' && it.id === ev.id) : -1
      if (idx < 0) {
        idx = lastIndex(
          items,
          (it) => it.kind === 'tool' && it.status === 'running' && (!ev.tool || it.name === ev.tool),
        )
      }
      const patch = {
        status: (ev.isError ? 'error' : 'complete') as 'error' | 'complete',
        result: ev.result,
        change: ev.change,
      }
      if (idx >= 0) {
        const cur = items[idx] as Extract<Item, { kind: 'tool' }>
        items = replaceAt(items, idx, { ...cur, ...patch })
      } else {
        items = [
          ...items,
          { kind: 'tool', id: ev.id ?? nextId(), name: ev.tool ?? 'tool', args: undefined, ...patch },
        ]
      }
      break
    }

    case 'message_end':
      completeThinking()
      responseUsage = mergeUsage(responseUsage, ev.usage)
      if (ev.usage) {
        const usedTokens = usageTokens(ev.usage)
        if (usedTokens > 0) {
          contextUsage = {
            provider: contextUsage?.provider ?? '',
            model: contextUsage?.model ?? '',
            usedTokens,
            contextWindow: contextUsage?.contextWindow ?? 0,
            measured: true,
          }
        }
      }
      {
        let idx = lastIndex(items, (it) => it.kind === 'assistant' && it.open)
        if (idx < 0 && ev.text) {
          const runIndex = lastIndex(items, (item) => item.kind === 'run')
          const matchingAssistant = lastIndex(
            items,
            (item) => item.kind === 'assistant' && item.markdown === ev.text,
          )
          if (matchingAssistant > runIndex && matchingAssistant === items.length - 1) {
            idx = matchingAssistant
          }
        }
        if (ev.text) {
          if (idx >= 0) {
            const cur = items[idx] as Extract<Item, { kind: 'assistant' }>
            items = replaceAt(items, idx, { ...cur, markdown: ev.text, open: false })
          } else {
            idx = items.length
            items = [
              ...items,
              {
                kind: 'assistant',
                id: nextId(),
                markdown: ev.text,
                open: false,
                complete: false,
              },
            ]
          }
        } else if (idx >= 0) {
          const cur = items[idx] as Extract<Item, { kind: 'assistant' }>
          items = replaceAt(items, idx, { ...cur, open: false })
        }

        if (ev.finalResponse && idx >= 0) {
          const cur = items[idx] as Extract<Item, { kind: 'assistant' }>
          items = replaceAt(items, idx, {
            ...cur,
            open: false,
            complete: true,
            usage: hasUsage(responseUsage) ? responseUsage : undefined,
            provider: ev.provider,
            model: ev.model,
            modelName: ev.modelName,
          })
          responseUsage = emptyUsage()
        }
      }
      break

    case 'approval_request': {
      completeThinking()
      running = true
      const id = ev.id ?? nextId()
      const idx = lastIndex(items, (it) => it.kind === 'approval' && it.id === id)
      const approval: ApprovalItem = {
        kind: 'approval',
        id,
        summary: ev.summary ?? '',
        reason: ev.reason ?? '',
      }
      items = idx >= 0 ? replaceAt(items, idx, approval) : [...items, approval]
      break
    }

    case 'approval_resolved':
    case 'approval_cancelled':
      if (ev.id) items = items.filter((item) => !(item.kind === 'approval' && item.id === ev.id))
      break

    case 'turn_discard': {
      const assistantIndex = lastIndex(items, (item) => item.kind === 'assistant')
      const boundaryIndex = lastIndex(
        items,
        (item) => item.kind === 'user' || item.kind === 'run' || item.kind === 'tool',
      )
      if (assistantIndex > boundaryIndex) {
        const assistant = items[assistantIndex]
        if (assistant.kind === 'assistant' && assistant.usage) {
          responseUsage = mergeUsage(responseUsage, assistant.usage)
        }
        let start = assistantIndex
        while (start > 0 && items[start - 1].kind === 'thinking') start--
        items = [...items.slice(0, start), ...items.slice(assistantIndex + 1)]
      } else {
        let end = items.length
        while (end > boundaryIndex + 1 && items[end - 1].kind === 'thinking') end--
        if (end < items.length) items = items.slice(0, end)
      }
      break
    }

    case 'compaction_start':
      autoCompacting = true
      break

    case 'compaction_end':
      autoCompacting = false
      if (!ev.isError && contextUsage) {
        contextUsage = { ...contextUsage, usedTokens: 0, measured: false }
      }
      break

    case 'error':
      completeRun(ev.durationMs, ev.startedAt)
      items = [...items, { kind: 'error', id: nextId(), text: ev.text ?? '' }]
      running = false
      autoCompacting = false
      closeAssistant()
      completeThinking()
      responseUsage = emptyUsage()
      break

    case 'done':
      completeRun(ev.durationMs, ev.startedAt)
      running = false
      autoCompacting = false
      closeAssistant()
      completeThinking()
      responseUsage = emptyUsage()
      break
  }

  return { ...state, items, queue, responseUsage, contextUsage, running, autoCompacting, seq }
}

function elapsedSince(startedAt: string): number {
  const start = new Date(startedAt).getTime()
  return Number.isFinite(start) ? Math.max(0, Date.now() - start) : 0
}

function completeOpenRun(items: Item[]): Item[] {
  const index = lastIndex(items, (item) => item.kind === 'run' && item.durationMs === undefined)
  if (index < 0) return items
  const run = items[index] as Extract<Item, { kind: 'run' }>
  return replaceAt(items, index, { ...run, durationMs: elapsedSince(run.startedAt) })
}

function replaceQueueAt(
  queue: QueuedMessage[],
  index: number,
  next: QueuedMessage,
): QueuedMessage[] {
  const copy = queue.slice()
  copy[index] = next
  return copy
}

function sameUserMessage(
  leftText: string,
  leftImages: MessageImage[],
  rightText: string,
  rightImages: MessageImage[],
): boolean {
  if (leftText !== rightText || leftImages.length !== rightImages.length) return false
  return leftImages.every(
    (image, index) =>
      image.mimeType === rightImages[index]?.mimeType && image.data === rightImages[index]?.data,
  )
}

function mergeUsage(current: Usage, next?: Usage): Usage {
  if (!next) return current
  return {
    input: current.input + next.input,
    output: current.output + next.output,
    cacheRead: current.cacheRead + next.cacheRead,
    cacheWrite: current.cacheWrite + next.cacheWrite,
    totalTokens:
      current.totalTokens +
      (next.totalTokens || next.input + next.output + next.cacheRead + next.cacheWrite),
    cost: {
      input: current.cost.input + next.cost.input,
      output: current.cost.output + next.cost.output,
      cacheRead: current.cost.cacheRead + next.cost.cacheRead,
      cacheWrite: current.cost.cacheWrite + next.cost.cacheWrite,
      total: current.cost.total + next.cost.total,
    },
  }
}

function usageTokens(usage: Usage): number {
  return (
    usage.totalTokens ||
    usage.input + usage.output + usage.cacheRead + usage.cacheWrite
  )
}

function hasUsage(usage: Usage): boolean {
  return (
    usage.input !== 0 ||
    usage.output !== 0 ||
    usage.cacheRead !== 0 ||
    usage.cacheWrite !== 0 ||
    usage.totalTokens !== 0 ||
    usage.cost.total !== 0
  )
}

function sessionURL(id: string, path: string): string {
  return apiURL(`/sessions/${encodeURIComponent(id)}${path}`)
}

function promptTitle(text: string): string {
  const compact = text.trim().replace(/\s+/g, ' ')
  const runes = [...compact]
  return runes.length > 42 ? `${runes.slice(0, 42).join('').trim()}…` : compact
}

export type Session = {
  sessions: SessionSummary[]
  workspaces: WorkspaceSummary[]
  draft?: SessionDraft
  activeSession?: SessionSummary
  activeSessionID?: string
  items: Item[]
  queuedMessages: QueuedMessage[]
  contextUsage?: ContextUsage
  approval?: ApprovalItem
  running: boolean
  autoCompacting: boolean
  loading: boolean
  creating: boolean
  updatingSettings: boolean
  compacting: boolean
  status: ConnectionStatus
  models: ModelOption[]
  refreshModels: () => Promise<void>
  registerWorkspace: (path: string) => Promise<WorkspaceSummary>
  removeWorkspace: (path: string) => Promise<void>
  startDraft: (workspacePath?: string, projectScoped?: boolean) => void
  updateDraftWorkspace: (workspacePath?: string, projectScoped?: boolean) => void
  deleteSession: (id: string) => Promise<void>
  renameSession: (id: string, customTitle: string) => Promise<SessionSummary>
  selectSession: (id: string) => void
  updateSettings: (provider: string, model: string, thinkingLevel: ThinkingLevel) => Promise<void>
  compactContext: () => Promise<CompactionResult>
  send: (text: string, images: MessageImage[], delivery?: DeliveryMode) => void
  removeQueuedMessage: (id: string) => Promise<void>
  stop: () => void
  resolveApproval: (id: string, choice: ApprovalChoice) => Promise<void>
}

export type SessionDraft = {
  id: string
  workspacePath?: string
  projectScoped: boolean
  modelProvider?: string
  modelID?: string
  thinkingLevel?: ThinkingLevel
}

type DraftSubmission = {
  sessionID: string
  text: string
  images: MessageImage[]
}

type ModelDefaults = {
  provider: string
  model: string
  thinkingLevel: ThinkingLevel
}

const selectedSessionKey = 'or-coding-active-session'

function newSessionDraft(
  workspacePath?: string,
  projectScoped = false,
  base?: SessionSummary,
  models: ModelOption[] = [],
  defaults?: ModelDefaults,
): SessionDraft {
  // Only the configured default is used. Falling back to whichever model the
  // catalog happens to list first would start a session on a model nobody
  // chose, while the settings page still reports the default as unset — and the
  // server deliberately leaves it unset until someone picks one.
  const fallback = models.find(
    (model) => model.provider === defaults?.provider && model.id === defaults?.model,
  )
  const fallbackThinking =
    (fallback?.provider === defaults?.provider && fallback?.id === defaults?.model
      ? defaults?.thinkingLevel
      : undefined) ??
    fallback?.thinkingLevels.find((level) => level === 'medium') ??
    fallback?.thinkingLevels.find((level) => level !== 'off') ??
    fallback?.thinkingLevels[0]
  return {
    id: crypto.randomUUID(),
    workspacePath,
    projectScoped,
    modelProvider: base?.modelProvider ?? fallback?.provider,
    modelID: base?.modelId ?? fallback?.id,
    thinkingLevel: base?.thinkingLevel ?? fallbackThinking,
  }
}

function resolveSessionDraft(
  draft: SessionDraft,
  models: ModelOption[],
  defaults?: ModelDefaults,
): SessionDraft {
  if (draft.modelProvider && draft.modelID && draft.thinkingLevel) return draft
  return {
    ...newSessionDraft(draft.workspacePath, draft.projectScoped, undefined, models, defaults),
    id: draft.id,
  }
}

export function useSession(): Session {
  const [threads, dispatch] = useReducer(threadsReducer, {})
  const [sessions, setSessions] = useState<SessionSummary[]>([])
  const [workspaces, setWorkspaces] = useState<WorkspaceSummary[]>([])
  const [draft, setDraft] = useState<SessionDraft>()
  const [pendingDraftSend, setPendingDraftSend] = useState<DraftSubmission>()
  const [activeSessionID, setActiveSessionID] = useState<string>()
  const [initializing, setInitializing] = useState(true)
  const [creating, setCreating] = useState(false)
  const [updatingSettings, setUpdatingSettings] = useState(false)
  const [compactingSessionID, setCompactingSessionID] = useState<string>()
  const [models, setModels] = useState<ModelOption[]>([])
  const [modelDefaults, setModelDefaults] = useState<ModelDefaults>()
  const [serviceStatus, setServiceStatus] = useState<ConnectionStatus>('connecting')
  const deletedSessionIDs = useRef(new Set<string>())
  const draftRef = useRef<SessionDraft | undefined>(undefined)

  const loadModels = useCallback(async (signal?: AbortSignal) => {
    try {
      const response = await fetch(apiURL('/models'), { cache: 'no-store', signal })
      if (!response.ok) throw new Error(`model catalog failed (${response.status})`)
      const catalog = (await response.json()) as ModelCatalogResponse
      setModels(catalog.models)
      setModelDefaults(
        catalog.defaultProvider && catalog.defaultModel
          ? {
              provider: catalog.defaultProvider,
              model: catalog.defaultModel,
              thinkingLevel: catalog.defaultThinkingLevel,
            }
          : undefined,
      )
    } catch (error) {
      if (error instanceof DOMException && error.name === 'AbortError') return
      setModels([])
    }
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    void loadModels(controller.signal)
    return () => controller.abort()
  }, [loadModels])

  const refreshSessions = useCallback(async (signal?: AbortSignal) => {
    const response = await fetch(apiURL('/sessions'), { cache: 'no-store', signal })
    if (!response.ok) throw new Error(`session list failed (${response.status})`)
    const received = (await response.json()) as SessionSummary[]
    const next = received.filter((session) => !deletedSessionIDs.current.has(session.id))
    if (next.length === 0 && !draftRef.current) {
      const initialDraft = newSessionDraft()
      draftRef.current = initialDraft
      setDraft(initialDraft)
    }
    setSessions((current) =>
      next.map((remote) => {
        const local = current.find((session) => session.id === remote.id)
        if (!local) return remote
        return new Date(local.updatedAt).getTime() > new Date(remote.updatedAt).getTime()
          ? local
          : remote
      }),
    )
    setActiveSessionID((current) => {
      if (draftRef.current) return undefined
      if (current && next.some((session) => session.id === current)) return current
      const stored = localStorage.getItem(selectedSessionKey)
      if (stored && next.some((session) => session.id === stored)) return stored
      return next[0]?.id
    })
    return next
  }, [])

  const refreshWorkspaces = useCallback(async (signal?: AbortSignal) => {
    const response = await fetch(apiURL('/workspaces'), { cache: 'no-store', signal })
    if (!response.ok) throw new Error(`workspace list failed (${response.status})`)
    const received = (await response.json()) as WorkspaceSummary[]
    setWorkspaces(received)
    return received
  }, [])

  useEffect(() => {
    let controller: AbortController | undefined
    let active = true

    const refresh = (initial = false) => {
      controller?.abort()
      controller = new AbortController()
      void Promise.all([
        refreshSessions(controller.signal),
        refreshWorkspaces(controller.signal),
      ])
        .then(() => setServiceStatus('ready'))
        .catch((error: unknown) => {
          if (error instanceof DOMException && error.name === 'AbortError') return
          setServiceStatus('disconnected')
        })
        .finally(() => {
          if (active && initial) setInitializing(false)
        })
    }

    const refreshWhenVisible = () => {
      if (document.visibilityState === 'visible') refresh()
    }

    const refreshOnFocus = () => refresh()

    refresh(true)
    window.addEventListener('focus', refreshOnFocus)
    document.addEventListener('visibilitychange', refreshWhenVisible)

    return () => {
      active = false
      controller?.abort()
      window.removeEventListener('focus', refreshOnFocus)
      document.removeEventListener('visibilitychange', refreshWhenVisible)
    }
  }, [refreshSessions, refreshWorkspaces])

  useEffect(() => {
    if (!activeSessionID) return
    const sessionID = activeSessionID
    localStorage.setItem(selectedSessionKey, sessionID)
    let active = true
    let es: EventSource | null = null
    let syncing = false
    const controller = new AbortController()
    dispatch({ t: 'status', sessionID, status: 'connecting' })

    const applyWire = (wire: WireEvent) => {
      dispatch({ t: 'wire', sessionID, ev: wire })
      if (wire.type === 'approval_request') {
        setSessions((current) =>
          current.map((session) =>
            session.id === sessionID
              ? { ...session, running: true, hasApproval: true }
              : session,
          ),
        )
      } else if (wire.type === 'approval_resolved' || wire.type === 'approval_cancelled') {
        setSessions((current) =>
          current.map((session) =>
            session.id === sessionID ? { ...session, hasApproval: false } : session,
          ),
        )
      } else if (wire.type === 'done' || wire.type === 'error') {
        setSessions((current) =>
          current.map((session) =>
            session.id === sessionID ? { ...session, running: false } : session,
          ),
        )
      } else if (wire.type === 'title_update') {
        setSessions((current) =>
          current.map((session) =>
            session.id === sessionID
              ? {
                  ...session,
                  title: wire.title ?? session.title,
                  aiTitle: wire.aiTitle,
                  customTitle: wire.customTitle,
                }
              : session,
          ),
        )
      }
    }

    const closeEvents = () => {
      es?.close()
      es = null
    }

    function connect(after: number) {
      if (!active) return
      closeEvents()
      const eventsPath = after > 0 ? `/events?after=${encodeURIComponent(after)}` : '/events'
      const source = new EventSource(sessionURL(sessionID, eventsPath))
      es = source
      source.onopen = () => {
        if (es !== source) return
        dispatch({ t: 'status', sessionID, status: 'ready' })
        setServiceStatus('ready')
      }
      source.onerror = () => {
        if (es !== source) return
        dispatch({ t: 'status', sessionID, status: 'disconnected' })
        setServiceStatus('disconnected')
      }
      source.onmessage = (event) => {
        if (es !== source) return
        try {
          const wire = JSON.parse(event.data) as WireEvent
          if (wire.type === 'sync_required') {
            void restoreHistory(false)
            return
          }
          applyWire(wire)
        } catch {
          applyWire({
            type: 'error',
            text: 'Received an invalid server event.',
          })
        }
      }
    }

    async function restoreHistory(initial: boolean) {
      if (!active || syncing) return
      syncing = true
      closeEvents()
      dispatch({ t: 'status', sessionID, status: 'connecting' })
      try {
        const response = await fetch(sessionURL(sessionID, '/history'), {
          cache: 'no-store',
          signal: controller.signal,
        })
        if (!response.ok) throw new Error(`history request failed (${response.status})`)
        const history = (await response.json()) as HistoryResponse
        if (!active) return
        dispatch({ t: 'reset', sessionID, history })
        const hasApproval = history.events.some((event) => event.type === 'approval_request')
        setSessions((current) =>
          current.map((session) =>
            session.id === sessionID
              ? { ...session, running: history.running, hasApproval }
              : session,
          ),
        )
        connect(history.eventSeq ?? 0)
      } catch (error: unknown) {
        if (!active || (error instanceof DOMException && error.name === 'AbortError')) return
        dispatch({ t: 'status', sessionID, status: 'disconnected' })
        setServiceStatus('disconnected')
        if (initial) {
          dispatch({
            t: 'reset',
            sessionID,
            history: {
              running: false,
              events: [{ type: 'error', text: 'History could not be restored.' }],
            },
          })
          connect(0)
        }
      } finally {
        syncing = false
      }
    }

    void restoreHistory(true)

    return () => {
      active = false
      controller.abort()
      closeEvents()
    }
  }, [activeSessionID])

  const activeSession = sessions.find((session) => session.id === activeSessionID)
  const effectiveDraft = draft ? resolveSessionDraft(draft, models, modelDefaults) : undefined

  const selectSession = (id: string) => {
    if (!sessions.some((session) => session.id === id)) return
    draftRef.current = undefined
    setDraft(undefined)
    setActiveSessionID(id)
  }

  const startDraft = (workspacePath?: string, projectScoped = false) => {
    const next = newSessionDraft(
      workspacePath,
      projectScoped,
      undefined,
      models,
      modelDefaults,
    )
    draftRef.current = next
    setDraft(next)
    setPendingDraftSend(undefined)
    setActiveSessionID(undefined)
  }

  const updateDraftWorkspace = (workspacePath?: string, projectScoped = false) => {
    const current = draftRef.current
    if (!current) return
    const next = {
      ...current,
      workspacePath: projectScoped ? workspacePath : undefined,
      projectScoped,
    }
    draftRef.current = next
    setDraft(next)
  }

  const registerWorkspace = async (path: string) => {
    const response = await fetch(apiURL('/workspaces'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path }),
    })
    if (!response.ok) {
      let message = `register workspace failed (${response.status})`
      try {
        const body = (await response.json()) as { error?: string }
        if (body.error) message = body.error
      } catch {
        // Keep the status-based fallback when the response has no JSON body.
      }
      throw new Error(message)
    }
    const workspace = (await response.json()) as WorkspaceSummary
    setWorkspaces((current) => [
      workspace,
      ...current.filter((candidate) => candidate.path !== workspace.path),
    ])
    return workspace
  }

  const removeWorkspace = async (path: string) => {
    const response = await fetch(
      `${apiURL('/workspaces')}?path=${encodeURIComponent(path)}`,
      { method: 'DELETE' },
    )
    if (!response.ok) {
      let message = `remove workspace failed (${response.status})`
      try {
        const body = (await response.json()) as { error?: string }
        if (body.error) message = body.error
      } catch {
        // Keep the status-based fallback when the response has no JSON body.
      }
      throw new Error(message)
    }
    setWorkspaces((current) => current.filter((workspace) => workspace.path !== path))
  }

  const createSessionRecord = async (
    workspacePath: string | undefined,
    projectScoped: boolean,
    provider: string,
    model: string,
    thinkingLevel: ThinkingLevel,
  ) => {
    const response = await fetch(apiURL('/sessions'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        workspacePath: projectScoped ? workspacePath : undefined,
        scope: projectScoped ? 'project' : 'chat',
        provider,
        model,
        thinkingLevel,
      }),
    })
    if (!response.ok) throw new Error(`create session failed (${response.status})`)
    const created = (await response.json()) as SessionSummary
    setSessions((current) => [created, ...current.filter((session) => session.id !== created.id)])
    if (created.scope === 'project') {
      setWorkspaces((current) => {
        if (current.some((workspace) => workspace.path === created.workspacePath)) return current
        return [
          {
            path: created.workspacePath,
            name: created.workspaceName,
            addedAt: created.createdAt,
          },
          ...current,
        ]
      })
    }
    setActiveSessionID(created.id)
    return created
  }

  const deleteSession = async (id: string) => {
    const response = await fetch(sessionURL(id, ''), { method: 'DELETE' })
    if (!response.ok) {
      let message = `delete session failed (${response.status})`
      try {
        const body = (await response.json()) as { error?: string }
        if (body.error) message = body.error
      } catch {
        // Keep the status-based fallback when the response has no JSON body.
      }
      throw new Error(message)
    }

    deletedSessionIDs.current.add(id)
    dispatch({ t: 'forget', sessionID: id })
    setSessions((current) => current.filter((session) => session.id !== id))
    setActiveSessionID((current) => (current === id ? undefined : current))
    await refreshSessions()
  }

  const renameSession = async (id: string, customTitle: string) => {
    const response = await fetch(sessionURL(id, '/title'), {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ customTitle }),
    })
    if (!response.ok) {
      let message = `rename session failed (${response.status})`
      try {
        const body = (await response.json()) as { error?: string }
        if (body.error) message = body.error
      } catch {
        // Keep the status-based fallback when the response has no JSON body.
      }
      throw new Error(message)
    }
    const updated = (await response.json()) as SessionSummary
    setSessions((current) =>
      current.map((session) => (session.id === updated.id ? updated : session)),
    )
    return updated
  }

  const patchSessionSettings = async (
    sessionID: string,
    provider: string,
    model: string,
    thinkingLevel: ThinkingLevel,
  ) => {
    const response = await fetch(sessionURL(sessionID, '/settings'), {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ provider, model, thinkingLevel }),
    })
    if (!response.ok) {
      let message = `update settings failed (${response.status})`
      try {
        const body = (await response.json()) as { error?: string }
        if (body.error) message = body.error
      } catch {
        // Keep the status-based fallback when the response has no JSON body.
      }
      throw new Error(message)
    }
    return (await response.json()) as SessionSummary
  }

  const updateSettings = async (
    provider: string,
    model: string,
    thinkingLevel: ThinkingLevel,
  ) => {
    if (draftRef.current) {
      const next = { ...draftRef.current, modelProvider: provider, modelID: model, thinkingLevel }
      draftRef.current = next
      setDraft(next)
      return
    }
    if (!activeSessionID || updatingSettings) return
    const sessionID = activeSessionID
    setUpdatingSettings(true)
    try {
      const updated = await patchSessionSettings(sessionID, provider, model, thinkingLevel)
      const previous = sessions.find((session) => session.id === sessionID)
      setSessions((current) => [
        updated,
        ...current.filter((session) => session.id !== updated.id),
      ])
      if (
        previous &&
        (previous.modelProvider !== updated.modelProvider || previous.modelId !== updated.modelId)
      ) {
        const contextWindow =
          models.find(
            (candidate) =>
              candidate.provider === updated.modelProvider && candidate.id === updated.modelId,
          )?.contextWindow ?? 0
        dispatch({
          t: 'contextInvalidate',
          sessionID,
          provider: updated.modelProvider,
          model: updated.modelId,
          contextWindow,
        })
      }
    } finally {
      setUpdatingSettings(false)
    }
  }

  const compactContext = async () => {
    if (!activeSessionID || compactingSessionID || activeSession?.running) {
      throw new Error('session is not idle')
    }
    const sessionID = activeSessionID
    setCompactingSessionID(sessionID)
    try {
      const response = await fetch(sessionURL(sessionID, '/compact'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({}),
      })
      if (!response.ok) {
        let message = `compact context failed (${response.status})`
        let code: string | undefined
        try {
          const body = (await response.json()) as { code?: string; error?: string }
          code = body.code
          if (body.error) message = body.error
        } catch {
          // Keep the status-based fallback when the response has no JSON body.
        }
        throw new APIError(message, code)
      }
      const result = (await response.json()) as CompactionResult
      const current = sessions.find((session) => session.id === sessionID)
      if (current) {
        const contextWindow =
          models.find(
            (model) =>
              model.provider === current.modelProvider && model.id === current.modelId,
          )?.contextWindow ?? 0
        dispatch({
          t: 'contextInvalidate',
          sessionID,
          provider: current.modelProvider,
          model: current.modelId,
          contextWindow,
        })
      }
      void refreshSessions().catch(() => undefined)
      return result
    } finally {
      setCompactingSessionID((current) => (current === sessionID ? undefined : current))
    }
  }

  const thread = activeSessionID ? threads[activeSessionID] : undefined
  const activeSessionRunning = activeSession?.running

  useEffect(() => {
    if (!activeSessionID || activeSessionRunning === undefined || !thread?.loaded) return
    dispatch({
      t: 'running',
      sessionID: activeSessionID,
      running: activeSessionRunning,
    })
  }, [activeSessionID, activeSessionRunning, thread?.loaded])

  useEffect(() => {
    if (
      !pendingDraftSend ||
      pendingDraftSend.sessionID !== activeSessionID ||
      !thread?.loaded ||
      thread.status !== 'ready'
    ) {
      return
    }
    const submission = pendingDraftSend
    setPendingDraftSend(undefined)
    const id = `local-${submission.sessionID}-${crypto.randomUUID()}`
    dispatch({
      t: 'sendUser',
      sessionID: submission.sessionID,
      id,
      text: submission.text,
      images: submission.images,
      startedAt: new Date().toISOString(),
    })
    setSessions((current) =>
      current.map((session) =>
        session.id === submission.sessionID
          ? {
              ...session,
              title:
                session.title === 'New session'
                  ? promptTitle(submission.text || 'Image')
                  : session.title,
              running: true,
              updatedAt: new Date().toISOString(),
            }
          : session,
      ),
    )
    void fetch(sessionURL(submission.sessionID, '/prompt'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ text: submission.text, images: submission.images }),
    })
      .then((response) => {
        if (!response.ok) throw new Error(`prompt request failed (${response.status})`)
      })
      .catch((error: unknown) => {
        dispatch({ t: 'queueFailed', sessionID: submission.sessionID, id })
        dispatch({
          t: 'wire',
          sessionID: submission.sessionID,
          ev: {
            type: 'error',
            text: error instanceof Error ? error.message : 'Prompt request failed.',
          },
        })
        void refreshSessions().catch(() => undefined)
      })
  }, [activeSessionID, pendingDraftSend, refreshSessions, thread?.loaded, thread?.status])

  const send = (text: string, images: MessageImage[], delivery?: DeliveryMode) => {
    const trimmed = text.trim()
    if ((!trimmed && images.length === 0)) return
    if (effectiveDraft) {
      if (delivery || creating || serviceStatus !== 'ready') return
      const requestedDraft = effectiveDraft
      if (
        !requestedDraft.modelProvider ||
        !requestedDraft.modelID ||
        !requestedDraft.thinkingLevel
      ) return
      const provider = requestedDraft.modelProvider
      const model = requestedDraft.modelID
      const thinkingLevel = requestedDraft.thinkingLevel
      setCreating(true)
      void (async () => {
        try {
          const created = await createSessionRecord(
            requestedDraft.workspacePath,
            requestedDraft.projectScoped,
            provider,
            model,
            thinkingLevel,
          )
          draftRef.current = undefined
          setDraft(undefined)
          setPendingDraftSend({ sessionID: created.id, text: trimmed, images })
        } catch {
          // Keep the unsaved draft active so the user can retry the first send.
        } finally {
          setCreating(false)
        }
      })()
      return
    }
    if (!activeSessionID || !thread) return
    if ((!trimmed && images.length === 0) || thread.status !== 'ready') return
    const sessionID = activeSessionID
    const queued = thread.running
    if (queued && !delivery) return
    if (!queued && delivery) return
    const id = `local-${sessionID}-${crypto.randomUUID()}`
    dispatch({
      t: 'sendUser',
      sessionID,
      id,
      text: trimmed,
      images,
      startedAt: new Date().toISOString(),
      delivery,
    })

    if (queued) {
      const endpoint = delivery === 'followup' ? '/follow-up' : '/steer'
      void fetch(sessionURL(sessionID, endpoint), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id, text: trimmed, images }),
      })
        .then(async (response) => {
          if (response.ok) return
          let message = `queue request failed (${response.status})`
          try {
            const body = (await response.json()) as { error?: string }
            if (body.error) message = body.error
          } catch {
            // Keep the status-based fallback when the response has no JSON body.
          }
          throw new Error(message)
        })
        .catch(() => {
          dispatch({ t: 'queueFailed', sessionID, id })
          void refreshSessions().catch(() => undefined)
        })
      return
    }

    setSessions((current) =>
      current.map((session) =>
        session.id === sessionID
          ? {
              ...session,
              title:
                session.title === 'New session' ? promptTitle(trimmed || 'Image') : session.title,
              running: true,
              updatedAt: new Date().toISOString(),
            }
          : session,
      ),
    )
    void fetch(sessionURL(sessionID, '/prompt'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ text: trimmed, images }),
    })
      .then((response) => {
        if (!response.ok) throw new Error(`prompt request failed (${response.status})`)
      })
      .catch((error: unknown) => {
        dispatch({ t: 'queueFailed', sessionID, id })
        dispatch({
          t: 'wire',
          sessionID,
          ev: { type: 'error', text: error instanceof Error ? error.message : 'Prompt request failed.' },
        })
        void refreshSessions().catch(() => undefined)
      })
  }

  const stop = () => {
    if (!activeSessionID) return
    void fetch(sessionURL(activeSessionID, '/abort'), { method: 'POST' })
  }

  const removeQueuedMessage = async (id: string) => {
    if (!activeSessionID || !thread) return
    const message = thread.queue.find((item) => item.id === id)
    if (!message || message.status === 'removing') return
    const sessionID = activeSessionID
    if (message.status === 'failed') {
      dispatch({ t: 'queueRemove', sessionID, id })
      return
    }

    dispatch({ t: 'queueStatus', sessionID, id, status: 'removing' })
    const response = await fetch(sessionURL(sessionID, `/queue/${encodeURIComponent(id)}`), {
      method: 'DELETE',
    })
    if (!response.ok) {
      dispatch({ t: 'queueStatus', sessionID, id, status: 'queued' })
      let message = `remove queued message failed (${response.status})`
      try {
        const body = (await response.json()) as { error?: string }
        if (body.error) message = body.error
      } catch {
        // Keep the status-based fallback when the response has no JSON body.
      }
      throw new Error(message)
    }
    dispatch({ t: 'queueRemove', sessionID, id })
  }

  const resolveApproval = async (id: string, choice: ApprovalChoice) => {
    if (!activeSessionID) throw new Error('no active session')
    const sessionID = activeSessionID
    const response = await fetch(sessionURL(sessionID, `/approvals/${encodeURIComponent(id)}`), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ choice }),
    })
    if (!response.ok) throw new Error('request failed')
    dispatch({ t: 'resolveApproval', sessionID, id })
    setSessions((current) =>
      current.map((session) =>
        session.id === sessionID ? { ...session, hasApproval: false } : session,
      ),
    )
  }

  const approval = thread?.items.findLast(
    (item): item is ApprovalItem => item.kind === 'approval',
  )
  const items = thread?.items.filter((item) => item.kind !== 'approval') ?? []

  return {
    sessions,
    workspaces,
    draft: effectiveDraft,
    activeSession,
    activeSessionID,
    items,
    queuedMessages: thread?.queue ?? [],
    contextUsage: thread?.contextUsage,
    approval,
    running: thread?.running ?? activeSession?.running ?? false,
    autoCompacting: thread?.autoCompacting ?? false,
    loading: initializing || (Boolean(activeSessionID) && !thread?.loaded),
    creating,
    updatingSettings,
    compacting: Boolean(activeSessionID && compactingSessionID === activeSessionID),
    status: thread?.status ?? serviceStatus,
    models,
    refreshModels: () => loadModels(),
    registerWorkspace,
    removeWorkspace,
    startDraft,
    updateDraftWorkspace,
    deleteSession,
    renameSession,
    selectSession,
    updateSettings,
    compactContext,
    send,
    removeQueuedMessage,
    stop,
    resolveApproval,
  }
}
