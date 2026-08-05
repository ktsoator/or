import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { Check, ChevronDown, Ellipsis } from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import { apiURL } from '@/api'
import { ProviderIcon } from '@/components/ProviderIdentity'
import { useI18n } from '@/i18n'
import { cn } from '@/lib/utils'
import { providerName } from '@/lib/provider'
import type {
  ProviderConnectionInfo,
  ProviderInfo,
  UtilityModelSelection,
} from '@/types'

type UtilityModelSectionProps = {
  providers: ProviderInfo[]
  selection?: UtilityModelSelection
  onChanged: () => Promise<void>
}

type SelectOption = {
  value: string
  label: string
  detail?: string
  icon?: ReactNode
}

export function UtilityModelSection({
  providers,
  selection,
  onChanged,
}: UtilityModelSectionProps) {
  const { t } = useI18n()
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [optimisticSelection, setOptimisticSelection] = useState<UtilityModelSelection>()
  const displayedSelection = optimisticSelection ?? selection

  useEffect(() => {
    if (optimisticSelection && selection && sameUtilitySelection(optimisticSelection, selection)) {
      setOptimisticSelection(undefined)
    }
  }, [optimisticSelection, selection])

  const routableProviders = useMemo(
    () => providers.filter((provider) => provider.utilityModels.length > 0 && provider.connections.some(hasKeys)),
    [providers],
  )
  const route = displayedSelection
  const routeProvider = routableProviders.find((provider) => provider.id === route?.provider)
  const routeConnection = routeProvider?.connections.find(
    (connection) => connection.id === route?.connectionId,
  )
  const persist = async (next: UtilityModelSelection) => {
    if (saving) return
    setOptimisticSelection(next)
    setSaving(true)
    setError('')
    try {
      const response = await fetch(apiURL('/utility-model-selection'), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(next),
      })
      if (!response.ok) {
        let message = t('settings.utilityModelSaveFailed')
        try {
          const body = (await response.json()) as { error?: string }
          if (body.error) message = body.error
        } catch {
          // Keep the localized fallback for an invalid non-JSON response.
        }
        throw new Error(message)
      }
      const saved = (await response.json()) as UtilityModelSelection
      setOptimisticSelection(saved)
      await onChanged()
    } catch (cause) {
      setOptimisticSelection(undefined)
      setError(cause instanceof Error ? cause.message : t('settings.utilityModelSaveFailed'))
    } finally {
      setSaving(false)
    }
  }

  const persistRoute = (next?: UtilityModelSelection) => {
    if (!next) return
    void persist(next)
  }

  const chooseProvider = (providerID: string) => {
    if (providerID === route?.provider) return
    persistRoute(firstRoute(routableProviders.find((provider) => provider.id === providerID)))
  }

  const chooseConnection = (connectionID: string) => {
    if (!routeProvider || !route) return
    const connection = routeProvider.connections.find((candidate) => candidate.id === connectionID)
    const key = preferredKey(connection)
    if (!connection || !key) return
    persistRoute({ ...route, connectionId: connection.id, keyId: key.id })
  }

  const providerOptions: SelectOption[] = routableProviders.map((provider) => ({
    value: provider.id,
    label: providerName(provider.id) || provider.name,
    icon: <ProviderIcon provider={provider.id} />,
  }))
  const connectionOptions: SelectOption[] = (routeProvider?.connections ?? [])
    .filter(hasKeys)
    .map((connection) => ({
      value: connection.id,
      label: connectionLabel(connection, t('providers.officialConnection'), t('providers.customConnection')),
      detail: connection.baseURL,
    }))
  const keyOptions: SelectOption[] = (routeConnection?.keys ?? []).map((key) => ({
    value: key.id,
    label: key.name || key.preview,
    detail: key.preview,
  }))
  const modelOptions: SelectOption[] = (routeProvider?.utilityModels ?? []).map((model) => ({
    value: model.id,
    label: model.name,
  }))

  const showAdvanced = Boolean(route) && (
    connectionOptions.length > 1 || keyOptions.length > 1
  )

  return (
    <UtilityRow
      label={t('settings.utilityModel')}
      description={t('settings.utilityModelDescription')}
    >
      <div className="flex min-w-0 flex-col items-end gap-1.5 max-sm:w-full max-sm:items-stretch">
        <div
          data-testid="utility-model-controls"
          className="flex min-w-0 items-center gap-1.5 max-sm:w-full"
        >
          <UtilitySelect
            ariaLabel={t('settings.utilityModelProvider')}
            value={route?.provider}
            fallback={t('settings.defaultModelNone')}
            options={providerOptions}
            disabled={false}
            busy={saving}
            className="sm:w-[9.5rem] max-sm:flex-1"
            onChange={chooseProvider}
          />
          <UtilitySelect
            ariaLabel={t('settings.utilityModelModel')}
            value={route?.model}
            fallback={t('settings.defaultModelNone')}
            options={modelOptions}
            disabled={!routeProvider}
            busy={saving}
            className="sm:w-auto sm:min-w-[11rem] sm:max-w-[20rem] max-sm:flex-[1.35]"
            onChange={(model) => route && persistRoute({ ...route, model })}
          />
          {showAdvanced && route && (
            <AdvancedRouteMenu
              connectionOptions={connectionOptions}
              keyOptions={keyOptions}
              connectionID={route.connectionId}
              keyID={route.keyId}
              busy={saving}
              onConnectionChange={chooseConnection}
              onKeyChange={(keyId) => persistRoute({ ...route, keyId })}
            />
          )}
        </div>
        {error && <p className="max-w-[25rem] text-right text-[0.75rem] text-danger-soft max-sm:text-left">{error}</p>}
      </div>
    </UtilityRow>
  )
}

