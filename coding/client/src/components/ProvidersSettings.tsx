import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import {
  Activity,
  Check,
  ChevronDown,
  LoaderCircle,
  Plus,
  Save as SaveIcon,
  TriangleAlert,
  Trash2,
} from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import { apiURL } from '@/api'
import { useI18n } from '@/i18n'
import { cn } from '@/lib/utils'
import { ProviderIcon } from '@/components/ProviderIdentity'
import { ProviderConnectionTestDialog } from '@/components/ProviderConnectionTestDialog'
import { FixedThinkingStatus } from '@/components/FixedThinkingStatus'
import { ThinkingModeToggle } from '@/components/ThinkingModeToggle'
import { UtilityModelSection } from '@/components/UtilityModelSection'
import { providerName } from '@/lib/provider'
import { isFixedThinking, isToggleThinking, toggleThinkingLevel } from '@/modelThinking'
import type {
  ModelOption,
  ProviderConnectionInfo,
  ProviderInfo,
  ProviderListResponse,
  ProviderSelectionRepair,
  ThinkingLevel,
} from '@/types'

type KeyDraft = {
  id: string
  name: string
  preview: string
  apiKey: string
  persisted: boolean
}

type ConnectionDraft = Omit<ProviderConnectionInfo, 'keys'> & {
  keys: KeyDraft[]
  persisted: boolean
}

export function ProvidersSettings({ onChanged }: { onChanged?: () => void }) {
  const { t } = useI18n()
  const [providerData, setProviderData] = useState<ProviderListResponse>()
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [selectedProviderId, setSelectedProviderId] = useState<string>()

  const load = useCallback(
    async (signal?: AbortSignal) => {
      try {
        const providersResponse = await fetch(apiURL('/providers'), { cache: 'no-store', signal })
        if (!providersResponse.ok) {
          throw new Error(`HTTP ${providersResponse.status}`)
        }
        const data = (await providersResponse.json()) as ProviderListResponse
        setProviderData(data)
        setSelectedProviderId((current) =>
          current && data.providers.some((provider) => provider.id === current)
            ? current
            : data.activeModel?.provider,
        )
        setError('')
      } catch (cause) {
        if (cause instanceof DOMException && cause.name === 'AbortError') return
        setError(t('providers.loadFailed'))
      } finally {
        if (!signal?.aborted) setLoading(false)
      }
    },
    [t],
  )

  useEffect(() => {
    const controller = new AbortController()
    void load(controller.signal)
    return () => controller.abort()
  }, [load])

  const afterChange = useCallback(async () => {
    await load()
    onChanged?.()
  }, [load, onChanged])

  const providers = providerData?.providers ?? []
  const selected = providers.find((provider) => provider.id === selectedProviderId)

  return (
    <div>
      {providerData?.repairs && providerData.repairs.length > 0 && (
        <div
          className="mb-6 flex gap-2.5 border-l-2 border-warning-edge bg-warning-surface/45 px-3 py-2.5 text-[0.8125rem] leading-5 text-warning"
          role="status"
        >
          <TriangleAlert className="mt-0.5 size-4 shrink-0 text-warning" aria-hidden="true" />
          <div className="min-w-0 space-y-1">
            {providerData.repairs.map((repair, index) => (
              <p key={`${repair.target}:${repair.previous.provider}:${repair.previous.model}:${index}`}>
                {selectionRepairMessage(repair, t)}
              </p>
            ))}
          </div>
        </div>
      )}
      <DefaultModelSection
        onChanged={afterChange}
        utilityModel={providerData && (
          <UtilityModelSection
            providers={providerData.providers}
            selection={providerData.utilityModel}
            onChanged={afterChange}
          />
        )}
      />

      {loading ? (
        <div className="flex items-center gap-2 py-6 text-[0.8125rem] text-ink-faint">
          <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
          {t('providers.loading')}
        </div>
      ) : error ? (
        <div className="rounded-lg border border-danger-edge bg-danger-surface/60 px-4 py-3 text-[0.8125rem] text-danger">
          {error}
        </div>
      ) : selected ? (
          <ProviderConfigPanel
            key={selected.id}
            providers={providers}
            info={selected}
            selectedProviderId={selected.id}
            preferredModelId={providerData?.activeModel?.provider === selected.id ? providerData.activeModel.model : ''}
            onSelectProvider={setSelectedProviderId}
            onChanged={afterChange}
          />
      ) : (
        <div className="flex min-h-36 items-start justify-end border-t border-edge-soft pt-5">
          <ProviderPicker
            providers={providers}
            value={selectedProviderId}
            onChange={setSelectedProviderId}
          />
        </div>
      )}
    </div>
  )
}

function selectionRepairMessage(
  repair: ProviderSelectionRepair,
  t: ReturnType<typeof useI18n>['t'],
): string {
  const previous = modelReferenceLabel(repair.previous)
  const replacement = repair.replacement && modelReferenceLabel(repair.replacement)
  if (repair.reason === 'unsupported_thinking_level' && repair.replacement) {
    return t('providers.defaultThinkingRecovered', {
      model: previous,
      previous: thinkingLevelLabel(repair.previous.thinkingLevel, t),
      replacement: thinkingLevelLabel(repair.replacement.thinkingLevel, t),
    })
  }
  if (repair.target === 'utility_model') {
    if (
      repair.replacement &&
      repair.replacement.provider === repair.previous.provider &&
      repair.replacement.model === repair.previous.model
    ) {
      return t('providers.utilityRouteRecovered', { model: previous })
    }
    return replacement
      ? t('providers.utilityModelRecovered', { previous, replacement })
      : t('providers.utilityModelCleared', { previous })
  }
  return replacement
    ? t('providers.defaultModelRecovered', { previous, replacement })
    : t('providers.defaultModelCleared', { previous })
}

