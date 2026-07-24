import { APIError, sessionURL } from './api'
import type {
  ApprovalChoice,
  BrowserResult,
  DeliveryMode,
  MessageImage,
} from './types'

export type PromptInput = {
  text: string
  images: MessageImage[]
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
  reportBrowserResult: (sessionID: string, id: string, result: BrowserResult) => Promise<void>
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
  const response = await request(url, init)
  if (response.ok) return

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

export function createSessionCommands(
  request: SessionRequest = browserRequest,
): SessionCommands {
  return {
    sendPrompt: (sessionID, input) =>
      requestOK(
        request,
        sessionURL(sessionID, '/prompt'),
        jsonRequest('POST', input),
        (status) => `prompt request failed (${status})`,
      ),

    enqueueMessage: (sessionID, delivery, input) => {
      const endpoint = delivery === 'followup' ? '/follow-up' : '/steer'
      return requestOK(
        request,
        sessionURL(sessionID, endpoint),
        jsonRequest('POST', input),
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

    reportBrowserResult: (sessionID, id, result) =>
      requestOK(
        request,
        sessionURL(sessionID, `/browser/${encodeURIComponent(id)}/result`),
        jsonRequest('POST', result),
        (status) => `browser result request failed (${status})`,
      ),
  }
}

export const sessionCommands = createSessionCommands()
