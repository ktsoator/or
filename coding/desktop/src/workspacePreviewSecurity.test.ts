import { describe, expect, test } from 'bun:test'
import {
  workspacePreviewNavigationAllowed,
  workspacePreviewPrefix,
  workspacePreviewRequestAllowed,
} from './workspacePreviewSecurity'

const desktopURL = 'http://127.0.0.1:4310/'
const previewURL = new URL(
  'http://127.0.0.1:4310/api/sessions/session-1/previews/grant-1/index.html',
)
const prefix = '/api/sessions/session-1/previews/grant-1/'

describe('workspace preview security', () => {
  test('extracts the session and grant-scoped prefix', () => {
    expect(workspacePreviewPrefix(previewURL, desktopURL)).toBe(prefix)
    expect(() => workspacePreviewPrefix(
      new URL('https://example.com/api/sessions/session-1/previews/grant-1/index.html'),
      desktopURL,
    )).toThrow('desktop origin')
    expect(() => workspacePreviewPrefix(
      new URL('http://127.0.0.1:4310/api/sessions/session-1/preview/index.html'),
      desktopURL,
    )).toThrow('invalid')
  })

  test('allows only read requests inside the current grant', () => {
    expect(workspacePreviewRequestAllowed(previewURL.href, 'GET', desktopURL, prefix)).toBe(true)
    expect(workspacePreviewRequestAllowed(
      'http://127.0.0.1:4310/api/sessions/session-1/previews/grant-1/app.css',
      'HEAD',
      desktopURL,
      prefix,
    )).toBe(true)
    expect(workspacePreviewRequestAllowed(previewURL.href, 'POST', desktopURL, prefix)).toBe(false)
    expect(workspacePreviewRequestAllowed(
      'http://127.0.0.1:4310/api/sessions/session-1/previews/grant-2/index.html',
      'GET',
      desktopURL,
      prefix,
    )).toBe(false)
    expect(workspacePreviewRequestAllowed('https://example.com/app.js', 'GET', desktopURL, prefix)).toBe(false)
    expect(workspacePreviewRequestAllowed('data:image/png;base64,AA==', 'GET', desktopURL, prefix)).toBe(true)
    expect(workspacePreviewRequestAllowed('blob:http://127.0.0.1:4310/id', 'GET', desktopURL, prefix)).toBe(true)
  })

  test('blocks external and cross-grant top-level navigation', () => {
    expect(workspacePreviewNavigationAllowed(previewURL.href, desktopURL, prefix)).toBe(true)
    expect(workspacePreviewNavigationAllowed('https://example.com', desktopURL, prefix)).toBe(false)
    expect(workspacePreviewNavigationAllowed(
      'http://127.0.0.1:4310/api/sessions/session-1/previews/grant-2/index.html',
      desktopURL,
      prefix,
    )).toBe(false)
    expect(workspacePreviewNavigationAllowed('data:text/html,unsafe', desktopURL, prefix)).toBe(false)
  })
})
