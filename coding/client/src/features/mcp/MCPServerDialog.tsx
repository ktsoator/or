import { useEffect, useMemo, useState } from 'react'
import {
  Globe2,
  Plus,
  TerminalSquare,
  Trash2,
  X,
} from 'lucide-react'
import { Dialog } from 'radix-ui'
import { useI18n } from '@/i18n'
import { cn } from '@/lib/utils'
import type { MCPServerConfig, MCPServerInfo } from './catalog'

type Pair = { id: number; key: string; value: string }
type Transport = 'stdio' | 'http'
type Scope = 'all' | 'selected'

let nextPairID = 0

function pairs(values?: Record<string, string>): Pair[] {
  return Object.entries(values ?? {}).map(([key, value]) => ({
    id: ++nextPairID,
    key,
    value,
  }))
}

function pairRecord(values: Pair[]): Record<string, string> {
  return Object.fromEntries(
    values
      .map((entry) => [entry.key.trim(), entry.value] as const)
      .filter(([key]) => key),
  )
}

const inputClass =
  'h-10 w-full rounded-[9px] border border-edge bg-canvas px-3 text-[0.84375rem] text-ink outline-none transition-[border-color,box-shadow] placeholder:text-ink-faint focus:border-edge-stronger focus:ring-2 focus:ring-edge disabled:bg-canvas-sunken disabled:text-ink-faint'

const textareaClass =
  'min-h-24 w-full resize-y rounded-[9px] border border-edge bg-canvas px-3 py-2.5 font-mono text-[0.78125rem] leading-5 text-ink outline-none transition-[border-color,box-shadow] placeholder:text-ink-faint focus:border-edge-stronger focus:ring-2 focus:ring-edge'

