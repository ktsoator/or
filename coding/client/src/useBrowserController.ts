import { useEffect, useRef, useState, type RefObject } from 'react'
import { apiURL } from './api'
import { isLocalPreviewURL } from './lib/browser'
import {
  hasBrowserRuntime,
  navigateBrowser,
  onBrowserState,
  type BrowserRuntimeState,
} from './lib/desktop'
import {
  registerWebviewBrowser,
  type BrowserWebviewElement,
} from './lib/webviewBrowser'

type ResolvedNavigation = {
  revision: number
  url: string
  kind: 'web' | 'workspace-preview'
}

const previewProbeRequests = new Map<string, Promise<string>>()

function failedBrowserState(
  tabID: string,
  revision: number,
  requestedURL: string,
  error: string,
): BrowserRuntimeState {
  return {
    tabID,
    appliedRevision: revision,
    requestedURL,
    committedURL: '',
    title: '',
    status: 'failed',
    canGoBack: false,
    canGoForward: false,
    error,
  }
}

function probeLocalPreview(url: string, revision: number): Promise<string> {
  const key = `${revision}:${url}`
  const pending = previewProbeRequests.get(key)
  if (pending) return pending

  const controller = new AbortController()
  const timeout = window.setTimeout(() => controller.abort(), 6000)
  const request = fetch(apiURL('/preview/check'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url }),
    signal: controller.signal,
  })
    .then(async (response) => {
      if (!response.ok) throw new Error('preview unavailable')
      const body = (await response.json()) as { url?: string }
      return body.url || url
    })
    .finally(() => {
      window.clearTimeout(timeout)
      if (previewProbeRequests.get(key) === request) previewProbeRequests.delete(key)
    })
  previewProbeRequests.set(key, request)
  return request
}

export function useBrowserController({
  kind,
  onResolveURL,
  onState,
  revision,
  tabID,
  url,
  webviewRef,
}: {
  kind: 'web' | 'workspace-preview'
  onResolveURL: (url: string) => void
  onState: (state: BrowserRuntimeState) => void
  revision: number
  tabID: string
  url: string
  webviewRef: RefObject<BrowserWebviewElement | null>
}): void {
  const browserAvailable = hasBrowserRuntime()
  const onResolveURLRef = useRef(onResolveURL)
  const onStateRef = useRef(onState)
  const revisionRef = useRef(revision)
  const issuedRevisionRef = useRef<number | undefined>(undefined)
  const [resolved, setResolved] = useState<ResolvedNavigation | undefined>(() =>
    url && (kind === 'workspace-preview' || !isLocalPreviewURL(url))
      ? { revision, url, kind }
      : undefined,
  )
  onResolveURLRef.current = onResolveURL
  onStateRef.current = onState
  revisionRef.current = revision

  useEffect(() => {
    if (!browserAvailable) return
    const webview = webviewRef.current
    if (!webview) return
    const release = registerWebviewBrowser(tabID, webview)
    return () => {
      // A navigation issued against this registration does not survive it, so
      // the next registration has to issue the current revision again.
      issuedRevisionRef.current = undefined
      release()
    }
  }, [browserAvailable, tabID, webviewRef])

  useEffect(() => {
    if (!url) {
      setResolved(undefined)
      return
    }
    if (!browserAvailable) {
      setResolved(undefined)
      onStateRef.current(
        failedBrowserState(tabID, revision, url, 'Browser runtime is unavailable'),
      )
      return
    }
    if (issuedRevisionRef.current === revision) return

    if (kind === 'workspace-preview' || !isLocalPreviewURL(url)) {
      setResolved({ revision, url, kind })
      return
    }

    let active = true
    setResolved(undefined)
    void probeLocalPreview(url, revision)
      .then((nextURL) => {
        if (!active || revisionRef.current !== revision) return
        if (nextURL !== url) onResolveURLRef.current(nextURL)
        setResolved({ revision, url: nextURL, kind })
      })
      .catch(() => {
        if (!active || revisionRef.current !== revision) return
        onStateRef.current(failedBrowserState(tabID, revision, url, 'preview unavailable'))
      })
    return () => {
      active = false
    }
  }, [browserAvailable, kind, revision, tabID, url])

  useEffect(() => {
    if (!resolved || resolved.revision !== revision || !browserAvailable) return
    if (issuedRevisionRef.current === revision) return
    issuedRevisionRef.current = revision
    void navigateBrowser({
      tabID,
      revision,
      url: resolved.url,
      kind: resolved.kind,
    })
      .then((state) => {
        if (!state || revisionRef.current !== revision) return
        onStateRef.current(state)
      })
      .catch((reason: unknown) => {
        if (revisionRef.current !== revision) return
        const message = reason instanceof Error ? reason.message : String(reason)
        onStateRef.current(failedBrowserState(tabID, revision, resolved.url, message))
      })
  }, [browserAvailable, resolved, revision, tabID])

  useEffect(() => onBrowserState((state) => {
    if (state.tabID !== tabID) return
    onStateRef.current(state)
  }), [tabID])
}
