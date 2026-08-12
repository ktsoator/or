import { PanelLeft } from 'lucide-react'
import { useI18n } from '@/i18n'
import { cn } from '@/lib/utils'

export function SidebarToggleButton({
  expanded,
  className,
  onToggle,
}: {
  expanded: boolean
  className?: string
  onToggle: () => void
}) {
  const { t } = useI18n()
  const label = expanded ? t('app.collapseSidebar') : t('app.expandSidebar')

  return (
    <button
      className={cn(
        'window-titlebar-control grid size-8 shrink-0 cursor-pointer place-items-center rounded-lg text-ink-muted outline-none transition-colors duration-100 hover:bg-canvas-strong/75 hover:text-ink focus-visible:bg-canvas-strong/75 focus-visible:text-ink',
        className,
      )}
      data-testid="sidebar-panel-toggle"
      type="button"
      title={label}
      aria-label={label}
      aria-expanded={expanded}
      onClick={onToggle}
    >
      <PanelLeft className="size-4" aria-hidden="true" />
    </button>
  )
}
