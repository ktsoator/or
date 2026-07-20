import { useCallback, useEffect, useState } from 'react'
import { Check, ChevronDown, LoaderCircle, Plus, Trash2 } from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import { apiURL } from '@/api'
import { useI18n } from '@/i18n'
import { cn } from '@/lib/utils'
import { ProviderIcon } from '@/components/ProviderIdentity'
import { providerName } from '@/lib/provider'
import type {
  ProviderConnectionInfo,
  ProviderInfo,
  ProviderListResponse,
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
  const [providers, setProviders] = useState<ProviderInfo[]>([])
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
        setProviders(data.providers)
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

  const selected = providers.find((provider) => provider.id === selectedProviderId)

  return (
    <div>
      <p className="mb-6 max-w-2xl text-[0.875rem] leading-6 text-stone-500">
        {t('providers.intro')}
      </p>

      {loading ? (
        <div className="flex items-center gap-2 py-6 text-[0.8125rem] text-stone-400">
          <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
          {t('providers.loading')}
        </div>
      ) : error ? (
        <div className="rounded-lg border border-red-200 bg-red-50/60 px-4 py-3 text-[0.8125rem] text-red-700">
          {error}
        </div>
      ) : selected ? (
          <ProviderConfigPanel
            key={selected.id}
            providers={providers}
            info={selected}
            selectedProviderId={selected.id}
            onSelectProvider={setSelectedProviderId}
            onChanged={afterChange}
          />
      ) : (
        <div className="flex min-h-36 items-start justify-end border-t border-stone-100 pt-5">
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
          className="group flex h-9 min-w-[10.5rem] max-w-[15rem] shrink-0 cursor-pointer items-center gap-2 rounded-[10px] bg-[rgb(246,246,246)] px-2.5 text-left text-[0.8125rem] outline-none transition-colors hover:bg-[rgb(241,241,241)] focus-visible:ring-2 focus-visible:ring-stone-300 data-[state=open]:bg-[rgb(237,237,237)] max-sm:min-w-0"
        >
          {selected && <ProviderIcon provider={selected.id} />}
          <span className={cn('min-w-0 flex-1 truncate', selected ? 'text-stone-800' : 'text-stone-500')}>
            {selected ? providerName(selected.id) : t('providers.selectProvider')}
          </span>
          <ChevronDown
            className="size-3.5 shrink-0 text-stone-400 transition-transform duration-150 group-data-[state=open]:rotate-180"
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
          className="z-[100] max-h-[min(24rem,60vh)] min-w-[15rem] overflow-y-auto animate-[fade-in_110ms_ease-out] rounded-[14px] border border-stone-200 bg-white p-1 text-[0.8125rem] text-stone-900 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
        >
          <DropdownMenu.RadioGroup value={value ?? ''} onValueChange={onChange}>
            <div className="flex flex-col gap-0.5">
              {providers.map((provider) => (
                <DropdownMenu.RadioItem
                  key={provider.id}
                  value={provider.id}
                  className="relative flex h-9 cursor-default select-none items-center gap-2 rounded-[9px] px-2.5 pr-8 outline-none data-[highlighted]:bg-[rgb(241,241,241)] data-[state=checked]:bg-[rgb(237,237,237)]"
                >
                  <ProviderIcon provider={provider.id} />
                  <span className="min-w-0 flex-1 truncate">{providerName(provider.id)}</span>
                  <DropdownMenu.ItemIndicator className="absolute right-2 grid size-4 place-items-center text-stone-700">
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
          className="group flex h-9 min-w-[10.5rem] max-w-[15rem] shrink-0 cursor-pointer items-center gap-2 rounded-[10px] bg-[rgb(246,246,246)] px-2.5 text-left text-[0.8125rem] outline-none transition-colors hover:bg-[rgb(241,241,241)] focus-visible:ring-2 focus-visible:ring-stone-300 data-[state=open]:bg-[rgb(237,237,237)] max-sm:min-w-0"
        >
          <span className={cn('size-2 shrink-0 rounded-full', selected.id === activeValue ? 'bg-stone-800' : 'bg-stone-300')} />
          <span className="min-w-0 flex-1 truncate text-stone-800">{selectedName}</span>
          <ChevronDown className="size-3.5 shrink-0 text-stone-400 transition-transform duration-150 group-data-[state=open]:rotate-180" aria-hidden="true" />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          side="bottom"
          align="end"
          sideOffset={7}
          collisionPadding={10}
          className="z-[100] min-w-[17rem] animate-[fade-in_110ms_ease-out] rounded-[14px] border border-stone-200 bg-white p-1 text-[0.8125rem] text-stone-900 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
        >
          <DropdownMenu.RadioGroup className="flex flex-col gap-0.5" value={value} onValueChange={onChange}>
            {connections.map((connection) => {
              const name = connection.official ? t('providers.officialConnection') : connection.name || t('providers.customConnection')
              const baseURL = connection.official ? officialBaseURL : connection.baseURL
              return (
                <DropdownMenu.RadioItem
                  key={connection.id}
                  value={connection.id}
                  className="relative flex min-h-10 cursor-default select-none items-center gap-2 rounded-[9px] px-2.5 py-1.5 pr-8 outline-none data-[highlighted]:bg-[rgb(241,241,241)] data-[state=checked]:bg-[rgb(237,237,237)]"
                >
                  <span className={cn('size-2 shrink-0 rounded-full', connection.id === activeValue ? 'bg-stone-800' : 'bg-stone-300')} />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-stone-800">{name}</span>
                    <span className="block truncate font-mono text-[0.6875rem] text-stone-400">{baseURL || t('providers.notSet')}</span>
                  </span>
                  <DropdownMenu.ItemIndicator className="absolute right-2 grid size-4 place-items-center text-stone-700">
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
  onSelectProvider,
  onChanged,
}: {
  providers: ProviderInfo[]
  info: ProviderInfo
  selectedProviderId: string
  onSelectProvider: (value: string) => void
  onChanged: () => void | Promise<void>
}) {
  const { t } = useI18n()
  const [connections, setConnections] = useState<ConnectionDraft[]>(() => draftsFromInfo(info))
  const [activeConnectionId, setActiveConnectionId] = useState(info.activeConnectionId)
  const [editingConnectionId, setEditingConnectionId] = useState(info.activeConnectionId)
  const [saving, setSaving] = useState(false)
  const [activating, setActivating] = useState('')
  const [rowError, setRowError] = useState('')
  const selectedConnection = connections.find((connection) => connection.id === editingConnectionId) ?? connections[0]
  const hasUnsavedChanges = profileHasUnsavedChanges(info, connections)

  useEffect(() => {
    setConnections(draftsFromInfo(info))
    setActiveConnectionId(info.activeConnectionId)
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
      await onChanged()
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
      await onChanged()
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

  const clear = async () => {
    setSaving(true)
    setRowError('')
    try {
      const response = await fetch(apiURL(`/providers/${info.id}`), { method: 'DELETE' })
      if (!response.ok) throw new Error(`HTTP ${response.status}`)
      await onChanged()
    } catch {
      setRowError(t('providers.saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <div className="mb-5 flex items-center justify-between gap-3 max-sm:items-start">
        <div>
          <div className="text-[0.875rem] font-medium text-stone-800">{t('providers.routing')}</div>
          <div className="mt-0.5 text-[0.71875rem] text-stone-400">{t('providers.routingHint')}</div>
        </div>
        <div className="flex shrink-0 items-center gap-1.5 max-sm:grid max-sm:w-full max-sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_2.25rem]">
          <ProviderPicker providers={providers} value={selectedProviderId} onChange={onSelectProvider} />
          <ConnectionPicker
            connections={connections}
            officialBaseURL={info.officialBaseURL ?? ''}
            value={editingConnectionId}
            activeValue={info.configured ? activeConnectionId : ''}
            onChange={setEditingConnectionId}
          />
          <button
            type="button"
            onClick={addConnection}
            aria-label={t('providers.addConnection')}
            title={t('providers.addConnection')}
            className="grid size-9 shrink-0 cursor-pointer place-items-center rounded-[10px] bg-[rgb(246,246,246)] text-stone-500 outline-none transition-colors hover:bg-[rgb(241,241,241)] hover:text-stone-950 focus-visible:ring-2 focus-visible:ring-stone-300"
          >
            <Plus className="size-4" aria-hidden="true" />
          </button>
        </div>
      </div>

      <div className="mb-2 px-0.5">
        <span className="text-[0.75rem] font-medium text-stone-500">{t('providers.management')}</span>
      </div>

      <div className="overflow-hidden rounded-xl border border-stone-200 bg-white">
      <div className="flex items-center gap-3 border-b border-stone-100 px-4 py-3">
        <ProviderIcon provider={info.id} />
        <span className="min-w-0 flex-1 truncate text-[0.875rem] font-medium text-stone-900">{providerName(info.id)}</span>
      </div>

      <div>
        {selectedConnection && (
          <ConnectionEditor
            key={selectedConnection.id}
            officialBaseURL={info.officialBaseURL ?? ''}
            connection={selectedConnection}
            active={info.configured && selectedConnection.id === activeConnectionId}
            activationBlocked={hasUnsavedChanges}
            activating={activating}
            onChange={(update) => updateConnection(selectedConnection.id, update)}
            onAddKey={() => addKey(selectedConnection.id)}
            onRemove={() => removeConnection(selectedConnection.id)}
            onActivateConnection={() => void activateConnection(selectedConnection.id)}
            onActivateKey={(keyID) => void activateKey(selectedConnection.id, keyID)}
          />
        )}
      </div>

      <div className="flex items-center justify-end gap-2 border-t border-stone-100 px-4 py-3">
          <button
            type="button"
            onClick={() => void clear()}
            disabled={saving || Boolean(activating)}
            className="inline-flex h-8 cursor-pointer items-center rounded-lg px-3 text-[0.8125rem] text-stone-500 transition-colors hover:bg-stone-100 hover:text-stone-900 disabled:opacity-45"
          >
            {t('providers.clear')}
          </button>
          <button
            type="button"
            onClick={() => void save()}
            disabled={saving || Boolean(activating) || !hasUnsavedChanges}
            className={cn(
              'inline-flex h-8 items-center gap-1.5 rounded-lg bg-stone-950 px-3.5 text-[0.8125rem] font-medium text-white transition-colors hover:bg-stone-800 disabled:opacity-45',
              saving ? 'cursor-wait' : 'cursor-pointer disabled:cursor-default',
            )}
          >
            {saving && <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />}
            {saving ? t('providers.saving') : t('providers.save')}
          </button>
      </div>
      {rowError && <p className="border-t border-red-100 bg-red-50/60 px-4 py-2 text-[0.75rem] text-red-600">{rowError}</p>}
      </div>
    </>
  )
}

function ConnectionEditor({
  officialBaseURL,
  connection,
  active,
  activationBlocked,
  activating,
  onChange,
  onAddKey,
  onRemove,
  onActivateConnection,
  onActivateKey,
}: {
  officialBaseURL: string
  connection: ConnectionDraft
  active: boolean
  activationBlocked: boolean
  activating: string
  onChange: (update: (current: ConnectionDraft) => ConnectionDraft) => void
  onAddKey: () => void
  onRemove: () => void
  onActivateConnection: () => void
  onActivateKey: (keyID: string) => void
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

  return (
    <section className="px-4 py-3.5">
      <div className="flex items-center gap-2.5">
        {connection.official ? (
          <span className="min-w-0 flex-1 text-[0.84375rem] font-medium text-stone-900">
            {t('providers.officialConnection')}
          </span>
        ) : (
          <input
            value={connection.name}
            onChange={(event) => onChange((current) => ({ ...current, name: event.target.value }))}
            placeholder={t('providers.connectionNamePlaceholder')}
            className="min-w-0 flex-1 border-0 bg-transparent p-0 text-[0.84375rem] font-medium text-stone-900 outline-none placeholder:text-stone-400"
          />
        )}
        {!connection.official && (
          <button
            type="button"
            onClick={onRemove}
            className="grid size-7 cursor-pointer place-items-center rounded-md text-stone-400 transition-colors hover:bg-white hover:text-red-600"
            aria-label={t('providers.removeConnection')}
          >
            <Trash2 className="size-3.5" aria-hidden="true" />
          </button>
        )}
        {active ? (
          <span className="inline-flex h-7 items-center gap-1.5 rounded-md bg-[rgb(237,237,237)] px-2.5 text-[0.71875rem] font-medium text-stone-700">
            <span className="size-1.5 rounded-full bg-stone-800" />
            {t('providers.active')}
          </span>
        ) : (
          <button
            type="button"
            onClick={onActivateConnection}
            disabled={
              !connection.persisted ||
              activationBlocked ||
              Boolean(activating) ||
              !connection.activeKeyId
            }
            title={
              !connection.persisted || activationBlocked
                ? t('providers.saveBeforeActivate')
                : !connection.activeKeyId
                  ? t('providers.selectKeyFirst')
                  : undefined
            }
            className="inline-flex h-7 cursor-pointer items-center rounded-md px-2.5 text-[0.71875rem] font-medium text-stone-500 transition-colors hover:bg-[rgb(241,241,241)] hover:text-stone-950 disabled:cursor-not-allowed disabled:text-stone-300 disabled:hover:bg-transparent"
          >
            {activating === `connection:${connection.id}` ? t('providers.activating') : t('providers.useConnection')}
          </button>
        )}
      </div>

      <div className="mt-3">
        <label className="block">
          <span className="mb-1 block text-[0.71875rem] text-stone-500">{t('providers.baseUrl')}</span>
          {connection.official ? (
            <div className="truncate rounded-lg bg-white px-3 py-2 font-mono text-[0.78125rem] text-stone-600 ring-1 ring-stone-200" title={officialBaseURL}>
              {officialBaseURL || t('providers.notSet')}
            </div>
          ) : (
            <input
              value={connection.baseURL}
              onChange={(event) => onChange((current) => ({ ...current, baseURL: event.target.value }))}
              placeholder="https://gateway.example.com/v1"
              spellCheck={false}
              className="w-full rounded-lg border border-stone-200 bg-white px-3 py-2 font-mono text-[0.78125rem] text-stone-900 outline-none placeholder:text-stone-400 focus:border-stone-400"
            />
          )}
        </label>

        <div className="mt-3 flex items-center justify-between">
          <span className="text-[0.71875rem] text-stone-500">{t('providers.keys')}</span>
          <button
            type="button"
            onClick={onAddKey}
            className="inline-flex h-6 cursor-pointer items-center gap-1 rounded-md px-1.5 text-[0.71875rem] text-stone-500 transition-colors hover:bg-white hover:text-stone-950"
          >
            <Plus className="size-3" aria-hidden="true" />
            {t('providers.addKey')}
          </button>
        </div>

        {connection.keys.length > 0 && (
          <div className="mt-1 divide-y divide-stone-100 overflow-hidden rounded-lg border border-stone-200 bg-white">
            {connection.keys.map((key) => (
              <div key={key.id} className="grid grid-cols-[1rem_minmax(6rem,0.75fr)_minmax(8rem,1fr)_auto_1.75rem] items-center gap-2 px-2.5 py-2">
                <SelectionDot selected={connection.activeKeyId === key.id} active={active && connection.activeKeyId === key.id} />
                <input
                  value={key.name}
                  onChange={(event) => updateKey(key.id, { name: event.target.value })}
                  placeholder={t('providers.keyNamePlaceholder')}
                  className="min-w-0 border-0 bg-transparent p-0 text-[0.78125rem] text-stone-800 outline-none placeholder:text-stone-400"
                />
                <input
                  type="password"
                  value={key.apiKey}
                  onChange={(event) => updateKey(key.id, { apiKey: event.target.value })}
                  placeholder={key.preview || t('providers.apiKeyPlaceholder')}
                  autoComplete="off"
                  className="min-w-0 border-0 bg-transparent p-0 font-mono text-[0.75rem] text-stone-700 outline-none placeholder:text-stone-400"
                />
                <ActivationButton
                  configured={connection.activeKeyId === key.id}
                  effective={active && connection.activeKeyId === key.id}
                  busy={activating === `key:${connection.id}:${key.id}`}
                  disabled={!connection.persisted || !key.persisted || activationBlocked || Boolean(activating)}
                  onClick={() => onActivateKey(key.id)}
                />
                <button
                  type="button"
                  onClick={() => removeKey(key.id)}
                  className="grid size-7 cursor-pointer place-items-center rounded-md text-stone-300 transition-colors hover:bg-stone-100 hover:text-red-600"
                  aria-label={t('providers.removeKey')}
                >
                  <Trash2 className="size-3.5" aria-hidden="true" />
                </button>
              </div>
            ))}
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
  const inactive = configured || effective
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled || inactive}
      className={cn(
        'inline-flex h-6 min-w-[3.5rem] cursor-pointer items-center justify-center gap-1 rounded-md px-2 text-[0.6875rem] font-medium transition-colors',
        effective
          ? 'bg-stone-900 text-white'
          : configured
            ? 'bg-[rgb(237,237,237)] text-stone-600'
            : 'text-stone-500 hover:bg-[rgb(241,241,241)] hover:text-stone-950',
        disabled && 'cursor-not-allowed text-stone-300 hover:bg-transparent hover:text-stone-300',
      )}
    >
      {busy && <LoaderCircle className="size-3 animate-spin" aria-hidden="true" />}
      {busy
        ? t('providers.activating')
        : effective
          ? t('providers.keyActive')
          : configured
            ? t('providers.keySelected')
            : t('providers.useKey')}
    </button>
  )
}

function SelectionDot({ selected, active }: { selected: boolean; active: boolean }) {
  return (
    <span
      className={cn(
        'grid size-3.5 place-items-center rounded-full border transition-colors',
        active
          ? 'border-stone-900 bg-stone-900'
          : selected
            ? 'border-stone-400 bg-stone-400'
            : 'border-stone-300',
      )}
    >
      {selected && <span className="size-1 rounded-full bg-white" />}
    </span>
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
