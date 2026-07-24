import { describe, expect, test } from 'bun:test'
import { APIError } from '../src/api'
import {
  createSessionCommands,
  type SessionRequest,
} from '../src/sessionCommands'

type RequestCall = {
  url: string
  init: RequestInit
}

function recordingRequest(response: () => Response = () => new Response(null, { status: 204 })) {
  const calls: RequestCall[] = []
  const request: SessionRequest = async (url, init) => {
    calls.push({ url, init })
    return response()
  }
  return { calls, commands: createSessionCommands(request) }
}

function jsonBody(call: RequestCall | undefined): unknown {
  if (typeof call?.init.body !== 'string') return undefined
  return JSON.parse(call.init.body)
}

describe('sessionCommands', () => {
  test('sends prompts with the expected endpoint and body', async () => {
    const { calls, commands } = recordingRequest()
    const input = {
      text: 'hello',
      images: [{ mimeType: 'image/png', data: 'aGVsbG8=' }],
    }

    await commands.sendPrompt('session / one', input)

    expect(calls).toHaveLength(1)
    expect(calls[0]?.url).toBe('/api/sessions/session%20%2F%20one/prompt')
    expect(calls[0]?.init.method).toBe('POST')
    expect(calls[0]?.init.headers).toEqual({ 'Content-Type': 'application/json' })
    expect(jsonBody(calls[0])).toEqual(input)
  })

  test('maps steer and follow-up delivery to their queue endpoints', async () => {
    const { calls, commands } = recordingRequest()
    const input = { id: 'local-1', text: 'next', images: [] }

    await commands.enqueueMessage('session-1', 'steer', input)
    await commands.enqueueMessage('session-1', 'followup', input)

    expect(calls.map((call) => call.url)).toEqual([
      '/api/sessions/session-1/steer',
      '/api/sessions/session-1/follow-up',
    ])
    expect(calls.every((call) => call.init.method === 'POST')).toBe(true)
    expect(calls.map(jsonBody)).toEqual([input, input])
  })

  test('encodes resource IDs for abort, queue removal, and approval', async () => {
    const { calls, commands } = recordingRequest()

    await commands.abortRun('session / one')
    await commands.removeQueuedMessage('session / one', 'queue / one')
    await commands.resolveApproval('session / one', 'approval / one', 'allow_once')

    expect(calls.map((call) => [call.url, call.init.method])).toEqual([
      ['/api/sessions/session%20%2F%20one/abort', 'POST'],
      ['/api/sessions/session%20%2F%20one/queue/queue%20%2F%20one', 'DELETE'],
      ['/api/sessions/session%20%2F%20one/approvals/approval%20%2F%20one', 'POST'],
    ])
    expect(jsonBody(calls[2])).toEqual({ choice: 'allow_once' })
  })

  test('decodes structured API errors', async () => {
    const { commands } = recordingRequest(() =>
      Response.json(
        { error: 'resolve the pending approval first', code: 'approval_pending' },
        { status: 409 },
      ),
    )

    try {
      await commands.enqueueMessage('session-1', 'followup', {
        id: 'queued-1',
        text: 'next',
        images: [],
      })
      throw new Error('expected command to fail')
    } catch (error) {
      expect(error).toBeInstanceOf(APIError)
      expect(error).toMatchObject({
        message: 'resolve the pending approval first',
        code: 'approval_pending',
      })
    }
  })

  test('uses a command-specific fallback for non-JSON failures', async () => {
    const { commands } = recordingRequest(
      () => new Response('bad gateway', { status: 502, headers: { 'Content-Type': 'text/plain' } }),
    )

    await expect(
      commands.sendPrompt('session-1', { text: 'hello', images: [] }),
    ).rejects.toThrow('prompt request failed (502)')
  })
})
