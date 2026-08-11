import { useCallback, useEffect, useRef, useState } from 'react'
import {
  ArrowLeft,
  CheckCircle2,
  ChevronDown,
  CircleAlert,
  LoaderCircle,
  Pencil,
  Plus,
  RefreshCw,
  Trash2,
  Unplug,
  X,
  Zap,
} from 'lucide-react'
import { Dialog } from 'radix-ui'
import { SidebarToggleButton } from '@/components/SidebarToggleButton'
import { useI18n } from '@/i18n'
import { cn } from '@/lib/utils'
import {
  deleteMCPServer,
  fetchMCPServers,
  mcpEndpoint,
  mcpTransport,
  saveMCPServer,
  testMCPServer,
  type MCPListResponse,
  type MCPProbeResult,
  type MCPServerConfig,
  type MCPServerInfo,
} from './catalog'
import { MCPServerDialog } from './MCPServerDialog'

type TestState =
  | { status: 'testing' }
  | { status: 'success'; result: MCPProbeResult }
  | { status: 'error'; error: string }

export function MCPPage({
  onBack,
  sidebarCollapsed,
  onExpandSidebar,
  workspacePath,
  workspaceName,
}: {
  onBack: () => void
  sidebarCollapsed?: boolean
  onExpandSidebar?: () => void
  workspacePath?: string
  workspaceName?: string
}) {
  const { t } = useI18n()
  const [data, setData] = useState<MCPListResponse>()
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [editor, setEditor] = useState<MCPServerInfo | 'new'>()
  const [saving, setSaving] = useState(false)
  const [mutating, setMutating] = useState<Set<string>>(new Set())
  const [tests, setTests] = useState<Record<string, TestState>>({})
  const [deleteTarget, setDeleteTarget] = useState<MCPServerInfo>()
  const [deleting, setDeleting] = useState(false)
  const [deleteError, setDeleteError] = useState('')
  const testControllers = useRef(new Map<string, AbortController>())

  const load = useCallback(async (signal?: AbortSignal) => {
    setLoading(true)
    setLoadError('')
    try {
      setData(await fetchMCPServers(workspacePath, signal))
    } catch (cause) {
      if (cause instanceof DOMException && cause.name === 'AbortError') return
      setLoadError(cause instanceof Error ? cause.message : t('mcp.loadError'))
    } finally {
      if (!signal?.aborted) setLoading(false)
    }
  }, [t, workspacePath])

  useEffect(() => {
    const controller = new AbortController()
    void load(controller.signal)
    return () => controller.abort()
  }, [load])

  useEffect(() => () => {
    for (const controller of testControllers.current.values()) controller.abort()
  }, [])

  const save = async (name: string, config: MCPServerConfig, previousName?: string) => {
    setSaving(true)
    try {
      await saveMCPServer({ name, previousName, config }, workspacePath)
      setEditor(undefined)
      setTests((current) => {
        if (!previousName || previousName === name || !current[previousName]) return current
        const next = { ...current, [name]: current[previousName] }
        delete next[previousName]
        return next
      })
      await load()
    } finally {
      setSaving(false)
    }
  }

  const toggleServer = async (server: MCPServerInfo) => {
    if (mutating.has(server.name)) return
    setMutating((current) => new Set(current).add(server.name))
    try {
      const updated = await saveMCPServer({
        name: server.name,
        previousName: server.name,
        config: { ...server.config, disabled: server.config.disabled ? undefined : true },
      }, workspacePath)
      setData((current) => current && {
        ...current,
        servers: current.servers.map((candidate) => candidate.name === server.name ? updated : candidate),
      })
    } catch (cause) {
      setLoadError(cause instanceof Error ? cause.message : t('mcp.saveFailed'))
    } finally {
      setMutating((current) => {
        const next = new Set(current)
        next.delete(server.name)
        return next
      })
    }
  }

  const testServer = async (server: MCPServerInfo) => {
    testControllers.current.get(server.name)?.abort()
    const controller = new AbortController()
    testControllers.current.set(server.name, controller)
    setTests((current) => ({ ...current, [server.name]: { status: 'testing' } }))
    try {
      const result = await testMCPServer(server.name, workspacePath, controller.signal)
      setTests((current) => ({ ...current, [server.name]: { status: 'success', result } }))
    } catch (cause) {
      if (cause instanceof DOMException && cause.name === 'AbortError') return
      setTests((current) => ({
        ...current,
        [server.name]: {
          status: 'error',
          error: cause instanceof Error ? cause.message : t('mcp.testFailed'),
        },
      }))
    } finally {
      if (testControllers.current.get(server.name) === controller) {
        testControllers.current.delete(server.name)
      }
    }
  }

  const confirmDelete = async () => {
    if (!deleteTarget || deleting) return
    setDeleting(true)
    setDeleteError('')
    try {
      await deleteMCPServer(deleteTarget.name)
      setDeleteTarget(undefined)
      setTests((current) => {
        const next = { ...current }
        delete next[deleteTarget.name]
        return next
      })
      await load()
    } catch (cause) {
      setDeleteError(cause instanceof Error ? cause.message : t('mcp.deleteFailed'))
    } finally {
      setDeleting(false)
    }
  }

  const servers = data?.servers ?? []

  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col overflow-hidden bg-canvas">
      <header className={`skills-header window-titlebar z-20 flex h-[45px] shrink-0 items-center gap-1 border-b border-edge/80 bg-canvas px-4 max-md:h-12 ${sidebarCollapsed ? 'sidebar-is-collapsed' : ''}`}>
        {sidebarCollapsed && onExpandSidebar && (
          <SidebarToggleButton expanded={false} className="desktop-sidebar-toggle hidden md:grid" onToggle={onExpandSidebar} />
        )}
        <button className="window-titlebar-control flex h-9 cursor-pointer items-center gap-2 rounded-[10px] px-2.5 text-[0.84375rem] font-normal text-ink-muted outline-none transition-colors hover:bg-canvas-strong/65 hover:text-ink focus-visible:bg-canvas-strong/65 focus-visible:text-ink" type="button" onClick={onBack}>
          <ArrowLeft className="size-4" aria-hidden="true" />
          <span>{t('mcp.back')}</span>
        </button>
      </header>

      <main className="min-h-0 flex-1 overflow-y-auto bg-canvas md:[scrollbar-gutter:stable_both-edges]">
        <div className="mx-auto w-full max-w-[62rem] px-10 pt-11 pb-24 max-lg:px-7 max-md:px-4 max-md:pt-8">
          <div className="flex items-start justify-between gap-6 max-sm:flex-col max-sm:gap-4">
            <div className="min-w-0">
              <div className="flex items-center gap-2.5">
                <h1 className="text-[1.75rem] leading-9 font-semibold text-ink max-md:text-[1.5rem]">{t('mcp.title')}</h1>
                {!loading && !loadError && servers.length > 0 && (
                  <span className="mt-1 rounded-full bg-canvas-sunken px-2 py-0.5 text-[0.6875rem] font-medium text-ink-muted">{servers.length}</span>
                )}
              </div>
              <p className="mt-1 max-w-[38rem] text-[0.84375rem] leading-5 text-ink-muted">{t('mcp.subtitle')}</p>
              {data?.path && (
                <p className="mt-2 truncate font-mono text-[0.6875rem] text-ink-faint" title={data.path}>{relativizeHome(data.path)}</p>
              )}
            </div>
            <div className="flex shrink-0 items-center gap-2">
              <button className="grid size-9 cursor-pointer place-items-center rounded-[9px] border border-edge bg-canvas text-ink-muted outline-none transition-colors hover:bg-canvas-raised hover:text-ink focus-visible:bg-canvas-raised focus-visible:text-ink disabled:cursor-wait disabled:opacity-45" type="button" title={t('mcp.refresh')} aria-label={t('mcp.refresh')} disabled={loading} onClick={() => void load()}>
                <RefreshCw className={cn('size-4', loading && 'animate-spin')} aria-hidden="true" />
              </button>
              <button className="flex h-9 cursor-pointer items-center gap-2 rounded-[9px] bg-ink px-3.5 text-[0.8125rem] font-medium text-canvas outline-none transition-[opacity,transform] hover:opacity-90 active:translate-y-px focus-visible:ring-2 focus-visible:ring-edge-stronger" type="button" onClick={() => setEditor('new')}>
                <Plus className="size-4" aria-hidden="true" />
                {t('mcp.addServer')}
              </button>
            </div>
          </div>

          {loadError && (
            <div className="mt-8 flex items-start justify-between gap-4 rounded-[10px] border border-danger-edge/80 bg-danger-surface/60 px-4 py-3.5 text-[0.8125rem] text-danger">
              <div className="flex min-w-0 items-start gap-2.5">
                <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
                <span className="break-words leading-5">{loadError}</span>
              </div>
              <button className="shrink-0 cursor-pointer font-medium underline decoration-danger/40 underline-offset-4 hover:decoration-danger" type="button" onClick={() => void load()}>{t('mcp.retry')}</button>
            </div>
          )}

          {loading && !data ? (
            <div className="mt-16 flex items-center justify-center gap-2 text-[0.84375rem] text-ink-faint">
              <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
              {t('mcp.loading')}
            </div>
          ) : !loadError && servers.length === 0 ? (
            <div className="mt-10 flex min-h-64 flex-col items-center justify-center rounded-[12px] border border-dashed border-edge bg-canvas-raised/35 px-8 py-12 text-center">
              <div className="grid size-11 place-items-center rounded-[10px] border border-edge bg-canvas text-ink-muted shadow-sm">
                <Unplug className="size-5" aria-hidden="true" />
              </div>
              <h2 className="mt-4 text-[0.9375rem] font-medium text-ink-soft">{t('mcp.emptyTitle')}</h2>
              <p className="mt-1 max-w-[24rem] text-[0.8125rem] leading-5 text-ink-muted">{t('mcp.emptyDescription')}</p>
              <button className="mt-5 flex h-9 cursor-pointer items-center gap-2 rounded-[9px] border border-edge-strong bg-canvas px-3.5 text-[0.8125rem] font-medium text-ink-soft transition-colors hover:bg-canvas-raised hover:text-ink" type="button" onClick={() => setEditor('new')}>
                <Plus className="size-4" aria-hidden="true" />
                {t('mcp.addServer')}
              </button>
            </div>
          ) : (
            <div className="mt-9 border-y border-edge/80">
              {servers.map((server) => (
                <ServerRow
                  key={server.name}
                  server={server}
                  test={tests[server.name]}
                  workspaceName={workspaceName}
                  mutating={mutating.has(server.name)}
                  onToggle={() => void toggleServer(server)}
                  onTest={() => void testServer(server)}
                  onEdit={() => setEditor(server)}
                  onDelete={() => {
                    setDeleteError('')
                    setDeleteTarget(server)
                  }}
                />
              ))}
            </div>
          )}
        </div>
      </main>

      {editor && (
        <MCPServerDialog
          key={editor === 'new' ? 'new' : editor.name}
          server={editor === 'new' ? undefined : editor}
          configPath={relativizeHome(data?.path ?? '~/.or/coding/mcp.json')}
          workspacePath={workspacePath}
          saving={saving}
          onClose={() => !saving && setEditor(undefined)}
          onSave={save}
        />
      )}
      {deleteTarget && (
        <DeleteServerDialog server={deleteTarget} deleting={deleting} error={deleteError} onClose={() => !deleting && setDeleteTarget(undefined)} onConfirm={() => void confirmDelete()} />
      )}
    </div>
  )
}