export function MCPServerDialog({
  server,
  configPath,
  workspacePath,
  saving,
  onClose,
  onSave,
}: {
  server?: MCPServerInfo
  configPath: string
  workspacePath?: string
  saving: boolean
  onClose: () => void
  onSave: (name: string, config: MCPServerConfig, previousName?: string) => Promise<void>
}) {
  const { t } = useI18n()
  const existing = server?.config
  const [name, setName] = useState(server?.name ?? '')
  const [transport, setTransport] = useState<Transport>(
    existing ? (existing.command ? 'stdio' : 'http') : 'stdio',
  )
  const [enabled, setEnabled] = useState(!existing?.disabled)
  const [command, setCommand] = useState(existing?.command ?? '')
  const [args, setArgs] = useState((existing?.args ?? []).join('\n'))
  const [cwd, setCwd] = useState(existing?.cwd ?? '')
  const [url, setURL] = useState(existing?.url ?? '')
  const [environment, setEnvironment] = useState<Pair[]>(() => pairs(existing?.env))
  const [headers, setHeaders] = useState<Pair[]>(() => pairs(existing?.headers))
  const [scope, setScope] = useState<Scope>(existing?.workspaces?.length ? 'selected' : 'all')
  const [workspaces, setWorkspaces] = useState((existing?.workspaces ?? []).join('\n'))
  const [timeout, setTimeoutValue] = useState(existing?.timeoutSeconds?.toString() ?? '')
  const [error, setError] = useState('')

  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !saving) onClose()
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [onClose, saving])

  const duplicateEnvironment = useMemo(() => duplicateKey(environment), [environment])
  const duplicateHeader = useMemo(() => duplicateKey(headers), [headers])

  const submit = async () => {
    const normalizedName = name.trim()
    if (!/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/.test(normalizedName)) {
      setError(t('mcp.validationName'))
      return
    }
    if (transport === 'stdio' && !command.trim()) {
      setError(t('mcp.validationCommand'))
      return
    }
    if (transport === 'http' && !url.trim()) {
      setError(t('mcp.validationURL'))
      return
    }
    if (duplicateEnvironment || duplicateHeader) {
      setError(t('mcp.validationDuplicateKey'))
      return
    }
    const timeoutSeconds = timeout.trim() ? Number(timeout) : 0
    if (!Number.isInteger(timeoutSeconds) || timeoutSeconds < 0) {
      setError(t('mcp.validationTimeout'))
      return
    }
    const selectedWorkspaces = workspaces
      .split('\n')
      .map((value) => value.trim())
      .filter(Boolean)
    if (scope === 'selected' && selectedWorkspaces.length === 0) {
      setError(t('mcp.validationWorkspace'))
      return
    }

    const config: MCPServerConfig = {
      disabled: enabled ? undefined : true,
      timeoutSeconds: timeoutSeconds || undefined,
      workspaces: scope === 'selected' ? selectedWorkspaces : undefined,
    }
    if (transport === 'stdio') {
      config.command = command.trim()
      config.args = args
        .split('\n')
        .map((value) => value.trim())
        .filter(Boolean)
      config.cwd = cwd.trim() || undefined
      const env = pairRecord(environment)
      config.env = Object.keys(env).length ? env : undefined
    } else {
      config.url = url.trim()
      const nextHeaders = pairRecord(headers)
      config.headers = Object.keys(nextHeaders).length ? nextHeaders : undefined
    }

    setError('')
    try {
      await onSave(normalizedName, config, server?.name)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t('mcp.saveFailed'))
    }
  }

  return (
    <Dialog.Root open onOpenChange={(open) => !open && !saving && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-[160] animate-[fade-in_120ms_ease-out] bg-scrim/25 backdrop-blur-[1px]" />
        <Dialog.Content className="fixed top-1/2 left-1/2 z-[170] flex max-h-[min(46rem,calc(100vh-2rem))] w-[min(42rem,calc(100vw-2rem))] -translate-x-1/2 -translate-y-1/2 flex-col overflow-hidden rounded-[14px] border border-edge bg-canvas shadow-[0_28px_80px_-32px_rgba(28,25,23,0.55)] outline-none">
          <header className="flex shrink-0 items-start gap-4 border-b border-edge/75 px-5 py-4">
            <div className="min-w-0 flex-1">
              <Dialog.Title className="text-[1rem] leading-6 font-semibold text-ink">
                {server ? t('mcp.editTitle') : t('mcp.addTitle')}
              </Dialog.Title>
              <Dialog.Description className="mt-0.5 truncate font-mono text-[0.6875rem] text-ink-faint">
                {configPath}
              </Dialog.Description>
            </div>
            <button
              className="grid size-8 shrink-0 cursor-pointer place-items-center rounded-lg text-ink-faint outline-none transition-colors hover:bg-canvas-sunken hover:text-ink focus-visible:bg-canvas-sunken focus-visible:text-ink disabled:cursor-wait disabled:opacity-40"
              type="button"
              title={t('mcp.close')}
              aria-label={t('mcp.close')}
              disabled={saving}
              onClick={onClose}
            >
              <X className="size-4" aria-hidden="true" />
            </button>
          </header>

          <div className="min-h-0 flex-1 overflow-y-auto px-5 py-5">
            <div className="grid grid-cols-[minmax(0,1fr)_auto] items-end gap-4 max-sm:grid-cols-1">
              <Field label={t('mcp.name')}>
                <input
                  autoFocus
                  className={inputClass}
                  value={name}
                  placeholder="github"
                  spellCheck={false}
                  onChange={(event) => setName(event.target.value)}
                />
              </Field>
              <div className="flex h-10 items-center gap-2.5 pb-px">
                <Switch checked={enabled} onChange={setEnabled} label={t('mcp.enabled')} />
              </div>
            </div>

            <div className="mt-5 border-t border-edge/75 pt-5">
              <div className="mb-2 text-[0.75rem] font-medium text-ink-muted">
                {t('mcp.transport')}
              </div>
              <div className="grid h-10 w-full grid-cols-2 rounded-[9px] bg-canvas-sunken p-1" role="radiogroup" aria-label={t('mcp.transport')}>
                <TransportButton
                  icon={TerminalSquare}
                  label={t('mcp.stdio')}
                  selected={transport === 'stdio'}
                  onClick={() => setTransport('stdio')}
                />
                <TransportButton
                  icon={Globe2}
                  label={t('mcp.http')}
                  selected={transport === 'http'}
                  onClick={() => setTransport('http')}
                />
              </div>
            </div>

            {transport === 'stdio' ? (
              <div className="mt-5 space-y-4">
                <Field label={t('mcp.command')}>
                  <input
                    className={`${inputClass} font-mono text-[0.78125rem]`}
                    value={command}
                    placeholder="npx"
                    spellCheck={false}
                    onChange={(event) => setCommand(event.target.value)}
                  />
                </Field>
                <Field label={t('mcp.arguments')}>
                  <textarea
                    className={textareaClass}
                    value={args}
                    placeholder={'-y\n@modelcontextprotocol/server-everything@2026.7.4'}
                    spellCheck={false}
                    onChange={(event) => setArgs(event.target.value)}
                  />
                </Field>
                <Field label={t('mcp.workingDirectory')} optional>
                  <input
                    className={`${inputClass} font-mono text-[0.78125rem]`}
                    value={cwd}
                    placeholder="${workspace}"
                    spellCheck={false}
                    onChange={(event) => setCwd(event.target.value)}
                  />
                </Field>
                <PairEditor
                  label={t('mcp.environment')}
                  keyPlaceholder="GITHUB_TOKEN"
                  valuePlaceholder="${env:GITHUB_TOKEN}"
                  values={environment}
                  duplicate={duplicateEnvironment}
                  onChange={setEnvironment}
                />
              </div>
            ) : (
              <div className="mt-5 space-y-4">
                <Field label={t('mcp.url')}>
                  <input
                    className={`${inputClass} font-mono text-[0.78125rem]`}
                    type="url"
                    value={url}
                    placeholder="https://mcp.example.com/mcp"
                    spellCheck={false}
                    onChange={(event) => setURL(event.target.value)}
                  />
                </Field>
                <PairEditor
                  label={t('mcp.headers')}
                  keyPlaceholder="Authorization"
                  valuePlaceholder="Bearer ${env:MCP_TOKEN}"
                  values={headers}
                  duplicate={duplicateHeader}
                  onChange={setHeaders}
                />
              </div>
            )}

            <div className="mt-6 border-t border-edge/75 pt-5">
              <div className="grid grid-cols-[minmax(0,1fr)_8.5rem] gap-4 max-sm:grid-cols-1">
                <div>
                  <div className="mb-2 text-[0.75rem] font-medium text-ink-muted">
                    {t('mcp.workspaceScope')}
                  </div>
                  <div className="grid h-10 grid-cols-2 rounded-[9px] bg-canvas-sunken p-1" role="radiogroup" aria-label={t('mcp.workspaceScope')}>
                    <ScopeButton label={t('mcp.allWorkspaces')} selected={scope === 'all'} onClick={() => setScope('all')} />
                    <ScopeButton label={t('mcp.selectedWorkspaces')} selected={scope === 'selected'} onClick={() => setScope('selected')} />
                  </div>
                </div>
                <Field label={t('mcp.timeout')}>
                  <input
                    className={inputClass}
                    type="number"
                    min="0"
                    step="1"
                    inputMode="numeric"
                    value={timeout}
                    placeholder="15"
                    onChange={(event) => setTimeoutValue(event.target.value)}
                  />
                </Field>
              </div>
              {scope === 'selected' && (
                <div className="mt-4">
                  <Field label={t('mcp.workspaces')}>
                    <textarea
                      className={textareaClass}
                      value={workspaces}
                      placeholder="/Users/example/project"
                      spellCheck={false}
                      onChange={(event) => setWorkspaces(event.target.value)}
                    />
                  </Field>
                  {workspacePath && !workspaces.split('\n').includes(workspacePath) && (
                    <button
                      className="mt-2 h-8 cursor-pointer rounded-lg px-2.5 text-[0.75rem] font-medium text-info outline-none transition-colors hover:bg-info-surface focus-visible:bg-info-surface"
                      type="button"
                      onClick={() => setWorkspaces((current) => current.trim() ? `${current.trim()}\n${workspacePath}` : workspacePath)}
                    >
                      {t('mcp.addCurrentWorkspace')}
                    </button>
                  )}
                </div>
              )}
            </div>

            {error && (
              <div className="mt-5 rounded-[9px] border border-danger-edge/70 bg-danger-surface/70 px-3.5 py-3 text-[0.8125rem] leading-5 text-danger">
                {error}
              </div>
            )}
          </div>

          <footer className="flex shrink-0 justify-end gap-2 border-t border-edge/75 px-5 py-4">
            <button
              className="h-9 cursor-pointer rounded-[9px] border border-edge bg-canvas px-4 text-[0.8125rem] font-medium text-ink-soft transition-colors hover:bg-canvas-raised disabled:cursor-wait disabled:opacity-45"
              type="button"
              disabled={saving}
              onClick={onClose}
            >
              {t('mcp.cancel')}
            </button>
            <button
              className="h-9 min-w-20 cursor-pointer rounded-[9px] bg-ink px-4 text-[0.8125rem] font-medium text-canvas transition-[opacity,transform] hover:opacity-90 active:translate-y-px disabled:cursor-wait disabled:opacity-40"
              type="button"
              disabled={saving}
              onClick={() => void submit()}
            >
              {saving ? t('mcp.saving') : t('mcp.save')}
            </button>
          </footer>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

function Field({ label, optional, children }: { label: string; optional?: boolean; children: React.ReactNode }) {
  const { t } = useI18n()
  return (
    <label className="block min-w-0">
      <span className="mb-2 flex items-center gap-2 text-[0.75rem] font-medium text-ink-muted">
        {label}
        {optional && <span className="font-normal text-ink-faint">{t('mcp.optional')}</span>}
      </span>
      {children}
    </label>
  )
}

function Switch({ checked, label, onChange }: { checked: boolean; label: string; onChange: (checked: boolean) => void }) {
  return (
    <label className="flex cursor-pointer select-none items-center gap-2.5 whitespace-nowrap text-[0.8125rem] font-medium text-ink-soft">
      <button
        className={cn(
          'relative h-5 w-9 shrink-0 cursor-pointer overflow-hidden rounded-full outline-none transition-colors focus-visible:ring-2 focus-visible:ring-edge-stronger',
          checked ? 'bg-success' : 'bg-canvas-strong',
        )}
        type="button"
        role="switch"
        aria-checked={checked}
        onClick={() => onChange(!checked)}
      >
        <span className={cn('absolute top-0.5 left-0.5 size-4 rounded-full bg-white shadow-sm transition-transform', checked ? 'translate-x-4' : 'translate-x-0')} />
      </button>
      {label}
    </label>
  )
}

function TransportButton({ icon: Icon, label, selected, onClick }: { icon: typeof TerminalSquare; label: string; selected: boolean; onClick: () => void }) {
  return (
    <button
      className={cn('flex cursor-pointer items-center justify-center gap-2 rounded-[7px] text-[0.78125rem] font-medium outline-none transition-[background-color,color,box-shadow]', selected ? 'bg-canvas text-ink shadow-sm' : 'text-ink-muted hover:text-ink-soft')}
      type="button"
      role="radio"
      aria-checked={selected}
      onClick={onClick}
    >
      <Icon className="size-3.5" aria-hidden="true" />
      {label}
    </button>
  )
}

function ScopeButton({ label, selected, onClick }: { label: string; selected: boolean; onClick: () => void }) {
  return (
    <button
      className={cn('cursor-pointer rounded-[7px] px-2 text-[0.75rem] font-medium outline-none transition-[background-color,color,box-shadow]', selected ? 'bg-canvas text-ink shadow-sm' : 'text-ink-muted hover:text-ink-soft')}
      type="button"
      role="radio"
      aria-checked={selected}
      onClick={onClick}
    >
      {label}
    </button>
  )
}

function PairEditor({ label, keyPlaceholder, valuePlaceholder, values, duplicate, onChange }: { label: string; keyPlaceholder: string; valuePlaceholder: string; values: Pair[]; duplicate?: string; onChange: (values: Pair[]) => void }) {
  const { t } = useI18n()
  const update = (id: number, field: 'key' | 'value', value: string) => {
    onChange(values.map((entry) => entry.id === id ? { ...entry, [field]: value } : entry))
  }
  return (
    <div>
      <div className="mb-2 flex items-center justify-between gap-3">
        <div className="text-[0.75rem] font-medium text-ink-muted">{label}</div>
        <button
          className="flex h-7 cursor-pointer items-center gap-1.5 rounded-lg px-2 text-[0.71875rem] font-medium text-ink-muted outline-none transition-colors hover:bg-canvas-sunken hover:text-ink focus-visible:bg-canvas-sunken focus-visible:text-ink"
          type="button"
          onClick={() => onChange([...values, { id: ++nextPairID, key: '', value: '' }])}
        >
          <Plus className="size-3.5" aria-hidden="true" />
          {t('mcp.addEntry')}
        </button>
      </div>
      {values.length > 0 && (
        <div className="space-y-2">
          {values.map((entry) => (
            <div key={entry.id} className="grid grid-cols-[minmax(0,0.8fr)_minmax(0,1.2fr)_2.25rem] gap-2 max-sm:grid-cols-[minmax(0,1fr)_2.25rem]">
              <input className={`${inputClass} font-mono text-[0.75rem] max-sm:col-span-1`} value={entry.key} placeholder={keyPlaceholder} spellCheck={false} onChange={(event) => update(entry.id, 'key', event.target.value)} />
              <input className={`${inputClass} font-mono text-[0.75rem] max-sm:col-span-1 max-sm:row-start-2`} value={entry.value} placeholder={valuePlaceholder} spellCheck={false} onChange={(event) => update(entry.id, 'value', event.target.value)} />
              <button className="grid size-9 self-center cursor-pointer place-items-center rounded-lg text-ink-faint outline-none transition-colors hover:bg-danger-surface hover:text-danger focus-visible:bg-danger-surface focus-visible:text-danger max-sm:col-start-2 max-sm:row-span-2 max-sm:row-start-1" type="button" title={t('mcp.removeEntry')} aria-label={t('mcp.removeEntry')} onClick={() => onChange(values.filter((candidate) => candidate.id !== entry.id))}>
                <Trash2 className="size-3.5" aria-hidden="true" />
              </button>
            </div>
          ))}
        </div>
      )}
      {duplicate && <div className="mt-2 text-[0.71875rem] text-danger">{t('mcp.duplicateKey', { key: duplicate })}</div>}
    </div>
  )
}

function duplicateKey(values: Pair[]): string | undefined {
  const seen = new Set<string>()
  for (const entry of values) {
    const key = entry.key.trim().toLocaleLowerCase()
    if (!key) continue
    if (seen.has(key)) return entry.key.trim()
    seen.add(key)
  }
  return undefined
}
