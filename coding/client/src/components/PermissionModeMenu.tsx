import { Check, ChevronDown, Eye, Hand, PencilLine, ShieldCheck } from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import type { LucideIcon } from 'lucide-react'
import type { PermissionMode } from '@/types'
import { useI18n } from '@/i18n'

type ModeOption = {
  value: PermissionMode
  labelKey: 'permission.ask' | 'permission.autoEdit' | 'permission.readOnly'
  shortLabelKey:
    | 'permission.askShort'
    | 'permission.autoEditShort'
    | 'permission.readOnlyShort'
  descriptionKey:
    | 'permission.askDescription'
    | 'permission.autoEditDescription'
    | 'permission.readOnlyDescription'
  icon: LucideIcon
}

const options: ModeOption[] = [
  {
    value: 'ask',
    labelKey: 'permission.ask',
    shortLabelKey: 'permission.askShort',
    descriptionKey: 'permission.askDescription',
    icon: Hand,
  },
  {
    value: 'auto_edit',
    labelKey: 'permission.autoEdit',
    shortLabelKey: 'permission.autoEditShort',
    descriptionKey: 'permission.autoEditDescription',
    icon: PencilLine,
  },
  {
    value: 'read_only',
    labelKey: 'permission.readOnly',
    shortLabelKey: 'permission.readOnlyShort',
    descriptionKey: 'permission.readOnlyDescription',
    icon: Eye,
  },
]

export function PermissionModeMenu({
  value,
  disabled,
  onChange,
}: {
  value: PermissionMode
  disabled: boolean
  onChange: (mode: PermissionMode) => Promise<void>
}) {
  const { t } = useI18n()
  const selected = options.find((option) => option.value === value) ?? options[0]

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          data-testid="permission-mode-trigger"
          type="button"
          className="group inline-flex h-9 min-w-0 max-w-[10rem] cursor-pointer items-center gap-1.5 rounded-full px-2.5 text-[0.875rem] font-medium text-stone-500 outline-none transition-colors hover:bg-[rgb(241,241,241)] focus-visible:bg-[rgb(241,241,241)] data-[state=open]:bg-[rgb(237,237,237)] disabled:cursor-not-allowed disabled:opacity-40 max-sm:px-2"
          aria-label={t('permission.choose')}
          title={t(selected.labelKey)}
          disabled={disabled}
        >
          <ShieldCheck className="size-4 shrink-0 text-stone-500" aria-hidden="true" />
          <span
            data-testid="permission-mode-label"
            className="min-w-0 truncate @max-[390px]:hidden max-sm:hidden"
          >
            {t(selected.shortLabelKey)}
          </span>
          <ChevronDown
            className="size-3.5 shrink-0 text-stone-400 transition-transform duration-150 group-data-[state=open]:rotate-180"
            aria-hidden="true"
          />
        </button>
      </DropdownMenu.Trigger>

      <DropdownMenu.Portal>
        <DropdownMenu.Content
          side="top"
          align="start"
          sideOffset={7}
          collisionPadding={10}
          className="z-[110] w-[25rem] max-w-[calc(100vw-1.25rem)] animate-[fade-in_110ms_ease-out] rounded-2xl border border-stone-200 bg-white p-1.5 text-stone-900 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
        >
          <DropdownMenu.Label className="px-3 pt-1.5 pb-2 text-[0.75rem] font-medium text-stone-400">
            {t('permission.choose')}
          </DropdownMenu.Label>
          <DropdownMenu.RadioGroup
            className="flex flex-col gap-0.5"
            value={value}
            onValueChange={(next) => {
              if (next !== value) void onChange(next as PermissionMode)
            }}
          >
            {options.map((option) => {
              const Icon = option.icon
              return (
                <DropdownMenu.RadioItem
                  key={option.value}
                  value={option.value}
                  className="relative flex min-h-12 cursor-default select-none items-center gap-2.5 rounded-[12px] px-2.5 py-1.5 pr-9 outline-none data-[highlighted]:bg-[rgb(244,244,244)]"
                >
                  <Icon className="size-4 shrink-0 text-stone-600" aria-hidden="true" />
                  <span className="min-w-0 flex-1">
                    <span className="block text-[0.875rem] leading-5 font-medium text-stone-900">
                      {t(option.labelKey)}
                    </span>
                    <span className="block text-[0.75rem] leading-4 text-stone-500">
                      {t(option.descriptionKey)}
                    </span>
                  </span>
                  <DropdownMenu.ItemIndicator className="absolute right-2.5 grid size-5 place-items-center text-stone-700">
                    <Check className="size-4" aria-hidden="true" />
                  </DropdownMenu.ItemIndicator>
                </DropdownMenu.RadioItem>
              )
            })}
          </DropdownMenu.RadioGroup>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}
