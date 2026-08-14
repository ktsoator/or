import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import {
  ArrowLeft,
  ChevronDown,
  ChevronRight,
  CircleAlert,
  Gauge,
  LoaderCircle,
  RefreshCw,
  RotateCcw,
  Search,
  Wrench,
  X,
} from 'lucide-react'
import { useI18n, type Locale } from '@/i18n'
import { cn } from '@/lib/utils'
import { SidebarToggleButton } from '@/shared/ui/SidebarToggleButton'
import {
  fetchDiagnosticRequest,
  fetchDiagnosticRuns,
  type DiagnosticEvent,
  type DiagnosticRun,
  type DiagnosticReport,
  type RequestSnapshot,
} from './catalog'
import {
  buildTraceRequestCatalog,
  buildTraceRequestView,
  buildTraceRun,
  findTraceProviderRequest,
  type TraceContentItem,
  type TraceOperation,
  type TraceProviderRequest,
  type TraceProviderRequestReference,
  type TraceRun,
  type TraceToolCall,
  type TraceTurn,
} from './viewModel'

export function DiagnosticsPage({
  onBack,
  sidebarCollapsed,
  onExpandSidebar,
  sessionID,
  initialRunID,
}: {
  onBack: () => void
  sidebarCollapsed?: boolean
  onExpandSidebar?: () => void
  sessionID?: string
  initialRunID?: string
}) {
  const { t } = useI18n()
  const [report, setReport] = useState<DiagnosticReport>()
  const [selectedRunID, setSelectedRunID] = useState(initialRunID ?? '')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const load = useCallback(async (signal?: AbortSignal, quiet = false) => {
    if (!quiet) setLoading(true)
    setError(false)
    try {
      const next = await fetchDiagnosticRuns(sessionID, signal)
      setReport(next)
      setSelectedRunID((current) =>
        next.runs.some((run) => run.id === current) ? current : (next.runs[0]?.id ?? ''),
      )
    } catch (cause) {
      if (cause instanceof DOMException && cause.name === 'AbortError') return
      setError(true)
    } finally {
      if (!signal?.aborted && !quiet) setLoading(false)
    }
  }, [sessionID])

  useEffect(() => {
    const controller = new AbortController()
    void load(controller.signal)
    const interval = window.setInterval(() => void load(undefined, true), 5000)
    return () => {
      controller.abort()
      window.clearInterval(interval)
    }
  }, [load])

  const selectedRun =
    report?.runs.find((run) => run.id === selectedRunID) ?? report?.runs[0]
  const requestCatalog = buildTraceRequestCatalog(report?.runs ?? [])

  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col overflow-hidden bg-canvas">
      <header
        className={cn(
          'skills-header window-titlebar z-20 flex h-[45px] shrink-0 items-center justify-between gap-3 border-b border-edge/80 bg-canvas px-4 max-md:h-12',
          sidebarCollapsed && 'sidebar-is-collapsed',
        )}
      >
        <div className="flex min-w-0 items-center gap-1">
          {sidebarCollapsed && onExpandSidebar && (
            <SidebarToggleButton
              expanded={false}
              className="desktop-sidebar-toggle hidden md:grid"
              onToggle={onExpandSidebar}
            />
          )}
          <button
            className="window-titlebar-control flex h-9 cursor-pointer items-center gap-2 rounded-[8px] px-2.5 text-[0.84375rem] text-ink-muted outline-none transition-colors hover:bg-canvas-strong/65 hover:text-ink focus-visible:bg-canvas-strong/65 focus-visible:text-ink"
            type="button"
            onClick={onBack}
          >
            <ArrowLeft className="size-4" aria-hidden="true" />
            <span>{t('diagnostics.back')}</span>
          </button>
        </div>
        <button
          className="window-titlebar-control grid size-8 cursor-pointer place-items-center rounded-[8px] text-ink-muted outline-none transition-colors hover:bg-canvas-strong/65 hover:text-ink focus-visible:bg-canvas-strong/65 focus-visible:text-ink disabled:cursor-wait disabled:opacity-50"
          type="button"
          title={t('diagnostics.refresh')}
          aria-label={t('diagnostics.refresh')}
          disabled={loading}
          onClick={() => void load()}
        >
          <RefreshCw className={cn('size-4', loading && 'animate-spin')} aria-hidden="true" />
        </button>
      </header>

      <main className="min-h-0 flex-1 overflow-hidden bg-canvas">
        <div className="flex h-full w-full min-w-0 flex-col px-7 pb-6 max-lg:px-5 max-md:px-3">
          <h1 className="sr-only">{t('diagnostics.title')}</h1>

          {loading && !report ? (
            <div className="flex min-h-0 flex-1 items-center justify-center gap-2 text-[0.8125rem] text-ink-faint">
              <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
              {t('diagnostics.loading')}
            </div>
          ) : error && !report ? (
            <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-3 text-center">
              <CircleAlert className="size-5 text-danger" aria-hidden="true" />
              <p className="text-[0.84375rem] text-ink-muted">{t('diagnostics.loadError')}</p>
              <button
                className="h-8 cursor-pointer rounded-[8px] border border-edge px-3 text-[0.8125rem] text-ink-soft hover:bg-canvas-sunken"
                type="button"
                onClick={() => void load()}
              >
                {t('diagnostics.retry')}
              </button>
            </div>
          ) : report?.runs.length === 0 ? (
            <div className="flex min-h-0 flex-1 flex-col items-center justify-center text-center">
              <Gauge className="size-6 text-ink-faint" aria-hidden="true" />
              <h2 className="mt-3 text-[0.9375rem] font-medium text-ink-soft">
                {t('diagnostics.emptyTitle')}
              </h2>
              <p className="mt-1 max-w-[25rem] text-[0.8125rem] leading-5 text-ink-muted">
                {t('diagnostics.emptyDescription')}
              </p>
            </div>
          ) : (
            <div className="min-h-0 flex-1">
              {selectedRun && (
                <RunDetail
                  key={selectedRun.id}
                  run={selectedRun}
                  requestCatalog={requestCatalog}
                />
              )}
            </div>
          )}
        </div>
      </main>
    </div>
  )
}

function RunDetail({
  run,
  requestCatalog,
}: {
  run: DiagnosticRun
  requestCatalog: TraceProviderRequestReference[]
}) {
  const { t } = useI18n()
  const [view, setView] = useState<'overview' | 'trajectory'>('trajectory')
  const trace = buildTraceRun(run)
  const durations = [
    { label: t('diagnostics.modelTime'), value: trace.providerDurationMs, tone: 'bg-info' },
    { label: t('diagnostics.toolTime'), value: run.toolDurationMs ?? 0, tone: 'bg-ink-muted' },
    { label: t('diagnostics.approvalWait'), value: run.approvalDurationMs ?? 0, tone: 'bg-warning' },
    { label: t('diagnostics.checkpoint'), value: run.checkpointDurationMs ?? 0, tone: 'bg-success' },
  ].filter((duration) => duration.value > 0)
  return (
    <section
      className={cn(
        'h-full min-h-0',
        view === 'trajectory' ? 'flex flex-col overflow-hidden' : 'overflow-y-auto',
      )}
      aria-label={t('diagnostics.runDetail')}
    >
      {(run.retries > 0 || run.contextRecoveries > 0) && (
        <div className="mt-3 flex shrink-0 items-center gap-2 pl-6 text-[0.75rem] text-ink-muted">
          {run.retries > 0 && (
            <span className="inline-flex items-center gap-1 rounded-[6px] bg-warning-surface px-2 py-1 text-warning">
              <RotateCcw className="size-3" aria-hidden="true" />
              {t('diagnostics.retryCount', { count: run.retries })}
            </span>
          )}
          {run.contextRecoveries > 0 && (
            <span className="rounded-[6px] bg-info-surface px-2 py-1 text-info">
              {t('diagnostics.recoveryCount', { count: run.contextRecoveries })}
            </span>
          )}
        </div>
      )}

      <div className="flex h-8 shrink-0 items-center gap-5 border-b border-edge-soft" role="tablist">
        <DetailTab active={view === 'overview'} label={t('diagnostics.overview')} onClick={() => setView('overview')} />
        <DetailTab active={view === 'trajectory'} label={t('diagnostics.trajectory')} onClick={() => setView('trajectory')} />
      </div>

      {view === 'trajectory' ? (
        <TrajectoryTrace run={run} trace={trace} requestCatalog={requestCatalog} />
      ) : (
        <RunOverview run={run} turns={trace.turns} durations={durations} />
      )}
    </section>
  )
}

