import {
  useCallback,
  useEffect,
  useReducer,
  useRef,
  useState,
} from 'react'
import { sessionCommands } from './commands'
import { useSessionConnection } from './connection'
import { threadsReducer } from './reducer'
import { sessionResourceAPI } from './resourceApi'
import { findEvictableThreadIDs } from './threadRetention'
import type { Session } from './types'
import { useBrowserResultOutbox } from './useBrowserResultOutbox'
import { useServiceConnection } from '@/serviceConnection'
import {
  createSessionDraft,
  createSessionStoreState,
  resolveSessionDraft,
  sessionStoreReducer,
  type ModelDefaults,
} from './store'
import type {
  ApprovalChoice,
  ApprovalItem,
  BrowserResult,
  ConnectionStatus,
  DeliveryMode,
  MessageFile,
  MessageImage,
  ModelOption,
  PermissionMode,
  PromptFile,
  QuestionAnswer,
  QuestionItem,
  ThreadSnapshot,
  ThinkingLevel,
  WireEvent,
} from '@/types'

export type { SessionDraft } from './store'
export type { Session, SessionThread } from './types'

export function useSession(secondarySessionID?: string): Session {
  const [threads, dispatch] = useReducer(threadsReducer, {})
  const [sessionStore, dispatchSessionStore] = useReducer(
    sessionStoreReducer,
    createSessionStoreState(),
  )
  const { sessions, workspaces, draft, pendingDraftSend, activeSessionID } = sessionStore
  const [creating, setCreating] = useState(false)
  const updatingSettingsSessionIDsRef = useRef<ReadonlySet<string>>(new Set())
  const [updatingSettingsSessionIDs, setUpdatingSettingsSessionIDs] =
    useState<ReadonlySet<string>>(() => new Set())
  const [compactingSessionID, setCompactingSessionID] = useState<string>()
  const [models, setModels] = useState<ModelOption[]>([])
  const [modelDefaults, setModelDefaults] = useState<ModelDefaults>()

  const acknowledgeBrowserResult = useCallback((sessionID: string, id: string) => {
    dispatch({ t: 'browserResultAcknowledged', sessionID, id })
  }, [])
  const queueBrowserResult = useCallback(
    (sessionID: string, id: string, result: BrowserResult) => {
      dispatch({ t: 'browserResultQueued', sessionID, id, result })
    },
    [],
  )
  useBrowserResultOutbox(threads, acknowledgeBrowserResult)

  const applySessionWire = useCallback((sessionID: string, wire: WireEvent, eventSeq?: number) => {
    dispatch({ t: 'wire', sessionID, ev: wire, serverEventSeq: eventSeq })
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
    },
    [],
  )

  const loadModels = useCallback(async (signal?: AbortSignal) => {
    const catalog = await sessionResourceAPI.loadModels(signal)
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
  }, [])

  const refreshSessions = useCallback(async (signal?: AbortSignal) => {
    const received = await sessionResourceAPI.loadSessions(signal)
    dispatchSessionStore({
      t: 'sessionsLoaded',
      sessions: received,
      emptyDraft: createSessionDraft(),
    })
    return received
  }, [])

  const refreshWorkspaces = useCallback(async (signal?: AbortSignal) => {
    const received = await sessionResourceAPI.loadWorkspaces(signal)
    dispatchSessionStore({ t: 'workspacesLoaded', workspaces: received })
    return received
  }, [])

  const refreshServiceState = useCallback(
    async (signal: AbortSignal) => {
      await Promise.all([
        loadModels(signal),
        refreshSessions(signal),
        refreshWorkspaces(signal),
      ])
    },
    [loadModels, refreshSessions, refreshWorkspaces],
  )
  const { status: serviceStatus, initializing } = useServiceConnection(refreshServiceState)

  const thread = activeSessionID ? threads[activeSessionID] : undefined
  const activeCheckpoint = thread?.loaded ? { eventSeq: thread.serverEventSeq } : undefined
  const connectedSecondarySessionID =
    secondarySessionID && secondarySessionID !== activeSessionID
      ? secondarySessionID
      : undefined
  const secondaryConnectionThread = connectedSecondarySessionID
    ? threads[connectedSecondarySessionID]
    : undefined
  const secondaryCheckpoint = secondaryConnectionThread?.loaded
    ? { eventSeq: secondaryConnectionThread.serverEventSeq }
    : undefined

  useEffect(() => {
    const retainedSessionIDs = new Set(
      [activeSessionID, connectedSecondarySessionID].filter(
        (sessionID): sessionID is string => Boolean(sessionID),
      ),
    )
    for (const sessionID of findEvictableThreadIDs(threads, retainedSessionIDs)) {
      dispatch({ t: 'forget', sessionID })
    }
  }, [activeSessionID, connectedSecondarySessionID, threads])

  useSessionConnection(activeSessionID, {
    onWire: applySessionWire,
    onSnapshot: applySessionSnapshot,
    onStatus: applyPrimarySessionStatus,
  }, activeCheckpoint)
  useSessionConnection(
    connectedSecondarySessionID,
    {
      onWire: applySessionWire,
      onSnapshot: applySessionSnapshot,
      onStatus: applySessionStatus,
    },
    secondaryCheckpoint,
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
    const workspace = await sessionResourceAPI.registerWorkspace(path)
    dispatchSessionStore({ t: 'workspaceUpserted', workspace })
    return workspace
  }

  const removeWorkspace = async (path: string) => {
    await sessionResourceAPI.removeWorkspace(path)
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
    const created = await sessionResourceAPI.createSession({
        workspacePath: projectScoped ? workspacePath : undefined,
        scope: projectScoped ? 'project' : 'chat',
        provider,
        model,
        thinkingLevel,
        permissionMode,
    })
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
    await sessionResourceAPI.deleteSession(id)
    dispatch({ t: 'forget', sessionID: id })
    dispatchSessionStore({ t: 'sessionDeleted', sessionID: id })
    await refreshSessions()
  }

  const renameSession = async (id: string, customTitle: string) => {
    const updated = await sessionResourceAPI.renameSession(id, customTitle)
    dispatchSessionStore({ t: 'sessionUpdated', session: updated, front: false })
    return updated
  }

  const patchSessionSettings = async (
    sessionID: string,
    provider: string,
    model: string,
    thinkingLevel: ThinkingLevel,
  ) => {
    return sessionResourceAPI.updateSettings(
      sessionID,
      provider,
      model,
      thinkingLevel,
    )
  }

  const updateSessionSettings = async (
    sessionID: string,
    provider: string,
    model: string,
    thinkingLevel: ThinkingLevel,
  ) => {
    if (updatingSettingsSessionIDsRef.current.has(sessionID)) return
    const pendingSessionIDs = new Set(updatingSettingsSessionIDsRef.current).add(sessionID)
    updatingSettingsSessionIDsRef.current = pendingSessionIDs
    setUpdatingSettingsSessionIDs(pendingSessionIDs)
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
      const remainingSessionIDs = new Set(updatingSettingsSessionIDsRef.current)
      remainingSessionIDs.delete(sessionID)
      updatingSettingsSessionIDsRef.current = remainingSessionIDs
      setUpdatingSettingsSessionIDs(remainingSessionIDs)
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
    if (updatingSettingsSessionIDsRef.current.has(sessionID)) return
    const pendingSessionIDs = new Set(updatingSettingsSessionIDsRef.current).add(sessionID)
    updatingSettingsSessionIDsRef.current = pendingSessionIDs
    setUpdatingSettingsSessionIDs(pendingSessionIDs)
    try {
      const updated = await sessionResourceAPI.updatePermissionMode(sessionID, mode)
      dispatchSessionStore({ t: 'sessionUpdated', session: updated, front: true })
    } finally {
      const remainingSessionIDs = new Set(updatingSettingsSessionIDsRef.current)
      remainingSessionIDs.delete(sessionID)
      updatingSettingsSessionIDsRef.current = remainingSessionIDs
      setUpdatingSettingsSessionIDs(remainingSessionIDs)
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
      const result = await sessionResourceAPI.compactContext(sessionID)
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

  const activeSessionRunning = activeSession?.running

  const startSessionPrompt = useCallback(
    (
      sessionID: string,
      id: string,
      text: string,
      images: MessageImage[],
      files: PromptFile[],
    ) => {
      dispatch({
        t: 'sendUser',
        sessionID,
        id,
        text,
        images,
        files: promptFileMetadata(files),
        startedAt: new Date().toISOString(),
      })
      dispatchSessionStore({
        t: 'sessionPromptStarted',
        sessionID,
        text,
        updatedAt: new Date().toISOString(),
      })
      void sessionCommands.sendPrompt(sessionID, { text, images, files }).catch((error: unknown) => {
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
    startSessionPrompt(
      submission.sessionID,
      id,
      submission.text,
      submission.images,
      submission.files,
    )
  }, [activeSessionID, pendingDraftSend, startSessionPrompt, thread?.loaded, thread?.status])

  const sendToSession = async (
    sessionID: string,
    text: string,
    images: MessageImage[],
    files: PromptFile[],
    delivery?: DeliveryMode,
  ): Promise<boolean> => {
    const trimmed = text.trim()
    const targetThread = threads[sessionID]
    if (
      (!trimmed && images.length === 0 && files.length === 0) ||
      targetThread?.status !== 'ready'
    ) {
      return false
    }
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
        files: promptFileMetadata(files),
        startedAt: new Date().toISOString(),
        delivery,
      })
      void sessionCommands
        .enqueueMessage(sessionID, delivery, { id, text: trimmed, images, files })
        .catch(() => {
          dispatch({ t: 'queueFailed', sessionID, id })
          void refreshSessions().catch(() => undefined)
        })
      return true
    }

    startSessionPrompt(sessionID, id, trimmed, images, files)
    return true
  }

  const send = async (
    text: string,
    images: MessageImage[],
    files: PromptFile[],
    delivery?: DeliveryMode,
  ): Promise<boolean> => {
    const trimmed = text.trim()
    if (!trimmed && images.length === 0 && files.length === 0) return false
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
          submission: { sessionID: created.id, text: trimmed, images, files },
        })
        return true
      } finally {
        setCreating(false)
      }
    }
    if (!activeSessionID) return false
    return sendToSession(activeSessionID, trimmed, images, files, delivery)
  }

  const stopSession = (sessionID: string) => {
    void sessionCommands.abortRun(sessionID).catch(() => undefined)
  }

  const stop = () => {
    if (activeSessionID) stopSession(activeSessionID)
  }

  const stopBackgroundTask = (sessionID: string, id: string) =>
    sessionCommands.stopTask(sessionID, id)

  const readBackgroundTaskOutput = (sessionID: string, id: string) =>
    sessionCommands.readTaskOutput(sessionID, id)

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

  const resolveSessionQuestion = async (
    sessionID: string,
    id: string,
    answers: QuestionAnswer[],
  ) => {
    await sessionCommands.resolveQuestion(sessionID, id, answers)
    dispatch({ t: 'resolveQuestion', sessionID, id })
    dispatchSessionStore({ t: 'sessionQuestionResolved', sessionID })
  }

  const resolveQuestion = async (id: string, answers: QuestionAnswer[]) => {
    if (!activeSessionID) throw new Error('no active session')
    await resolveSessionQuestion(activeSessionID, id, answers)
  }

  const approval = thread?.items.findLast(
    (item): item is ApprovalItem => item.kind === 'approval',
  )
  const question = thread?.items.findLast(
    (item): item is QuestionItem => item.kind === 'question',
  )
  const items =
    thread?.items.filter((item) => item.kind !== 'approval' && item.kind !== 'question') ?? []

  const secondarySession = sessions.find((session) => session.id === secondarySessionID)
  const secondaryState = secondarySessionID ? threads[secondarySessionID] : undefined
  const secondaryApproval = secondaryState?.items.findLast(
    (item): item is ApprovalItem => item.kind === 'approval',
  )
  const secondaryQuestion = secondaryState?.items.findLast(
    (item): item is QuestionItem => item.kind === 'question',
  )
  const secondaryThread = secondarySession
    ? {
        session: secondarySession,
        items:
          secondaryState?.items.filter(
            (item) => item.kind !== 'approval' && item.kind !== 'question',
          ) ?? [],
        tasks: Object.values(secondaryState?.tasks ?? {}),
        queuedMessages: secondaryState?.queue ?? [],
        contextUsage: secondaryState?.contextUsage,
        preview: secondaryState?.preview,
        browserCommands: secondaryState?.browserCommands ?? [],
        browserTabsRequests: secondaryState?.browserTabsRequests ?? [],
        browserInspections: secondaryState?.browserInspections ?? [],
        previewOpen: secondaryState?.previewOpen ?? false,
        approval: secondaryApproval,
        question: secondaryQuestion,
        running: secondaryState?.running ?? secondarySession.running,
        autoCompacting: secondaryState?.autoCompacting ?? false,
        loading: !secondaryState?.loaded,
        updatingSettings: updatingSettingsSessionIDs.has(secondarySession.id),
        compacting: compactingSessionID === secondarySession.id,
        status: secondaryState?.status ?? serviceStatus,
        send: (
          text: string,
          images: MessageImage[],
          files: PromptFile[],
          delivery?: DeliveryMode,
        ) => sendToSession(secondarySession.id, text, images, files, delivery),
        removeQueuedMessage: (id: string) =>
          removeSessionQueuedMessage(secondarySession.id, id),
        stop: () => stopSession(secondarySession.id),
        stopTask: (id: string) => stopBackgroundTask(secondarySession.id, id),
        readTaskOutput: (id: string) => readBackgroundTaskOutput(secondarySession.id, id),
        resolveApproval: (id: string, choice: ApprovalChoice) =>
          resolveSessionApproval(secondarySession.id, id, choice),
        resolveQuestion: (id: string, answers: QuestionAnswer[]) =>
          resolveSessionQuestion(secondarySession.id, id, answers),
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
    tasks: Object.values(thread?.tasks ?? {}),
    queuedMessages: thread?.queue ?? [],
    contextUsage: thread?.contextUsage,
    preview: thread?.preview,
    browserCommands: thread?.browserCommands ?? [],
    browserTabsRequests: thread?.browserTabsRequests ?? [],
    browserInspections: thread?.browserInspections ?? [],
    previewOpen: thread?.previewOpen ?? false,
    approval,
    question,
    running: thread?.running ?? activeSession?.running ?? false,
    autoCompacting: thread?.autoCompacting ?? false,
    loading: initializing || (Boolean(activeSessionID) && !thread?.loaded),
    creating,
    updatingSettings: Boolean(
      activeSessionID && updatingSettingsSessionIDs.has(activeSessionID),
    ),
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
    stopTask: (id: string) => {
      if (!activeSessionID) return Promise.reject(new Error('no active session'))
      return stopBackgroundTask(activeSessionID, id)
    },
    readTaskOutput: (id: string) => {
      if (!activeSessionID) return Promise.reject(new Error('no active session'))
      return readBackgroundTaskOutput(activeSessionID, id)
    },
    resolveApproval,
    resolveQuestion,
    queueBrowserResult,
    handleBrowserTabs: (sessionID: string, id: string) =>
      dispatch({ t: 'browserTabsHandled', sessionID, id }),
    handleBrowserInspection: (sessionID: string, id: string) =>
      dispatch({ t: 'browserInspectionHandled', sessionID, id }),
    secondaryThread,
  }
}

function promptFileMetadata(files: PromptFile[]): MessageFile[] {
  return files.map(({ name, mimeType, size }) => ({ name, mimeType, size }))
}
