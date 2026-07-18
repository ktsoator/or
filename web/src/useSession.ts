import { useCallback, useEffect, useReducer, useRef, useState } from 'react'
import { apiURL } from './api'
import type {
  ConfirmItem,
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
  | { t: 'resolve'; sessionID: string; id: string }
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
        running: action.history.running || next.running,
      }
      break
    }
    case 'status':
      if (current.status === action.status) return state
      next = { ...current, status: action.status }
      break
    case 'running':
      if (current.running === action.running) return state
      next = { ...current, running: action.running }
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
    case 'resolve':
      next = {
        ...current,
        items: current.items.filter(
          (item) => !(item.kind === 'confirm' && item.id === action.id),
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

  switch (ev.type) {
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
        const user = {
          kind: 'user' as const,
          id: ev.id ?? (idx >= 0 ? items[idx].id : nextId()),
          text,
          images,
        }
        items = idx >= 0 ? replaceAt(items, idx, user) : [...items, user]
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
          })
          responseUsage = emptyUsage()
        }
      }
      break

    case 'confirm_request': {
      completeThinking()
      running = true
      const id = ev.id ?? nextId()
      const idx = lastIndex(items, (it) => it.kind === 'confirm' && it.id === id)
      const confirm: ConfirmItem = { kind: 'confirm', id, summary: ev.summary ?? '' }
      items = idx >= 0 ? replaceAt(items, idx, confirm) : [...items, confirm]
      break
    }

    case 'error':
      items = [...items, { kind: 'error', id: nextId(), text: ev.text ?? '' }]
      running = false
      closeAssistant()
      completeThinking()
      responseUsage = emptyUsage()
      break

    case 'done':
      running = false
      closeAssistant()
      completeThinking()
      responseUsage = emptyUsage()
      break
  }

  return { ...state, items, queue, responseUsage, contextUsage, running, seq }
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
  activeSession?: SessionSummary
  activeSessionID?: string
  items: Item[]
  queuedMessages: QueuedMessage[]
  contextUsage?: ContextUsage
  confirmation?: ConfirmItem
  running: boolean
  loading: boolean
  creating: boolean
  updatingSettings: boolean
  status: ConnectionStatus
  models: ModelOption[]
  createSession: () => Promise<void>
  deleteSession: (id: string) => Promise<void>
  selectSession: (id: string) => void
  updateSettings: (provider: string, model: string, thinkingLevel: ThinkingLevel) => Promise<void>
  send: (text: string, images: MessageImage[], delivery?: DeliveryMode) => void
  removeQueuedMessage: (id: string) => Promise<void>
  stop: () => void
  resolveConfirm: (id: string, allow: boolean) => Promise<void>
}

const selectedSessionKey = 'or-coding-active-session'