function DetailTab({ active, label, onClick }: { active: boolean; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      className={cn(
        'relative h-8 cursor-pointer px-0.5 text-[0.8125rem] font-medium outline-none transition-colors',
        active ? 'text-ink' : 'text-ink-muted hover:text-ink-soft',
      )}
      onClick={onClick}
    >
      {label}
      {active && <span className="absolute inset-x-0 bottom-0 h-0.5 rounded-full bg-info" />}
    </button>
  )
}

function RunOverview({
  run,
  turns,
  durations,
}: {
  run: DiagnosticRun
  turns: TraceTurn[]
  durations: Array<{ label: string; value: number; tone: string }>
}) {
  const { t } = useI18n()
  return (
    <>
      {durations.length > 0 && (
        <section className="mt-6">
          <h3 className="text-[0.875rem] font-semibold text-ink-soft">{t('diagnostics.durationBreakdown')}</h3>
          <div className="mt-4 max-w-[52rem] space-y-4">
            {durations.map((duration) => (
              <DurationBar
                key={duration.label}
                label={duration.label}
                value={duration.value}
                total={run.durationMs ?? 0}
                tone={duration.tone}
              />
            ))}
          </div>
        </section>
      )}

      <div className={cn('border-t border-edge-soft pt-6', durations.length > 0 ? 'mt-8' : 'mt-6')}>
        <div className="flex items-center justify-between gap-4">
          <h3 className="text-[0.875rem] font-semibold text-ink-soft">{t('diagnostics.turns')}</h3>
          <span className="text-[0.75rem] text-ink-muted">{t('diagnostics.turnCount', { count: turns.length })}</span>
        </div>
        <div className="mt-3 space-y-3">
          {turns.map((turn, index) => (
            <TurnSection key={turn.id} turn={turn} index={index} />
          ))}
        </div>
      </div>

      <RawEvents run={run} />
    </>
  )
}

type TraceItem = TraceContentItem & {
  label: string
  title: string
}

type TraceDetailMode = 'summary' | 'preview' | 'tools' | 'raw'

