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
        'window-titlebar-control grid size-8 shrink-0 cursor-pointer place-items-center rounded-lg text-stone-500 outline-none transition-colors duration-100 hover:bg-stone-200/75 hover:text-stone-950 focus-visible:ring-2 focus-visible:ring-stone-300',
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
