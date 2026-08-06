import { DropdownMenu } from 'radix-ui'
import { Check, ChevronDown } from 'lucide-react'
import type { DeliveryMode } from '@/types'
import { useI18n } from '@/i18n'

export function RunDeliveryMenu({
  value,
  onValueChange,
}: {
  value: DeliveryMode
  onValueChange: (value: DeliveryMode) => void
}) {
  const { t } = useI18n()

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          className="group inline-flex h-[30px] cursor-pointer items-center gap-1 rounded-[10px] px-2.5 text-[0.8125rem] font-medium text-ink-muted outline-none transition-colors hover:bg-surface-active focus-visible:bg-surface-active data-[state=open]:bg-surface-selected"
          type="button"
          aria-label={t('delivery.choose')}
        >
          <span>{value === 'steer' ? t('queue.steer') : t('queue.followUp')}</span>
          <ChevronDown
            className="size-3.5 text-ink-faint transition-transform group-data-[state=open]:rotate-180"
            aria-hidden="true"
          />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          side="top"
          align="end"
          sideOffset={7}
          collisionPadding={10}
          className="z-[110] min-w-[14.75rem] animate-[fade-in_110ms_ease-out] rounded-2xl border border-edge bg-canvas p-1 text-[0.8125rem] text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
        >
          <DropdownMenu.RadioGroup
            className="flex flex-col gap-0.5"
            value={value}
            onValueChange={(next) => onValueChange(next as DeliveryMode)}
          >
            <DeliveryOption
              value="steer"
              label={t('composer.steerRun')}
              hint={t('delivery.steerHint')}
            />
            <DeliveryOption
              value="followup"
              label={t('composer.queueFollowUp')}
              hint={t('delivery.followUpHint')}
            />
          </DropdownMenu.RadioGroup>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

function DeliveryOption({
  value,
  label,
  hint,
}: {
  value: DeliveryMode
  label: string
  hint: string
}) {
  return (
    <DropdownMenu.RadioItem
      value={value}
      className="relative flex h-[35px] cursor-default select-none items-center gap-2 rounded-[10px] px-2.5 pr-8 outline-none data-[highlighted]:bg-surface-active data-[state=checked]:bg-surface-selected"
    >
      <span className="font-medium">{label}</span>
      <span className="ml-auto text-[0.71875rem] text-ink-faint">{hint}</span>
      <DropdownMenu.ItemIndicator className="absolute right-2 grid size-4 place-items-center text-ink-soft">
        <Check className="size-3.5" aria-hidden="true" />
      </DropdownMenu.ItemIndicator>
    </DropdownMenu.RadioItem>
  )
}
