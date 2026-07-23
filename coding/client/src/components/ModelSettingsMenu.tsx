import { useMemo, useState } from 'react'
import { Check, ChevronDown, ChevronRight, LoaderCircle, Minimize2 } from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import type { ContextUsage, ModelOption, ThinkingLevel } from '@/types'
import { cn } from '@/lib/utils'
import { useI18n } from '@/i18n'
import { ProviderIcon } from '@/components/ProviderIdentity'
import { providerName } from '@/lib/provider'

export function ModelSettingsMenu({
  models,
  modelProvider,
  modelID,
  thinkingLevel,
  contextUsage,
  disabled,
  compacting,
  onChange,
  onCompact,
}: {
  models: ModelOption[]
  modelProvider?: string
  modelID?: string
  thinkingLevel?: ThinkingLevel
  contextUsage?: ContextUsage
  disabled: boolean
  compacting: boolean
  onChange: (
    provider: string,
    model: string,
    thinkingLevel: ThinkingLevel,
  ) => Promise<void>
  onCompact?: () => void
}) {
  const { t } = useI18n()
  const [open, setOpen] = useState(false)
  const [selectedProvider, setSelectedProvider] = useState(modelProvider ?? '')
  const currentModel = models.find(
    (model) => model.provider === modelProvider && model.id === modelID,
  )
  const modelKey = modelProvider && modelID ? JSON.stringify([modelProvider, modelID]) : ''
  const thinkingLevels = currentModel?.thinkingLevels ?? (thinkingLevel ? [thinkingLevel] : [])
  const groupedModels = useMemo(
    () =>
      models.reduce<Record<string, ModelOption[]>>((groups, model) => {
        ;(groups[model.provider] ??= []).push(model)
        return groups
      }, {}),
    [models],
  )
  const providers = Object.keys(groupedModels)
  const providerModels = groupedModels[selectedProvider] ?? []
  const modelName = currentModel?.name ?? modelID ?? t('model.fallback')
  const selectedModelName = selectedProvider === modelProvider ? modelName : t('model.select')
  const selectedProviderName = providerName(selectedProvider || modelProvider || '')
  const effortName = thinkingLevel ? t(`effort.${thinkingLevel}`) : t('model.effort')
  const unavailable = disabled || !modelKey || models.length === 0
  const contextWindow = currentModel?.contextWindow ?? contextUsage?.contextWindow ?? 0
  const currentContextUsage =
    contextUsage && contextUsage.provider === modelProvider && contextUsage.model === modelID
      ? contextUsage
      : undefined

  const handleOpenChange = (nextOpen: boolean) => {
    if (nextOpen) setSelectedProvider(modelProvider ?? '')
    setOpen(nextOpen)
  }

  const selectModel = (value: string) => {
    const [provider, id] = JSON.parse(value) as [string, string]
    const model = models.find(
      (candidate) => candidate.provider === provider && candidate.id === id,
    )
    if (!model || (provider === modelProvider && id === modelID)) return
    const requestedThinking = thinkingLevel ?? 'medium'
    const nextThinking = model.thinkingLevels.includes(requestedThinking)
      ? requestedThinking
      : (model.thinkingLevels.find((level) => level === 'medium') ??
        model.thinkingLevels.find((level) => level !== 'off') ??
        model.thinkingLevels[0])
    if (nextThinking) void onChange(provider, id, nextThinking)
  }

  const selectEffort = (level: string) => {
    if (!modelProvider || !modelID || level === thinkingLevel) return
    void onChange(modelProvider, modelID, level as ThinkingLevel)
  }

  return (
    <DropdownMenu.Root open={open} onOpenChange={handleOpenChange}>
      <DropdownMenu.Trigger asChild>
        <button
          data-testid="model-settings-trigger"
          type="button"
          className="group inline-flex h-9 min-w-0 max-w-[15.5rem] cursor-pointer items-center gap-1.5 rounded-full px-2.5 text-[0.875rem] font-medium outline-none transition-colors hover:bg-[rgb(241,241,241)] focus-visible:bg-[rgb(241,241,241)] data-[state=open]:bg-[rgb(237,237,237)] disabled:cursor-not-allowed disabled:opacity-40 max-sm:max-w-[8rem] max-sm:px-2"
          aria-label={t('model.settings')}
          disabled={unavailable}
        >
          <ProviderIcon provider={modelProvider ?? ''} />
          <span
            data-testid="model-settings-name"
            className="min-w-0 max-w-[9.375rem] flex-1 truncate text-stone-500 max-sm:max-w-[5.5rem]"
          >
            {modelName}
          </span>
          <span
            data-testid="model-settings-effort"
            className="shrink-0 text-stone-400 @max-[430px]:hidden max-sm:hidden"
          >
            {effortName}
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
          align="end"
          sideOffset={7}
          collisionPadding={10}
          className="z-[100] min-w-[15.5rem] animate-[fade-in_110ms_ease-out] rounded-2xl border border-stone-200 bg-white p-1 text-[0.875rem] text-stone-900 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
        >
          <DropdownMenu.Sub>
            <DropdownMenu.SubTrigger className={subTriggerClass}>
              <span>{t('model.provider')}</span>
              <span className="ml-auto flex min-w-0 items-center gap-1.5 text-stone-500">
                <ProviderIcon provider={selectedProvider || modelProvider || ''} />
                <span className="max-w-[8rem] truncate">{selectedProviderName}</span>
                <ChevronRight className="size-3.5 shrink-0" aria-hidden="true" />
              </span>
            </DropdownMenu.SubTrigger>
            <DropdownMenu.Portal>
              <DropdownMenu.SubContent
                sideOffset={6}
                alignOffset={-4}
                collisionPadding={10}
                className="z-[110] min-w-[13rem] animate-[fade-in_110ms_ease-out] rounded-2xl border border-stone-200 bg-white p-1 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
              >
                <DropdownMenu.Label className={menuLabelClass}>
                  {t('model.provider')}
                </DropdownMenu.Label>
                <DropdownMenu.Separator className={separatorClass} />
                <DropdownMenu.RadioGroup
                  className="flex flex-col gap-0.5"
                  value={selectedProvider}
                  onValueChange={setSelectedProvider}
                >
                  {providers.map((provider) => (
                    <DropdownMenu.RadioItem
                      key={provider}
                      value={provider}
                      className={radioItemClass}
                      onSelect={(event) => event.preventDefault()}
                    >
                      <ProviderIcon provider={provider} />
                      <span className="min-w-0 flex-1 truncate">{providerName(provider)}</span>
                      <DropdownMenu.ItemIndicator className="absolute right-2.5 grid size-4 place-items-center text-stone-700">
                        <Check className="size-3.5" aria-hidden="true" />
                      </DropdownMenu.ItemIndicator>
                    </DropdownMenu.RadioItem>
                  ))}
                </DropdownMenu.RadioGroup>
              </DropdownMenu.SubContent>
            </DropdownMenu.Portal>
          </DropdownMenu.Sub>

          <DropdownMenu.Sub>
            <DropdownMenu.SubTrigger className={subTriggerClass}>
              <span>{t('model.model')}</span>
              <span className="ml-auto flex min-w-0 items-center gap-1.5 text-stone-500">
                <span className="max-w-[8rem] truncate">{selectedModelName}</span>
                <ChevronRight className="size-3.5 shrink-0" aria-hidden="true" />
              </span>
            </DropdownMenu.SubTrigger>
            <DropdownMenu.Portal>
              <DropdownMenu.SubContent
                sideOffset={6}
                alignOffset={-4}
                collisionPadding={10}
                className="z-[110] max-h-[min(26.25rem,var(--radix-dropdown-menu-content-available-height))] min-w-[16.25rem] animate-[fade-in_110ms_ease-out] overflow-y-auto rounded-2xl border border-stone-200 bg-white p-1 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
              >
                <DropdownMenu.Label className={menuLabelClass}>
                  {t('model.models', { provider: selectedProviderName })}
                </DropdownMenu.Label>
                <DropdownMenu.Separator className={separatorClass} />
                <DropdownMenu.RadioGroup
                  className="flex flex-col gap-0.5"
                  value={modelKey}
                  onValueChange={selectModel}
                >
                  {providerModels.map((model) => (
                    <DropdownMenu.RadioItem
                      key={`${model.provider}/${model.id}`}
                      value={JSON.stringify([model.provider, model.id])}
                      className={radioItemClass}
                    >
                      <span className="min-w-0 flex-1 truncate">{model.name}</span>
                      <DropdownMenu.ItemIndicator className="absolute right-2.5 grid size-4 place-items-center text-stone-700">
                        <Check className="size-3.5" aria-hidden="true" />
                      </DropdownMenu.ItemIndicator>
                    </DropdownMenu.RadioItem>
                  ))}
                </DropdownMenu.RadioGroup>
              </DropdownMenu.SubContent>
            </DropdownMenu.Portal>
          </DropdownMenu.Sub>

          <DropdownMenu.Sub>
            <DropdownMenu.SubTrigger
              className={subTriggerClass}
              disabled={selectedProvider !== modelProvider}
            >
              <span>{t('model.effort')}</span>
              <span className="ml-auto flex items-center gap-1.5 text-stone-500">
                <span>{effortName}</span>
                <ChevronRight className="size-3.5" aria-hidden="true" />
              </span>
            </DropdownMenu.SubTrigger>
            <DropdownMenu.Portal>
              <DropdownMenu.SubContent
                sideOffset={6}
                alignOffset={-4}
                collisionPadding={10}
                className="z-[110] min-w-[13rem] animate-[fade-in_110ms_ease-out] rounded-2xl border border-stone-200 bg-white p-1 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
              >
                <DropdownMenu.Label className={menuLabelClass}>
                  {t('model.effort')}
                </DropdownMenu.Label>
                <DropdownMenu.Separator className={separatorClass} />
                <DropdownMenu.RadioGroup
                  className="flex flex-col gap-0.5"
                  value={thinkingLevel ?? ''}
                  onValueChange={selectEffort}
                >
                  {thinkingLevels.map((level) => (
                    <DropdownMenu.RadioItem key={level} value={level} className={radioItemClass}>
                      <span>{t(`effort.${level}`)}</span>
                      <DropdownMenu.ItemIndicator className="absolute right-2.5 grid size-4 place-items-center text-stone-700">
                        <Check className="size-3.5" aria-hidden="true" />
                      </DropdownMenu.ItemIndicator>
                    </DropdownMenu.RadioItem>
                  ))}
                </DropdownMenu.RadioGroup>
              </DropdownMenu.SubContent>
            </DropdownMenu.Portal>
          </DropdownMenu.Sub>

          <DropdownMenu.Separator className="mx-2 my-1 h-px bg-stone-100" />
          <ContextMeter usage={currentContextUsage} contextWindow={contextWindow} />
          {onCompact && (
            <>
              <DropdownMenu.Separator className="mx-2 my-1 h-px bg-stone-100" />
              <DropdownMenu.Item
                className="flex h-[30px] cursor-default select-none items-center gap-2.5 rounded-[10px] px-2.5 outline-none data-[highlighted]:bg-[rgb(241,241,241)] data-[disabled]:opacity-40"
                disabled={compacting}
                onSelect={onCompact}
              >
                {compacting ? (
                  <LoaderCircle className="size-4 animate-spin text-stone-500" aria-hidden="true" />
                ) : (
                  <Minimize2 className="size-4 text-stone-600" aria-hidden="true" />
                )}
                <span>{compacting ? t('model.compacting') : t('model.compact')}</span>
              </DropdownMenu.Item>
            </>
          )}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

function ContextMeter({
  usage,
  contextWindow,
}: {
  usage?: ContextUsage
  contextWindow: number
}) {
  const { t, formatNumber } = useI18n()
  const measured = Boolean(usage?.measured && usage.usedTokens > 0 && contextWindow > 0)
  const usedTokens = measured ? usage?.usedTokens ?? 0 : 0
  const percentage = measured ? Math.min((usedTokens / contextWindow) * 100, 100) : 0

  return (
    <div className="px-2.5 pt-1.5 pb-2" aria-label={t('model.contextUsage')}>
      <div className="flex items-baseline justify-between gap-4 text-[0.75rem] leading-5 tabular-nums">
        <span className="font-medium text-stone-600">{t('model.context')}</span>
        <span className="text-stone-400">
          {measured ? formatTokens(usedTokens, formatNumber) : '—'} /{' '}
          {formatTokens(contextWindow, formatNumber)}
          {measured && <span> · {formatNumber(Math.round(percentage))}%</span>}
        </span>
      </div>
      <div className="mt-1 h-1 overflow-hidden rounded-full bg-stone-100">
        <div
          className={cn(
            'h-full rounded-full transition-[width,background-color] duration-300 ease-out',
            percentage >= 90
              ? 'bg-red-500'
              : percentage >= 75
                ? 'bg-amber-500'
                : 'bg-stone-500',
            !measured && 'bg-transparent',
          )}
          style={{ width: `${percentage}%` }}
        />
      </div>
      {!measured && contextWindow > 0 && (
        <p className="mt-1 text-[0.6875rem] leading-4 text-stone-400">
          {t('model.measureAfterResponse')}
        </p>
      )}
    </div>
  )
}

type NumberFormatter = (value: number, options?: Intl.NumberFormatOptions) => string

function formatTokens(value: number, formatNumber: NumberFormatter): string {
  if (value <= 0) return '—'
  if (value >= 1_000_000) return `${formatTokenDecimal(value / 1_000_000, formatNumber)}m`
  if (value >= 1_000) return `${formatTokenDecimal(value / 1_000, formatNumber)}k`
  return formatNumber(Math.round(value))
}

function formatTokenDecimal(value: number, formatNumber: NumberFormatter): string {
  return formatNumber(value, { maximumFractionDigits: value >= 100 ? 0 : 1 })
}

const subTriggerClass = cn(
  'mb-0.5 flex h-[30px] cursor-default select-none items-center rounded-[10px] px-2.5 outline-none last:mb-0',
  'data-[highlighted]:bg-[rgb(241,241,241)] data-[state=open]:bg-[rgb(237,237,237)]',
  'data-[disabled]:opacity-40',
)

const radioItemClass = cn(
  'relative flex h-[30px] cursor-default select-none items-center gap-2 rounded-[10px] px-2.5 pr-9 text-[0.875rem] outline-none',
  'data-[highlighted]:bg-[rgb(241,241,241)] data-[state=checked]:bg-[rgb(237,237,237)] data-[state=checked]:font-medium',
)

const menuLabelClass = 'px-2.5 py-1.5 text-[0.75rem] font-medium text-stone-400'
const separatorClass = 'mx-1.5 my-0.5 h-px bg-stone-100'
