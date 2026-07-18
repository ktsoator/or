import { useMemo, useState } from 'react'
import { Check, ChevronDown, ChevronRight } from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import type { ModelOption, ThinkingLevel } from '@/types'
import { cn } from '@/lib/utils'

export function ModelSettingsMenu({
  models,
  modelProvider,
  modelID,
  thinkingLevel,
  disabled,
  onChange,
}: {
  models: ModelOption[]
  modelProvider?: string
  modelID?: string
  thinkingLevel?: ThinkingLevel
  disabled: boolean
  onChange: (
    provider: string,
    model: string,
    thinkingLevel: ThinkingLevel,
  ) => Promise<void>
}) {
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
  const modelName = currentModel?.name ?? modelID ?? 'Model'
  const selectedModelName = selectedProvider === modelProvider ? modelName : 'Select model'
  const selectedProviderName = providerName(selectedProvider || modelProvider || '')
  const effortName = thinkingLevel ? thinkingLabel[thinkingLevel] : 'Effort'
  const unavailable = disabled || !modelKey || models.length === 0

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
          className="group inline-flex h-9 max-w-[248px] cursor-pointer items-center gap-1.5 rounded-full px-2.5 text-[14px] font-medium outline-none transition-colors hover:bg-[rgb(241,241,241)] focus-visible:ring-2 focus-visible:ring-stone-300 data-[state=open]:bg-[rgb(241,241,241)] disabled:cursor-not-allowed disabled:opacity-40 max-sm:max-w-[128px] max-sm:px-2"
          aria-label="Model and thinking settings"
          disabled={unavailable}
        >
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
              <span>Provider</span>
              <span className="ml-auto flex min-w-0 items-center gap-1.5 text-stone-500">
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
                <DropdownMenu.Label className={menuLabelClass}>Provider</DropdownMenu.Label>
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
              <span>Model</span>
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
                  {selectedProviderName} models
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
              <span>Effort</span>
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
                <DropdownMenu.Label className={menuLabelClass}>Effort</DropdownMenu.Label>
                <DropdownMenu.Separator className={separatorClass} />
                <DropdownMenu.RadioGroup
                  value={thinkingLevel ?? ''}
                  onValueChange={selectEffort}
                >
                  {thinkingLevels.map((level) => (
                    <DropdownMenu.RadioItem key={level} value={level} className={radioItemClass}>
                      <span>{thinkingLabel[level]}</span>
                      <DropdownMenu.ItemIndicator className="absolute right-2.5 grid size-4 place-items-center text-stone-700">
                        <Check className="size-3.5" aria-hidden="true" />
                      </DropdownMenu.ItemIndicator>
                    </DropdownMenu.RadioItem>
                  ))}
                </DropdownMenu.RadioGroup>
              </DropdownMenu.SubContent>
            </DropdownMenu.Portal>
          </DropdownMenu.Sub>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

const subTriggerClass = cn(
  'flex h-9 cursor-default select-none items-center rounded-[10px] px-2.5 outline-none',
  'data-[highlighted]:bg-[rgb(241,241,241)] data-[state=open]:bg-[rgb(241,241,241)]',
  'data-[disabled]:opacity-40',
)

const radioItemClass = cn(
  'relative flex h-9 cursor-default select-none items-center rounded-[10px] px-2.5 pr-9 text-[14px] outline-none',
  'data-[highlighted]:bg-[rgb(241,241,241)] data-[state=checked]:bg-[rgb(241,241,241)] data-[state=checked]:font-medium',
)

const menuLabelClass = 'px-2.5 py-1.5 text-[12px] font-semibold text-stone-400'
const separatorClass = 'mx-1.5 my-0.5 h-px bg-stone-100'

const thinkingLabel: Record<ThinkingLevel, string> = {
  off: 'Off',
  minimal: 'Minimal',
  low: 'Low',
  medium: 'Medium',
  high: 'High',
  xhigh: 'Extra High',
}

const providerNames: Record<string, string> = {
  anthropic: 'Anthropic',
  deepseek: 'DeepSeek',
  google: 'Google',
  minimax: 'MiniMax',
  'minimax-cn': 'MiniMax CN',
  moonshotai: 'Moonshot AI',
  'moonshotai-cn': 'Moonshot AI CN',
  openai: 'OpenAI',
  'opencode-go': 'OpenCode',
  openrouter: 'OpenRouter',
  xai: 'xAI',
  xiaomi: 'Xiaomi',
  zai: 'Z.AI',
  'zai-coding-cn': 'Z.AI Coding CN',
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
