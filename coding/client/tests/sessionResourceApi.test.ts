import { describe, expect, test } from 'bun:test'
import { APIError } from '../src/api'
import { createSessionResourceAPI } from '../src/features/session/resourceApi'

describe('session resource API', () => {
  test('loads service resources from their API endpoints', async () => {
    const urls: string[] = []
    const api = createSessionResourceAPI(async (url) => {
      urls.push(url)
      return Response.json([])
    })

    await api.loadModels()
    await api.loadSessions()
    await api.loadWorkspaces()

    expect(urls).toEqual(['/api/models', '/api/sessions', '/api/workspaces'])
  })

  test('serializes session creation independently from React state', async () => {
    let requestURL = ''
    let requestInit: RequestInit | undefined
    const api = createSessionResourceAPI(async (url, init) => {
      requestURL = url
      requestInit = init
      return Response.json({ id: 'session-1' })
    })

    await api.createSession({
      workspacePath: '/work/or',
      scope: 'project',
      provider: 'openai',
      model: 'gpt-test',
      thinkingLevel: 'medium',
      permissionMode: 'ask',
    })

    expect(requestURL).toBe('/api/sessions')
    expect(requestInit?.method).toBe('POST')
    expect(JSON.parse(String(requestInit?.body))).toEqual({
      workspacePath: '/work/or',
      scope: 'project',
      provider: 'openai',
      model: 'gpt-test',
      thinkingLevel: 'medium',
      permissionMode: 'ask',
    })
  })

  test('preserves server error messages and compaction error codes', async () => {
    const api = createSessionResourceAPI(async (url) => {
      if (url.endsWith('/compact')) {
        return Response.json(
          { error: 'Nothing to compact.', code: 'nothing_to_compact' },
          { status: 409 },
        )
      }
      return Response.json({ error: 'Workspace is unavailable.' }, { status: 400 })
    })

    expect(api.registerWorkspace('/missing')).rejects.toThrow('Workspace is unavailable.')
    try {
      await api.compactContext('session-1')
      throw new Error('expected compaction to fail')
    } catch (error) {
      expect(error).toBeInstanceOf(APIError)
      expect((error as APIError).code).toBe('nothing_to_compact')
    }
  })
})
