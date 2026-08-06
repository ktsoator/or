import { ArrowDown } from 'lucide-react'
import { useI18n } from '@/i18n'

export function ScrollToLatestButton({
  hasNewContent,
  onClick,
}: {
  hasNewContent: boolean
  onClick: () => void
}) {
  const { t } = useI18n()
  const label = hasNewContent
    ? t('app.newContentJumpToLatest')
    : t('app.jumpToLatest')

  return (
    <>
      {hasNewContent && (
        <span className="sr-only" role="status">
          {t('app.newContentAvailable')}
        </span>
      )}
      <button
        className="absolute bottom-3 left-1/2 z-20 grid size-9 -translate-x-1/2 cursor-pointer place-items-center rounded-full border border-edge bg-canvas text-ink-muted shadow-[0_8px_24px_-12px_rgba(28,25,23,0.55)] outline-none transition-[background-color,border-color,color,transform] duration-150 hover:border-edge-strong hover:bg-canvas-raised hover:text-ink focus-visible:ring-2 focus-visible:ring-edge-stronger focus-visible:ring-offset-2 focus-visible:ring-offset-canvas active:translate-y-px motion-reduce:transition-none"
        type="button"
        aria-label={label}
        title={t('app.jumpToLatest')}
        onClick={onClick}
      >
        <ArrowDown className="size-4" aria-hidden="true" />
        {hasNewContent && (
          <span
            className="absolute top-1 right-1 size-1.5 rounded-full bg-info"
            aria-hidden="true"
          />
        )}
      </button>
    </>
  )
}
