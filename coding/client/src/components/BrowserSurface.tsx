import { useRef } from 'react'
import { CircleAlert, LoaderCircle, RefreshCw } from 'lucide-react'
import { useI18n } from '@/i18n'
import type { ObservedNavigation } from '@/browserTabs'
import { hasBrowserRuntime, type BrowserRuntimeState } from '@/lib/desktop'
import type { BrowserWebviewElement } from '@/lib/webviewBrowser'
import { useBrowserController } from '@/useBrowserController'
import type { BrowserRuntimeTabID } from '@/browserRuntime'

// React omits unknown boolean attributes, while Electron treats this as a
// presence attribute that must exist before the webview guest is attached.
const allowPopupsAttribute = 'true' as unknown as boolean

export function BrowserSurface({
  active,
  runtimeTabID,
  tabID,
  navigation,
  onResolveURL,
  onRetry,
  onState,
  observed,
  url,
  workspaceFile = false,
}: {
  active: boolean
  runtimeTabID: BrowserRuntimeTabID
  tabID: string
  navigation: number
  onResolveURL: (url: string) => void
  onRetry: () => void
  onState: (state: BrowserRuntimeState) => void
  observed: ObservedNavigation
  url: string
  workspaceFile?: boolean
}) {
  const { t } = useI18n()
  const webviewRef = useRef<BrowserWebviewElement>(null)
  const browserRuntime = hasBrowserRuntime()
  useBrowserController({
    kind: workspaceFile ? 'workspace-preview' : 'web',
    onResolveURL,
    onState,
    revision: navigation,
    runtimeTabID,
    url,
    webviewRef,
  })
  const status = observed.status === 'navigating' ? 'loading' : observed.status

  return (
    <div
      className="relative min-h-0 flex-1 bg-canvas"
      data-testid={active ? 'browser-surface' : undefined}
      data-browser-tab-id={tabID}
      data-browser-runtime-tab-id={runtimeTabID}
      data-status={status}
      title={observed.error || undefined}
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
        <div className="pointer-events-none absolute inset-0 z-10 grid place-items-center bg-canvas">
          <div className="flex items-center gap-2 text-[0.8125rem] text-ink-faint" role="status">
            <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
            {t('preview.loading')}
          </div>
        </div>
      )}
      {observed.status === 'failed' && (
        <div className="absolute inset-0 z-10 grid place-items-center bg-canvas px-8" role="alert">
          <div className="flex max-w-[19rem] flex-col items-center text-center">
            <CircleAlert className="size-5 text-ink-faint" aria-hidden="true" />
            <p className="mt-3 text-[0.875rem] font-medium text-ink-soft">
              {t('preview.loadFailed')}
            </p>
            <p className="mt-1 text-[0.8125rem] leading-5 text-ink-muted">
              {t('preview.loadFailedHint')}
            </p>
            <button
              className="mt-4 inline-flex h-8 cursor-pointer items-center gap-2 rounded-md border border-edge bg-canvas px-3 text-[0.8125rem] font-medium text-ink-soft outline-none transition-colors hover:border-edge-strong hover:bg-canvas-raised hover:text-ink focus-visible:border-edge-stronger focus-visible:bg-canvas-raised focus-visible:text-ink"
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
