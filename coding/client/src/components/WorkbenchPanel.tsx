import { useEffect, useState } from 'react'
import { CircleAlert, PanelTopDashed, X } from 'lucide-react'
import type { ModelOption, PreviewState, WorkspaceSummary } from '@/types'
import type { SessionThread } from '@/useSession'
import { cn } from '@/lib/utils'
import { BrowserView, WorkbenchHeaderActions } from './BrowserView'
import { useI18n } from '@/i18n'

type WorkbenchMode = 'launcher' | 'browser'

export function WorkbenchPanel({
  open,
  preview,
  sessionID,
  activatePreview,
  conversation,
  models,
  workspaces,
  maximized,
  creatingConversation,
  creationError,
  onCreateConversation,
  onDismissCreationError,
  onCloseConversation,
  onConfigureModel,
  onToggleMaximized,
}: {
  open: boolean
  preview?: PreviewState
  sessionID?: string
  activatePreview: boolean
  conversation?: SessionThread
  models: ModelOption[]
  workspaces: WorkspaceSummary[]
  maximized: boolean
  creatingConversation: boolean
  creationError: string
  onCreateConversation: () => void
  onDismissCreationError: () => void
  onCloseConversation: () => void
  onConfigureModel: () => void
  onToggleMaximized: () => void
}) {
  const { t } = useI18n()
  const [mode, setMode] = useState<WorkbenchMode>(
    preview || conversation ? 'browser' : 'launcher',
  )

  useEffect(() => {
    if (preview || conversation) setMode('browser')
  }, [conversation, preview])

  return (
    <section
      className={cn(
        'relative h-full min-h-0 bg-white transition-[opacity,transform] duration-[220ms] ease-[cubic-bezier(0.4,0,0.2,1)] motion-reduce:transition-none [contain:layout_paint] md:absolute md:inset-y-0 md:right-0',
        maximized ? 'md:w-full' : 'md:w-[var(--workbench-expanded-width)]',
        open
          ? 'translate-x-0 opacity-100 delay-[40ms]'
          : 'translate-x-2 opacity-0 delay-0',
      )}
      data-testid="workbench-panel"
      aria-label={t('workbench.title')}
    >
      {mode === 'browser' ? (
        <BrowserView
          preview={preview}
          sessionID={sessionID}
          activatePreview={activatePreview}
          conversation={conversation}
          creatingConversation={creatingConversation}
          models={models}
          workspaces={workspaces}
          onCloseTab={() => setMode('launcher')}
          onCloseConversation={onCloseConversation}
          onConfigureModel={onConfigureModel}
          onCreateConversation={onCreateConversation}
          maximized={maximized}
          onToggleMaximized={onToggleMaximized}
        />
      ) : (
        <WorkbenchLauncher
          maximized={maximized}
          creatingConversation={creatingConversation}
          onCreateConversation={onCreateConversation}
          onToggleMaximized={onToggleMaximized}
          onOpenBrowser={() => setMode('browser')}
        />
      )}
      {creationError && (
        <div
          className="absolute inset-x-3 top-[49px] z-[80] flex min-h-9 items-center gap-2 rounded-lg border border-red-200/80 bg-white px-2.5 py-2 text-xs leading-4 text-red-700 shadow-[0_10px_28px_-18px_rgba(127,29,29,0.45)]"
          role="alert"
        >
          <CircleAlert className="size-3.5 shrink-0" aria-hidden="true" />
          <span className="min-w-0 flex-1 truncate" title={creationError}>
            {creationError}
          </span>
          <button
            className="grid size-5 shrink-0 cursor-pointer place-items-center rounded text-red-500 hover:bg-red-50 hover:text-red-800 focus-visible:outline-2 focus-visible:outline-red-300"
            type="button"
            title={t('workbench.dismissError')}
            aria-label={t('workbench.dismissError')}
            onClick={onDismissCreationError}
          >
            <X className="size-3.5" aria-hidden="true" />
          </button>
        </div>
      )}
    </section>
  )
}

function WorkbenchLauncher({
  maximized,
  creatingConversation,
  onCreateConversation,
  onToggleMaximized,
  onOpenBrowser,
}: {
  maximized: boolean
  creatingConversation: boolean
  onCreateConversation: () => void
  onToggleMaximized: () => void
  onOpenBrowser: () => void
}) {
  const { t } = useI18n()

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div
        className="window-drag-region flex h-[45px] shrink-0 items-center justify-end bg-white px-2.5 pr-11"
        data-testid="workbench-titlebar"
      >
        <WorkbenchHeaderActions
          maximized={maximized}
          creatingConversation={creatingConversation}
          onCreateConversation={onCreateConversation}
          onToggleMaximized={onToggleMaximized}
          onOpenBrowser={onOpenBrowser}
        />
      </div>
      <div className="grid min-h-0 flex-1 place-items-center px-8 pb-[5vh]">
        <div
          className="flex flex-col items-center gap-2 text-stone-400"
          data-testid="workbench-empty"
        >
          <PanelTopDashed className="size-5 text-stone-300" aria-hidden="true" />
          <span className="text-[0.8125rem] leading-5">{t('workbench.empty')}</span>
        </div>
      </div>
    </div>
  )
}
