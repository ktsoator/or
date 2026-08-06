import { Lock } from 'lucide-react'
import { Tooltip } from 'radix-ui'
import { cn } from '@/lib/utils'
import { useI18n } from '@/i18n'

export function FixedThinkingStatus({
  className,
  focusable = true,
  iconOnly = false,
  hidden = false,
}: {
  className?: string
  focusable?: boolean
  iconOnly?: boolean
  hidden?: boolean
}) {
  const { t } = useI18n()
  const description = hidden
    ? t('model.hiddenThinkingDescription')
    : t('model.fixedThinkingDescription')

  return (
    <Tooltip.Provider delayDuration={180} skipDelayDuration={80}>
      <Tooltip.Root>
        <Tooltip.Trigger asChild>
          <span
            className={cn('inline-flex min-w-0 items-center gap-1.5', className)}
            tabIndex={focusable ? 0 : undefined}
            aria-label={`${t('model.fixedThinking')}. ${description}`}
            data-testid="fixed-thinking-status"
          >
            <Lock className="size-3.5 shrink-0" aria-hidden="true" />
            {!iconOnly && <span className="truncate">{t('model.fixedThinking')}</span>}
          </span>
        </Tooltip.Trigger>
        <Tooltip.Portal>
          <Tooltip.Content
            side="top"
            sideOffset={6}
            collisionPadding={8}
            className="z-[210] max-w-[17rem] animate-[fade-in_100ms_ease-out] rounded-md bg-canvas-inverse px-2.5 py-1.5 text-[0.6875rem] leading-4 text-ink-inverse shadow-lg"
          >
            {description}
          </Tooltip.Content>
        </Tooltip.Portal>
      </Tooltip.Root>
    </Tooltip.Provider>
  )
}
