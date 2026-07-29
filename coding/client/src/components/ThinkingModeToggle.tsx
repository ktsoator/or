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
        'inline-flex h-9 cursor-pointer items-center gap-2 rounded-[10px] px-2.5 text-[0.8125rem] text-stone-700 outline-none transition-colors hover:bg-[rgb(241,241,241)] focus-visible:ring-2 focus-visible:ring-stone-300 disabled:cursor-not-allowed disabled:opacity-60',
        className,
      )}
      onClick={() => onCheckedChange(!checked)}
    >
      <span className="min-w-[2.25rem] text-right text-stone-500">
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
        checked ? 'bg-stone-900' : 'bg-stone-300',
        disabled && 'opacity-60',
      )}
      aria-hidden="true"
    >
      <span
        className={cn(
          'absolute top-[0.1875rem] left-[0.1875rem] size-3.5 rounded-full bg-white shadow-sm transition-transform duration-150 ease-out',
          checked && 'translate-x-3',
        )}
      />
    </span>
  )
}
