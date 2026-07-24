import type {
  MessageImage,
  ModelOption,
  PermissionMode,
  SessionSummary,
  ThreadSnapshot,
  ThinkingLevel,
  WireEvent,
  WorkspaceSummary,
} from './types'

export type SessionDraft = {
  id: string
  workspacePath?: string
  projectScoped: boolean
  modelProvider?: string
  modelID?: string
  thinkingLevel?: ThinkingLevel
  permissionMode: PermissionMode
}

export type DraftSubmission = {
  sessionID: string
  text: string
  images: MessageImage[]
}

export type ModelDefaults = {
  provider: string
  model: string
  thinkingLevel: ThinkingLevel
}

export type SessionStoreState = {
  sessions: SessionSummary[]
  workspaces: WorkspaceSummary[]
  draft?: SessionDraft
  pendingDraftSend?: DraftSubmission
  activeSessionID?: string
  deletedSessionIDs: Record<string, true>
}

export type SessionStoreAction =
  | {
      t: 'sessionsLoaded'
      sessions: SessionSummary[]
      storedSessionID?: string
      emptyDraft: SessionDraft
    }
  | { t: 'workspacesLoaded'; workspaces: WorkspaceSummary[] }
  | { t: 'sessionWire'; sessionID: string; event: WireEvent }
  | { t: 'sessionSnapshot'; sessionID: string; history: ThreadSnapshot }
  | { t: 'sessionSelected'; sessionID: string }
  | { t: 'draftStarted'; draft: SessionDraft }
  | { t: 'draftWorkspaceUpdated'; workspacePath?: string; projectScoped: boolean }
  | {
      t: 'draftModelUpdated'
      provider: string
      model: string
      thinkingLevel: ThinkingLevel
    }
  | { t: 'draftPermissionUpdated'; permissionMode: PermissionMode }
  | { t: 'draftSendQueued'; submission: DraftSubmission }
  | { t: 'draftSendConsumed'; sessionID: string }
  | { t: 'workspaceUpserted'; workspace: WorkspaceSummary }
  | { t: 'workspaceRemoved'; path: string }
  | { t: 'sessionCreated'; session: SessionSummary; select: boolean }
  | { t: 'sessionUpdated'; session: SessionSummary; front: boolean }
  | { t: 'sessionDeleted'; sessionID: string }
  | { t: 'sessionPromptStarted'; sessionID: string; text: string; updatedAt: string }
  | { t: 'sessionApprovalResolved'; sessionID: string }

export const createSessionStoreState = (): SessionStoreState => ({
  sessions: [],
  workspaces: [],
  draft: undefined,
  pendingDraftSend: undefined,
  activeSessionID: undefined,
  deletedSessionIDs: {},
})

export function createSessionDraft(
  workspacePath?: string,
  projectScoped = false,
  base?: SessionSummary,
  models: ModelOption[] = [],
  defaults?: ModelDefaults,
  id: string = crypto.randomUUID(),
): SessionDraft {
  // Only use the configured default. Picking the first catalog entry would
  // silently start a session on a model the user never selected.
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
    id,
    workspacePath,
    projectScoped,
    modelProvider: base?.modelProvider ?? fallback?.provider,
    modelID: base?.modelId ?? fallback?.id,
    thinkingLevel: base?.thinkingLevel ?? fallbackThinking,
    permissionMode: base?.permissionMode ?? 'ask',
  }
}

export function resolveSessionDraft(
  draft: SessionDraft,
  models: ModelOption[],
  defaults?: ModelDefaults,
): SessionDraft {
  if (draft.modelProvider && draft.modelID && draft.thinkingLevel) return draft
  return {
    ...createSessionDraft(
      draft.workspacePath,
      draft.projectScoped,
      undefined,
      models,
      defaults,
      draft.id,
    ),
    permissionMode: draft.permissionMode,
  }
}