function ServerRow({ server, test, workspaceName, mutating, onToggle, onTest, onEdit, onDelete }: { server: MCPServerInfo; test?: TestState; workspaceName?: string; mutating: boolean; onToggle: () => void; onTest: () => void; onEdit: () => void; onDelete: () => void }) {
  const { t } = useI18n()
  const transport = mcpTransport(server.config)
  const endpoint = mcpEndpoint(server.config)
  const status = serverStatus(server, test, t)
  const workspaces = server.config.workspaces ?? []
  const testDisabled = Boolean(server.config.disabled || server.diagnostic || server.inScope === false || test?.status === 'testing')
  const hasDetails = Boolean(server.diagnostic || test?.status === 'error' || test?.status === 'success')

  return (
    <article className="border-b border-edge/75 last:border-b-0">
      <div className={cn('grid min-h-[5.5rem] grid-cols-[minmax(0,1fr)_auto] items-start gap-x-3 px-1 pt-4', !hasDetails && 'pb-4')}>
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-2.5">
            <h2 className="truncate font-mono text-[0.875rem] font-semibold text-ink">{server.name}</h2>
            <span className={cn('flex shrink-0 items-center gap-1.5 text-[0.6875rem] font-medium', status.color)}>
              <span className={cn('size-1.5 rounded-full', status.dot)} aria-hidden="true" />
              {status.label}
            </span>
          </div>
          <p className="mt-1 truncate font-mono text-[0.71875rem] text-ink-muted" title={endpoint}>{endpoint || t('mcp.noEndpoint')}</p>
          <div className="mt-1.5 flex min-w-0 items-center gap-1.5 text-[0.6875rem] text-ink-faint">
            <span>{transport === 'stdio' ? t('mcp.stdio') : t('mcp.http')}</span>
            <span aria-hidden="true">&middot;</span>
            <span className="truncate">{workspaces.length === 0 ? t('mcp.allWorkspaces') : workspaces.length === 1 && server.inScope && workspaceName ? workspaceName : t('mcp.workspaceCount', { count: workspaces.length })}</span>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1 max-sm:col-start-1 max-sm:col-end-3 max-sm:mt-2 max-sm:justify-end">
          <MiniSwitch checked={!server.config.disabled} loading={mutating} label={server.config.disabled ? t('mcp.enableServer', { name: server.name }) : t('mcp.disableServer', { name: server.name })} onChange={onToggle} />
          <IconButton label={t('mcp.test')} disabled={testDisabled} onClick={onTest}>
            {test?.status === 'testing' ? <LoaderCircle className="size-4 animate-spin" aria-hidden="true" /> : <Zap className="size-4" aria-hidden="true" />}
          </IconButton>
          <IconButton label={t('mcp.edit')} onClick={onEdit}><Pencil className="size-4" aria-hidden="true" /></IconButton>
          <IconButton label={t('mcp.delete')} danger onClick={onDelete}><Trash2 className="size-4" aria-hidden="true" /></IconButton>
        </div>
      </div>
      {hasDetails && (
        <div className="mx-1 pb-4">
          {server.diagnostic && (
            <div className="mt-2 flex items-start gap-1.5 text-[0.75rem] leading-5 text-danger">
              <CircleAlert className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
              <span className="break-words">{server.diagnostic}</span>
            </div>
          )}
          {test?.status === 'error' && (
            <div className="mt-2 flex items-start gap-1.5 text-[0.75rem] leading-5 text-danger">
              <CircleAlert className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
              <span className="break-words">{test.error}</span>
            </div>
          )}
          {test?.status === 'success' && <DiscoveredTools result={test.result} />}
        </div>
      )}
    </article>
  )
}

