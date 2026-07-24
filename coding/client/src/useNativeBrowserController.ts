import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type RefObject,
} from 'react'
import { apiURL } from './api'
import { isLocalPreviewURL } from './lib/browser'
import {
  hasNativeBrowser,
  navigateNativeBrowser,
  onNativeBrowserState,
  setNativeBrowserViewport,
  type NativeBrowserState,
} from './lib/desktop'

export type NativeBrowserControllerStatus = 'idle' | 'loading' | 'ready' | 'failed'

type ResolvedNavigation = {
  revision: number
  url: string
  kind: 'web' | 'workspace-preview'
}

const previewProbeRequests = new Map<string, Promise<string>>()

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

export function useNativeBrowserController({
  kind,
  onResolveURL,
  onState,
  revision,
  surfaceRef,
  tabID,
  url,
  visible,
}: {
  kind: 'web' | 'workspace-preview'
  onResolveURL: (url: string) => void
  onState: (state: NativeBrowserState) => void
  revision: number
  surfaceRef: RefObject<HTMLDivElement | null>
  tabID: string
  url: string
  visible: boolean
}): { error: string; status: NativeBrowserControllerStatus } {
  const nativeAvailable = hasNativeBrowser()
  const onResolveURLRef = useRef(onResolveURL)
  const onStateRef = useRef(onState)
  const revisionRef = useRef(revision)
  const issuedRevisionRef = useRef<number | undefined>(undefined)
  const statusRevisionRef = useRef(revision)
  const [resolved, setResolved] = useState<ResolvedNavigation | undefined>(() =>
    url && (kind === 'workspace-preview' || !isLocalPreviewURL(url))
      ? { revision, url, kind }
      : undefined,
  )
  const [status, setStatus] = useState<NativeBrowserControllerStatus>(
    url ? 'loading' : 'idle',
  )
  const [error, setError] = useState('')

  onResolveURLRef.current = onResolveURL
  onStateRef.current = onState
  revisionRef.current = revision

  useEffect(() => {
    if (!url) {
      setResolved(undefined)
      setStatus('idle')
      setError('')
      return
    }
    if (!nativeAvailable) {
      setResolved(undefined)
      setStatus('failed')
      setError('Native browser is unavailable')
      return
    }
    if (issuedRevisionRef.current === revision) return

    if (statusRevisionRef.current !== revision) {
      statusRevisionRef.current = revision
      setStatus('loading')
      setError('')
    }
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
        setStatus('failed')
        setError('preview unavailable')
      })
    return () => {
      active = false
    }
  }, [kind, nativeAvailable, revision, url])

  useEffect(() => {
    if (!resolved || resolved.revision !== revision || !nativeAvailable) return
    if (issuedRevisionRef.current === revision) return
    issuedRevisionRef.current = revision
    setStatus('loading')
    setError('')
    void navigateNativeBrowser({
      tabID,
      revision,
      url: resolved.url,
      kind: resolved.kind,
    })
      .then((state) => {
        if (!state || revisionRef.current !== revision) return
        onStateRef.current(state)
        setStatus(state.status === 'navigating' ? 'loading' : state.status)
        setError(state.error ?? '')
      })
      .catch((reason: unknown) => {
        if (revisionRef.current !== revision) return
        setStatus('failed')
        setError(reason instanceof Error ? reason.message : String(reason))
      })
  }, [nativeAvailable, resolved, revision, tabID])

  useEffect(() => onNativeBrowserState((state) => {
    if (state.tabID !== tabID) return
    onStateRef.current(state)
    if (state.appliedRevision < revisionRef.current) return
    setStatus(state.status === 'navigating' ? 'loading' : state.status)
    setError(state.error ?? '')
  }), [tabID])

  const viewportEnabled = Boolean(
    nativeAvailable &&
    visible &&
    resolved?.revision === revision &&
    status !== 'failed',
  )

  useLayoutEffect(() => {
    const surface = surfaceRef.current
    let active = true
    const hide = () => setNativeBrowserViewport({ tabID, visible: false })
    const sync = () => {
      if (!active) return
      if (!surface || !viewportEnabled) {
        void hide()
        return
      }
      const bounds = surface.getBoundingClientRect()
      if (bounds.width < 1 || bounds.height < 1) {
        void hide()
        return
      }
      void setNativeBrowserViewport({
        tabID,
        visible: true,
        bounds: {
          x: bounds.x,
          y: bounds.y,
          width: bounds.width,
          height: bounds.height,
        },
      }).catch((reason: unknown) => {
        if (!active) return
        setStatus('failed')
        setError(reason instanceof Error ? reason.message : String(reason))
      })
    }
    const observer = surface ? new ResizeObserver(sync) : undefined
    if (surface) observer?.observe(surface)
    window.addEventListener('resize', sync)
    sync()
    return () => {
      active = false
      observer?.disconnect()
      window.removeEventListener('resize', sync)
      void hide()
    }
  }, [surfaceRef, tabID, viewportEnabled])

  return { error, status }
}
