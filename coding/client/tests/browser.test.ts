import { describe, expect, test } from 'bun:test'
import { workspacePreviewURL } from '../src/lib/browser'

describe('workspacePreviewURL', () => {
  test('uses the session grant route and encodes each component', () => {
    expect(workspacePreviewURL('session/1', 'grant+1', 'pages/demo file.html')).toBe(
      '/api/sessions/session%2F1/previews/grant%2B1/pages/demo%20file.html',
    )
  })
})
