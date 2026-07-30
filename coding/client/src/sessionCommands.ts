import { APIError, sessionURL } from './api'
import type {
  ApprovalChoice,
  BrowserInspectionResult,
  BrowserResult,
  BrowserTabsResult,
  DeliveryMode,
  PromptFile,
  MessageImage,
  QuestionAnswer,
  TaskOutputResponse,
} from './types'

export type PromptInput = {
  text: string
  images: MessageImage[]
  files: PromptFile[]
}

export type QueuedPromptInput = PromptInput & {
  id: string
}

export type SessionRequest = (url: string, init: RequestInit) => Promise<Response>

export type SessionCommands = {
  sendPrompt: (sessionID: string, input: PromptInput) => Promise<void>
  enqueueMessage: (
    sessionID: string,
    delivery: DeliveryMode,
    input: QueuedPromptInput,
  ) => Promise<void>
  abortRun: (sessionID: string) => Promise<void>
  removeQueuedMessage: (sessionID: string, id: string) => Promise<void>
  resolveApproval: (sessionID: string, id: string, choice: ApprovalChoice) => Promise<void>
  resolveQuestion: (sessionID: string, id: string, answers: QuestionAnswer[]) => Promise<void>
  stopTask: (sessionID: string, id: string) => Promise<void>
  readTaskOutput: (sessionID: string, id: string) => Promise<TaskOutputResponse>
  reportBrowserResult: (sessionID: string, id: string, result: BrowserResult) => Promise<void>
  reportBrowserInspection: (
    sessionID: string,
    id: string,
    result: BrowserInspectionResult,
  ) => Promise<void>
  reportBrowserTabs: (
    sessionID: string,
    id: string,
    result: BrowserTabsResult,
  ) => Promise<void>
}

type ErrorBody = {
  error?: string
  code?: string
}

const browserRequest: SessionRequest = (url, init) => fetch(url, init)

async function requestOK(
  request: SessionRequest,
  url: string,
  init: RequestInit,
  fallback: (status: number) => string,
): Promise<void> {
  await requestResponse(request, url, init, fallback)
}

async function requestResponse(
  request: SessionRequest,
  url: string,
  init: RequestInit,
  fallback: (status: number) => string,
): Promise<Response> {
  const response = await request(url, init)
  if (response.ok) return response

  let message = fallback(response.status)
  let code: string | undefined
  try {
    const body = (await response.json()) as ErrorBody
    if (body.error) message = body.error
    code = body.code
  } catch {
    // Keep the command-specific fallback when the response has no JSON body.
  }
  throw new APIError(message, code)
}

const jsonRequest = (method: string, body: unknown): RequestInit => ({
  method,
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(body),
})

function messageRequest(
  method: string,
  input: PromptInput | QueuedPromptInput,
): RequestInit {
  const files = input.files ?? []
  const payload = {
    ...('id' in input ? { id: input.id } : {}),
    text: input.text,
    images: input.images,
  }
  if (files.length === 0) return jsonRequest(method, payload)
  const body = new FormData()
  body.set('payload', JSON.stringify(payload))
  for (const attached of files) {
    const upload =
      attached.file.type === attached.mimeType
        ? attached.file
        : new File([attached.file], attached.name, { type: attached.mimeType })
    body.append('files', upload, attached.name)
  }
  return { method, body }
}

export function createSessionCommands(
  request: SessionRequest = browserRequest,
): SessionCommands {
  return {
    sendPrompt: (sessionID, input) =>
      requestOK(
        request,
        sessionURL(sessionID, '/prompt'),
        messageRequest('POST', input),
        (status) => `prompt request failed (${status})`,
      ),

    enqueueMessage: (sessionID, delivery, input) => {
      const endpoint = delivery === 'followup' ? '/follow-up' : '/steer'
      return requestOK(
        request,
        sessionURL(sessionID, endpoint),
        messageRequest('POST', input),
        (status) => `queue request failed (${status})`,
      )
    },

    abortRun: (sessionID) =>
      requestOK(
        request,
        sessionURL(sessionID, '/abort'),
        { method: 'POST' },
        (status) => `abort request failed (${status})`,
      ),

    removeQueuedMessage: (sessionID, id) =>
      requestOK(
        request,
        sessionURL(sessionID, `/queue/${encodeURIComponent(id)}`),
        { method: 'DELETE' },
        (status) => `remove queued message failed (${status})`,
      ),

    resolveApproval: (sessionID, id, choice) =>
      requestOK(
        request,
        sessionURL(sessionID, `/approvals/${encodeURIComponent(id)}`),
        jsonRequest('POST', { choice }),
        () => 'request failed',
      ),

    resolveQuestion: (sessionID, id, answers) =>
      requestOK(
        request,
        sessionURL(sessionID, `/questions/${encodeURIComponent(id)}`),
        jsonRequest('POST', { answers }),
        () => 'request failed',
      ),

    stopTask: (sessionID, id) =>
      requestOK(
        request,
        sessionURL(sessionID, `/tasks/${encodeURIComponent(id)}/stop`),
        { method: 'POST' },
        (status) => `stop background task failed (${status})`,
      ),

    readTaskOutput: async (sessionID, id) => {
      const response = await requestResponse(
        request,
        sessionURL(sessionID, `/tasks/${encodeURIComponent(id)}/output`),
        { method: 'GET', cache: 'no-store' },
        (status) => `read background task output failed (${status})`,
      )
      return (await response.json()) as TaskOutputResponse
    },

    reportBrowserResult: (sessionID, id, result) =>
      requestOK(
        request,
        sessionURL(sessionID, `/browser/${encodeURIComponent(id)}/result`),
        jsonRequest('POST', result),
        (status) => `browser result request failed (${status})`,
      ),

    reportBrowserTabs: (sessionID, id, result) =>
      requestOK(
        request,
        sessionURL(sessionID, `/browser/tabs/${encodeURIComponent(id)}/result`),
        jsonRequest('POST', result),
        (status) => `browser tabs result request failed (${status})`,
      ),

    reportBrowserInspection: (sessionID, id, result) =>
      requestOK(
        request,
        sessionURL(sessionID, `/browser/inspect/${encodeURIComponent(id)}/result`),
        jsonRequest('POST', result),
        (status) => `browser inspection result request failed (${status})`,
      ),
  }
}

export const sessionCommands = createSessionCommands()