export function useSession(): Session {
  const [threads, dispatch] = useReducer(threadsReducer, {})
  const [sessions, setSessions] = useState<SessionSummary[]>([])
  const [activeSessionID, setActiveSessionID] = useState<string>()
  const [initializing, setInitializing] = useState(true)
  const [creating, setCreating] = useState(false)
  const [updatingSettings, setUpdatingSettings] = useState(false)
  const [models, setModels] = useState<ModelOption[]>([])
  const [serviceStatus, setServiceStatus] = useState<ConnectionStatus>('connecting')
  const deletedSessionIDs = useRef(new Set<string>())

  useEffect(() => {
    const controller = new AbortController()
    void fetch(apiURL('/models'), { cache: 'no-store', signal: controller.signal })
      .then((response) =>
        response.ok
          ? response.json()
          : Promise.reject(new Error(`model catalog failed (${response.status})`)),
      )
      .then((catalog: ModelCatalogResponse) => setModels(catalog.models))
      .catch((error: unknown) => {
        if (error instanceof DOMException && error.name === 'AbortError') return
        setModels([])
      })
    return () => controller.abort()
  }, [])

  const refreshSessions = useCallback(async (signal?: AbortSignal) => {
    const response = await fetch(apiURL('/sessions'), { cache: 'no-store', signal })
    if (!response.ok) throw new Error(`session list failed (${response.status})`)
    const received = (await response.json()) as SessionSummary[]
    const next = received.filter((session) => !deletedSessionIDs.current.has(session.id))
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
      if (current && next.some((session) => session.id === current)) return current
      const stored = localStorage.getItem(selectedSessionKey)
      if (stored && next.some((session) => session.id === stored)) return stored
      return next[0]?.id
    })
    return next
  }, [])

  useEffect(() => {
    let controller: AbortController | undefined
    let active = true

    const refresh = (initial = false) => {
      controller?.abort()
      controller = new AbortController()
      void refreshSessions(controller.signal).catch((error: unknown) => {
        if (error instanceof DOMException && error.name === 'AbortError') return
        setServiceStatus('disconnected')
      }).finally(() => {
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
  }, [refreshSessions])

  useEffect(() => {
    if (!activeSessionID) return
    localStorage.setItem(selectedSessionKey, activeSessionID)
    let active = true
    let es: EventSource | null = null
    const controller = new AbortController()
    dispatch({ t: 'status', sessionID: activeSessionID, status: 'connecting' })

    const connect = () => {
      if (!active) return
      es = new EventSource(sessionURL(activeSessionID, '/events'))
      es.onopen = () => {
        dispatch({ t: 'status', sessionID: activeSessionID, status: 'ready' })
        setServiceStatus('ready')
      }
      es.onerror = () => {
        dispatch({ t: 'status', sessionID: activeSessionID, status: 'disconnected' })
        setServiceStatus('disconnected')
      }
      es.onmessage = (event) => {
        try {
          const wire = JSON.parse(event.data) as WireEvent
          dispatch({ t: 'wire', sessionID: activeSessionID, ev: wire })
          if (wire.type === 'confirm_request') {
            setSessions((current) =>
              current.map((session) =>
                session.id === activeSessionID
                  ? { ...session, running: true, hasApproval: true }
                  : session,
              ),
            )
          } else if (wire.type === 'done' || wire.type === 'error') {
            setSessions((current) =>
              current.map((session) =>
                session.id === activeSessionID ? { ...session, running: false } : session,
              ),
            )
          }
        } catch {
          dispatch({
            t: 'wire',
            sessionID: activeSessionID,
            ev: { type: 'error', text: 'Received an invalid server event.' },
          })
        }
      }
    }

    fetch(sessionURL(activeSessionID, '/history'), {
      cache: 'no-store',
      signal: controller.signal,
    })
      .then((response) =>
        response.ok
          ? response.json()
          : Promise.reject(new Error(`history request failed (${response.status})`)),
      )
      .then((history: HistoryResponse) => {
        if (active) dispatch({ t: 'reset', sessionID: activeSessionID, history })
      })
      .catch((error: unknown) => {
        if (!active || (error instanceof DOMException && error.name === 'AbortError')) return
        dispatch({
          t: 'reset',
          sessionID: activeSessionID,
          history: {
            running: false,
            events: [{ type: 'error', text: 'History could not be restored.' }],
          },
        })
      })
      .finally(connect)

    return () => {
      active = false
      controller.abort()
      es?.close()
    }
  }, [activeSessionID])

  const selectSession = (id: string) => {
    if (sessions.some((session) => session.id === id)) setActiveSessionID(id)
  }

  const createSession = async () => {
    if (creating) return
    setCreating(true)
    try {
      const response = await fetch(apiURL('/sessions'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({}),
      })
      if (!response.ok) throw new Error(`create session failed (${response.status})`)
      const created = (await response.json()) as SessionSummary
      setSessions((current) => [created, ...current.filter((session) => session.id !== created.id)])
      setActiveSessionID(created.id)
    } finally {
      setCreating(false)
    }
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

  const updateSettings = async (
    provider: string,
    model: string,
    thinkingLevel: ThinkingLevel,
  ) => {
    if (!activeSessionID || updatingSettings) return
    const sessionID = activeSessionID
    setUpdatingSettings(true)
    try {
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
      const updated = (await response.json()) as SessionSummary
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

  const activeSession = sessions.find((session) => session.id === activeSessionID)
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

  const send = (text: string, images: MessageImage[], delivery?: DeliveryMode) => {
    if (!activeSessionID || !thread) return
    const trimmed = text.trim()
    if ((!trimmed && images.length === 0) || thread.status !== 'ready') return
    const sessionID = activeSessionID
    const queued = thread.running
    if (queued && !delivery) return
    if (!queued && delivery) return
    const id = `local-${sessionID}-${crypto.randomUUID()}`
    dispatch({ t: 'sendUser', sessionID, id, text: trimmed, images, delivery })

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

  const resolveConfirm = async (id: string, allow: boolean) => {
    if (!activeSessionID) throw new Error('no active session')
    const sessionID = activeSessionID
    const response = await fetch(sessionURL(sessionID, '/confirm'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id, allow }),
    })
    if (!response.ok) throw new Error('request failed')
    dispatch({ t: 'resolve', sessionID, id })
    setSessions((current) =>
      current.map((session) =>
        session.id === sessionID ? { ...session, hasApproval: false } : session,
      ),
    )
  }

  const confirmation = thread?.items.findLast(
    (item): item is ConfirmItem => item.kind === 'confirm',
  )
  const items = thread?.items.filter((item) => item.kind !== 'confirm') ?? []

  return {
    sessions,
    activeSession,
    activeSessionID,
    items,
    queuedMessages: thread?.queue ?? [],
    contextUsage: thread?.contextUsage,
    confirmation,
    running: thread?.running ?? activeSession?.running ?? false,
    loading: initializing || (Boolean(activeSessionID) && !thread?.loaded),
    creating,
    updatingSettings,
    status: thread?.status ?? serviceStatus,
    models,
    createSession,
    deleteSession,
    selectSession,
    updateSettings,
    send,
    removeQueuedMessage,
    stop,
    resolveConfirm,
  }
}
