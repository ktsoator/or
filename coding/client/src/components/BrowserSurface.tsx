import { useRef } from 'react'
import { CircleAlert, LoaderCircle, RefreshCw } from 'lucide-react'
import { useI18n } from '@/i18n'
import { hasBrowserRuntime, type BrowserRuntimeState } from '@/lib/desktop'
import type { BrowserWebviewElement } from '@/lib/webviewBrowser'
import { useBrowserController } from '@/useBrowserController'

// React omits unknown boolean attributes, while Electron treats this as a
// presence attribute that must exist before the webview guest is attached.
const allowPopupsAttribute = 'true' as unknown as boolean

export function BrowserSurface({
  active,
  tabID,
  navigation,
  onResolveURL,
  onRetry,
  onState,
  url,
  workspaceFile = false,
}: {
  active: boolean
  tabID: string
  navigation: number
  onResolveURL: (url: string) => void
  onRetry: () => void
  onState: (state: BrowserRuntimeState) => void
  url: string
  workspaceFile?: boolean
}) {
  const { t } = useI18n()
  const webviewRef = useRef<BrowserWebviewElement>(null)
  const browserRuntime = hasBrowserRuntime()
  const { error, status } = useBrowserController({
    kind: workspaceFile ? 'workspace-preview' : 'web',
    onResolveURL,
    onState,
    revision: navigation,
    tabID,
    url,
    webviewRef,
  })

  return (
    <div
      className="relative min-h-0 flex-1 bg-white"
      data-testid={active ? 'browser-surface' : undefined}
      data-browser-tab-id={tabID}
      data-status={status}
      title={error || undefined}
    >
      {browserRuntime && (
        <webview
          ref={webviewRef}
          allowpopups={allowPopupsAttribute}
          className="absolute inset-0 flex h-full w-full"
          src="about:blank"
        />
      )}
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
    </div>
  )
}