export function sessionStoreReducer(
  state: SessionStoreState,
  action: SessionStoreAction,
): SessionStoreState {
  switch (action.t) {
    case 'sessionsLoaded': {
      const received = action.sessions.filter((session) => !state.deletedSessionIDs[session.id])
      const sessions = received.map((remote) => {
        const local = state.sessions.find((session) => session.id === remote.id)
        if (!local) return remote
        return new Date(local.updatedAt).getTime() > new Date(remote.updatedAt).getTime()
          ? local
          : remote
      })
      const draft = sessions.length === 0 && !state.draft ? action.emptyDraft : state.draft
      const activeSessionID = draft
        ? undefined
        : state.activeSessionID && sessions.some((session) => session.id === state.activeSessionID)
          ? state.activeSessionID
          : action.storedSessionID &&
              sessions.some((session) => session.id === action.storedSessionID)
            ? action.storedSessionID
            : sessions[0]?.id
      return { ...state, sessions, draft, activeSessionID }
    }

    case 'workspacesLoaded':
      return { ...state, workspaces: action.workspaces }

    case 'sessionWire': {
      const event = action.event
      if (event.type === 'approval_request') {
        return patchSession(state, action.sessionID, { running: true, hasApproval: true })
      }
      if (event.type === 'approval_resolved' || event.type === 'approval_cancelled') {
        return patchSession(state, action.sessionID, { hasApproval: false })
      }
      if (event.type === 'done' || event.type === 'error') {
        return patchSession(state, action.sessionID, { running: false })
      }
      if (event.type === 'title_update') {
        return {
          ...state,
          sessions: state.sessions.map((session) =>
            session.id === action.sessionID
              ? {
                  ...session,
                  title: event.title ?? session.title,
                  aiTitle: event.aiTitle,
                  customTitle: event.customTitle,
                }
              : session,
          ),
        }
      }
      return state
    }

    case 'sessionSnapshot': {
      const hasApproval = action.history.events.reduce(
        (pending, event) =>
          event.type === 'approval_request'
            ? true
            : event.type === 'approval_resolved' || event.type === 'approval_cancelled'
              ? false
              : pending,
        false,
      )
      const titlePatch = action.history.title === undefined
        ? {}
        : {
            title: action.history.title,
            aiTitle: action.history.aiTitle,
            customTitle: action.history.customTitle,
          }
      return patchSession(state, action.sessionID, {
        running: action.history.running,
        hasApproval,
        ...titlePatch,
      })
    }

    case 'sessionSelected':
      if (!state.sessions.some((session) => session.id === action.sessionID)) return state
      return { ...state, draft: undefined, activeSessionID: action.sessionID }

    case 'draftStarted':
      return {
        ...state,
        draft: action.draft,
        pendingDraftSend: undefined,
        activeSessionID: undefined,
      }

    case 'draftWorkspaceUpdated':
      if (!state.draft) return state
      return {
        ...state,
        draft: {
          ...state.draft,
          workspacePath: action.projectScoped ? action.workspacePath : undefined,
          projectScoped: action.projectScoped,
        },
      }

    case 'draftModelUpdated':
      if (!state.draft) return state
      return {
        ...state,
        draft: {
          ...state.draft,
          modelProvider: action.provider,
          modelID: action.model,
          thinkingLevel: action.thinkingLevel,
        },
      }

    case 'draftPermissionUpdated':
      if (!state.draft) return state
      return {
        ...state,
        draft: { ...state.draft, permissionMode: action.permissionMode },
      }

    case 'draftSendQueued':
      return {
        ...state,
        draft: undefined,
        pendingDraftSend: action.submission,
        activeSessionID: action.submission.sessionID,
      }

    case 'draftSendConsumed':
      if (state.pendingDraftSend?.sessionID !== action.sessionID) return state
      return { ...state, pendingDraftSend: undefined }

    case 'workspaceUpserted':
      return {
        ...state,
        workspaces: [
          action.workspace,
          ...state.workspaces.filter((workspace) => workspace.path !== action.workspace.path),
        ],
      }

    case 'workspaceRemoved':
      return {
        ...state,
        workspaces: state.workspaces.filter((workspace) => workspace.path !== action.path),
      }

    case 'sessionCreated': {
      const deletedSessionIDs = { ...state.deletedSessionIDs }
      delete deletedSessionIDs[action.session.id]
      const workspace =
        action.session.scope === 'project' &&
        !state.workspaces.some((candidate) => candidate.path === action.session.workspacePath)
          ? {
              path: action.session.workspacePath,
              name: action.session.workspaceName,
              addedAt: action.session.createdAt,
            }
          : undefined
      return {
        ...state,
        sessions: [
          action.session,
          ...state.sessions.filter((session) => session.id !== action.session.id),
        ],
        workspaces: workspace ? [workspace, ...state.workspaces] : state.workspaces,
        draft: action.select ? undefined : state.draft,
        activeSessionID: action.select ? action.session.id : state.activeSessionID,
        deletedSessionIDs,
      }
    }

    case 'sessionUpdated':
      return {
        ...state,
        sessions: action.front
          ? [action.session, ...state.sessions.filter((session) => session.id !== action.session.id)]
          : state.sessions.map((session) =>
              session.id === action.session.id ? action.session : session,
            ),
      }

    case 'sessionDeleted':
      return {
        ...state,
        sessions: state.sessions.filter((session) => session.id !== action.sessionID),
        activeSessionID:
          state.activeSessionID === action.sessionID ? undefined : state.activeSessionID,
        deletedSessionIDs: { ...state.deletedSessionIDs, [action.sessionID]: true },
      }

    case 'sessionPromptStarted':
      return {
        ...state,
        sessions: state.sessions.map((session) =>
          session.id === action.sessionID
            ? {
                ...session,
                title:
                  session.title === 'New session'
                    ? promptTitle(action.text || 'Image')
                    : session.title,
                running: true,
                updatedAt: action.updatedAt,
              }
            : session,
        ),
      }

    case 'sessionApprovalResolved':
      return patchSession(state, action.sessionID, { hasApproval: false })
  }
}

function patchSession(
  state: SessionStoreState,
  sessionID: string,
  patch: Partial<SessionSummary>,
): SessionStoreState {
  return {
    ...state,
    sessions: state.sessions.map((session) =>
      session.id === sessionID ? { ...session, ...patch } : session,
    ),
  }
}

function promptTitle(text: string): string {
  const compact = text.trim().replace(/\s+/g, ' ')
  const runes = [...compact]
  return runes.length > 42 ? `${runes.slice(0, 42).join('').trim()}…` : compact
}
