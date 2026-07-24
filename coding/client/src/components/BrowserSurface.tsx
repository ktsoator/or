import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { CircleAlert, LoaderCircle, RefreshCw } from 'lucide-react'
import { apiURL } from '@/api'
import { useI18n } from '@/i18n'
import { isLocalPreviewURL } from '@/lib/browser'
import {
  hasNativeBrowser,
  hideNativeBrowser,
  onNativeBrowserState,
  showNativeBrowser,
  type NativeBrowserState,
} from '@/lib/desktop'

type SurfaceStatus = 'idle' | 'loading' | 'ready' | 'failed'
const previewProbeRequests = new Map<string, Promise<string>>()

function probeLocalPreview(url: string, navigation: number): Promise<string> {
  const key = `${navigation}:${url}`
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

export function BrowserSurface({
  tabID,
  navigation,
  onResolveURL,
  onRetry,
  onState,
  url,
  visible,
  workspaceFile = false,
}: {
  tabID: string
  navigation: number
  onResolveURL: (url: string) => void
  onRetry: () => void
  onState: (state: NativeBrowserState) => void
  url: string
  visible: boolean
  workspaceFile?: boolean
}) {
  const { t } = useI18n()
  const surfaceRef = useRef<HTMLDivElement>(null)
  const onResolveURLRef = useRef(onResolveURL)
  const onStateRef = useRef(onState)
  const [resolvedURL, setResolvedURL] = useState(
    workspaceFile || !isLocalPreviewURL(url) ? url : '',
  )
  const [status, setStatus] = useState<SurfaceStatus>(url ? 'loading' : 'idle')
  const [error, setError] = useState('')
  const nativeAvailable = hasNativeBrowser()

  useEffect(() => {
    onResolveURLRef.current = onResolveURL
    onStateRef.current = onState
  }, [onResolveURL, onState])

  useEffect(() => {
    if (!url) {
      setResolvedURL('')
      setStatus('idle')
      setError('')
      return
    }
    if (!nativeAvailable) {
      setResolvedURL('')
      setStatus('failed')
      setError('Native browser is unavailable')
      return
    }
    if (workspaceFile || !isLocalPreviewURL(url)) {
      setResolvedURL(url)
      setStatus('loading')
      setError('')
      return
    }

    let active = true
    setResolvedURL('')
    setStatus('loading')
    setError('')
    void probeLocalPreview(url, navigation)
      .then((nextURL) => {
        if (!active) return
        if (nextURL !== url) onResolveURLRef.current(nextURL)
        setResolvedURL(nextURL)
      })
      .catch(() => {
        if (!active) return
        setStatus('failed')
        setError('preview unavailable')
      })
    return () => {
      active = false
    }
  }, [nativeAvailable, navigation, url, workspaceFile])

  useEffect(() => onNativeBrowserState((state) => {
    if (state.tabID !== tabID) return
    onStateRef.current(state)
    if (state.error) {
      setStatus('failed')
      setError(state.error)
      void hideNativeBrowser(tabID)
      return
    }
    setError('')
    setStatus(state.loading ? 'loading' : 'ready')
  }), [tabID])

  useLayoutEffect(() => {
    const surface = surfaceRef.current
    if (!surface || !nativeAvailable || !visible || !resolvedURL) {
      void hideNativeBrowser(tabID)
      return
    }

    let active = true
    const sync = () => {
      if (!active) return
      const bounds = surface.getBoundingClientRect()
      if (bounds.width < 1 || bounds.height < 1) {
        void hideNativeBrowser(tabID)
        return
      }
      void showNativeBrowser({
        tabID,
        url: resolvedURL,
        navigation,
        workspacePreview: workspaceFile,
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
        void hideNativeBrowser(tabID)
      })
    }
    const observer = new ResizeObserver(sync)
    observer.observe(surface)
    window.addEventListener('resize', sync)
    sync()
    return () => {
      active = false
      observer.disconnect()
      window.removeEventListener('resize', sync)
      void hideNativeBrowser(tabID)
    }
  }, [nativeAvailable, navigation, resolvedURL, tabID, visible, workspaceFile])

  return (
    <div
      ref={surfaceRef}
      className="relative min-h-0 flex-1 bg-white"
      data-testid="native-browser-surface"
      data-status={status}
      title={error || undefined}
    >
      {status === 'loading' && (
        <div className="pointer-events-none absolute inset-0 grid place-items-center bg-white">
          <div className="flex items-center gap-2 text-[0.8125rem] text-stone-400" role="status">
            <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
            {t('preview.loading')}
          </div>
        </div>
      )}
      {status === 'failed' && (
        <div className="absolute inset-0 grid place-items-center bg-white px-8" role="alert">
          <div className="flex max-w-[19rem] flex-col items-center text-center">
            <CircleAlert className="size-5 text-stone-400" aria-hidden="true" />
            <p className="mt-3 text-[0.875rem] font-medium text-stone-800">
              {t('preview.loadFailed')}
            </p>
            <p className="mt-1 text-[0.8125rem] leading-5 text-stone-500">
              {t('preview.loadFailedHint')}
            </p>
            <button
              className="mt-4 inline-flex h-8 cursor-pointer items-center gap-2 rounded-md border border-stone-200 bg-white px-3 text-[0.8125rem] font-medium text-stone-700 transition-colors hover:bg-stone-50 hover:text-stone-950 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-stone-400"
              type="button"
              onClick={onRetry}
            >
              <RefreshCw className="size-3.5" aria-hidden="true" />
              {t('preview.retry')}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
