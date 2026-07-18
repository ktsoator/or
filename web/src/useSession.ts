import { useCallback, useEffect, useReducer, useRef, useState } from 'react'
import { apiURL } from './api'
import type {
  ConfirmItem,
  ConnectionStatus,
  HistoryResponse,
  Item,
  ModelCatalogResponse,
  ModelOption,
  MessageImage,
  SessionSummary,
  ThinkingLevel,
  WireEvent,
} from './types'

type ThreadState = {
  items: Item[]
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
  | { t: 'sendUser'; sessionID: string; text: string; images: MessageImage[] }
  | { t: 'resolve'; sessionID: string; id: string }
  | { t: 'forget'; sessionID: string }

const emptyThread = (): ThreadState => ({
  items: [],
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
      if (action.history.running) next = { ...next, running: true }
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
      next = {
        ...current,
        seq: current.seq + 1,
        running: true,
        items: [
          ...current.items,
          {
            kind: 'user',
            id: `local-${action.sessionID}-${current.seq}`,
            text: action.text,
            images: action.images,
          },
        ],
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
      items = [
        ...items,
        { kind: 'user', id: nextId(), text: ev.text ?? '', images: ev.images ?? [] },
      ]
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
            { kind: 'assistant', id: nextId(), markdown: ev.delta ?? '', open: true },
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
      if (ev.text) {
        const idx = lastIndex(items, (it) => it.kind === 'assistant' && it.open)
        if (idx >= 0) {
          const cur = items[idx] as Extract<Item, { kind: 'assistant' }>
          items = replaceAt(items, idx, { ...cur, markdown: ev.text, open: false })
        } else {
          items = [
            ...items,
            { kind: 'assistant', id: nextId(), markdown: ev.text, open: false },
          ]
        }
      } else {
        closeAssistant()
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
      break

    case 'done':
      running = false
      closeAssistant()
      completeThinking()
      break
  }

  return { ...state, items, running, seq }
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
  send: (text: string, images: MessageImage[]) => void
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
      setSessions((current) => [
        updated,
        ...current.filter((session) => session.id !== updated.id),
      ])
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

  const send = (text: string, images: MessageImage[]) => {
    if (!activeSessionID || !thread) return
    const trimmed = text.trim()
    if ((!trimmed && images.length === 0) || thread.running || thread.status !== 'ready') return
    const sessionID = activeSessionID
    dispatch({ t: 'sendUser', sessionID, text: trimmed, images })
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
    stop,
    resolveConfirm,
  }
}
