import { LoaderCircle, PanelLeft, PanelRight, SquarePen } from 'lucide-react'
import type { ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useI18n } from '@/i18n'
import { cn } from '@/lib/utils'
import { HeaderControlTooltip } from '@/shared/ui/HeaderControlTooltip'

export function DesktopTitlebarControls({
  sidebarExpanded,
  workbenchAvailable,
  workbenchExpanded,
  previewAvailable,
  creatingSession,
  onToggleSidebar,
  onToggleWorkbench,
  onCreateSession,
}: {
  sidebarExpanded: boolean
  workbenchAvailable: boolean
  workbenchExpanded: boolean
  previewAvailable: boolean
  creatingSession: boolean
  onToggleSidebar: () => void
  onToggleWorkbench: () => void
  onCreateSession: () => void
}) {
  const { t } = useI18n()

  return createPortal(
    <div
      className="desktop-titlebar-controls pointer-events-none fixed inset-x-0 top-0 z-[90] hidden h-[45px] md:block"
      data-testid="desktop-titlebar-controls"
    >
      <TitlebarPaneButton
        label={sidebarExpanded ? t('app.collapseSidebar') : t('app.expandSidebar')}
        expanded={sidebarExpanded}
        side="left"
        testID="sidebar-panel-toggle"
        onClick={onToggleSidebar}
      >
        <PanelLeft className="size-4" aria-hidden="true" />
      </TitlebarPaneButton>

      <button
        className={cn(
          'desktop-titlebar-new-session window-titlebar-control pointer-events-auto absolute top-[0.6875rem] left-[8rem] grid size-[30px] shrink-0 cursor-pointer place-items-center rounded-lg text-ink-muted outline-none',
          'transition-[opacity,transform,background-color,color,box-shadow] duration-150 ease-out motion-reduce:transition-none',
          'hover:bg-canvas-strong/75 hover:text-ink focus-visible:bg-canvas-strong/75 focus-visible:text-ink focus-visible:ring-1 focus-visible:ring-edge-stronger/70 active:bg-canvas-sunken/90 active:text-ink disabled:cursor-wait',
          sidebarExpanded
            ? 'pointer-events-none -translate-x-1 scale-95 opacity-0'
            : 'translate-x-0 scale-100 opacity-100 delay-75',
        )}
        data-testid="desktop-new-session"
        type="button"
        aria-label={t('app.newSession')}
        aria-hidden={sidebarExpanded}
        tabIndex={sidebarExpanded ? -1 : undefined}
        disabled={creatingSession}
        onClick={() => onCreateSession()}
      >
        {creatingSession ? (
          <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
        ) : (
          <SquarePen className="size-4" aria-hidden="true" />
        )}
        <HeaderControlTooltip>{t('app.newSession')}</HeaderControlTooltip>
      </button>

      {workbenchAvailable && (
        <TitlebarPaneButton
          label={workbenchExpanded ? t('workbench.hide') : t('workbench.show')}
          expanded={workbenchExpanded}
          side="right"
          testID="workbench-panel-toggle"
          onClick={onToggleWorkbench}
        >
          <PanelRight className="size-4" aria-hidden="true" />
          {previewAvailable && !workbenchExpanded && (
            <span
              className="absolute top-0.5 right-0.5 size-1.5 rounded-full border border-canvas bg-info"
              aria-hidden="true"
            />
          )}
        </TitlebarPaneButton>
      )}
    </div>,
    document.body,
  )
}

function TitlebarPaneButton({
  children,
  expanded,
  label,
  side,
  testID,
  onClick,
}: {
  children: ReactNode
  expanded: boolean
  label: string
  side: 'left' | 'right'
  testID: string
  onClick: () => void
}) {
  return (
    <button
      className={cn(
        'desktop-titlebar-pane-button window-titlebar-control pointer-events-auto absolute top-[0.6875rem] grid size-[30px] shrink-0 cursor-pointer place-items-center rounded-lg text-ink-muted outline-none transition-[background-color,color,box-shadow] duration-120',
        'hover:bg-canvas-strong/75 hover:text-ink focus-visible:bg-canvas-strong/75 focus-visible:text-ink focus-visible:ring-1 focus-visible:ring-edge-stronger/70',
        'active:bg-canvas-sunken/90 active:text-ink',
        side === 'left' ? 'left-[5.75rem]' : 'right-2',
      )}
      data-testid={testID}
      data-side={side}
      type="button"
      aria-label={label}
      aria-expanded={expanded}
      onClick={onClick}
    >
      {children}
      <HeaderControlTooltip align={side === 'right' ? 'end' : 'center'}>
        {label}
      </HeaderControlTooltip>
    </button>
  )
}
