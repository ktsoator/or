import { useI18n } from '@/i18n'
import { cn } from '@/lib/utils'

export function ThinkingModeToggle({
  checked,
  disabled = false,
  ariaLabel,
  className,
  onCheckedChange,
}: {
  checked: boolean
  disabled?: boolean
  ariaLabel?: string
  className?: string
  onCheckedChange: (checked: boolean) => void
}) {
  const { t } = useI18n()

  return (
    <button
      type="button"
      role="switch"
      aria-label={ariaLabel ?? t('model.thinking')}
      aria-checked={checked}
      disabled={disabled}
      className={cn(
        'inline-flex h-9 cursor-pointer items-center gap-2 rounded-[10px] px-2.5 text-[0.8125rem] text-ink-soft outline-none transition-colors hover:bg-surface-active focus-visible:bg-surface-active disabled:cursor-not-allowed disabled:opacity-60',
        className,
      )}
      onClick={() => onCheckedChange(!checked)}
    >
      <span className="min-w-[2.25rem] text-right text-ink-muted">
        {t(checked ? 'model.thinkingOn' : 'model.thinkingOff')}
      </span>
      <ThinkingSwitchIndicator checked={checked} disabled={disabled} />
    </button>
  )
}

export function ThinkingSwitchIndicator({
  checked,
  disabled = false,
}: {
  checked: boolean
  disabled?: boolean
}) {
  return (
    <span
      className={cn(
        'relative h-[1.25rem] w-8 shrink-0 rounded-full transition-colors duration-150',
        checked ? 'bg-canvas-inverse' : 'bg-ink-ghost',
        disabled && 'opacity-60',
      )}
      aria-hidden="true"
    >
      <span
        className={cn(
          'absolute top-[0.1875rem] left-[0.1875rem] size-3.5 rounded-full bg-canvas shadow-sm transition-transform duration-150 ease-out',
          checked && 'translate-x-3',
        )}
      />
    </span>
  )
}
