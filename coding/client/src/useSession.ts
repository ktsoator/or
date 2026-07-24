import {
  useCallback,
  useEffect,
  useReducer,
  useState,
} from 'react'
import { APIError, apiURL, sessionURL } from './api'
import { sessionCommands } from './sessionCommands'
import { useSessionConnection } from './sessionConnection'
import { threadsReducer } from './sessionReducer'
import {
  createSessionDraft,
  createSessionStoreState,
  resolveSessionDraft,
  sessionStoreReducer,
  type ModelDefaults,
  type SessionDraft,
} from './sessionStore'
import type {
  ApprovalChoice,
  ApprovalItem,
  BrowserCommandState,
  CompactionResult,
  ConnectionStatus,
  ContextUsage,
  DeliveryMode,
  Item,
  ModelCatalogResponse,
  ModelOption,
  PermissionMode,
  PreviewState,
  MessageImage,
  QueuedMessage,
  SessionSummary,
  ThreadSnapshot,
  WorkspaceSummary,
  ThinkingLevel,
  WireEvent,
} from './types'

export type { SessionDraft } from './sessionStore'

export type Session = {
  sessions: SessionSummary[]
  workspaces: WorkspaceSummary[]
  draft?: SessionDraft
  activeSession?: SessionSummary
  activeSessionID?: string
  items: Item[]
  queuedMessages: QueuedMessage[]
  contextUsage?: ContextUsage
  preview?: PreviewState
  browserCommands: BrowserCommandState[]
  previewOpen: boolean
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
  createChatSession: () => Promise<SessionSummary>
  updateDraftWorkspace: (workspacePath?: string, projectScoped?: boolean) => void
  deleteSession: (id: string) => Promise<void>
  renameSession: (id: string, customTitle: string) => Promise<SessionSummary>
  selectSession: (id: string) => void
  updateSettings: (provider: string, model: string, thinkingLevel: ThinkingLevel) => Promise<void>
  updatePermissionMode: (mode: PermissionMode) => Promise<void>
  compactContext: () => Promise<CompactionResult>
  send: (text: string, images: MessageImage[], delivery?: DeliveryMode) => Promise<boolean>
  removeQueuedMessage: (id: string) => Promise<void>
  stop: () => void
  resolveApproval: (id: string, choice: ApprovalChoice) => Promise<void>
  handleBrowserCommand: (sessionID: string, id: string) => void
  secondaryThread?: SessionThread
}

export type SessionThread = {
  session: SessionSummary
  items: Item[]
  queuedMessages: QueuedMessage[]
  contextUsage?: ContextUsage
  preview?: PreviewState
  browserCommands: BrowserCommandState[]
  previewOpen: boolean
  approval?: ApprovalItem
  running: boolean
  autoCompacting: boolean
  loading: boolean
  updatingSettings: boolean
  compacting: boolean
  status: ConnectionStatus
  send: (text: string, images: MessageImage[], delivery?: DeliveryMode) => Promise<boolean>
  removeQueuedMessage: (id: string) => Promise<void>
  stop: () => void
  resolveApproval: (id: string, choice: ApprovalChoice) => Promise<void>
  updateSettings: (provider: string, model: string, thinkingLevel: ThinkingLevel) => Promise<void>
  updatePermissionMode: (mode: PermissionMode) => Promise<void>
  compactContext: () => Promise<CompactionResult>
}

const selectedSessionKey = 'or-coding-active-session'