function TrajectoryTrace({
  run,
  trace,
  requestCatalog,
}: {
  run: DiagnosticRun
  trace: TraceRun
  requestCatalog: TraceProviderRequestReference[]
}) {
  const { t, locale, formatNumber } = useI18n()
  const requests = trace.providerRequests
  const [selectedRequestID, setSelectedRequestID] = useState(requests.at(-1)?.providerRequestId ?? '')
  const [snapshot, setSnapshot] = useState<RequestSnapshot>()
  const [loading, setLoading] = useState(false)
  const [unavailable, setUnavailable] = useState(false)
  const [error, setError] = useState(false)
  const [detailMode, setDetailMode] = useState<TraceDetailMode>('summary')
  const [selectedItemID, setSelectedItemID] = useState('')
  const [query, setQuery] = useState('')

  const activeRequestID = requests.some((request) => request.providerRequestId === selectedRequestID)
    ? selectedRequestID
    : (requests.at(-1)?.providerRequestId ?? '')
  const activeRequest = requests.find((request) => request.providerRequestId === activeRequestID)

  useEffect(() => {
    if (!activeRequestID) {
      setSnapshot(undefined)
      return
    }
    const controller = new AbortController()
    setLoading(true)
    setError(false)
    setUnavailable(false)
    void fetchDiagnosticRequest(activeRequestID, run.sessionId, run.id, controller.signal)
      .then((next) => {
        setSnapshot(next)
        setUnavailable(!next)
        setSelectedItemID('')
      })
      .catch((cause) => {
        if (cause instanceof DOMException && cause.name === 'AbortError') return
        setError(true)
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [run.id, run.sessionId, activeRequestID])

  const requestView = snapshot && activeRequest
    ? buildTraceRequestView(activeRequest, snapshot)
    : undefined
  const items = presentTraceItems(requestView?.trajectoryItems ?? [], t)
  const toolItems = presentTraceItems(requestView?.toolItems ?? [], t)
  const normalizedQuery = query.trim().toLocaleLowerCase()
  const visibleItems = normalizedQuery
    ? items.filter((item) => `${item.label} ${item.title} ${item.thinkingPreview ?? ''} ${item.preview} ${item.resultPreview ?? ''} ${item.source ?? ''}`.toLocaleLowerCase().includes(normalizedQuery))
    : items
  const selectedItem = items.find((item) => item.id === selectedItemID)
  const selectedItemRequest = findTraceProviderRequest(
    requestCatalog,
    selectedItem?.providerRequestId,
  )
  const selectedToolOperation = selectedItem?.toolCallId
    ? trace.operations.find((operation): operation is TraceToolCall =>
        operation.kind === 'tool' && operation.toolCallId === selectedItem.toolCallId,
      )
    : undefined

  return (
    <div className="flex min-h-0 flex-1 flex-col pb-5 pt-2">
      {requests.length === 0 ? (
        <TraceEmpty title={t('diagnostics.noProviderRequests')} description={t('diagnostics.noProviderRequestsDescription')} />
      ) : (
        <>
          <TrajectoryTimeline
            requests={requests}
            requestCatalog={requestCatalog}
            selectedRequestID={activeRequestID}
            items={loading ? [] : visibleItems}
            selectedItemID={selectedItemID}
            onSelectRequest={(id) => {
              setSelectedRequestID(id)
              setSelectedItemID('')
              setDetailMode('preview')
            }}
            onSelectItem={(id) => {
              setSelectedItemID(id)
              setDetailMode(defaultTraceDetailMode(items.find((item) => item.id === id), requestCatalog))
            }}
          />

          <div className="mt-2 flex min-h-0 flex-1 flex-col overflow-hidden border-y border-edge">
            {loading ? (
              <div className="flex min-h-0 flex-1 items-center justify-center gap-2 text-[0.8125rem] text-ink-muted">
                <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
                {t('diagnostics.loadingRequest')}
              </div>
            ) : unavailable ? (
              <TraceEmpty title={t('diagnostics.snapshotUnavailable')} description={t('diagnostics.snapshotUnavailableDescription')} />
            ) : error ? (
              <TraceEmpty title={t('diagnostics.snapshotError')} description={t('diagnostics.snapshotErrorDescription')} />
            ) : snapshot ? (
              <>
                <div className="flex min-h-11 shrink-0 flex-wrap items-center justify-between gap-3 border-b border-edge px-2 py-1.5 max-sm:flex-col max-sm:items-stretch max-sm:gap-1">
                  <span className="shrink-0 pl-1 text-[0.71875rem] text-ink-faint">{t('diagnostics.traceItemCount', { count: formatNumber(items.length) })}</span>
                  <div className="flex min-w-0 flex-1 items-center justify-end max-sm:w-full max-sm:flex-none">
                    <label className="flex h-8 w-full max-w-56 items-center gap-2 rounded-[7px] bg-canvas-sunken px-2.5 text-ink-muted focus-within:ring-1 focus-within:ring-edge-strong max-sm:max-w-none">
                      <Search className="size-3.5 shrink-0" aria-hidden="true" />
                      <input
                        value={query}
                        onChange={(event) => setQuery(event.target.value)}
                        placeholder={t('diagnostics.searchInput')}
                        className="min-w-0 flex-1 bg-transparent text-[0.78125rem] text-ink outline-none placeholder:text-ink-faint"
                      />
                      {query && (
                        <button type="button" className="grid size-5 place-items-center" aria-label={t('diagnostics.clearSearch')} onClick={() => setQuery('')}>
                          <X className="size-3.5" aria-hidden="true" />
                        </button>
                      )}
                    </label>
                  </div>
                </div>

                <div
                  className={cn(
                    'grid min-h-0 flex-1 overflow-hidden',
                    selectedItem
                      ? 'grid-cols-[minmax(0,1fr)_minmax(23rem,40%)] max-md:grid-cols-1 max-md:grid-rows-[minmax(10rem,1fr)_minmax(12rem,0.9fr)]'
                      : 'grid-cols-1',
                  )}
                >
                  <TrajectoryLedger
                    items={items}
                    visibleItems={visibleItems}
                    selectedItemID={selectedItemID}
                    showTurns
                    onSelect={(item) => {
                      setSelectedItemID(item.id)
                      setDetailMode(defaultTraceDetailMode(item, requestCatalog))
                    }}
                  />
                  {selectedItem && (
                    <TrajectoryInspector
                      key={selectedItem.id}
                      item={selectedItem}
                      step={traceStepNumber(items, selectedItem)}
                      request={selectedItemRequest?.request}
                      requestNumber={selectedItemRequest?.requestNumber}
                      toolOperation={selectedToolOperation}
                      tools={toolItems}
                      mode={detailMode}
                      onModeChange={setDetailMode}
                      onClose={() => setSelectedItemID('')}
                    />
                  )}
                </div>
              </>
            ) : null}
          </div>

          {snapshot && (
            <div className="mt-2 flex shrink-0 flex-wrap items-center gap-x-2 gap-y-1 text-[0.6875rem] text-ink-faint">
              <span>{formatTimestamp(snapshot.capturedAt, locale)}</span>
              <span aria-hidden="true">·</span>
              <span>{snapshot.provider}</span>
              <span aria-hidden="true">·</span>
              <span className="font-mono">{shortID(snapshot.providerRequestId)}</span>
              <span aria-hidden="true">·</span>
              <span>{t('diagnostics.localSnapshot')}</span>
            </div>
          )}
        </>
      )}
    </div>
  )
}

function TrajectoryTimeline({
  requests,
  requestCatalog,
  selectedRequestID,
  items,
  selectedItemID,
  onSelectRequest,
  onSelectItem,
}: {
  requests: TraceRun['providerRequests']
  requestCatalog: TraceProviderRequestReference[]
  selectedRequestID: string
  items: TraceItem[]
  selectedItemID: string
  onSelectRequest: (id: string) => void
  onSelectItem: (id: string) => void
}) {
  const { t } = useI18n()
  const slotWidth = 100 / Math.max(1, items.length)
  const trackMinWidth = Math.max(320, (items.length * 32) + 68)
  const lanes = [
    { kind: 'input' as const, label: t('diagnostics.timelineInput') },
    { kind: 'model' as const, label: t('diagnostics.timelineModel') },
    { kind: 'tool' as const, label: t('diagnostics.timelineTools') },
  ]
  return (
    <section className="shrink-0" aria-label={t('diagnostics.requestTimeline')}>
      <div className="mb-1 flex min-h-7 flex-wrap items-center justify-between gap-x-4 gap-y-1">
        <div className="flex items-center gap-3">
          <span className="text-[0.75rem] font-medium text-ink-muted">{t('diagnostics.executionTimeline')}</span>
          <span className="text-[0.6875rem] text-ink-faint">{t('diagnostics.traceItemCount', { count: items.length })}</span>
        </div>
        <label className="relative flex h-7 min-w-0 max-w-[18rem] items-center rounded-[6px] border border-edge bg-canvas pl-2.5 pr-7 text-ink-soft hover:border-edge-strong max-sm:w-full max-sm:max-w-none">
          <span className="sr-only">{t('diagnostics.modelRequests')}</span>
          <select
            className="h-full min-w-0 flex-1 cursor-pointer appearance-none bg-transparent font-mono text-[0.6875rem] outline-none"
            value={selectedRequestID}
            aria-label={t('diagnostics.modelRequests')}
            onChange={(event) => onSelectRequest(event.target.value)}
          >
            {requests.map((request, index) => (
              <option key={request.providerRequestId} value={request.providerRequestId}>
                {t('diagnostics.requestLabel', {
                  count: findTraceProviderRequest(requestCatalog, request.providerRequestId)?.requestNumber ?? index + 1,
                })} · {request.model ?? request.provider ?? shortID(request.providerRequestId)}
              </option>
            ))}
          </select>
          <ChevronDown className="pointer-events-none absolute right-2 size-3 text-ink-faint" aria-hidden="true" />
        </label>
      </div>
      <div className="code-scroll-area overflow-x-auto border-y border-edge-soft">
        <div style={{ minWidth: `${trackMinWidth}px` }}>
          {lanes.map((lane) => (
            <div key={lane.kind} className="grid h-6 grid-cols-[4.25rem_minmax(0,1fr)] items-center">
              <span className="px-2 text-[0.6875rem] font-medium text-ink-faint">{lane.label}</span>
              <div className="relative h-4 border-l border-edge-soft bg-[linear-gradient(to_right,var(--edge-soft)_1px,transparent_1px)] bg-[size:25%_100%]">
                {items.map((item, index) => {
                  if (trajectoryTimelineLane(item) !== lane.kind) return null
                  const label = [item.label, item.title, singleLine(item.preview || item.thinkingPreview || '')].filter(Boolean).join(' · ')
                  const className = cn(
                    'absolute top-0.5 h-3 min-w-2 cursor-pointer overflow-hidden rounded-[2px] outline-none transition-[filter,box-shadow] hover:brightness-95 focus-visible:ring-1 focus-visible:ring-info focus-visible:ring-offset-1 focus-visible:ring-offset-canvas',
                    trajectoryTimelineTone(item),
                    item.id === selectedItemID && 'ring-1 ring-info ring-offset-1 ring-offset-canvas',
                  )
                  const style = {
                    left: `${index * slotWidth}%`,
                    width: `calc(${slotWidth}% - 2px)`,
                  }
                  return (
                    <button
                      key={item.id}
                      type="button"
                      className={className}
                      style={style}
                      title={label}
                      aria-label={label}
                      aria-pressed={item.id === selectedItemID}
                      onClick={() => onSelectItem(item.id)}
                    >
                      {item.kind === 'assistant' && item.thinkingPreview && item.preview && (
                        <span className="absolute inset-y-0 right-0 w-1/4 bg-violet-500" aria-hidden="true" />
                      )}
                    </button>
                  )
                })}
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}

function trajectoryTimelineLane(item: TraceItem): 'input' | 'model' | 'tool' {
  if (item.kind === 'toolCall' || item.kind === 'toolResult' || item.kind === 'tool' || item.kind === 'toolSchema') return 'tool'
  if (item.kind === 'assistant' || item.kind === 'thinking') return 'model'
  return 'input'
}

function trajectoryTimelineTone(item: TraceItem): string {
  if (item.kind === 'system') return 'bg-ink-faint'
  if (item.kind === 'user' || item.kind === 'image') return 'bg-info'
  if (item.kind === 'context' || item.kind === 'skill') return 'bg-success'
  if (trajectoryTimelineLane(item) === 'tool') return 'bg-warning'
  if (item.kind === 'assistant' && item.thinkingPreview) return item.preview ? 'bg-violet-300' : 'bg-violet-500'
  return 'bg-violet-400'
}

function TrajectoryLedger({
  items,
  visibleItems,
  selectedItemID,
  showTurns,
  onSelect,
}: {
  items: TraceItem[]
  visibleItems: TraceItem[]
  selectedItemID: string
  showTurns: boolean
  onSelect: (item: TraceItem) => void
}) {
  const { t } = useI18n()
  const selectedRowRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!selectedItemID) return
    selectedRowRef.current?.scrollIntoView({ block: 'center' })
  }, [selectedItemID])
  if (visibleItems.length === 0) {
    return <p className="px-3 py-8 text-center text-[0.8125rem] text-ink-muted">{t('diagnostics.noSearchResults')}</p>
  }
  let previousTurn: number | undefined
  return (
    <div
      className="code-scroll-area min-h-0 flex-1 overflow-y-auto outline-none"
      tabIndex={0}
      aria-label={t('diagnostics.trajectoryRecords')}
    >
      {visibleItems.map((item) => {
        const itemIndex = items.findIndex((candidate) => candidate.id === item.id)
        const showTurn = showTurns && item.turn !== undefined && item.turn !== previousTurn
        previousTurn = item.turn
        return (
          <div key={item.id} ref={item.id === selectedItemID ? selectedRowRef : undefined}>
            {showTurn && (
              <div className="sticky top-0 z-10 flex h-8 items-center gap-3 border-b border-edge-soft bg-canvas-raised/95 px-3 backdrop-blur-sm">
                <span className="font-mono text-[0.75rem] font-semibold text-info">{t('diagnostics.turnLabel', { count: item.turn ?? 1 })}</span>
                <span className="h-px flex-1 bg-edge-soft" aria-hidden="true" />
              </div>
            )}
            <TrajectoryRow
              item={item}
              index={itemIndex}
              active={item.id === selectedItemID}
              onSelect={() => onSelect(item)}
            />
          </div>
        )
      })}
    </div>
  )
}

function TrajectoryRow({ item, index, active, onSelect }: { item: TraceItem; index: number; active: boolean; onSelect: () => void }) {
  const preview = singleLine(item.preview || item.thinkingPreview || '')
  return (
    <button
      type="button"
      className={cn(
        'relative grid min-h-9 w-full cursor-pointer grid-cols-[2.75rem_7.25rem_minmax(0,1fr)] items-start gap-2 border-b border-edge-soft px-2 py-1.5 text-left outline-none transition-colors max-sm:grid-cols-[2.25rem_auto_minmax(0,1fr)]',
        active ? 'bg-surface-selected' : 'hover:bg-surface-hover focus-visible:bg-surface-hover',
      )}
      aria-expanded={active}
      onClick={onSelect}
    >
      {active && <span className="absolute inset-y-0 left-0 w-0.5 bg-info" aria-hidden="true" />}
      <span className="pt-0.5 text-right font-mono text-[0.75rem] font-medium text-ink-muted">{String(index + 1).padStart(2, '0')}</span>
      <TraceBadge kind={item.kind} label={item.label} />
      <span className="flex min-w-0 items-baseline gap-3 text-[0.84375rem] leading-5 max-sm:block">
        {item.title && <span className="shrink-0 font-medium text-ink max-sm:mr-2">{item.title}</span>}
        {item.resultPreview !== undefined ? (
          <span className="grid min-w-0 flex-1 grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-baseline gap-3 text-ink-muted max-md:grid-cols-[minmax(0,1fr)_auto] max-md:[&>*:last-child]:col-span-2 max-sm:inline">
            <span className="truncate">{singleLine(item.preview)}</span>
            <span className="text-ink-faint max-sm:px-1" aria-hidden="true">→</span>
            <span className={cn('truncate', item.isError && 'text-danger')}>{singleLine(item.resultPreview)}</span>
          </span>
        ) : (
          <span className={cn(
            'min-w-0 truncate',
            item.kind === 'system' || item.kind === 'toolSchema' ? 'text-ink-muted' : 'text-ink',
          )}>{preview}</span>
        )}
      </span>
    </button>
  )
}

function TraceBadge({ kind, label }: { kind: TraceItem['kind']; label: string }) {
  const tone = kind === 'system'
    ? 'bg-canvas-strong text-ink-soft'
    : kind === 'user'
      ? 'bg-info-surface text-info'
    : kind === 'context' || kind === 'skill'
      ? 'bg-success/10 text-success'
      : kind === 'toolCall' || kind === 'toolResult' || kind === 'tool' || kind === 'toolSchema'
        ? 'bg-warning-surface text-warning'
        : kind === 'assistant' || kind === 'thinking'
          ? 'bg-violet-50 text-violet-600'
          : 'bg-canvas-sunken text-ink-muted'
  return <span className={cn('w-fit shrink-0 rounded-[5px] px-2 py-0.5 text-[0.6875rem] font-bold uppercase', tone)}>{label}</span>
}

function TrajectoryInspector({
  item,
  step,
  request,
  requestNumber,
  toolOperation,
  tools,
  mode,
  onModeChange,
  onClose,
}: {
  item: TraceItem
  step: number
  request?: TraceProviderRequest
  requestNumber?: number
  toolOperation?: TraceToolCall
  tools: TraceItem[]
  mode: TraceDetailMode
  onModeChange: (mode: TraceDetailMode) => void
  onClose: () => void
}) {
  const { t } = useI18n()
  const raw = mode === 'raw'
    ? JSON.stringify(item.resultRaw === undefined ? item.raw : { input: item.raw, output: item.resultRaw }, null, 2)
    : undefined
  const showTools = item.kind === 'system' && tools.length > 0
  const isSystem = item.kind === 'system'
  const hasRequestSummary = request !== undefined && requestNumber !== undefined && item.kind === 'assistant'
  const toolSchema = item.toolName
    ? tools.find((tool) => tool.toolName === item.toolName)
    : undefined
  return (
    <aside className="flex min-h-0 flex-col overflow-hidden border-l border-edge bg-canvas-raised/35 max-md:border-l-0 max-md:border-t" aria-label={item.label}>
      <div className="flex min-h-11 shrink-0 items-center gap-2 border-b border-edge px-3">
        <TraceBadge kind={item.kind} label={item.label} />
        <span className="min-w-0 flex-1 truncate font-mono text-[0.6875rem] text-ink-muted">
          {item.turn !== undefined ? `${t('diagnostics.turnLabel', { count: item.turn })} · ` : ''}
          {t('diagnostics.stepLabel', { count: step })}
        </span>
        <button
          type="button"
          className="grid size-7 shrink-0 cursor-pointer place-items-center rounded-[6px] text-ink-muted outline-none hover:bg-canvas-strong hover:text-ink focus-visible:bg-canvas-strong focus-visible:text-ink"
          aria-label={t('common.close')}
          onClick={onClose}
        >
          <X className="size-3.5" aria-hidden="true" />
        </button>
      </div>

      <div className="flex h-11 shrink-0 items-end gap-2 border-b border-edge-soft px-4" role="tablist">
        {isSystem ? (
          <DetailModeButton active={mode === 'preview'} label={t('diagnostics.systemPrompt')} onClick={() => onModeChange('preview')} />
        ) : (
          <>
            {hasRequestSummary && (
              <DetailModeButton active={mode === 'summary'} label={t('diagnostics.summary')} onClick={() => onModeChange('summary')} />
            )}
            <DetailModeButton active={mode === 'preview'} label={t('diagnostics.preview')} onClick={() => onModeChange('preview')} />
          </>
        )}
        {showTools && <DetailModeButton active={mode === 'tools'} label={t('diagnostics.availableTools', { count: tools.length })} onClick={() => onModeChange('tools')} />}
        <DetailModeButton active={mode === 'raw'} label={t('diagnostics.raw')} onClick={() => onModeChange('raw')} />
      </div>

      <div className="code-scroll-area min-h-0 flex-1 overflow-auto">
        {mode === 'tools' && showTools ? (
          <ToolSchemaList tools={tools} />
        ) : raw !== undefined ? (
          <pre className="m-0 px-4 py-4 font-mono text-[0.78125rem] leading-6 whitespace-pre-wrap break-words text-ink-soft">{raw}</pre>
        ) : mode === 'summary' && !isSystem && hasRequestSummary ? (
          <RequestItemSummary
            item={item}
            request={request}
            requestNumber={requestNumber}
            onOpenPreview={() => onModeChange('preview')}
          />
        ) : (
          <TraceItemPreview item={item} toolOperation={toolOperation} toolSchema={toolSchema} />
        )}
      </div>
    </aside>
  )
}

function RequestItemSummary({
  item,
  request,
  requestNumber,
  onOpenPreview,
}: {
  item: TraceItem
  request: TraceProviderRequest
  requestNumber: number
  onOpenPreview: () => void
}) {
  const { t } = useI18n()
  const generationMs = request.durationMs !== undefined && request.timeToFirstOutputMs !== undefined
    ? Math.max(0, request.durationMs - request.timeToFirstOutputMs)
    : undefined
  const throughput = generationMs && request.outputTokens !== undefined
    ? request.outputTokens / (generationMs / 1000)
    : undefined
  const token = (value?: number) => value === undefined
    ? '—'
    : t('diagnostics.tokenValue', { count: formatCompactNumber(value) })
  const status = request.status ?? (request.lifecycle === 'complete' ? 'completed' : 'running')
  return (
    <div className="px-5 py-5 max-sm:px-4">
      <dl className="grid grid-cols-[8.5rem_minmax(0,1fr)] gap-x-4 gap-y-2 text-[0.875rem] leading-5 max-sm:grid-cols-[7rem_minmax(0,1fr)]">
        <SummaryRow label={t('diagnostics.detailSource')} value={t('diagnostics.requestNumber', { count: requestNumber })} />
        <SummaryRow label={t('diagnostics.detailStatus')} value={statusLabel(status, t)} />
        <SummaryRow label={t('diagnostics.tokens')} value={token(request.totalTokens)} />
        <SummaryRow nested label={t('diagnostics.tokenInput')} value={request.inputUnknown ? '—' : token(request.inputTokens)} />
        <SummaryRow nested label={t('diagnostics.tokenOutput')} value={token(request.outputTokens)} />
        <SummaryRow
          nested
          label={t('diagnostics.tokenReasoning')}
          value="—"
          title={t('diagnostics.tokenReasoningUnavailable')}
        />
        {request.cacheReadTokens !== undefined && request.cacheReadTokens > 0 && (
          <SummaryRow nested label={t('diagnostics.tokenCacheRead')} value={token(request.cacheReadTokens)} />
        )}
        {request.cacheWriteTokens !== undefined && request.cacheWriteTokens > 0 && (
          <SummaryRow nested label={t('diagnostics.tokenCacheWrite')} value={token(request.cacheWriteTokens)} />
        )}
      </dl>

      <section className="mt-8 border-t border-edge-soft pt-5">
        <button
          type="button"
          className="group flex cursor-pointer items-center gap-1 text-[0.875rem] font-semibold text-ink-soft outline-none hover:text-ink focus-visible:text-info"
          onClick={onOpenPreview}
        >
          {t('diagnostics.preview')}
          <ChevronRight className="size-4 text-ink-faint transition-transform group-hover:translate-x-0.5" aria-hidden="true" />
        </button>
        <div className="mt-4 space-y-5">
          {item.thinkingPreview && <ThinkingDetail value={item.thinkingPreview} />}
          <SummaryContent item={item} />
        </div>
      </section>

      <section className="mt-8 border-t border-edge-soft pt-5">
        <h3 className="text-[0.875rem] font-semibold text-ink-soft">{t('diagnostics.requestTiming')}</h3>
        <dl className="mt-4 grid grid-cols-[8.5rem_minmax(0,1fr)] gap-x-4 gap-y-2 text-[0.875rem] leading-5 max-sm:grid-cols-[7rem_minmax(0,1fr)]">
          <SummaryRow label={t('diagnostics.timingStarted')} value={formatDetailedTimestamp(request.startedAt)} />
          <SummaryRow label={t('diagnostics.totalDuration')} value={formatDuration(request.durationMs)} />
          <SummaryRow label={t('diagnostics.timingTTFT')} value={formatDuration(request.timeToFirstOutputMs)} />
          <SummaryRow label={t('diagnostics.timingGeneration')} value={formatDuration(generationMs)} />
          <SummaryRow
            label={t('diagnostics.timingThroughput')}
            value={throughput === undefined ? '—' : t('diagnostics.throughputValue', { count: throughput.toFixed(1) })}
          />
        </dl>
      </section>
    </div>
  )
}

function SummaryRow({
  label,
  value,
  nested = false,
  title,
}: {
  label: string
  value: string
  nested?: boolean
  title?: string
}) {
  return (
    <>
      <dt className={cn('text-ink-muted', nested && 'pl-5 text-ink-faint')} title={title}>{label}</dt>
      <dd className="m-0 min-w-0 break-words font-medium text-ink">{value}</dd>
    </>
  )
}

function SummaryContent({ item }: { item: TraceItem }) {
  const { t } = useI18n()
  if (item.resultPreview !== undefined) {
    return (
      <div className="space-y-3">
        <div className="flex min-w-0 items-start gap-2 text-[0.8125rem] leading-5 text-ink-muted">
          <Wrench className="mt-0.5 size-3.5 shrink-0 text-ink-faint" aria-hidden="true" />
          <span className="min-w-0 break-words font-mono">{item.title || t('diagnostics.toolCall')} {singleLine(item.preview)}</span>
        </div>
        <p className={cn('m-0 line-clamp-5 whitespace-pre-wrap text-[0.875rem] leading-6 text-ink-soft', item.isError && 'text-danger')}>
          {item.resultPreview || '—'}
        </p>
      </div>
    )
  }
  if (!item.preview && !item.toolCalls?.length) return null
  return (
    <div className="space-y-5">
      {item.preview && (
        <p className="m-0 line-clamp-8 whitespace-pre-wrap text-[0.875rem] leading-6 text-ink-soft">{item.preview}</p>
      )}
      <ToolCallDetails toolCalls={item.toolCalls ?? []} />
    </div>
  )
}

function TraceItemPreview({
  item,
  toolOperation,
  toolSchema,
}: {
  item: TraceItem
  toolOperation?: TraceToolCall
  toolSchema?: TraceItem
}) {
  const { t } = useI18n()
  if (item.kind === 'tool' || item.kind === 'toolCall' || item.kind === 'toolResult') {
    return <ToolExecutionPreview item={item} operation={toolOperation} schema={toolSchema} />
  }
  return (
    <div className="px-5 py-5 max-sm:px-4">
      <div className="mb-5 min-w-0">
        {item.title && <div className="text-[0.875rem] font-semibold text-ink-soft">{item.title}</div>}
        {item.source && <div className="mt-1 break-all font-mono text-[0.75rem] leading-5 text-ink-faint">{item.source}</div>}
      </div>
      <div className="space-y-6">
        {item.thinkingPreview && <ThinkingDetail value={item.thinkingPreview} />}
        {item.resultPreview !== undefined ? (
          <>
            <DetailBlock label={t('diagnostics.toolInput')} value={item.preview} />
            <DetailBlock label={t('diagnostics.toolOutput')} value={item.resultPreview} danger={item.isError} />
          </>
        ) : item.preview ? (
          <DetailBlock label={item.label} value={item.preview} />
        ) : null}
        <ToolCallDetails toolCalls={item.toolCalls ?? []} />
      </div>
    </div>
  )
}

function ToolExecutionPreview({
  item,
  operation,
  schema,
}: {
  item: TraceItem
  operation?: TraceToolCall
  schema?: TraceItem
}) {
  const { t } = useI18n()
  const argumentsValue = toolArguments(item)
  const schemaRaw = isRecord(schema?.raw)
  const schemaDescription = typeof schemaRaw?.description === 'string'
    ? schemaRaw.description
    : undefined
  const status = operation?.status
    ?? (item.resultPreview === undefined ? 'running' : item.isError ? 'failed' : 'completed')

  return (
    <div className="px-5 py-5 max-sm:px-4">
      <dl className="grid grid-cols-[7.5rem_minmax(0,1fr)] gap-x-4 gap-y-2 text-[0.875rem] leading-5 max-sm:grid-cols-[6.5rem_minmax(0,1fr)]">
        <SummaryRow label={t('diagnostics.toolHierarchy')} value={t('diagnostics.assistantMessage')} />
        <SummaryRow label={t('diagnostics.detailStatus')} value={statusLabel(status, t)} />
      </dl>

      <InspectorSection title={t('diagnostics.toolInput')}>
        <ToolArguments value={argumentsValue} />
      </InspectorSection>

      {item.resultPreview !== undefined && (
        <InspectorSection title={t('diagnostics.toolOutput')}>
          <pre className={cn(
            'code-scroll-area m-0 max-h-64 overflow-auto rounded-[6px] bg-canvas-sunken/75 px-3 py-2.5 font-mono text-[0.78125rem] leading-5 whitespace-pre-wrap break-words text-ink-soft',
            item.isError && 'text-danger',
          )}>
            {item.resultPreview || '—'}
          </pre>
        </InspectorSection>
      )}

      {schema && (
        <InspectorSection title={t('diagnostics.toolSchema')}>
          <div className="flex min-w-0 items-baseline gap-3 max-sm:block">
            <span className="shrink-0 font-mono text-[0.8125rem] font-semibold text-ink">{schema.toolName}</span>
            {schemaDescription && (
              <p className="m-0 min-w-0 text-[0.8125rem] leading-5 text-ink-muted max-sm:mt-1.5">{schemaDescription}</p>
            )}
          </div>
        </InspectorSection>
      )}

      <InspectorSection title={t('diagnostics.toolTiming')}>
        <dl className="grid grid-cols-[7.5rem_minmax(0,1fr)] gap-x-4 gap-y-2 text-[0.8125rem] leading-5 max-sm:grid-cols-[6.5rem_minmax(0,1fr)]">
          <SummaryRow label={t('diagnostics.timingStarted')} value={operation ? formatDetailedTimestamp(operation.startedAt) : '—'} />
          <SummaryRow label={t('diagnostics.totalDuration')} value={formatDuration(operation?.durationMs)} />
          {operation?.approvalDurationMs !== undefined && (
            <SummaryRow label={t('diagnostics.approvalWait')} value={formatDuration(operation.approvalDurationMs)} />
          )}
          {operation?.executionDurationMs !== undefined && (
            <SummaryRow label={t('diagnostics.toolExecution')} value={formatDuration(operation.executionDurationMs)} />
          )}
        </dl>
      </InspectorSection>
    </div>
  )
}

function InspectorSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="mt-5 border-t border-edge-soft pt-4">
      <h3 className="mb-3 text-[0.75rem] font-semibold text-ink-muted">{title}</h3>
      {children}
    </section>
  )
}

function ToolArguments({ value }: { value: Record<string, unknown> }) {
  const entries = Object.entries(value)
  if (entries.length === 0) return <span className="text-[0.8125rem] text-ink-faint">—</span>
  return (
    <dl className="overflow-hidden rounded-[6px] bg-canvas-sunken/75">
      {entries.map(([key, entry], index) => (
        <div
          key={key}
          className={cn(
            'grid grid-cols-[minmax(5rem,auto)_minmax(0,1fr)] gap-4 px-3 py-2.5 text-[0.78125rem] leading-5',
            index > 0 && 'border-t border-edge-soft',
          )}
        >
          <dt className="font-mono font-medium text-ink-muted">{key}</dt>
          <dd className="m-0 min-w-0 whitespace-pre-wrap break-words font-mono text-ink-soft">
            {formatToolArgument(entry)}
          </dd>
        </div>
      ))}
    </dl>
  )
}

function toolArguments(item: TraceItem): Record<string, unknown> {
  const raw = isRecord(item.raw)
  return isRecord(raw?.arguments) ?? {}
}

function formatToolArgument(value: unknown): string {
  return typeof value === 'string' ? value : JSON.stringify(value, null, 2) ?? String(value)
}

function ToolCallDetails({
  toolCalls,
}: {
  toolCalls: NonNullable<TraceItem['toolCalls']>
}) {
  const { t } = useI18n()
  return toolCalls.map((toolCall, index) => (
    <div
      key={toolCall.toolCallId ?? `${toolCall.toolName ?? 'tool'}:${index}`}
    >
      <div className="flex min-w-0 items-center gap-2 text-[0.75rem] font-medium text-ink-muted">
        <Wrench className="size-3.5 shrink-0 text-warning" aria-hidden="true" />
        <span>{t('diagnostics.toolCall')}</span>
        <span className="truncate font-mono text-ink-soft">{toolCall.toolName ?? '—'}</span>
      </div>
      <pre className="m-0 mt-2 font-mono text-[0.75rem] leading-5 whitespace-pre-wrap break-words text-ink-soft">
        {JSON.stringify(toolCall.arguments ?? {}, null, 2)}
      </pre>
    </div>
  ))
}

function ThinkingDetail({ value }: { value: string }) {
  const { t } = useI18n()
  return (
    <details open className="group">
      <summary className="flex cursor-pointer list-none items-center gap-1.5 text-[0.75rem] font-medium text-ink-muted outline-none hover:text-ink focus-visible:text-ink">
        <ChevronRight className="size-3.5 shrink-0 transition-transform group-open:rotate-90" aria-hidden="true" />
        {t('diagnostics.trace.thinking')}
      </summary>
      <p className="m-0 mt-2 text-[0.8125rem] leading-6 whitespace-pre-wrap break-words text-ink-muted">{value}</p>
    </details>
  )
}

function ToolSchemaList({ tools }: { tools: TraceItem[] }) {
  const { t } = useI18n()
  const [expandedID, setExpandedID] = useState(tools[0]?.id ?? '')
  return (
    <div>
      {tools.map((tool) => {
        const expanded = tool.id === expandedID
        const raw = isRecord(tool.raw)
        const description = typeof raw?.description === 'string' ? raw.description : singleLine(tool.preview)
        const parameters = raw?.parameters ?? tool.raw
        return (
          <div key={tool.id} className="border-b border-edge-soft">
            <button
              type="button"
              className="flex min-h-11 w-full cursor-pointer items-center gap-2 px-3 py-2 text-left outline-none hover:bg-surface-hover focus-visible:bg-surface-hover"
              aria-expanded={expanded}
              onClick={() => setExpandedID(expanded ? '' : tool.id)}
            >
              <ChevronRight className={cn('size-3.5 shrink-0 text-ink-faint transition-transform', expanded && 'rotate-90')} aria-hidden="true" />
              <Wrench className="size-3.5 shrink-0 text-ink-faint" aria-hidden="true" />
              <span className="shrink-0 font-mono text-[0.78125rem] font-semibold text-ink-soft">{tool.title}</span>
              <span className="min-w-0 truncate text-[0.75rem] text-ink-muted">{description}</span>
            </button>
            {expanded && (
              <div className="px-10 pt-1 pb-4">
                {description && <p className="mb-3 text-[0.78125rem] leading-5 text-ink-muted">{description}</p>}
                <div className="mb-1.5 text-[0.6875rem] font-medium text-ink-faint">{t('diagnostics.raw')}</div>
                <pre className="m-0 font-mono text-[0.75rem] leading-5 whitespace-pre-wrap break-words text-ink-soft">{JSON.stringify(parameters, null, 2)}</pre>
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}

function isRecord(value: unknown): Record<string, unknown> | undefined {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : undefined
}

function DetailModeButton({ active, label, onClick }: { active: boolean; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      className={cn(
        'relative h-10 cursor-pointer px-1 text-[0.8125rem] font-medium outline-none transition-colors',
        active ? 'text-info' : 'text-ink-muted hover:text-ink',
      )}
      onClick={onClick}
    >
      {label}
      {active && <span className="absolute inset-x-1 bottom-0 h-0.5 bg-info" aria-hidden="true" />}
    </button>
  )
}

function DetailBlock({ label, value, danger = false }: { label: string; value: string; danger?: boolean }) {
  return (
    <div className="min-w-0">
      <div className="mb-1.5 text-[0.6875rem] font-medium text-ink-faint">{label}</div>
      <pre className={cn('m-0 font-mono text-[0.8125rem] leading-6 whitespace-pre-wrap break-words text-ink-soft', danger && 'text-danger')}>
        {value || '—'}
      </pre>
    </div>
  )
}

function TraceEmpty({ title, description }: { title: string; description: string }) {
  return (
    <div className="flex min-h-0 flex-1 flex-col items-center justify-center px-6 py-12 text-center">
      <h3 className="text-[0.9375rem] font-medium text-ink-soft">{title}</h3>
      <p className="mt-1.5 max-w-[28rem] text-[0.8125rem] leading-5 text-ink-muted">{description}</p>
    </div>
  )
}

function presentTraceItems(items: TraceContentItem[], t: Translate): TraceItem[] {
  return items.map((item) => ({
    ...item,
    label: traceKindLabel(item.kind, t),
    title: traceItemTitle(item, t),
    preview: item.image
      ? t('diagnostics.imageMetadata', {
          type: item.image.mimeType,
          size: formatBytes(item.image.encodedBytes ?? 0),
        })
      : item.preview,
  }))
}

function traceItemTitle(item: TraceContentItem, t: Translate): string {
  if (item.kind === 'system') return t('diagnostics.systemPrompt')
  if (item.kind === 'context' || item.kind === 'skill') {
    return attachmentKindLabel(item.attachmentKind ?? '', t)
  }
  if (item.kind === 'toolCall') return item.toolName ?? t('diagnostics.toolCall')
  if (item.kind === 'toolResult') return item.toolName ?? t('diagnostics.toolResult')
  if (item.kind === 'tool' || item.kind === 'toolSchema') return item.toolName ?? t('diagnostics.toolCall')
  if (item.kind === 'user' || item.kind === 'assistant' || item.kind === 'thinking' || item.kind === 'image') return ''
  return roleTitle(item.role ?? item.kind, t)
}

function singleLine(value: string): string {
  return value.replace(/\s+/g, ' ').trim()
}

function defaultTraceDetailMode(
  item: TraceItem | undefined,
  requestCatalog: TraceProviderRequestReference[],
): TraceDetailMode {
  if (!item || item.kind === 'system') return 'preview'
  if (item.kind === 'tool' || item.kind === 'toolCall' || item.kind === 'toolResult') return 'preview'
  return findTraceProviderRequest(requestCatalog, item.providerRequestId)
    ? 'summary'
    : 'preview'
}

function traceStepNumber(items: TraceItem[], selected: TraceItem): number {
  const executionItems = items.filter((item) =>
    item.turn === selected.turn && (
      item.kind === 'assistant' || item.kind === 'thinking' ||
      item.kind === 'toolCall' || item.kind === 'toolResult' || item.kind === 'tool'
    ),
  )
  const executionIndex = executionItems.findIndex((item) => item.id === selected.id)
  if (executionIndex >= 0) return executionIndex + 1
  const turnItems = items.filter((item) => item.turn === selected.turn)
  return Math.max(1, turnItems.findIndex((item) => item.id === selected.id) + 1)
}

function formatCompactNumber(value: number): string {
  if (value < 1000) return new Intl.NumberFormat('en-US').format(value)
  return new Intl.NumberFormat('en-US', {
    notation: 'compact',
    maximumFractionDigits: 1,
  }).format(value).replace('K', 'k').replace('M', 'm').replace('B', 'b')
}

function formatDetailedTimestamp(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  const parts = new Intl.DateTimeFormat('en-CA', {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  }).formatToParts(date)
  const part = (type: Intl.DateTimeFormatPartTypes) =>
    parts.find((candidate) => candidate.type === type)?.value ?? ''
  return `${part('year')}-${part('month')}-${part('day')} ${part('hour')}:${part('minute')}:${part('second')}.${String(date.getMilliseconds()).padStart(3, '0')}`
}

function traceKindLabel(kind: TraceContentItem['kind'], t: Translate): string {
  const labels: Record<TraceContentItem['kind'], string> = {
    system: t('diagnostics.trace.system'), user: t('diagnostics.trace.user'),
    assistant: t('diagnostics.trace.assistant'), context: t('diagnostics.trace.context'),
    skill: t('diagnostics.trace.skill'), toolCall: t('diagnostics.trace.toolCall'),
    toolResult: t('diagnostics.trace.toolResult'), thinking: t('diagnostics.trace.thinking'),
    image: t('diagnostics.trace.image'), tool: t('diagnostics.trace.tool'),
    toolSchema: t('diagnostics.trace.toolSchema'),
  }
  return labels[kind]
}

function roleTitle(role: string, t: Translate): string {
  if (role === 'user') return t('diagnostics.userMessage')
  if (role === 'assistant') return t('diagnostics.assistantMessage')
  return role
}

function attachmentKindLabel(kind: string, t: Translate): string {
  const labels: Record<string, string> = {
    base: t('diagnostics.context.base'), skill_listing: t('diagnostics.context.skillListing'),
    skills_update: t('diagnostics.context.skillsUpdate'), activated_skill: t('diagnostics.context.activatedSkill'),
    context_update: t('diagnostics.context.runtime'), task_status: t('diagnostics.context.taskStatus'),
  }
  return labels[kind] ?? kind
}

function formatBytes(value: number): string {
  if (value <= 0) return '0 B'
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}

function DurationBar({ label, value, total, tone }: { label: string; value: number; total: number; tone: string }) {
  const width = total > 0 ? Math.max(1.5, Math.min(100, (value / total) * 100)) : 0
  return (
    <div className="grid grid-cols-[7.5rem_minmax(5rem,1fr)_5rem] items-center gap-4 text-[0.8125rem] max-sm:grid-cols-[6.25rem_minmax(4rem,1fr)_4rem] max-sm:gap-2.5">
      <span className="truncate font-medium text-ink-muted">{label}</span>
      <div className="h-2 overflow-hidden rounded-full bg-canvas-sunken" aria-hidden="true">
        <div className={cn('h-full rounded-full', tone)} style={{ width: `${width}%` }} />
      </div>
      <span className="text-right font-mono text-[0.75rem] font-medium text-ink-soft">{formatDuration(value)}</span>
    </div>
  )
}

function TurnSection({ turn, index }: { turn: TraceTurn; index: number }) {
  const { t } = useI18n()
  const steps = turn.operations.filter(isTraceStep)
  return (
    <section className="rounded-[8px] bg-canvas-sunken/55 px-4 py-3.5" aria-label={t('diagnostics.turnLabel', { count: index + 1 })}>
      <div className="flex items-center justify-between gap-4">
        <div className="flex min-w-0 items-center gap-2">
          <span className="text-[0.8125rem] font-semibold text-ink-soft">{t('diagnostics.turnLabel', { count: index + 1 })}</span>
          {isAbnormalStatus(turn.status) && (
            <span className="text-[0.75rem] font-medium text-danger">{statusLabel(turn.status ?? '', t)}</span>
          )}
        </div>
        <span className="shrink-0 font-mono text-[0.75rem] font-medium text-ink-muted">{formatDuration(turn.durationMs)}</span>
      </div>
      {steps.length > 0 ? (
        <div className="mt-3 space-y-1.5">
          {steps.map((step) => (
            <StepRow key={step.id} step={step} />
          ))}
        </div>
      ) : (
        <p className="mt-2 text-[0.8125rem] text-ink-muted">{t('diagnostics.noMeasuredSteps')}</p>
      )}
    </section>
  )
}

type TraceStep = Exclude<TraceOperation, { kind: 'approval' }>

function isTraceStep(operation: TraceOperation): operation is TraceStep {
  return operation.kind !== 'approval'
}

function StepRow({ step }: { step: TraceStep }) {
  const { t, formatNumber } = useI18n()
  const label = step.kind === 'provider'
    ? t('diagnostics.modelResponse')
    : step.kind === 'tool'
      ? (step.toolName || t('diagnostics.toolCall'))
      : t('diagnostics.checkpoint')
  const abnormal = isAbnormalStatus(step.status) || Boolean(step.errorCode)
  return (
    <div className="grid min-h-11 grid-cols-[minmax(0,1fr)_auto] items-center gap-4 py-1">
      <div className="min-w-0">
        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5">
          <span className="truncate text-[0.8125rem] font-semibold text-ink-soft">{label}</span>
          {step.kind === 'provider' && step.model && (
            <span className="truncate font-mono text-[0.75rem] text-ink-muted">{step.model}</span>
          )}
          {abnormal && step.status && (
            <span className="text-[0.75rem] font-medium text-danger">{statusLabel(step.status, t)}</span>
          )}
        </div>
        <div className="mt-1 flex flex-wrap gap-x-3 gap-y-0.5 text-[0.75rem] text-ink-muted">
          {step.kind === 'provider' && step.timeToFirstOutputMs ? <span>{t('diagnostics.outputStartedAfter', { duration: formatDuration(step.timeToFirstOutputMs) })}</span> : null}
          {step.kind === 'tool' && step.approvalDurationMs ? <span>{t('diagnostics.approvalInline', { duration: formatDuration(step.approvalDurationMs) })}</span> : null}
          {step.kind === 'provider' && step.attempts.length > 1 ? <span className="text-warning">{t('diagnostics.attemptCount', { count: step.attempts.length })}</span> : null}
          {step.kind === 'provider' && step.totalTokens ? <span>{t('diagnostics.requestTokenUsage', { count: formatNumber(step.totalTokens) })}</span> : null}
          {step.errorCode ? <span className="font-mono text-danger">{step.errorCode}</span> : null}
        </div>
      </div>
      <span className="pl-2 font-mono text-[0.75rem] font-medium text-ink-soft">
        {step.durationMs ? formatDuration(step.durationMs) : statusLabel(step.status ?? 'running', t)}
      </span>
    </div>
  )
}

function RawEvents({ run }: { run: DiagnosticRun }) {
  const { t } = useI18n()
  const [expanded, setExpanded] = useState(false)
  return (
    <div className="mt-7 border-t border-edge-soft pt-3 pb-8">
      <button
        type="button"
        className="flex h-10 w-full cursor-pointer items-center justify-between rounded-[6px] px-1 text-left outline-none hover:bg-canvas-sunken focus-visible:bg-canvas-sunken"
        aria-expanded={expanded}
        onClick={() => setExpanded((current) => !current)}
      >
        <span className="text-[0.8125rem] font-medium text-ink-muted">{t('diagnostics.rawEvents')}</span>
        <span className="flex items-center gap-2 text-[0.75rem] text-ink-muted">
          {t('diagnostics.eventCount', { count: run.events.length })}
          <ChevronDown className={cn('size-3.5 transition-transform duration-150', expanded && 'rotate-180')} aria-hidden="true" />
        </span>
      </button>
      {expanded && (
        <div className="mt-1 border-t border-edge-soft">
          {run.omittedEvents ? (
            <div className="ml-[4.85rem] border-l border-edge py-2 pl-5 text-[0.71875rem] text-ink-faint">
              {t('diagnostics.omittedEvents', { count: run.omittedEvents })}
            </div>
          ) : null}
          {run.events.map((event, index) => (
            <EventRow
              key={`${event.timestamp}-${event.name}-${index}`}
              event={event}
              runStartedAt={run.startedAt}
              last={index === run.events.length - 1}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function EventRow({ event, runStartedAt, last }: { event: DiagnosticEvent; runStartedAt: string; last: boolean }) {
  const { t } = useI18n()
  const tone = eventTone(event)
  return (
    <div className="grid grid-cols-[4.85rem_1.25rem_minmax(0,1fr)] text-[0.75rem]">
      <div className="pt-2.5 pr-2 text-right font-mono text-[0.65625rem] text-ink-faint">
        +{formatOffset(runStartedAt, event.timestamp)}
      </div>
      <div className="relative flex justify-center">
        {!last && <span className="absolute top-4 bottom-0 w-px bg-edge" aria-hidden="true" />}
        <span className={cn('relative z-10 mt-3 size-2 rounded-full ring-4 ring-canvas', tone.dot)} aria-hidden="true" />
      </div>
      <div className="min-w-0 border-b border-edge-soft py-2 pl-2 last:border-b-0">
        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
          <span className="font-medium text-ink-soft">{eventLabel(event.name, t)}</span>
          {event.toolName && <span className="font-mono text-[0.6875rem] text-ink-muted">{event.toolName}</span>}
          {event.status && (
            <span className={cn('text-[0.65625rem] font-medium', tone.text)}>{statusLabel(event.status, t)}</span>
          )}
          {event.durationMs ? (
            <span className="font-mono text-[0.65625rem] text-ink-faint">{formatDuration(event.durationMs)}</span>
          ) : null}
          {event.timeToFirstOutputMs ? (
            <span className="text-[0.65625rem] text-ink-faint">
              {t('diagnostics.firstTokenInline', { duration: formatDuration(event.timeToFirstOutputMs) })}
            </span>
          ) : null}
        </div>
        <div className="mt-0.5 flex min-w-0 flex-wrap gap-x-2 gap-y-0.5 font-mono text-[0.625rem] text-ink-faint">
          {event.turnId && <span>{shortID(event.turnId)}</span>}
          {event.providerRequestId && <span>{shortID(event.providerRequestId)}</span>}
          {event.toolCallId && <span>{shortID(event.toolCallId)}</span>}
          {event.provider && event.model && <span>{event.provider}/{event.model}</span>}
          {event.attempt ? <span>{t('diagnostics.attempt', { count: event.attempt })}</span> : null}
          {event.httpStatus ? <span>HTTP {event.httpStatus}</span> : null}
          {event.totalTokens ? <span>{t('diagnostics.tokenCount', { count: event.totalTokens })}</span> : null}
          {event.errorCode && <span className="text-danger">{event.errorCode}</span>}
          {event.reason && <span>{event.reason}</span>}
        </div>
      </div>
    </div>
  )
}

function eventTone(event: DiagnosticEvent): { dot: string; text: string } {
  const name = event.name ?? ''
  if (event.status === 'failed' || event.status === 'cancelled' || event.status === 'denied' || name.endsWith('.failed')) {
    return { dot: 'bg-danger', text: 'text-danger' }
  }
  if (event.status === 'completed' || event.status === 'success' || event.status === 'allowed' || name.endsWith('.completed')) {
    return { dot: 'bg-success', text: 'text-success' }
  }
  if (event.status === 'waiting') return { dot: 'bg-warning', text: 'text-warning' }
  return { dot: 'bg-info', text: 'text-info' }
}

type Translate = ReturnType<typeof useI18n>['t']

function eventLabel(name: string, t: Translate): string {
  const labels: Record<string, string> = {
    'run.started': t('diagnostics.event.runStarted'),
    'run.completed': t('diagnostics.event.runCompleted'),
    'run.failed': t('diagnostics.event.runFailed'),
    'turn.started': t('diagnostics.event.turnStarted'),
    'turn.completed': t('diagnostics.event.turnCompleted'),
    'turn.discarded': t('diagnostics.event.turnDiscarded'),
    'checkpoint.completed': t('diagnostics.event.checkpointCompleted'),
    'checkpoint.failed': t('diagnostics.event.checkpointFailed'),
    'provider.request.started': t('diagnostics.event.providerStarted'),
    'provider.request.completed': t('diagnostics.event.providerCompleted'),
    'provider.request.failed': t('diagnostics.event.providerFailed'),
    'provider.http_attempt.started': t('diagnostics.event.attemptStarted'),
    'provider.http_attempt.response': t('diagnostics.event.attemptResponse'),
    'tool.call.started': t('diagnostics.event.toolStarted'),
    'tool.call.completed': t('diagnostics.event.toolCompleted'),
    'tool.call.failed': t('diagnostics.event.toolFailed'),
    'tool.approval.started': t('diagnostics.event.approvalStarted'),
    'tool.approval.completed': t('diagnostics.event.approvalCompleted'),
    'tool.approval.failed': t('diagnostics.event.approvalFailed'),
  }
  return labels[name] ?? name
}

function statusLabel(status: string, t: Translate): string {
  const labels: Record<string, string> = {
    running: t('diagnostics.status.running'),
    completed: t('diagnostics.status.completed'),
    success: t('diagnostics.status.success'),
    failed: t('diagnostics.status.failed'),
    cancelled: t('diagnostics.status.cancelled'),
    waiting: t('diagnostics.status.waiting'),
    allowed: t('diagnostics.status.allowed'),
    denied: t('diagnostics.status.denied'),
    discarded: t('diagnostics.status.discarded'),
  }
  return labels[status] ?? status
}

function isAbnormalStatus(status?: string): boolean {
  return status === 'failed' || status === 'cancelled' || status === 'denied' || status === 'discarded'
}

function shortID(value: string): string {
  const separator = value.indexOf('_')
  if (separator < 0 || value.length <= separator + 9) return value
  return `${value.slice(0, separator + 1)}${value.slice(separator + 1, separator + 9)}`
}

function formatDuration(value?: number): string {
  if (value === undefined || value === 0) return '—'
  if (value < 1000) return `${Math.max(1, Math.round(value))} ms`
  if (value < 60_000) return `${(value / 1000).toFixed(value < 10_000 ? 2 : 1)} s`
  const minutes = Math.floor(value / 60_000)
  const seconds = Math.round((value % 60_000) / 1000)
  return `${minutes}m ${seconds}s`
}

function formatOffset(startedAt: string, timestamp: string): string {
  const offset = Math.max(0, Date.parse(timestamp) - Date.parse(startedAt))
  return offset < 1000 ? `${offset}ms` : `${(offset / 1000).toFixed(2)}s`
}

function formatTimestamp(value: string, locale: Locale): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return new Intl.DateTimeFormat(locale === 'zh-CN' ? 'zh-CN' : 'en-US', {
    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit',
  }).format(date)
}
