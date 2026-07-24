import { useEffect } from 'react'
import { sessionURL } from './api'
import type { ConnectionStatus, HistoryResponse, ThreadSnapshot, WireEvent } from './types'

export type SessionConnectionHandlers = {
  onWire: (sessionID: string, event: WireEvent) => void
  onSnapshot: (sessionID: string, history: ThreadSnapshot) => void
  onStatus: (sessionID: string, status: ConnectionStatus) => void
}

export type SessionEventSource = {
  onopen: ((event: Event) => void) | null
  onerror: ((event: Event) => void) | null
  onmessage: ((event: MessageEvent<string>) => void) | null
  close: () => void
}

export type SessionConnectionDependencies = {
  request: (url: string, init: RequestInit) => Promise<Response>
  openEvents: (url: string) => SessionEventSource
}

const browserDependencies: SessionConnectionDependencies = {
  request: (url, init) => fetch(url, init),
  openEvents: (url) => new EventSource(url),
}

export function startSessionConnection(
  sessionID: string,
  handlers: SessionConnectionHandlers,
  dependencies: SessionConnectionDependencies = browserDependencies,
): () => void {
  let active = true
  let events: SessionEventSource | null = null
  let syncing = false
  const controller = new AbortController()

  const updateStatus = (status: ConnectionStatus) => {
    handlers.onStatus(sessionID, status)
  }

  const closeEvents = () => {
    events?.close()
    events = null
  }

  function connect(after: number) {
    if (!active) return
    closeEvents()
    const eventsPath = after > 0 ? `/events?after=${encodeURIComponent(after)}` : '/events'
    const source = dependencies.openEvents(sessionURL(sessionID, eventsPath))
    events = source
    source.onopen = () => {
      if (events !== source) return
      updateStatus('ready')
    }
    source.onerror = () => {
      if (events !== source) return
      updateStatus('disconnected')
    }
    source.onmessage = (event) => {
      if (events !== source) return
      try {
        const wire = JSON.parse(event.data) as WireEvent
        if (wire.type === 'sync_required') {
          void restoreHistory(false)
          return
        }
        handlers.onWire(sessionID, wire)
      } catch {
        handlers.onWire(sessionID, {
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
    updateStatus('connecting')
    try {
      const response = await dependencies.request(sessionURL(sessionID, '/history'), {
        cache: 'no-store',
        signal: controller.signal,
      })
      if (!response.ok) throw new Error(`history request failed (${response.status})`)
      const history = (await response.json()) as HistoryResponse
      if (!active) return
      handlers.onSnapshot(sessionID, history)
      connect(history.eventSeq ?? 0)
    } catch (error: unknown) {
      if (!active || (error instanceof DOMException && error.name === 'AbortError')) return
      updateStatus('disconnected')
      if (initial) {
        handlers.onSnapshot(sessionID, {
          running: false,
          events: [{ type: 'error', text: 'History could not be restored.' }],
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
}

export function useSessionConnection(
  sessionID: string | undefined,
  handlers: SessionConnectionHandlers,
) {
  const { onWire, onSnapshot, onStatus } = handlers

  useEffect(() => {
    if (!sessionID) return
    return startSessionConnection(sessionID, { onWire, onSnapshot, onStatus })
  }, [onSnapshot, onStatus, onWire, sessionID])
}