export function useSession(secondarySessionID?: string): Session {
  const [threads, dispatch] = useReducer(threadsReducer, {})
  const [sessionStore, dispatchSessionStore] = useReducer(
    sessionStoreReducer,
    createSessionStoreState(),
  )
  const { sessions, workspaces, draft, pendingDraftSend, activeSessionID } = sessionStore
  const [initializing, setInitializing] = useState(true)
  const [creating, setCreating] = useState(false)
  const [updatingSettings, setUpdatingSettings] = useState(false)
  const [compactingSessionID, setCompactingSessionID] = useState<string>()
  const [models, setModels] = useState<ModelOption[]>([])
  const [modelDefaults, setModelDefaults] = useState<ModelDefaults>()
  const [serviceStatus, setServiceStatus] = useState<ConnectionStatus>('connecting')

  const applySessionWire = useCallback((sessionID: string, wire: WireEvent) => {
    dispatch({ t: 'wire', sessionID, ev: wire })
    dispatchSessionStore({ t: 'sessionWire', sessionID, event: wire })
  }, [])

  const applySessionSnapshot = useCallback((sessionID: string, history: ThreadSnapshot) => {
    dispatch({ t: 'reset', sessionID, history })
    dispatchSessionStore({ t: 'sessionSnapshot', sessionID, history })
  }, [])

  const applySessionStatus = useCallback(
    (sessionID: string, status: ConnectionStatus) => {
      dispatch({ t: 'status', sessionID, status })
    },
    [],
  )

  const applyPrimarySessionStatus = useCallback(
    (sessionID: string, status: ConnectionStatus) => {
      dispatch({ t: 'status', sessionID, status })
      if (status !== 'connecting') setServiceStatus(status)
    },
    [],
  )

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
    dispatchSessionStore({
      t: 'sessionsLoaded',
      sessions: received,
      storedSessionID: localStorage.getItem(selectedSessionKey) ?? undefined,
      emptyDraft: createSessionDraft(),
    })
    return received
  }, [])

  const refreshWorkspaces = useCallback(async (signal?: AbortSignal) => {
    const response = await fetch(apiURL('/workspaces'), { cache: 'no-store', signal })
    if (!response.ok) throw new Error(`workspace list failed (${response.status})`)
    const received = (await response.json()) as WorkspaceSummary[]
    dispatchSessionStore({ t: 'workspacesLoaded', workspaces: received })
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
    if (activeSessionID) localStorage.setItem(selectedSessionKey, activeSessionID)
  }, [activeSessionID])

  useSessionConnection(activeSessionID, {
    onWire: applySessionWire,
    onSnapshot: applySessionSnapshot,
    onStatus: applyPrimarySessionStatus,
  })
  useSessionConnection(
    secondarySessionID && secondarySessionID !== activeSessionID
      ? secondarySessionID
      : undefined,
    {
      onWire: applySessionWire,
      onSnapshot: applySessionSnapshot,
      onStatus: applySessionStatus,
    },
  )

  const activeSession = sessions.find((session) => session.id === activeSessionID)
  const effectiveDraft = draft ? resolveSessionDraft(draft, models, modelDefaults) : undefined

  const selectSession = (id: string) => {
    dispatchSessionStore({ t: 'sessionSelected', sessionID: id })
  }

  const startDraft = (workspacePath?: string, projectScoped = false) => {
    dispatchSessionStore({
      t: 'draftStarted',
      draft: createSessionDraft(
        workspacePath,
        projectScoped,
        undefined,
        models,
        modelDefaults,
      ),
    })
  }

  const updateDraftWorkspace = (workspacePath?: string, projectScoped = false) => {
    dispatchSessionStore({ t: 'draftWorkspaceUpdated', workspacePath, projectScoped })
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
    dispatchSessionStore({ t: 'workspaceUpserted', workspace })
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
    dispatchSessionStore({ t: 'workspaceRemoved', path })
  }

  const createSessionRecord = async (
    workspacePath: string | undefined,
    projectScoped: boolean,
    provider: string,
    model: string,
    thinkingLevel: ThinkingLevel,
    permissionMode: PermissionMode,
    select = true,
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
        permissionMode,
      }),
    })
    if (!response.ok) {
      let message = `create session failed (${response.status})`
      try {
        const body = (await response.json()) as { error?: string }
        if (body.error) message = body.error
      } catch {
        // Keep the status-based fallback when the response has no JSON body.
      }
      throw new Error(message)
    }
    const created = (await response.json()) as SessionSummary
    dispatchSessionStore({ t: 'sessionCreated', session: created, select })
    return created
  }

  const createChatSession = async () => {
    if (creating) throw new Error('session creation is already in progress')
    const settings = effectiveDraft ?? createSessionDraft(
      undefined,
      false,
      activeSession,
      models,
      modelDefaults,
    )
    if (!settings.modelProvider || !settings.modelID || !settings.thinkingLevel) {
      throw new Error('configure a model before creating a session')
    }
    setCreating(true)
    try {
      return await createSessionRecord(
        undefined,
        false,
        settings.modelProvider,
        settings.modelID,
        settings.thinkingLevel,
        settings.permissionMode,
        false,
      )
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

    dispatch({ t: 'forget', sessionID: id })
    dispatchSessionStore({ t: 'sessionDeleted', sessionID: id })
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
    dispatchSessionStore({ t: 'sessionUpdated', session: updated, front: false })
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

  const updateSessionSettings = async (
    sessionID: string,
    provider: string,
    model: string,
    thinkingLevel: ThinkingLevel,
  ) => {
    if (updatingSettings) return
    setUpdatingSettings(true)
    try {
      const updated = await patchSessionSettings(sessionID, provider, model, thinkingLevel)
      const previous = sessions.find((session) => session.id === sessionID)
      dispatchSessionStore({ t: 'sessionUpdated', session: updated, front: true })
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

  const updateSettings = async (
    provider: string,
    model: string,
    thinkingLevel: ThinkingLevel,
  ) => {
    if (draft) {
      dispatchSessionStore({
        t: 'draftModelUpdated',
        provider,
        model,
        thinkingLevel,
      })
      return
    }
    if (!activeSessionID) return
    await updateSessionSettings(activeSessionID, provider, model, thinkingLevel)
  }

  const updateSessionPermissionMode = async (sessionID: string, mode: PermissionMode) => {
    if (updatingSettings) return
    setUpdatingSettings(true)
    try {
      const response = await fetch(sessionURL(sessionID, '/permission-mode'), {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ mode }),
      })
      if (!response.ok) {
        let message = `update permission mode failed (${response.status})`
        try {
          const body = (await response.json()) as { error?: string }
          if (body.error) message = body.error
        } catch {
          // Keep the status-based fallback when the response has no JSON body.
        }
        throw new Error(message)
      }
      const updated = (await response.json()) as SessionSummary
      dispatchSessionStore({ t: 'sessionUpdated', session: updated, front: true })
    } finally {
      setUpdatingSettings(false)
    }
  }

  const updatePermissionMode = async (mode: PermissionMode) => {
    if (draft) {
      dispatchSessionStore({ t: 'draftPermissionUpdated', permissionMode: mode })
      return
    }
    if (!activeSessionID) return
    await updateSessionPermissionMode(activeSessionID, mode)
  }

  const compactSessionContext = async (sessionID: string) => {
    const target = sessions.find((session) => session.id === sessionID)
    if (compactingSessionID || target?.running) {
      throw new Error('session is not idle')
    }
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

  const compactContext = async () => {
    if (!activeSessionID) throw new Error('session is not idle')
    return compactSessionContext(activeSessionID)
  }

  const thread = activeSessionID ? threads[activeSessionID] : undefined
  const activeSessionRunning = activeSession?.running

  const startSessionPrompt = useCallback(
    (sessionID: string, id: string, text: string, images: MessageImage[]) => {
      dispatch({
        t: 'sendUser',
        sessionID,
        id,
        text,
        images,
        startedAt: new Date().toISOString(),
      })
      dispatchSessionStore({
        t: 'sessionPromptStarted',
        sessionID,
        text,
        updatedAt: new Date().toISOString(),
      })
      void sessionCommands.sendPrompt(sessionID, { text, images }).catch((error: unknown) => {
        dispatch({ t: 'queueFailed', sessionID, id })
        dispatch({
          t: 'wire',
          sessionID,
          ev: {
            type: 'error',
            text: error instanceof Error ? error.message : 'Prompt request failed.',
          },
        })
        void refreshSessions().catch(() => undefined)
      })
    },
    [refreshSessions],
  )

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
    dispatchSessionStore({ t: 'draftSendConsumed', sessionID: submission.sessionID })
    const id = `local-${submission.sessionID}-${crypto.randomUUID()}`
    startSessionPrompt(submission.sessionID, id, submission.text, submission.images)
  }, [activeSessionID, pendingDraftSend, startSessionPrompt, thread?.loaded, thread?.status])

  const sendToSession = async (
    sessionID: string,
    text: string,
    images: MessageImage[],
    delivery?: DeliveryMode,
  ): Promise<boolean> => {
    const trimmed = text.trim()
    const targetThread = threads[sessionID]
    if ((!trimmed && images.length === 0) || targetThread?.status !== 'ready') return false
    const queued = targetThread.running
    if (queued && !delivery) return false
    if (!queued && delivery) return false
    const id = `local-${sessionID}-${crypto.randomUUID()}`

    if (queued) {
      if (!delivery) return false
      dispatch({
        t: 'sendUser',
        sessionID,
        id,
        text: trimmed,
        images,
        startedAt: new Date().toISOString(),
        delivery,
      })
      void sessionCommands
        .enqueueMessage(sessionID, delivery, { id, text: trimmed, images })
        .catch(() => {
          dispatch({ t: 'queueFailed', sessionID, id })
          void refreshSessions().catch(() => undefined)
        })
      return true
    }

    startSessionPrompt(sessionID, id, trimmed, images)
    return true
  }

  const send = async (
    text: string,
    images: MessageImage[],
    delivery?: DeliveryMode,
  ): Promise<boolean> => {
    const trimmed = text.trim()
    if ((!trimmed && images.length === 0)) return false
    if (effectiveDraft) {
      if (delivery || creating || serviceStatus !== 'ready') return false
      const requestedDraft = effectiveDraft
      if (
        !requestedDraft.modelProvider ||
        !requestedDraft.modelID ||
        !requestedDraft.thinkingLevel
      ) return false
      const provider = requestedDraft.modelProvider
      const model = requestedDraft.modelID
      const thinkingLevel = requestedDraft.thinkingLevel
      const permissionMode = requestedDraft.permissionMode
      setCreating(true)
      try {
        const created = await createSessionRecord(
          requestedDraft.workspacePath,
          requestedDraft.projectScoped,
          provider,
          model,
          thinkingLevel,
          permissionMode,
        )
        dispatchSessionStore({
          t: 'draftSendQueued',
          submission: { sessionID: created.id, text: trimmed, images },
        })
        return true
      } finally {
        setCreating(false)
      }
    }
    if (!activeSessionID) return false
    return sendToSession(activeSessionID, trimmed, images, delivery)
  }

  const stopSession = (sessionID: string) => {
    void sessionCommands.abortRun(sessionID).catch(() => undefined)
  }

  const stop = () => {
    if (activeSessionID) stopSession(activeSessionID)
  }

  const removeSessionQueuedMessage = async (sessionID: string, id: string) => {
    const targetThread = threads[sessionID]
    if (!targetThread) return
    const message = targetThread.queue.find((item) => item.id === id)
    if (!message || message.status === 'removing') return
    if (message.status === 'failed') {
      dispatch({ t: 'queueRemove', sessionID, id })
      return
    }

    dispatch({ t: 'queueStatus', sessionID, id, status: 'removing' })
    try {
      await sessionCommands.removeQueuedMessage(sessionID, id)
      dispatch({ t: 'queueRemove', sessionID, id })
    } catch (error) {
      dispatch({ t: 'queueStatus', sessionID, id, status: 'queued' })
      throw error
    }
  }

  const removeQueuedMessage = async (id: string) => {
    if (activeSessionID) await removeSessionQueuedMessage(activeSessionID, id)
  }

  const resolveSessionApproval = async (
    sessionID: string,
    id: string,
    choice: ApprovalChoice,
  ) => {
    await sessionCommands.resolveApproval(sessionID, id, choice)
    dispatch({ t: 'resolveApproval', sessionID, id })
    dispatchSessionStore({ t: 'sessionApprovalResolved', sessionID })
  }

  const resolveApproval = async (id: string, choice: ApprovalChoice) => {
    if (!activeSessionID) throw new Error('no active session')
    await resolveSessionApproval(activeSessionID, id, choice)
  }

  const approval = thread?.items.findLast(
    (item): item is ApprovalItem => item.kind === 'approval',
  )
  const items = thread?.items.filter((item) => item.kind !== 'approval') ?? []

  const secondarySession = sessions.find((session) => session.id === secondarySessionID)
  const secondaryState = secondarySessionID ? threads[secondarySessionID] : undefined
  const secondaryApproval = secondaryState?.items.findLast(
    (item): item is ApprovalItem => item.kind === 'approval',
  )
  const secondaryThread = secondarySession
    ? {
        session: secondarySession,
        items: secondaryState?.items.filter((item) => item.kind !== 'approval') ?? [],
        queuedMessages: secondaryState?.queue ?? [],
        contextUsage: secondaryState?.contextUsage,
        preview: secondaryState?.preview,
        browserCommands: secondaryState?.browserCommands ?? [],
        previewOpen: secondaryState?.previewOpen ?? false,
        approval: secondaryApproval,
        running: secondaryState?.running ?? secondarySession.running,
        autoCompacting: secondaryState?.autoCompacting ?? false,
        loading: !secondaryState?.loaded,
        updatingSettings,
        compacting: compactingSessionID === secondarySession.id,
        status: secondaryState?.status ?? serviceStatus,
        send: (text: string, images: MessageImage[], delivery?: DeliveryMode) =>
          sendToSession(secondarySession.id, text, images, delivery),
        removeQueuedMessage: (id: string) =>
          removeSessionQueuedMessage(secondarySession.id, id),
        stop: () => stopSession(secondarySession.id),
        resolveApproval: (id: string, choice: ApprovalChoice) =>
          resolveSessionApproval(secondarySession.id, id, choice),
        updateSettings: (provider: string, model: string, thinkingLevel: ThinkingLevel) =>
          updateSessionSettings(secondarySession.id, provider, model, thinkingLevel),
        updatePermissionMode: (mode: PermissionMode) =>
          updateSessionPermissionMode(secondarySession.id, mode),
        compactContext: () => compactSessionContext(secondarySession.id),
      }
    : undefined

  return {
    sessions,
    workspaces,
    draft: effectiveDraft,
    activeSession,
    activeSessionID,
    items,
    queuedMessages: thread?.queue ?? [],
    contextUsage: thread?.contextUsage,
    preview: thread?.preview,
    browserCommands: thread?.browserCommands ?? [],
    previewOpen: thread?.previewOpen ?? false,
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
    createChatSession,
    updateDraftWorkspace,
    deleteSession,
    renameSession,
    selectSession,
    updateSettings,
    updatePermissionMode,
    compactContext,
    send,
    removeQueuedMessage,
    stop,
    resolveApproval,
    handleBrowserCommand: (sessionID: string, id: string) =>
      dispatch({ t: 'browserCommandHandled', sessionID, id }),
    secondaryThread,
  }
}