function UtilityRow({
  label,
  description,
  children,
}: {
  label: string
  description: string
  children: ReactNode
}) {
  return (
    <div className="grid min-h-[4.625rem] grid-cols-[minmax(14rem,1fr)_auto] items-center gap-x-8 border-b border-edge/75 px-1 py-3.5 last:border-b-0 max-sm:grid-cols-1 max-sm:items-start max-sm:gap-2.5 max-sm:px-0">
      <div className="min-w-0 flex-1">
        <div className="text-[0.84375rem] leading-5 font-medium text-ink">{label}</div>
        <p className="mt-0.5 max-w-[31rem] text-[0.78125rem] leading-[1.45] text-ink-muted">{description}</p>
      </div>
      <div className="min-w-0 shrink-0 max-sm:w-full">{children}</div>
    </div>
  )
}

function UtilitySelect({
  ariaLabel,
  value,
  fallback,
  options,
  disabled,
  busy = false,
  className,
  onChange,
}: {
  ariaLabel: string
  value?: string
  fallback: string
  options: SelectOption[]
  disabled: boolean
  busy?: boolean
  className?: string
  onChange: (value: string) => void
}) {
  const selected = options.find((option) => option.value === value)
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          aria-label={ariaLabel}
          aria-busy={busy}
          disabled={busy || disabled || options.length === 0}
          className={cn(
            'inline-flex h-9 w-full min-w-0 cursor-pointer items-center gap-1.5 rounded-[10px] bg-surface-hover px-2.5 text-left text-[0.8125rem] text-ink-soft outline-none transition-colors hover:bg-surface-active focus-visible:bg-surface-active data-[state=open]:bg-surface-selected',
            busy
              ? 'disabled:cursor-wait disabled:opacity-100'
              : 'disabled:cursor-not-allowed disabled:opacity-60',
            className,
          )}
        >
          {selected?.icon}
          <span
            className="min-w-0 flex-1 truncate"
            title={selected?.label ?? fallback}
          >
            {selected?.label ?? fallback}
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
          className="z-[100] max-h-[min(24rem,60vh)] min-w-[15rem] overflow-y-auto animate-[fade-in_110ms_ease-out] rounded-[14px] border border-edge bg-canvas p-1 text-[0.8125rem] text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
        >
          <DropdownMenu.RadioGroup value={value} onValueChange={onChange}>
            <div className="flex flex-col gap-0.5">
              {options.map((option) => (
                <DropdownMenu.RadioItem
                  key={option.value}
                  value={option.value}
                  className="relative flex min-h-9 cursor-pointer items-center gap-2 rounded-[9px] px-2.5 py-1.5 pr-8 outline-none select-none data-[highlighted]:bg-surface-active data-[state=checked]:bg-surface-selected"
                >
                  {option.icon}
                  <span className="min-w-0 flex-1">
                    <span className="block truncate">{option.label}</span>
                    {option.detail && (
                      <span className="block truncate font-mono text-[0.6875rem] text-ink-faint">
                        {option.detail}
                      </span>
                    )}
                  </span>
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

function AdvancedRouteMenu({
  connectionOptions,
  keyOptions,
  connectionID,
  keyID,
  busy,
  onConnectionChange,
  onKeyChange,
}: {
  connectionOptions: SelectOption[]
  keyOptions: SelectOption[]
  connectionID: string
  keyID: string
  busy: boolean
  onConnectionChange: (value: string) => void
  onKeyChange: (value: string) => void
}) {
  const { t } = useI18n()
  const showConnections = connectionOptions.length > 1
  const showKeys = keyOptions.length > 1

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          aria-label={t('settings.utilityModelAdvanced')}
          title={t('settings.utilityModelAdvanced')}
          aria-busy={busy}
          disabled={busy}
          className="grid size-9 shrink-0 cursor-pointer place-items-center rounded-[10px] bg-surface-hover text-ink-muted outline-none transition-colors hover:bg-surface-active hover:text-ink-soft focus-visible:bg-surface-active focus-visible:text-ink-soft data-[state=open]:bg-surface-selected disabled:cursor-wait disabled:opacity-100"
        >
          <Ellipsis className="size-4" aria-hidden="true" />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          side="bottom"
          align="end"
          sideOffset={7}
          collisionPadding={10}
          className="z-[100] max-h-[min(26rem,65vh)] min-w-[17rem] overflow-y-auto animate-[fade-in_110ms_ease-out] rounded-[14px] border border-edge bg-canvas p-1 text-[0.8125rem] text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
        >
          {showConnections && (
            <RouteOptionGroup
              label={t('settings.utilityModelConnection')}
              value={connectionID}
              options={connectionOptions}
              onChange={onConnectionChange}
            />
          )}
          {showConnections && showKeys && <DropdownMenu.Separator className="my-1 h-px bg-canvas-strong/80" />}
          {showKeys && (
            <RouteOptionGroup
              label={t('settings.utilityModelKey')}
              value={keyID}
              options={keyOptions}
              inlineDetail
              onChange={onKeyChange}
            />
          )}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

function RouteOptionGroup({
  label,
  value,
  options,
  inlineDetail = false,
  onChange,
}: {
  label: string
  value: string
  options: SelectOption[]
  inlineDetail?: boolean
  onChange: (value: string) => void
}) {
  return (
    <>
      <DropdownMenu.Label className="px-2.5 pt-1.5 pb-1 text-[0.6875rem] font-medium text-ink-faint">
        {label}
      </DropdownMenu.Label>
      <DropdownMenu.RadioGroup
        value={value}
        onValueChange={onChange}
        className="flex flex-col gap-0.5"
      >
        {options.map((option) => (
          <DropdownMenu.RadioItem
            key={option.value}
            value={option.value}
            className="relative flex min-h-9 cursor-pointer items-center gap-2 rounded-[9px] px-2.5 py-1.5 pr-8 outline-none select-none data-[highlighted]:bg-surface-active data-[state=checked]:bg-surface-selected"
          >
            <span className={cn('min-w-0 flex-1', inlineDetail && 'flex items-center gap-3')}>
              <span className={cn('block truncate', inlineDetail && 'min-w-0 flex-1')}>
                {option.label}
              </span>
              {option.detail && (
                <span className={cn(
                  'block truncate font-mono text-[0.6875rem] text-ink-faint',
                  inlineDetail && 'shrink-0',
                )}>
                  {option.detail}
                </span>
              )}
            </span>
            <DropdownMenu.ItemIndicator className="absolute right-2 grid size-4 place-items-center text-ink-soft">
              <Check className="size-3.5" aria-hidden="true" />
            </DropdownMenu.ItemIndicator>
          </DropdownMenu.RadioItem>
        ))}
      </DropdownMenu.RadioGroup>
    </>
  )
}

function hasKeys(connection: ProviderConnectionInfo): boolean {
  return connection.keys.length > 0
}

function preferredKey(connection?: ProviderConnectionInfo) {
  return connection?.keys.find((key) => key.id === connection.activeKeyId) ?? connection?.keys[0]
}

function firstRoute(provider?: ProviderInfo): UtilityModelSelection | undefined {
  if (!provider) return undefined
  const connection = provider.connections.find(
    (candidate) => candidate.id === provider.activeConnectionId && hasKeys(candidate),
  ) ?? provider.connections.find(hasKeys)
  const key = preferredKey(connection)
  const model = provider.utilityModels[0]
  if (!connection || !key || !model) return undefined
  return {
    provider: provider.id,
    connectionId: connection.id,
    keyId: key.id,
    model: model.id,
  }
}

function sameUtilitySelection(left: UtilityModelSelection, right: UtilityModelSelection): boolean {
  return left.provider === right.provider &&
    left.model === right.model &&
    left.connectionId === right.connectionId &&
    left.keyId === right.keyId
}

function connectionLabel(
  connection: ProviderConnectionInfo | undefined,
  officialLabel: string,
  customFallback: string,
): string {
  if (!connection) return ''
  return connection.official ? officialLabel : connection.name || customFallback
}
