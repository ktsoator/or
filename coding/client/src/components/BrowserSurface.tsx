import { useRef } from 'react'
import { CircleAlert, LoaderCircle, RefreshCw } from 'lucide-react'
import { useI18n } from '@/i18n'
import type { NativeBrowserState } from '@/lib/desktop'
import { useNativeBrowserController } from '@/useNativeBrowserController'

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
  const { error, status } = useNativeBrowserController({
    kind: workspaceFile ? 'workspace-preview' : 'web',
    onResolveURL,
    onState,
    revision: navigation,
    surfaceRef,
    tabID,
    url,
    visible,
  })

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
