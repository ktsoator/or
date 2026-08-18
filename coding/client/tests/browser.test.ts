import { describe, expect, test } from 'bun:test'
import { workspacePreviewURL } from '../src/features/browser/urls'

describe('workspacePreviewURL', () => {
  test('uses the session grant route and encodes each component', () => {
    expect(workspacePreviewURL('session/1', 'grant+1', 'pages/demo file.html')).toBe(
      '/api/sessions/session%2F1/previews/grant%2B1/pages/demo%20file.html',
    )
  })

  test('uses the isolated desktop preview origin when provided', () => {
    expect(
      workspacePreviewURL(
        'session-1',
        'grant-1',
        'index.html',
        'http://127.0.0.1:43123',
      ),
    ).toBe('http://127.0.0.1:43123/sessions/session-1/previews/grant-1/index.html')
  })
})