function modelReferenceLabel(reference: { provider: string; model: string }): string {
  return `${providerName(reference.provider) || reference.provider}/${reference.model}`
}

const thinkingLabelKeys = {
  off: 'effort.off',
  minimal: 'effort.minimal',
  low: 'effort.low',
  medium: 'effort.medium',
  high: 'effort.high',
  xhigh: 'effort.xhigh',
  max: 'effort.max',
} as const

function thinkingLevelLabel(
  level: ThinkingLevel | undefined,
  t: ReturnType<typeof useI18n>['t'],
): string {
  return t(thinkingLabelKeys[level ?? 'off'])
}

type ModelsResponse = {
  models: ModelOption[]
  defaultProvider: string
  defaultModel: string
  defaultThinkingLevel: ThinkingLevel
}

// DefaultModelSection lets the user pick the application-wide default model and
// thinking mode that new sessions start with. It reads the model catalog and
// current default from /api/models and persists changes to /api/model-selection.
// The UI uses provider, model, and model-specific thinking controls.
function DefaultModelSection({
  onChanged,
  utilityModel,
}: {
  onChanged?: () => void
  utilityModel?: ReactNode
}) {
  const { t } = useI18n()
  const [models, setModels] = useState<ModelOption[]>([])
  const [provider, setProvider] = useState('')
  const [model, setModel] = useState('')
  const [thinking, setThinking] = useState<ThinkingLevel>('medium')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  const loadModels = useCallback(
    async (signal?: AbortSignal) => {
      try {
        const response = await fetch(apiURL('/models'), { cache: 'no-store', signal })
        if (!response.ok) throw new Error(`HTTP ${response.status}`)
        const data = (await response.json()) as ModelsResponse
        setModels(data.models)
        setProvider(data.defaultProvider)
        setModel(data.defaultModel)
        if (data.defaultThinkingLevel) setThinking(data.defaultThinkingLevel)
        setError('')
      } catch (cause) {
        if (cause instanceof DOMException && cause.name === 'AbortError') return
        setError(t('providers.loadFailed'))
      } finally {
        if (!signal?.aborted) setLoading(false)
      }
    },
    [t],
  )

  useEffect(() => {
    const controller = new AbortController()
    void loadModels(controller.signal)
    return () => controller.abort()
  }, [loadModels])

  // Unique providers from the model catalog.
  const providers = useMemo(() => [...new Set(models.map((m) => m.provider))], [models])

  // Models filtered by the currently selected provider.
  const providerModels = useMemo(
    () => models.filter((m) => m.provider === provider),
    [models, provider],
  )

  const current = models.find((entry) => entry.provider === provider && entry.id === model)
  const thinkingLevels = current?.thinkingLevels ?? []
  const fixedThinking = isFixedThinking(current)
  const toggleThinking = isToggleThinking(current)

  const persist = async (nextProvider: string, nextModel: string, nextThinking: ThinkingLevel) => {
    setSaving(true)
    setError('')
    try {
      const response = await fetch(apiURL('/model-selection'), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ provider: nextProvider, model: nextModel, thinkingLevel: nextThinking }),
      })
      if (!response.ok) {
        let message = t('settings.defaultModelSaveFailed')
        try {
          const body = (await response.json()) as { error?: string }
          if (body.error) message = body.error
        } catch {
          // fall back to the localized message for non-JSON responses
        }
        throw new Error(message)
      }
      const saved = (await response.json()) as { provider: string; model: string; thinkingLevel: ThinkingLevel }
      setProvider(saved.provider)
      setModel(saved.model)
      setThinking(saved.thinkingLevel)
      onChanged?.()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t('settings.defaultModelSaveFailed'))
    } finally {
      setSaving(false)
    }
  }

  const chooseProvider = (nextProvider: string) => {
    if (nextProvider === provider) return
    const candidates = models.filter((m) => m.provider === nextProvider)
    if (candidates.length === 0) return
    // Try to keep the same model id; otherwise pick the first available.
    const entry = candidates.find((c) => c.id === model) ?? candidates[0]
    const nextThinking = entry.thinkingLevels.includes(thinking)
      ? thinking
      : (entry.thinkingLevels.find((level) => level === 'medium') ??
        entry.thinkingLevels.find((level) => level !== 'off') ??
        entry.thinkingLevels[0] ??
        'off')
    void persist(nextProvider, entry.id, nextThinking)
  }

  const chooseModel = (nextModel: string) => {
    if (nextModel === model) return
    const entry = providerModels.find((c) => c.id === nextModel)
    if (!entry) return
    const nextThinking = entry.thinkingLevels.includes(thinking)
      ? thinking
      : (entry.thinkingLevels.find((level) => level === 'medium') ??
        entry.thinkingLevels.find((level) => level !== 'off') ??
        entry.thinkingLevels[0] ??
        'off')
    void persist(provider, nextModel, nextThinking)
  }

  const chooseThinking = (level: string) => {
    if (!provider || !model || level === thinking) return
    void persist(provider, model, level as ThinkingLevel)
  }

  // providerName returns an empty string for an unset provider, which rendered
  // as a bare icon placeholder with no label beside it.
  const providerLabel = provider ? providerName(provider) : t('settings.defaultModelNone')
  const modelLabel = current?.name ?? (model || t('settings.defaultModelNone'))

  // The three selectors are one control, so they share a trigger style. The
  // cursor has to say why a trigger is dead: a wait cursor during a save, and
  // not-allowed when there is simply nothing to choose from yet.
  const triggerClass = cn(
    'inline-flex h-9 min-w-0 items-center gap-1.5 rounded-[10px] bg-surface-hover px-2.5 text-left text-[0.8125rem] text-ink-soft outline-none transition-colors hover:bg-surface-active focus-visible:bg-surface-active data-[state=open]:bg-surface-selected disabled:opacity-60',
    saving ? 'cursor-wait' : 'cursor-pointer disabled:cursor-not-allowed',
  )

  return (
    <section className="mb-8">
      <div className="overflow-hidden rounded-[18px] border border-edge/90 bg-canvas px-4 shadow-[0_10px_32px_-30px_rgba(28,25,23,0.45)]">
        {loading ? (
          <div className={cn(
            'flex items-center gap-2 py-6 text-[0.8125rem] text-ink-faint',
            utilityModel && 'border-b border-edge/75',
          )}>
            <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
            {t('providers.loading')}
          </div>
        ) : models.length === 0 ? (
          <div className={cn(
            'py-6 text-[0.8125rem] text-ink-faint',
            utilityModel && 'border-b border-edge/75',
          )}>
            {t('settings.defaultModelEmpty')}
          </div>
        ) : (
          <SettingsRowLike label={t('settings.defaultModel')} description={t('settings.defaultModelDescription')}>
            <div className="flex items-center gap-1.5">
                {/* Provider */}
                <DropdownMenu.Root>
                  <DropdownMenu.Trigger asChild>
                    <button
                      type="button"
                      aria-label={t('settings.defaultModelProvider')}
                      className={triggerClass}
                      disabled={saving}
                    >
                      {provider && <ProviderIcon provider={provider} />}
                      <span className="max-w-[7rem] truncate">{providerLabel}</span>
                      <ChevronDown className="size-3.5 shrink-0 text-ink-faint" aria-hidden="true" />
                    </button>
                  </DropdownMenu.Trigger>
                  <DropdownMenu.Portal>
                    <DropdownMenu.Content
                      side="bottom"
                      align="end"
                      sideOffset={7}
                      collisionPadding={10}
                      className="z-[100] max-h-[min(24rem,60vh)] min-w-[14rem] overflow-y-auto animate-[fade-in_110ms_ease-out] rounded-[14px] border border-edge bg-canvas p-1 text-[0.8125rem] text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
                    >
                      <DropdownMenu.RadioGroup value={provider} onValueChange={chooseProvider}>
                        <div className="flex flex-col gap-0.5">
                          {providers.map((p) => (
                            <DropdownMenu.RadioItem
                              key={p}
                              value={p}
                              className="relative flex h-9 cursor-pointer items-center gap-2 rounded-[9px] px-2.5 pr-8 outline-none select-none data-[highlighted]:bg-surface-active data-[state=checked]:bg-surface-selected"
                            >
                              <ProviderIcon provider={p} />
                              <span className="min-w-0 flex-1 truncate">{providerName(p)}</span>
                              <DropdownMenu.ItemIndicator className="absolute right-2 grid size-4 place-items-center text-ink-soft">
                                <Check className="size-3.5" aria-hidden="true" />
                              </DropdownMenu.ItemIndicator>
                            </DropdownMenu.RadioItem>
                          ))}
                        </div>
                      </DropdownMenu.RadioGroup>
                    </DropdownMenu.Content>
                  </DropdownMenu.Portal>
                </DropdownMenu.Root>

                {/* Model */}
                <DropdownMenu.Root>
                  <DropdownMenu.Trigger asChild>
                    <button
                      type="button"
                      aria-label={t('settings.defaultModelModel')}
                      className={triggerClass}
                      disabled={saving || providerModels.length === 0}
                    >
                      <span
                        className="max-w-[9rem] truncate xl:max-w-[20rem]"
                        title={modelLabel}
                      >
                        {modelLabel}
                      </span>
                      <ChevronDown className="size-3.5 shrink-0 text-ink-faint" aria-hidden="true" />
                    </button>
                  </DropdownMenu.Trigger>
                  <DropdownMenu.Portal>
                    <DropdownMenu.Content
                      side="bottom"
                      align="end"
                      sideOffset={7}
                      collisionPadding={10}
                      className="z-[100] max-h-[min(24rem,60vh)] min-w-[16rem] overflow-y-auto animate-[fade-in_110ms_ease-out] rounded-[14px] border border-edge bg-canvas p-1 text-[0.8125rem] text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
                    >
                      <DropdownMenu.RadioGroup value={model} onValueChange={chooseModel}>
                        <div className="flex flex-col gap-0.5">
                          {providerModels.map((entry) => (
                            <DropdownMenu.RadioItem
                              key={entry.id}
                              value={entry.id}
                              className="relative flex h-9 cursor-pointer items-center rounded-[9px] px-2.5 pr-8 outline-none select-none data-[highlighted]:bg-surface-active data-[state=checked]:bg-surface-selected"
                            >
                              <span className="min-w-0 flex-1 truncate">{entry.name}</span>
                              <DropdownMenu.ItemIndicator className="absolute right-2 grid size-4 place-items-center text-ink-soft">
                                <Check className="size-3.5" aria-hidden="true" />
                              </DropdownMenu.ItemIndicator>
                            </DropdownMenu.RadioItem>
                          ))}
                        </div>
                      </DropdownMenu.RadioGroup>
                    </DropdownMenu.Content>
                  </DropdownMenu.Portal>
                </DropdownMenu.Root>

                {/* Model-specific thinking control */}
                {fixedThinking ? (
                  <FixedThinkingStatus
                    className="h-9 rounded-[10px] bg-surface-hover px-2.5 text-[0.8125rem] text-ink-muted outline-none hover:bg-surface-active focus-visible:bg-surface-active"
                    hidden={current?.thinkingVisibility === 'hidden'}
                  />
                ) : toggleThinking ? (
                  <ThinkingModeToggle
                    checked={thinking === 'high'}
                    disabled={saving}
                    ariaLabel={t('model.thinking')}
                    className="bg-surface-hover"
                    onCheckedChange={(checked) => {
                      const level = toggleThinkingLevel(checked)
                      if (provider && model && level !== thinking) {
                        void persist(provider, model, level)
                      }
                    }}
                  />
                ) : (
                  <DropdownMenu.Root>
                    <DropdownMenu.Trigger asChild>
                      <button
                        type="button"
                        aria-label={t('settings.defaultModelThinking')}
                        className={triggerClass}
                        disabled={saving || thinkingLevels.length === 0}
                      >
                        <span className="truncate">{t(`effort.${thinking}` as Parameters<typeof t>[0])}</span>
                        <ChevronDown className="size-3.5 shrink-0 text-ink-faint" aria-hidden="true" />
                      </button>
                    </DropdownMenu.Trigger>
                    <DropdownMenu.Portal>
                      <DropdownMenu.Content
                        side="bottom"
                        align="end"
                        sideOffset={7}
                        collisionPadding={10}
                        className="z-[100] min-w-[10rem] animate-[fade-in_110ms_ease-out] rounded-[14px] border border-edge bg-canvas p-1 text-[0.8125rem] text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
                      >
                        <DropdownMenu.RadioGroup value={thinking} onValueChange={chooseThinking}>
                          <div className="flex flex-col gap-0.5">
                            {thinkingLevels.map((level) => (
                              <DropdownMenu.RadioItem
                                key={level}
                                value={level}
                                className="relative flex h-9 cursor-pointer items-center rounded-[9px] px-2.5 pr-8 outline-none select-none data-[highlighted]:bg-surface-active data-[state=checked]:bg-surface-selected"
                              >
                                <span>{t(`effort.${level}` as Parameters<typeof t>[0])}</span>
                                <DropdownMenu.ItemIndicator className="absolute right-2 grid size-4 place-items-center text-ink-soft">
                                  <Check className="size-3.5" aria-hidden="true" />
                                </DropdownMenu.ItemIndicator>
                              </DropdownMenu.RadioItem>
                            ))}
                          </div>
                        </DropdownMenu.RadioGroup>
                      </DropdownMenu.Content>
                    </DropdownMenu.Portal>
                  </DropdownMenu.Root>
                )}
            </div>
          </SettingsRowLike>
        )}
        {utilityModel}
      </div>
      {error && <p className="mt-2 text-[0.8125rem] text-danger-soft">{error}</p>}
    </section>
  )
}