function DiscoveredTools({ result }: { result: MCPProbeResult }) {
  const { t } = useI18n()
  const [expanded, setExpanded] = useState(false)

  return (
    <div className="mt-2.5">
      <button
        className="flex max-w-full cursor-pointer items-center gap-1.5 rounded px-0.5 py-0.5 text-left text-ink-muted outline-none transition-colors hover:text-ink focus-visible:bg-canvas-sunken focus-visible:text-ink"
        type="button"
        aria-expanded={expanded}
        onClick={() => setExpanded((current) => !current)}
      >
        <CheckCircle2 className="size-3.5 shrink-0 text-success" aria-hidden="true" />
        <span className="truncate text-[0.71875rem] font-medium">{t('mcp.toolsDiscovered', { count: result.tools.length })}</span>
        <span aria-hidden="true">&middot;</span>
        <span className="shrink-0 font-mono text-[0.6875rem] tabular-nums text-ink-faint">{formatProbeLatency(result.latencyMs)}</span>
        <ChevronDown className={cn('size-3.5 shrink-0 text-ink-faint transition-transform duration-150', expanded && 'rotate-180')} aria-hidden="true" />
      </button>
      {expanded && result.tools.length > 0 && (
        <ul className="mt-3 grid grid-cols-2 gap-x-8 gap-y-3 border-t border-edge/65 pt-3 pb-1 max-md:grid-cols-1">
          {result.tools.map((tool) => {
            const title = tool.title?.trim()
            return (
              <li key={tool.name} className="min-w-0">
                <div className="flex min-w-0 items-baseline gap-2">
                  <span className="min-w-0 truncate font-mono text-[0.75rem] font-medium text-ink-soft">{title || tool.original}</span>
                  {title && title !== tool.original && (
                    <span className="min-w-0 truncate font-mono text-[0.65625rem] text-ink-faint">{tool.original}</span>
                  )}
                </div>
                {tool.description && (
                  <p className="mt-1 line-clamp-2 break-words text-[0.71875rem] leading-[1.45] text-ink-muted">{tool.description}</p>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}

function formatProbeLatency(latencyMs: number): string {
  return latencyMs >= 1000 ? `${(latencyMs / 1000).toFixed(2)} s` : `${latencyMs} ms`
}

function IconButton({ label, danger, disabled, onClick, children }: { label: string; danger?: boolean; disabled?: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button className={cn('grid size-8 cursor-pointer place-items-center rounded-lg text-ink-faint outline-none transition-colors hover:bg-canvas-sunken hover:text-ink focus-visible:bg-canvas-sunken focus-visible:text-ink disabled:cursor-not-allowed disabled:opacity-30', danger && 'hover:bg-danger-surface hover:text-danger focus-visible:bg-danger-surface focus-visible:text-danger')} type="button" title={label} aria-label={label} disabled={disabled} onClick={onClick}>{children}</button>
  )
}

function MiniSwitch({ checked, loading, label, onChange }: { checked: boolean; loading: boolean; label: string; onChange: () => void }) {
  return (
    <button className={cn('relative mr-1 h-5 w-9 shrink-0 cursor-pointer overflow-hidden rounded-full outline-none transition-colors focus-visible:ring-2 focus-visible:ring-edge-stronger disabled:cursor-wait disabled:opacity-45', checked ? 'bg-success' : 'bg-canvas-strong')} type="button" role="switch" aria-label={label} title={label} aria-checked={checked} disabled={loading} onClick={onChange}>
      <span className={cn('absolute top-0.5 left-0.5 size-4 rounded-full bg-white shadow-sm transition-transform', checked ? 'translate-x-4' : 'translate-x-0')} />
    </button>
  )
}

function DeleteServerDialog({ server, deleting, error, onClose, onConfirm }: { server: MCPServerInfo; deleting: boolean; error: string; onClose: () => void; onConfirm: () => void }) {
  const { t } = useI18n()
  return (
    <Dialog.Root open onOpenChange={(open) => !open && !deleting && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-[180] animate-[fade-in_120ms_ease-out] bg-scrim/25 backdrop-blur-[1px]" />
        <Dialog.Content className="fixed top-1/2 left-1/2 z-[190] w-[min(28rem,calc(100vw-2rem))] -translate-x-1/2 -translate-y-1/2 rounded-[14px] border border-edge bg-canvas p-5 shadow-[0_28px_80px_-32px_rgba(28,25,23,0.55)] outline-none">
          <button className="absolute top-4 right-4 grid size-8 cursor-pointer place-items-center rounded-lg text-ink-faint transition-colors hover:bg-canvas-sunken hover:text-ink disabled:cursor-wait disabled:opacity-40" type="button" aria-label={t('mcp.close')} disabled={deleting} onClick={onClose}><X className="size-4" aria-hidden="true" /></button>
          <Dialog.Title className="pr-10 text-[1rem] font-semibold text-ink">{t('mcp.deleteTitle')}</Dialog.Title>
          <Dialog.Description className="mt-1.5 pr-8 text-[0.8125rem] leading-5 text-ink-muted">{t('mcp.deleteDescription', { name: server.name })}</Dialog.Description>
          <div className="mt-4 border-y border-edge/75 py-3 font-mono text-[0.8125rem] text-ink-soft">{mcpEndpoint(server.config)}</div>
          {error && <div className="mt-4 rounded-[9px] border border-danger-edge/70 bg-danger-surface/70 px-3.5 py-3 text-[0.8125rem] text-danger">{error}</div>}
          <div className="mt-5 flex justify-end gap-2">
            <button className="h-9 cursor-pointer rounded-[9px] border border-edge bg-canvas px-4 text-[0.8125rem] font-medium text-ink-soft hover:bg-canvas-raised disabled:cursor-wait disabled:opacity-45" type="button" disabled={deleting} onClick={onClose}>{t('mcp.cancel')}</button>
            <button className="flex h-9 min-w-24 cursor-pointer items-center justify-center gap-2 rounded-[9px] bg-danger-solid px-4 text-[0.8125rem] font-medium text-ink-inverse hover:bg-danger-solid-hover disabled:cursor-wait disabled:opacity-45" type="button" disabled={deleting} onClick={onConfirm}>{deleting && <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />}{deleting ? t('mcp.deleting') : t('mcp.delete')}</button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

function serverStatus(server: MCPServerInfo, test: TestState | undefined, t: ReturnType<typeof useI18n>['t']) {
  if (server.config.disabled) return { label: t('mcp.statusDisabled'), color: 'text-ink-faint', dot: 'bg-ink-ghost' }
  if (server.diagnostic || test?.status === 'error') return { label: t('mcp.statusProblem'), color: 'text-danger', dot: 'bg-danger' }
  if (server.inScope === false) return { label: t('mcp.statusOutOfScope'), color: 'text-warning', dot: 'bg-warning' }
  if (test?.status === 'success') return { label: t('mcp.statusVerified'), color: 'text-success', dot: 'bg-success' }
  if (test?.status === 'testing') return { label: t('mcp.statusTesting'), color: 'text-info', dot: 'bg-info' }
  return { label: t('mcp.statusConfigured'), color: 'text-ink-muted', dot: 'bg-ink-faint' }
}

function relativizeHome(path: string): string {
  const home = path.match(/^(\/(?:Users|home)\/[^/]+)/)?.[1]
  return home ? path.replace(home, '~') : path
}
