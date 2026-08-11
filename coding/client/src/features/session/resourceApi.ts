import { APIError, apiURL, sessionURL } from '@/api'
import type {
  CompactionResult,
  ModelCatalogResponse,
  PermissionMode,
  SessionSummary,
  ThinkingLevel,
  WorkspaceSummary,
} from '@/types'

type Request = (url: string, init: RequestInit) => Promise<Response>

export type CreateSessionInput = {
  workspacePath?: string
  scope: 'chat' | 'project'
  provider: string
  model: string
  thinkingLevel: ThinkingLevel
  permissionMode: PermissionMode
}

export type ForkSessionInput =
  | { messageID: string; mode: 'after_assistant' }
  | { messageID: string; mode: 'before_user'; text: string }

export type SessionResourceAPI = {
  loadModels: (signal?: AbortSignal) => Promise<ModelCatalogResponse>
  loadSessions: (signal?: AbortSignal) => Promise<SessionSummary[]>
  loadWorkspaces: (signal?: AbortSignal) => Promise<WorkspaceSummary[]>
  registerWorkspace: (path: string) => Promise<WorkspaceSummary>
  removeWorkspace: (path: string) => Promise<void>
  createSession: (input: CreateSessionInput) => Promise<SessionSummary>
  forkSession: (id: string, input: ForkSessionInput) => Promise<SessionSummary>
  deleteSession: (id: string) => Promise<void>
  renameSession: (id: string, customTitle: string) => Promise<SessionSummary>
  updateSettings: (
    id: string,
    provider: string,
    model: string,
    thinkingLevel: ThinkingLevel,
  ) => Promise<SessionSummary>
  updatePermissionMode: (id: string, mode: PermissionMode) => Promise<SessionSummary>
  compactContext: (id: string) => Promise<CompactionResult>
}

type ErrorBody = {
  code?: string
  error?: string
}

const browserRequest: Request = (url, init) => fetch(url, init)

const jsonRequest = (method: string, body: unknown): RequestInit => ({
  method,
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(body),
})

async function responseError(response: Response, fallback: string): Promise<ErrorBody & {
  message: string
}> {
  let message = fallback
  let body: ErrorBody = {}
  try {
    body = (await response.json()) as ErrorBody
    if (body.error) message = body.error
  } catch {
    // Keep the operation-specific fallback when the response has no JSON body.
  }
  return { ...body, message }
}

async function requestJSON<T>(
  request: Request,
  url: string,
  init: RequestInit,
  fallback: (status: number) => string,
): Promise<T> {
  const response = await request(url, init)
  if (!response.ok) {
    const failure = await responseError(response, fallback(response.status))
    throw new Error(failure.message)
  }
  return (await response.json()) as T
}

async function requestOK(
  request: Request,
  url: string,
  init: RequestInit,
  fallback: (status: number) => string,
): Promise<void> {
  const response = await request(url, init)
  if (response.ok) return
  const failure = await responseError(response, fallback(response.status))
  throw new Error(failure.message)
}

export function createSessionResourceAPI(request: Request = browserRequest): SessionResourceAPI {
  return {
    loadModels: (signal) =>
      requestJSON(
        request,
        apiURL('/models'),
        { cache: 'no-store', signal },
        (status) => `model catalog failed (${status})`,
      ),
    loadSessions: (signal) =>
      requestJSON(
        request,
        apiURL('/sessions'),
        { cache: 'no-store', signal },
        (status) => `session list failed (${status})`,
      ),
    loadWorkspaces: (signal) =>
      requestJSON(
        request,
        apiURL('/workspaces'),
        { cache: 'no-store', signal },
        (status) => `workspace list failed (${status})`,
      ),
    registerWorkspace: (path) =>
      requestJSON(
        request,
        apiURL('/workspaces'),
        jsonRequest('POST', { path }),
        (status) => `register workspace failed (${status})`,
      ),
    removeWorkspace: (path) =>
      requestOK(
        request,
        `${apiURL('/workspaces')}?path=${encodeURIComponent(path)}`,
        { method: 'DELETE' },
        (status) => `remove workspace failed (${status})`,
      ),
    createSession: (input) =>
      requestJSON(
        request,
        apiURL('/sessions'),
        jsonRequest('POST', input),
        (status) => `create session failed (${status})`,
      ),
    forkSession: (id, input) =>
      requestJSON(
        request,
        sessionURL(id, '/forks'),
        jsonRequest('POST', input),
        (status) => `branch session failed (${status})`,
      ),
    deleteSession: (id) =>
      requestOK(
        request,
        sessionURL(id, ''),
        { method: 'DELETE' },
        (status) => `delete session failed (${status})`,
      ),
    renameSession: (id, customTitle) =>
      requestJSON(
        request,
        sessionURL(id, '/title'),
        jsonRequest('PATCH', { customTitle }),
        (status) => `rename session failed (${status})`,
      ),
    updateSettings: (id, provider, model, thinkingLevel) =>
      requestJSON(
        request,
        sessionURL(id, '/settings'),
        jsonRequest('PATCH', { provider, model, thinkingLevel }),
        (status) => `update settings failed (${status})`,
      ),
    updatePermissionMode: (id, mode) =>
      requestJSON(
        request,
        sessionURL(id, '/permission-mode'),
        jsonRequest('PATCH', { mode }),
        (status) => `update permission mode failed (${status})`,
      ),
    compactContext: async (id) => {
      const response = await request(
        sessionURL(id, '/compact'),
        jsonRequest('POST', {}),
      )
      if (!response.ok) {
        const failure = await responseError(
          response,
          `compact context failed (${response.status})`,
        )
        throw new APIError(failure.message, failure.code)
      }
      return (await response.json()) as CompactionResult
    },
  }
}

export const sessionResourceAPI = createSessionResourceAPI()
