import { useEffect, useRef, useState } from 'react'
import { CircleAlert, ExternalLink, Globe2, LoaderCircle, RefreshCw } from 'lucide-react'
import { apiURL } from '@/api'
import { useI18n } from '@/i18n'
import { isLocalPreviewURL } from '@/lib/browser'
import { openExternalURL } from '@/lib/desktop'

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

function addressHost(address: string): string {
  try {
    return new URL(address).host
  } catch {
    return address
  }
}

export function BrowserSurface({
  navigation,
  onResolveURL,
  onRetry,
  title,
  url,
  workspaceFile = false,
}: {
  navigation: number
  onResolveURL: (url: string) => void
  onRetry: () => void
  title: string
  url: string
  workspaceFile?: boolean
}) {
  const { t } = useI18n()
  const local = workspaceFile || isLocalPreviewURL(url)
  const [status, setStatus] = useState<SurfaceStatus>(url && local ? 'loading' : 'idle')
  const probePassedRef = useRef(false)
  const frameLoadedRef = useRef(false)
  const timeoutRef = useRef<number | undefined>(undefined)
  const onResolveURLRef = useRef(onResolveURL)

  useEffect(() => {
    onResolveURLRef.current = onResolveURL
  }, [onResolveURL])

  useEffect(() => {
    window.clearTimeout(timeoutRef.current)
    probePassedRef.current = false
    frameLoadedRef.current = false
    if (!url || !local) {
      setStatus('idle')
      return
    }

    setStatus('loading')
    let active = true
    timeoutRef.current = window.setTimeout(() => {
      setStatus('failed')
    }, 6000)

    if (workspaceFile) {
      probePassedRef.current = true
      if (frameLoadedRef.current) {
        window.clearTimeout(timeoutRef.current)
        setStatus('ready')
      }
      return () => {
        active = false
        window.clearTimeout(timeoutRef.current)
      }
    }

    void probeLocalPreview(url, navigation)
      .then((resolvedURL) => {
        if (!active) return
        probePassedRef.current = true
        if (resolvedURL !== url) onResolveURLRef.current(resolvedURL)
        if (frameLoadedRef.current) {
          window.clearTimeout(timeoutRef.current)
          setStatus('ready')
        }
      })
      .catch(() => {
        if (!active) return
        window.clearTimeout(timeoutRef.current)
        setStatus('failed')
      })

    return () => {
      active = false
      window.clearTimeout(timeoutRef.current)
    }
  }, [local, navigation, url, workspaceFile])

  if (!url) return <div className="min-h-0 flex-1 bg-white" />

  if (!local) {
    return (
      <div
        className="grid min-h-0 flex-1 place-items-center bg-white px-8 pb-[4vh]"
        data-testid="external-page"
      >
        <div className="flex max-w-[20rem] flex-col items-center text-center">
          <Globe2 className="size-5 text-stone-400" aria-hidden="true" />
          <p className="mt-3 max-w-full truncate text-[0.875rem] font-medium text-stone-800">
            {addressHost(url)}
          </p>
          <button
            className="mt-4 inline-flex h-8 cursor-pointer items-center gap-2 rounded-md border border-stone-200 bg-white px-3 text-[0.8125rem] font-medium text-stone-700 transition-colors hover:bg-stone-50 hover:text-stone-950 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-stone-400"
            type="button"
            onClick={() => openExternalURL(url)}
          >
            <ExternalLink className="size-3.5" aria-hidden="true" />
            {t('preview.openExternal')}
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="relative min-h-0 flex-1 bg-white">
      {status === 'loading' && (
        <div className="pointer-events-none absolute inset-0 z-10 grid place-items-center bg-white">
          <div className="flex items-center gap-2 text-[0.8125rem] text-stone-400" role="status">
            <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
            {t('preview.loading')}
          </div>
        </div>
      )}
      {status === 'failed' && (
        <div className="absolute inset-0 z-10 grid place-items-center bg-white px-8" role="alert">
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
      <iframe
        className="h-full w-full border-0 bg-white"
        data-testid="browser-frame"
        src={url}
        title={title}
        sandbox={
          workspaceFile
            ? 'allow-downloads allow-forms allow-modals allow-popups allow-scripts'
            : 'allow-downloads allow-forms allow-modals allow-popups allow-same-origin allow-scripts'
        }
        allow="clipboard-read; clipboard-write"
        onLoad={() => {
          frameLoadedRef.current = true
          if (probePassedRef.current) {
            window.clearTimeout(timeoutRef.current)
            setStatus('ready')
          }
        }}
        onError={() => {
          window.clearTimeout(timeoutRef.current)
          setStatus('failed')
        }}
      />
    </div>
  )
}