function SettingsRowLike({
  label,
  description,
  children,
}: {
  label: string
  description: string
  children: ReactNode
}) {
  return (
    <div className="flex min-h-[4.375rem] items-center gap-6 border-b border-edge/75 py-3 last:border-b-0 max-sm:items-start max-sm:gap-3">
      <div className="min-w-0 flex-1">
        <div className="text-[0.84375rem] leading-5 font-medium text-ink">{label}</div>
        <p className="mt-0.5 max-w-[38.75rem] text-[0.78125rem] leading-[1.45] text-ink-muted">{description}</p>
      </div>
      <div className="shrink-0 max-sm:pt-0.5">{children}</div>
    </div>
  )
}

function ProviderPicker({
  providers,
  value,
  onChange,
}: {
  providers: ProviderInfo[]
  value?: string
  onChange: (id: string) => void
}) {
  const { t } = useI18n()
  const selected = providers.find((provider) => provider.id === value)
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          aria-label={t('providers.provider')}
          className="group flex h-9 min-w-[10.5rem] max-w-[15rem] shrink-0 cursor-pointer items-center gap-2 rounded-[10px] bg-surface-hover px-2.5 text-left text-[0.8125rem] outline-none transition-colors hover:bg-surface-active focus-visible:bg-surface-active data-[state=open]:bg-surface-selected max-sm:min-w-0"
        >
          {selected && <ProviderIcon provider={selected.id} />}
          <span className={cn('min-w-0 flex-1 truncate', selected ? 'text-ink-soft' : 'text-ink-muted')}>
            {selected ? providerName(selected.id) : t('providers.selectProvider')}
          </span>
          <ChevronDown
            className="size-3.5 shrink-0 text-ink-faint transition-transform duration-150 group-data-[state=open]:rotate-180"
            aria-hidden="true"
          />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          side="bottom"
          align="end"
          sideOffset={7}
          collisionPadding={10}
          className="z-[100] max-h-[min(24rem,60vh)] min-w-[15rem] overflow-y-auto animate-[fade-in_110ms_ease-out] rounded-[14px] border border-edge bg-canvas p-1 text-[0.8125rem] text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
        >
          <DropdownMenu.RadioGroup value={value ?? ''} onValueChange={onChange}>
            <div className="flex flex-col gap-0.5">
              {providers.map((provider) => (
                <DropdownMenu.RadioItem
                  key={provider.id}
                  value={provider.id}
                  className="relative flex h-9 cursor-default select-none items-center gap-2 rounded-[9px] px-2.5 pr-8 outline-none data-[highlighted]:bg-surface-active data-[state=checked]:bg-surface-selected"
                >
                  <ProviderIcon provider={provider.id} />
                  <span className="min-w-0 flex-1 truncate">{providerName(provider.id)}</span>
                  <DropdownMenu.ItemIndicator className="absolute right-2 grid size-4 place-items-center text-ink-soft">
                    <Check className="size-3.5" aria-hidden="true" />
                  </DropdownMenu.ItemIndicator>
                </DropdownMenu.RadioItem>
              ))}
            </div>
          </DropdownMenu.RadioGroup>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

function ConnectionPicker({
  connections,
  officialBaseURL,
  value,
  activeValue,
  onChange,
}: {
  connections: ConnectionDraft[]
  officialBaseURL: string
  value: string
  activeValue: string
  onChange: (id: string) => void
}) {
  const { t } = useI18n()
  const selected = connections.find((connection) => connection.id === value) ?? connections[0]
  if (!selected) return null
  const selectedName = selected.official ? t('providers.officialConnection') : selected.name || t('providers.customConnection')
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          aria-label={t('providers.connection')}
          className="group flex h-9 min-w-[10.5rem] max-w-[15rem] shrink-0 cursor-pointer items-center gap-2 rounded-[10px] bg-surface-hover px-2.5 text-left text-[0.8125rem] outline-none transition-colors hover:bg-surface-active focus-visible:bg-surface-active data-[state=open]:bg-surface-selected max-sm:min-w-0"
        >
          <span className={cn('size-2 shrink-0 rounded-full', selected.id === activeValue ? 'bg-canvas-inverse' : 'bg-ink-ghost')} />
          <span className="min-w-0 flex-1 truncate text-ink-soft">{selectedName}</span>
          <ChevronDown className="size-3.5 shrink-0 text-ink-faint transition-transform duration-150 group-data-[state=open]:rotate-180" aria-hidden="true" />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          side="bottom"
          align="end"
          sideOffset={7}
          collisionPadding={10}
          className="z-[100] min-w-[17rem] animate-[fade-in_110ms_ease-out] rounded-[14px] border border-edge bg-canvas p-1 text-[0.8125rem] text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
        >
          <DropdownMenu.RadioGroup className="flex flex-col gap-0.5" value={value} onValueChange={onChange}>
            {connections.map((connection) => {
              const name = connection.official ? t('providers.officialConnection') : connection.name || t('providers.customConnection')
              const baseURL = connection.official ? officialBaseURL : connection.baseURL
              return (
                <DropdownMenu.RadioItem
                  key={connection.id}
                  value={connection.id}
                  className="relative flex min-h-10 cursor-default select-none items-center gap-2 rounded-[9px] px-2.5 py-1.5 pr-8 outline-none data-[highlighted]:bg-surface-active data-[state=checked]:bg-surface-selected"
                >
                  <span className={cn('size-2 shrink-0 rounded-full', connection.id === activeValue ? 'bg-canvas-inverse' : 'bg-ink-ghost')} />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-ink-soft">{name}</span>
                    <span className="block truncate font-mono text-[0.6875rem] text-ink-faint">{baseURL || t('providers.notSet')}</span>
                  </span>
                  <DropdownMenu.ItemIndicator className="absolute right-2 grid size-4 place-items-center text-ink-soft">
                    <Check className="size-3.5" aria-hidden="true" />
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

function ProviderConfigPanel({
  providers,
  info,
  selectedProviderId,
  preferredModelId,
  onSelectProvider,
  onChanged,
}: {
  providers: ProviderInfo[]
  info: ProviderInfo
  selectedProviderId: string
  preferredModelId: string
  onSelectProvider: (value: string) => void
  onChanged: () => void | Promise<void>
}) {
  const { t } = useI18n()
  const [connections, setConnections] = useState<ConnectionDraft[]>(() => draftsFromInfo(info))
  const [activeConnectionId, setActiveConnectionId] = useState(info.activeConnectionId)
  const [configured, setConfigured] = useState(info.configured)
  const [editingConnectionId, setEditingConnectionId] = useState(info.activeConnectionId)
  const [saving, setSaving] = useState(false)
  const [activating, setActivating] = useState('')
  const [rowError, setRowError] = useState('')
  const [testTarget, setTestTarget] = useState<{ connectionID: string; keyID: string }>()
  const selectedConnection = connections.find((connection) => connection.id === editingConnectionId) ?? connections[0]
  const testConnection = connections.find((connection) => connection.id === testTarget?.connectionID)
  const testKey = testConnection?.keys.find((key) => key.id === testTarget?.keyID)
  const hasUnsavedChanges = profileHasUnsavedChanges(info, connections)

  useEffect(() => {
    setConnections(draftsFromInfo(info))
    setActiveConnectionId(info.activeConnectionId)
    setConfigured(info.configured)
    setEditingConnectionId((current) =>
      info.connections.some((connection) => connection.id === current) ? current : info.activeConnectionId,
    )
  }, [info])

  const updateConnection = (id: string, update: (current: ConnectionDraft) => ConnectionDraft) => {
    setConnections((current) => current.map((connection) => (connection.id === id ? update(connection) : connection)))
  }

  const addConnection = () => {
    const id = localID('conn')
    setConnections((current) => [
      ...current,
      { id, name: '', baseURL: '', official: false, activeKeyId: '', keys: [], persisted: false },
    ])
    setEditingConnectionId(id)
  }

  const removeConnection = (id: string) => {
    setConnections((current) => current.filter((connection) => connection.id !== id))
    if (editingConnectionId === id) setEditingConnectionId('official')
  }

  const addKey = (connectionID: string) => {
    const id = localID('key')
    updateConnection(connectionID, (connection) => ({
      ...connection,
      keys: [...connection.keys, { id, name: '', preview: '', apiKey: '', persisted: false }],
    }))
  }

  const applyProviderInfo = (updated: ProviderInfo) => {
    setConnections(draftsFromInfo(updated))
    setActiveConnectionId(updated.activeConnectionId)
    setConfigured(updated.configured)
    setEditingConnectionId((current) =>
      updated.connections.some((connection) => connection.id === current)
        ? current
        : updated.activeConnectionId,
    )
  }

  const activateConnection = async (connectionID: string) => {
    setActivating(`connection:${connectionID}`)
    setRowError('')
    try {
      const response = await fetch(apiURL(`/providers/${info.id}/active-connection`), {
        method: 'PATCH',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ connectionId: connectionID }),
      })
      if (!response.ok) {
        const body = (await response.json().catch(() => ({}))) as { error?: string }
        throw new Error(body.error || `HTTP ${response.status}`)
      }
      const updated = (await response.json()) as ProviderInfo
      setActivating('')
      applyProviderInfo(updated)
      void onChanged()
    } catch (cause) {
      setRowError(cause instanceof Error && cause.message ? cause.message : t('providers.activateFailed'))
    } finally {
      setActivating('')
    }
  }

  const activateKey = async (connectionID: string, keyID: string) => {
    setActivating(`key:${connectionID}:${keyID}`)
    setRowError('')
    try {
      const response = await fetch(apiURL(`/providers/${info.id}/connections/${connectionID}/active-key`), {
        method: 'PATCH',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ keyId: keyID }),
      })
      if (!response.ok) {
        const body = (await response.json().catch(() => ({}))) as { error?: string }
        throw new Error(body.error || `HTTP ${response.status}`)
      }
      const updated = (await response.json()) as ProviderInfo
      setActivating('')
      applyProviderInfo(updated)
      void onChanged()
    } catch (cause) {
      setRowError(cause instanceof Error && cause.message ? cause.message : t('providers.activateFailed'))
    } finally {
      setActivating('')
    }
  }

  const save = async () => {
    setSaving(true)
    setRowError('')
    try {
      const response = await fetch(apiURL(`/providers/${info.id}`), {
        method: 'PUT',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          activeConnectionId,
          connections: connections.map((connection) => ({
            id: connection.id,
            name: connection.name,
            baseURL: connection.official ? '' : connection.baseURL,
            activeKeyId: connection.activeKeyId,
            keys: connection.keys.map((key) => ({ id: key.id, name: key.name, apiKey: key.apiKey })),
          })),
        }),
      })
      if (!response.ok) {
        const body = (await response.json().catch(() => ({}))) as { error?: string }
        throw new Error(body.error || `HTTP ${response.status}`)
      }
      await onChanged()
    } catch (cause) {
      setRowError(cause instanceof Error && cause.message ? cause.message : t('providers.saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <div className="mb-5 flex items-center justify-between gap-3 max-sm:items-start">
        <div className="text-[0.875rem] font-medium text-ink-soft">{t('providers.routing')}</div>
        <div className="flex shrink-0 items-center gap-1.5 max-sm:grid max-sm:w-full max-sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_2.25rem]">
          <ProviderPicker providers={providers} value={selectedProviderId} onChange={onSelectProvider} />
          <ConnectionPicker
            connections={connections}
            officialBaseURL={info.officialBaseURL ?? ''}
            value={editingConnectionId}
            activeValue={configured ? activeConnectionId : ''}
            onChange={setEditingConnectionId}
          />
          <button
            type="button"
            onClick={addConnection}
            aria-label={t('providers.addConnection')}
            title={t('providers.addConnection')}
            className="grid size-9 shrink-0 cursor-pointer place-items-center rounded-[10px] bg-surface-hover text-ink-muted outline-none transition-colors hover:bg-surface-active hover:text-ink focus-visible:bg-surface-active focus-visible:text-ink"
          >
            <Plus className="size-4" aria-hidden="true" />
          </button>
        </div>
      </div>

      <div className="overflow-hidden rounded-lg border border-edge bg-canvas shadow-[0_12px_32px_-32px_rgba(28,25,23,0.45)]">
        {selectedConnection && (
          <ConnectionEditor
            key={selectedConnection.id}
            officialBaseURL={info.officialBaseURL ?? ''}
            connection={selectedConnection}
            active={configured && selectedConnection.id === activeConnectionId}
            activationBlocked={hasUnsavedChanges}
            activating={activating}
            saving={saving}
            saveDisabled={!hasUnsavedChanges || Boolean(activating)}
            onChange={(update) => updateConnection(selectedConnection.id, update)}
            onAddKey={() => addKey(selectedConnection.id)}
            onRemove={() => removeConnection(selectedConnection.id)}
            onActivateConnection={() => void activateConnection(selectedConnection.id)}
            onActivateKey={(keyID) => void activateKey(selectedConnection.id, keyID)}
            onTestKey={(keyID) => setTestTarget({ connectionID: selectedConnection.id, keyID })}
            onSave={() => void save()}
          />
        )}

        {rowError && (
          <p className="border-t border-danger-edge bg-danger-surface/60 px-4 py-2 text-[0.75rem] text-danger-soft">
            {rowError}
          </p>
        )}
      </div>

      {testConnection && testKey && (
        <ProviderConnectionTestDialog
          providerId={info.id}
          providerLabel={providerName(info.id)}
          preferredModelId={preferredModelId}
          connection={testConnection}
          credential={testKey}
          onOpenChange={(open) => {
            if (!open) setTestTarget(undefined)
          }}
        />
      )}
    </>
  )
}

function ConnectionEditor({
  officialBaseURL,
  connection,
  active,
  activationBlocked,
  activating,
  saving,
  saveDisabled,
  onChange,
  onAddKey,
  onRemove,
  onActivateConnection,
  onActivateKey,
  onTestKey,
  onSave,
}: {
  officialBaseURL: string
  connection: ConnectionDraft
  active: boolean
  activationBlocked: boolean
  activating: string
  saving: boolean
  saveDisabled: boolean
  onChange: (update: (current: ConnectionDraft) => ConnectionDraft) => void
  onAddKey: () => void
  onRemove: () => void
  onActivateConnection: () => void
  onActivateKey: (keyID: string) => void
  onTestKey: (keyID: string) => void
  onSave: () => void
}) {
  const { t } = useI18n()

  const updateKey = (id: string, patch: Partial<KeyDraft>) => {
    onChange((current) => ({
      ...current,
      keys: current.keys.map((key) => (key.id === id ? { ...key, ...patch } : key)),
    }))
  }

  const removeKey = (id: string) => {
    onChange((current) => ({
      ...current,
      activeKeyId: current.activeKeyId === id ? '' : current.activeKeyId,
      keys: current.keys.filter((key) => key.id !== id),
    }))
  }

  const connectionBusy = activating === `connection:${connection.id}`
  const connectionActivationDisabled =
    !connection.persisted ||
    activationBlocked ||
    Boolean(activating) ||
    !connection.activeKeyId

  return (
    <section className="px-5 py-4 max-sm:px-4">
      <div className="flex items-center gap-2.5">
        {connection.official ? (
          <span className="min-w-0 flex-1 text-[0.84375rem] font-medium text-ink">
            {t('providers.officialConnection')}
          </span>
        ) : (
          <input
            value={connection.name}
            onChange={(event) => onChange((current) => ({ ...current, name: event.target.value }))}
            placeholder={t('providers.connectionNamePlaceholder')}
            className="min-w-0 flex-1 border-0 bg-transparent p-0 text-[0.9375rem] font-medium text-ink outline-none placeholder:text-ink-faint"
          />
        )}
        <div className="flex shrink-0 items-center gap-1">
          <button
            type="button"
            onClick={onActivateConnection}
            disabled={active || connectionActivationDisabled}
            aria-busy={connectionBusy}
            aria-label={connectionBusy ? t('providers.activating') : undefined}
            title={
              active
                ? undefined
                : !connection.persisted || activationBlocked
                  ? t('providers.saveBeforeActivate')
                  : !connection.activeKeyId
                    ? t('providers.selectKeyFirst')
                    : undefined
            }
            className={cn(
              'inline-flex h-7 min-w-[5.5rem] items-center justify-center gap-1.5 rounded-md px-2.5 text-[0.71875rem] font-medium transition-colors',
              active
                ? 'cursor-default text-ink-muted'
                : 'cursor-pointer text-ink-muted hover:bg-surface-active hover:text-ink disabled:cursor-not-allowed disabled:text-ink-ghost disabled:hover:bg-transparent',
            )}
          >
            {connectionBusy ? (
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
            ) : active ? (
              <Check className="size-3.5" aria-hidden="true" />
            ) : null}
            <span>{active ? t('providers.active') : t('providers.useConnection')}</span>
          </button>
          <button
            type="button"
            onClick={onSave}
            disabled={saving || saveDisabled}
            className={cn(
              'inline-flex h-7 items-center gap-1.5 rounded-md bg-canvas-inverse px-2.5 text-[0.71875rem] font-medium text-ink-inverse transition-colors hover:bg-canvas-inverse disabled:bg-canvas-sunken disabled:text-ink-faint',
              saving ? 'cursor-wait' : 'cursor-pointer disabled:cursor-default',
            )}
          >
            {saving ? (
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
            ) : (
              <SaveIcon className="size-3.5" aria-hidden="true" />
            )}
            {saving ? t('providers.saving') : t('providers.save')}
          </button>
        </div>
        {!connection.official && (
          <button
            type="button"
            onClick={onRemove}
            className="grid size-7 cursor-pointer place-items-center rounded-md text-ink-faint transition-colors hover:bg-canvas-sunken hover:text-danger-soft"
            aria-label={t('providers.removeConnection')}
            title={t('providers.removeConnection')}
          >
            <Trash2 className="size-3.5" aria-hidden="true" />
          </button>
        )}
      </div>

      <div className="mt-3">
        <label className="block">
          <span className="mb-1 block text-[0.71875rem] text-ink-muted">{t('providers.baseUrl')}</span>
          {connection.official ? (
            <div className="truncate rounded-md bg-canvas-raised px-3 py-2 font-mono text-[0.78125rem] text-ink-muted ring-1 ring-edge" title={officialBaseURL}>
              {officialBaseURL || t('providers.notSet')}
            </div>
          ) : (
            <input
              value={connection.baseURL}
              onChange={(event) => onChange((current) => ({ ...current, baseURL: event.target.value }))}
              placeholder="https://gateway.example.com/v1"
              spellCheck={false}
              className="w-full rounded-md border border-edge bg-canvas px-3 py-2 font-mono text-[0.78125rem] text-ink outline-none placeholder:text-ink-faint focus:border-edge-stronger focus:ring-2 focus:ring-edge-soft"
            />
          )}
        </label>

        <div className="mt-3 flex items-center justify-between">
          <span className="text-[0.71875rem] text-ink-muted">{t('providers.keys')}</span>
          <button
            type="button"
            onClick={onAddKey}
            className="inline-flex h-6 cursor-pointer items-center gap-1 rounded-md px-1.5 text-[0.71875rem] text-ink-muted transition-colors hover:bg-canvas hover:text-ink"
          >
            <Plus className="size-3" aria-hidden="true" />
            {t('providers.addKey')}
          </button>
        </div>

        {connection.keys.length > 0 && (
          <div className="mt-1 divide-y divide-edge-soft border-y border-edge-soft">
            {connection.keys.map((key) => {
              const effective = active && connection.activeKeyId === key.id
              return (
                <div
                  key={key.id}
                  className={cn(
                    'grid grid-cols-[minmax(6rem,0.75fr)_minmax(8rem,1fr)_auto] items-center gap-2 px-2 py-2.5 transition-colors max-sm:grid-cols-[minmax(0,1fr)_auto]',
                    effective && 'bg-canvas-raised/80',
                  )}
                >
                  <input
                    value={key.name}
                    onChange={(event) => updateKey(key.id, { name: event.target.value })}
                    placeholder={t('providers.keyNamePlaceholder')}
                    className="min-w-0 border-0 bg-transparent p-0 text-[0.78125rem] text-ink-soft outline-none placeholder:text-ink-faint"
                  />
                  <input
                    type="password"
                    value={key.apiKey}
                    onChange={(event) => updateKey(key.id, { apiKey: event.target.value })}
                    placeholder={key.preview || t('providers.apiKeyPlaceholder')}
                    autoComplete="off"
                    className="min-w-0 border-0 bg-transparent p-0 font-mono text-[0.75rem] text-ink-soft outline-none placeholder:text-ink-faint max-sm:col-span-2 max-sm:row-start-2"
                  />
                  <div className="flex shrink-0 items-center gap-1 max-sm:col-start-2 max-sm:row-start-1">
                    <button
                      type="button"
                      onClick={() => onTestKey(key.id)}
                      disabled={
                        (!key.persisted && !key.apiKey.trim()) ||
                        (!connection.official && !connection.baseURL.trim())
                      }
                      className="inline-flex h-6 cursor-pointer items-center gap-1 rounded-md px-2 text-[0.6875rem] font-medium text-ink-muted transition-colors hover:bg-surface-active hover:text-ink disabled:cursor-not-allowed disabled:text-ink-ghost disabled:hover:bg-transparent"
                    >
                      <Activity className="size-3" aria-hidden="true" />
                      {t('providers.test')}
                    </button>
                    <ActivationButton
                      configured={connection.activeKeyId === key.id}
                      effective={effective}
                      busy={activating === `key:${connection.id}:${key.id}`}
                      disabled={!connection.persisted || !key.persisted || activationBlocked || Boolean(activating)}
                      onClick={() => onActivateKey(key.id)}
                    />
                    <button
                      type="button"
                      onClick={() => removeKey(key.id)}
                      className="grid size-7 cursor-pointer place-items-center rounded-md text-ink-ghost transition-colors hover:bg-canvas-sunken hover:text-danger-soft"
                      aria-label={t('providers.removeKey')}
                    >
                      <Trash2 className="size-3.5" aria-hidden="true" />
                    </button>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </section>
  )
}

function ActivationButton({
  configured,
  effective,
  disabled,
  busy,
  onClick,
}: {
  configured: boolean
  effective: boolean
  disabled: boolean
  busy: boolean
  onClick: () => void
}) {
  const { t } = useI18n()
  const selected = configured || effective
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled || selected}
      aria-busy={busy}
      aria-label={busy ? t('providers.activating') : undefined}
      className={cn(
        'inline-flex h-6 w-[4.5rem] items-center justify-center gap-1 rounded-md px-2 text-[0.6875rem] font-medium transition-colors',
        effective
          ? 'cursor-default bg-canvas-sunken text-ink-soft'
          : configured
            ? 'cursor-default bg-surface-selected text-ink-muted'
            : 'cursor-pointer text-ink-muted hover:bg-surface-active hover:text-ink',
        disabled && !selected && 'cursor-not-allowed text-ink-ghost hover:bg-transparent hover:text-ink-ghost',
      )}
    >
      {busy ? (
        <LoaderCircle className="size-3 animate-spin" aria-hidden="true" />
      ) : selected ? (
        <Check className="size-3" aria-hidden="true" />
      ) : null}
      <span>
        {effective
          ? t('providers.keyActive')
          : configured
            ? t('providers.keySelected')
            : t('providers.useKey')}
      </span>
    </button>
  )
}

function draftsFromInfo(info: ProviderInfo): ConnectionDraft[] {
  return info.connections.map((connection) => ({
    ...connection,
    persisted: true,
    keys: connection.keys.map((key) => ({ ...key, apiKey: '', persisted: true })),
  }))
}

function profileHasUnsavedChanges(info: ProviderInfo, drafts: ConnectionDraft[]): boolean {
  if (drafts.length !== info.connections.length) return true
  for (const draft of drafts) {
    const saved = info.connections.find((connection) => connection.id === draft.id)
    if (!saved || !draft.persisted) return true
    if (draft.name !== saved.name || draft.baseURL !== saved.baseURL || draft.keys.length !== saved.keys.length) return true
    for (const key of draft.keys) {
      const savedKey = saved.keys.find((candidate) => candidate.id === key.id)
      if (!savedKey || !key.persisted || key.name !== savedKey.name || key.apiKey !== '') return true
    }
  }
  return false
}

function localID(prefix: string): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return `${prefix}_${crypto.randomUUID().replaceAll('-', '')}`
  }
  return `${prefix}_${Date.now()}_${Math.random().toString(16).slice(2)}`
}
