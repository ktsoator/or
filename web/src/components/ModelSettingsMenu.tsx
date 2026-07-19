import { useMemo, useState } from 'react'
import { Check, ChevronDown, ChevronRight } from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import type { ContextUsage, ModelOption, ThinkingLevel } from '@/types'
import { cn } from '@/lib/utils'
import deepseekIcon from '@/assets/providers/deepseek.svg'
import kimiIcon from '@/assets/providers/kimi.svg'
import minimaxIcon from '@/assets/providers/minimax.svg'
import xiaomiMimoIcon from '@/assets/providers/xiaomi-mimo.svg'
import zaiIcon from '@/assets/providers/zai.svg'
import { useI18n } from '@/i18n'

export function ModelSettingsMenu({
  models,
  modelProvider,
  modelID,
  thinkingLevel,
  contextUsage,
  disabled,
  onChange,
}: {
  models: ModelOption[]
  modelProvider?: string
  modelID?: string
  thinkingLevel?: ThinkingLevel
  contextUsage?: ContextUsage
  disabled: boolean
  onChange: (
    provider: string,
    model: string,
    thinkingLevel: ThinkingLevel,
  ) => Promise<void>
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
          type="button"
          className="group inline-flex h-9 max-w-[248px] cursor-pointer items-center gap-1.5 rounded-full px-2.5 text-[14px] font-medium outline-none transition-colors hover:bg-[rgb(241,241,241)] focus-visible:bg-[rgb(241,241,241)] data-[state=open]:bg-[rgb(237,237,237)] disabled:cursor-not-allowed disabled:opacity-40 max-sm:max-w-[128px] max-sm:px-2"
          aria-label={t('model.settings')}
          disabled={unavailable}
        >
          <ProviderIcon provider={modelProvider ?? ''} />
          <span className="max-w-[150px] truncate text-stone-800 max-sm:max-w-[88px]">
            {modelName}
          </span>
          <span className="shrink-0 text-stone-400 max-sm:hidden">{effortName}</span>
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
          className="z-[100] min-w-[248px] animate-[fade-in_110ms_ease-out] rounded-2xl border border-stone-200 bg-white p-1 text-[14px] text-stone-900 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
        >
          <DropdownMenu.Sub>
            <DropdownMenu.SubTrigger className={subTriggerClass}>
              <span>{t('model.provider')}</span>
              <span className="ml-auto flex min-w-0 items-center gap-1.5 text-stone-500">
                <ProviderIcon provider={selectedProvider || modelProvider || ''} />
                <span className="max-w-[128px] truncate">{selectedProviderName}</span>
                <ChevronRight className="size-3.5 shrink-0" aria-hidden="true" />
              </span>
            </DropdownMenu.SubTrigger>
            <DropdownMenu.Portal>
              <DropdownMenu.SubContent
                sideOffset={6}
                alignOffset={-4}
                collisionPadding={10}
                className="z-[110] min-w-[208px] animate-[fade-in_110ms_ease-out] rounded-2xl border border-stone-200 bg-white p-1 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
              >
                <DropdownMenu.Label className={menuLabelClass}>
                  {t('model.provider')}
                </DropdownMenu.Label>
                <DropdownMenu.Separator className={separatorClass} />
                <DropdownMenu.RadioGroup
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
                <span className="max-w-[128px] truncate">{selectedModelName}</span>
                <ChevronRight className="size-3.5 shrink-0" aria-hidden="true" />
              </span>
            </DropdownMenu.SubTrigger>
            <DropdownMenu.Portal>
              <DropdownMenu.SubContent
                sideOffset={6}
                alignOffset={-4}
                collisionPadding={10}
                className="z-[110] max-h-[min(420px,var(--radix-dropdown-menu-content-available-height))] min-w-[260px] animate-[fade-in_110ms_ease-out] overflow-y-auto rounded-2xl border border-stone-200 bg-white p-1 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
              >
                <DropdownMenu.Label className={menuLabelClass}>
                  {t('model.models', { provider: selectedProviderName })}
                </DropdownMenu.Label>
                <DropdownMenu.Separator className={separatorClass} />
                <DropdownMenu.RadioGroup value={modelKey} onValueChange={selectModel}>
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
                className="z-[110] min-w-[208px] animate-[fade-in_110ms_ease-out] rounded-2xl border border-stone-200 bg-white p-1 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
              >
                <DropdownMenu.Label className={menuLabelClass}>
                  {t('model.effort')}
                </DropdownMenu.Label>
                <DropdownMenu.Separator className={separatorClass} />
                <DropdownMenu.RadioGroup
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
      <div className="flex items-baseline justify-between gap-4 text-[12px] leading-5 tabular-nums">
        <span className="font-[560] text-stone-600">{t('model.context')}</span>
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
        <p className="mt-1 text-[11px] leading-4 text-stone-400">
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
  'flex h-9 cursor-default select-none items-center rounded-[10px] px-2.5 outline-none',
  'data-[highlighted]:bg-[rgb(241,241,241)] data-[state=open]:bg-[rgb(237,237,237)]',
  'data-[disabled]:opacity-40',
)

const radioItemClass = cn(
  'relative flex h-9 cursor-default select-none items-center gap-2 rounded-[10px] px-2.5 pr-9 text-[14px] outline-none',
  'data-[highlighted]:bg-[rgb(241,241,241)] data-[state=checked]:bg-[rgb(237,237,237)] data-[state=checked]:font-medium',
)

const menuLabelClass = 'px-2.5 py-1.5 text-[12px] font-semibold text-stone-400'
const separatorClass = 'mx-1.5 my-0.5 h-px bg-stone-100'

const providerNames: Record<string, string> = {
  anthropic: 'Anthropic',
  deepseek: 'DeepSeek',
  google: 'Google',
  minimax: 'MiniMax',
  'minimax-cn': 'MiniMax CN',
  'kimi-coding': 'Kimi Coding',
  moonshotai: 'Moonshot AI',
  'moonshotai-cn': 'Moonshot AI CN',
  openai: 'OpenAI',
  'opencode-go': 'OpenCode',
  openrouter: 'OpenRouter',
  xai: 'xAI',
  xiaomi: 'Xiaomi',
  'xiaomi-token-plan-ams': 'Xiaomi MiMo AMS',
  'xiaomi-token-plan-cn': 'Xiaomi MiMo CN',
  'xiaomi-token-plan-sgp': 'Xiaomi MiMo SGP',
  zai: 'Z.AI',
  'zai-coding-cn': 'Z.AI Coding CN',
}

const providerIcons: Record<string, string> = {
  deepseek: deepseekIcon,
  minimax: minimaxIcon,
  'minimax-cn': minimaxIcon,
  'kimi-coding': kimiIcon,
  moonshotai: kimiIcon,
  'moonshotai-cn': kimiIcon,
  xiaomi: xiaomiMimoIcon,
  'xiaomi-token-plan-ams': xiaomiMimoIcon,
  'xiaomi-token-plan-cn': xiaomiMimoIcon,
  'xiaomi-token-plan-sgp': xiaomiMimoIcon,
  zai: zaiIcon,
  'zai-coding-cn': zaiIcon,
}

function ProviderIcon({ provider }: { provider: string }) {
  const source = providerIcons[provider]
  const kimi = source === kimiIcon

  if (!source) {
    return (
      <span
        className="grid size-[17px] shrink-0 place-items-center rounded-[5px] bg-stone-100 text-[9px] font-semibold text-stone-500"
        aria-hidden="true"
      >
        {providerName(provider).charAt(0) || '·'}
      </span>
    )
  }

  return (
    <span
      className={cn(
        'grid size-[17px] shrink-0 place-items-center overflow-hidden',
        kimi && 'rounded-[5px] bg-[#1783ff] p-[2px]',
      )}
      aria-hidden="true"
    >
      <img className="size-full object-contain" src={source} alt="" />
    </span>
  )
}

function providerName(provider: string): string {
  return (
    providerNames[provider] ??
    provider
      .split('-')
      .filter(Boolean)
      .map((part) => part[0]?.toUpperCase() + part.slice(1))
      .join(' ')
  )
}
