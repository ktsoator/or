import { useEffect, useState } from 'react'
import { PanelTopDashed } from 'lucide-react'
import type { PreviewState } from '@/types'
import { cn } from '@/lib/utils'
import { BrowserView, WorkbenchHeaderActions } from './BrowserView'
import { useI18n } from '@/i18n'

type WorkbenchMode = 'launcher' | 'browser'

export function WorkbenchPanel({
  open,
  preview,
  sessionID,
  maximized,
  onToggleMaximized,
}: {
  open: boolean
  preview?: PreviewState
  sessionID?: string
  maximized: boolean
  onToggleMaximized: () => void
}) {
  const { t } = useI18n()
  const [mode, setMode] = useState<WorkbenchMode>(preview ? 'browser' : 'launcher')

  useEffect(() => {
    if (preview) setMode('browser')
  }, [preview])

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
          onCloseTab={() => setMode('launcher')}
          maximized={maximized}
          onToggleMaximized={onToggleMaximized}
        />
      ) : (
        <WorkbenchLauncher
          maximized={maximized}
          onToggleMaximized={onToggleMaximized}
          onOpenBrowser={() => setMode('browser')}
        />
      )}
    </section>
  )
}

function WorkbenchLauncher({
  maximized,
  onToggleMaximized,
  onOpenBrowser,
}: {
  maximized: boolean
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
