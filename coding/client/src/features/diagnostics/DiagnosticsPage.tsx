import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  ArrowLeft,
  ChevronRight,
  CircleAlert,
  Gauge,
  LoaderCircle,
  RefreshCw,
  Search,
  X,
} from 'lucide-react'
import { useI18n } from '@/i18n'
import { cn } from '@/lib/utils'
import { SidebarToggleButton } from '@/shared/ui/SidebarToggleButton'
import type { Item } from '@/types'
import {
  DiagnosticTraceError,
  fetchDiagnosticTrace,
  mergeDiagnosticTracePage,
  mergeDiagnosticTraceRun,
  mergeLatestDiagnosticTrace,
  type DiagnosticEvent,
  type RequestSnapshotAttachment,
  type RequestSnapshotContent,
  type RequestSnapshotMessage,
  type TraceBundle,
  type TraceBundleRequest,
  type TraceBundleTask,
  type TraceBundleTool,
} from './catalog'
import { liveTraceRefreshKey, mergeLiveTraceBundle } from './liveTrace'

type TraceView = 'overview' | 'trajectory'
type InspectorMode = 'summary' | 'content' | 'input' | 'system' | 'tools' | 'raw'

const TRAJECTORY_VIRTUALIZATION_THRESHOLD = 100
const TRAJECTORY_OVERSCAN = 12
const TRAJECTORY_ROW_ESTIMATE = 36
const TRAJECTORY_TASK_HEADER_HEIGHT = 28
const TIMELINE_COLUMN_WIDTH = 50

export type DiagnosticsSessionState = {
  view?: TraceView
  selectedItemID?: string
  inspectorOpen?: boolean
  inspectorMode?: InspectorMode
  query?: string
  ledgerScrollTop?: number
  ledgerAnchorID?: string
  ledgerAnchorOffset?: number
}

type DiagnosticsStatePatch = Partial<DiagnosticsSessionState>

type TrajectoryScrollTarget = {
  itemID: string
  revision: number
}

type TrajectoryAnchor = {
  itemID: string
  offset: number
  viewportTop?: number
}

type TraceRequestSlot = {
  revision: number
  controller?: AbortController
}

export function DiagnosticsPage({
  onBack,
  sidebarCollapsed,
  onExpandSidebar,
  sessionID,
  initialRunID,
  embedded = false,
  liveItems,
  running = false,
  initialState,
  onStateChange,
}: {
  onBack?: () => void
  sidebarCollapsed?: boolean
  onExpandSidebar?: () => void
  sessionID?: string
  initialRunID?: string
  embedded?: boolean
  liveItems?: Item[]
  running?: boolean
  initialState?: DiagnosticsSessionState
  onStateChange?: (patch: DiagnosticsStatePatch) => void
}) {
  const { t } = useI18n()
  const [bundle, setBundle] = useState<TraceBundle>()
  const [loading, setLoading] = useState(true)
  const [loadingEarlier, setLoadingEarlier] = useState(false)
  const [earlierError, setEarlierError] = useState(false)
  const [error, setError] = useState(false)
  const [view, setView] = useState<TraceView>(initialState?.view ?? 'trajectory')
  const latestRequestRef = useRef<TraceRequestSlot>({ revision: 0 })
  const runRequestRef = useRef<TraceRequestSlot>({ revision: 0 })
  const earlierRequestRef = useRef<TraceRequestSlot>({ revision: 0 })
  const refreshKey = liveTraceRefreshKey(liveItems, running)
  const liveRunID = useMemo(() => {
    const run = liveItems?.findLast((item) => item.kind === 'run')
    return run?.kind === 'run' ? run.runId ?? run.id : undefined
  }, [liveItems])
  const previousRefreshKeyRef = useRef(refreshKey)

  const cancelLoad = useCallback(() => {
    cancelTraceRequest(latestRequestRef.current)
    cancelTraceRequest(runRequestRef.current)
    cancelTraceRequest(earlierRequestRef.current)
  }, [])

  const loadLatest = useCallback(async (quiet = false) => {
    const requestState = startTraceRequest(latestRequestRef.current)
    if (!quiet) setLoading(true)
    setError(false)
    if (!sessionID) {
      cancelTraceRequest(latestRequestRef.current)
      setError(true)
      setLoading(false)
      return
    }
    try {
      const nextBundle = await fetchDiagnosticTrace(sessionID, {
        ...(initialRunID ? { runID: initialRunID } : { limit: 12 }),
        signal: requestState.controller.signal,
      })
      if (isCurrentTraceRequest(latestRequestRef.current, requestState)) {
        setBundle((current) => initialRunID
          ? nextBundle
          : mergeLatestDiagnosticTrace(current, nextBundle))
      }
    } catch (cause) {
      if (!isCurrentTraceRequest(latestRequestRef.current, requestState) || isAbortError(cause)) return
      if (cause instanceof DiagnosticTraceError && cause.status === 404) {
        setBundle((current) => quiet && current ? current : emptyTraceBundle(sessionID, initialRunID))
      } else {
        setError(true)
      }
    } finally {
      if (isCurrentTraceRequest(latestRequestRef.current, requestState)) {
        latestRequestRef.current.controller = undefined
        setLoading(false)
      }
    }
  }, [initialRunID, sessionID])

  const loadRun = useCallback(async (runID: string) => {
    if (!sessionID) return
    const requestState = startTraceRequest(runRequestRef.current)
    try {
      const runBundle = await fetchDiagnosticTrace(sessionID, {
        runID,
        signal: requestState.controller.signal,
      })
      if (isCurrentTraceRequest(runRequestRef.current, requestState)) {
        setBundle((current) => mergeDiagnosticTraceRun(current, runBundle))
      }
    } catch (cause) {
      if (!isCurrentTraceRequest(runRequestRef.current, requestState) || isAbortError(cause)) return
      if (!(cause instanceof DiagnosticTraceError && cause.status === 404)) setError(true)
    } finally {
      if (isCurrentTraceRequest(runRequestRef.current, requestState)) {
        runRequestRef.current.controller = undefined
      }
    }
  }, [sessionID])

  const loadEarlier = useCallback(async (): Promise<boolean> => {
    const beforeCursor = bundle?.page.beforeCursor
    if (!sessionID || !bundle?.page.hasMore || !beforeCursor) return false
    const requestState = startTraceRequest(earlierRequestRef.current)
    setLoadingEarlier(true)
    setEarlierError(false)
    try {
      const olderPage = await fetchDiagnosticTrace(sessionID, {
        beforeCursor,
        limit: 12,
        signal: requestState.controller.signal,
      })
      if (!isCurrentTraceRequest(earlierRequestRef.current, requestState)) return false
      setBundle((current) => current ? mergeDiagnosticTracePage(current, olderPage) : olderPage)
      return true
    } catch (cause) {
      if (!isCurrentTraceRequest(earlierRequestRef.current, requestState) || isAbortError(cause)) return false
      setEarlierError(true)
      return false
    } finally {
      if (isCurrentTraceRequest(earlierRequestRef.current, requestState)) {
        earlierRequestRef.current.controller = undefined
        setLoadingEarlier(false)
      }
    }
  }, [bundle?.page.beforeCursor, bundle?.page.hasMore, sessionID])

  useEffect(() => {
    setBundle(undefined)
    setLoadingEarlier(false)
    setEarlierError(false)
    void loadLatest()
    return cancelLoad
  }, [cancelLoad, loadLatest])

  useEffect(() => {
    if (!running) return
    const interval = window.setInterval(() => {
      if (liveRunID) void loadRun(liveRunID)
      else void loadLatest(true)
    }, 15_000)
    return () => { window.clearInterval(interval) }
  }, [liveRunID, loadLatest, loadRun, running])

  const hasLiveItems = Boolean(liveItems?.length)
  useEffect(() => {
    const previousRefreshKey = previousRefreshKeyRef.current
    previousRefreshKeyRef.current = refreshKey
    if (previousRefreshKey === refreshKey || !sessionID || !hasLiveItems || !liveRunID) return
    const timeout = window.setTimeout(() => void loadRun(liveRunID), 250)
    return () => { window.clearTimeout(timeout) }
  }, [hasLiveItems, liveRunID, loadRun, refreshKey, sessionID])

  const displayBundle = useMemo(
    () => mergeLiveTraceBundle(bundle, sessionID, liveItems, running),
    [bundle, liveItems, running, sessionID],
  )
  const changeView = (nextView: TraceView) => {
    setView(nextView)
    onStateChange?.({ view: nextView })
  }

  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col overflow-hidden bg-canvas">
      {embedded ? (
        <header
          className="flex h-10 shrink-0 items-stretch justify-between border-b border-edge/80 bg-canvas px-4"
          data-testid="diagnostics-toolbar"
        >
          <div className="flex items-stretch gap-1" role="tablist">
            {displayBundle && displayBundle.tasks.length > 0 && (
              <>
                <DetailTab active={view === 'overview'} label={t('diagnostics.overview')} onClick={() => changeView('overview')} />
                <DetailTab active={view === 'trajectory'} label={t('diagnostics.trajectory')} onClick={() => changeView('trajectory')} />
              </>
            )}
          </div>
          <RefreshButton loading={loading} onRefresh={() => void loadLatest()} />
        </header>
      ) : (
        <header
          className={cn(
            'skills-header window-titlebar z-20 flex h-[45px] shrink-0 items-center justify-between gap-3 border-b border-edge/80 bg-canvas px-4 max-md:h-12',
            sidebarCollapsed && 'sidebar-is-collapsed',
          )}
        >
          <div className="flex min-w-0 self-stretch items-center gap-1">
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
            {displayBundle && displayBundle.tasks.length > 0 && (
              <div className="ml-2 flex self-stretch items-stretch gap-1" role="tablist">
                <DetailTab active={view === 'overview'} label={t('diagnostics.overview')} onClick={() => changeView('overview')} />
                <DetailTab active={view === 'trajectory'} label={t('diagnostics.trajectory')} onClick={() => changeView('trajectory')} />
              </div>
            )}
          </div>
          <RefreshButton loading={loading} onRefresh={() => void loadLatest()} />
        </header>
      )}

      <main className="min-h-0 flex-1 overflow-hidden bg-canvas">
        <h1 className="sr-only">{t('diagnostics.title')}</h1>
        {loading && !displayBundle ? (
          <PageState icon={<LoaderCircle className="size-4 animate-spin" />}>
            {t('diagnostics.loading')}
          </PageState>
        ) : error && !displayBundle ? (
          <div className="flex h-full flex-col items-center justify-center gap-3 text-center">
            <CircleAlert className="size-5 text-danger" aria-hidden="true" />
            <p className="text-[0.84375rem] text-ink-muted">{t('diagnostics.loadError')}</p>
            <button
              className="h-8 cursor-pointer rounded-[8px] border border-edge px-3 text-[0.8125rem] text-ink-soft hover:bg-canvas-sunken"
              type="button"
              onClick={() => void loadLatest()}
            >
              {t('diagnostics.retry')}
            </button>
          </div>
        ) : displayBundle && displayBundle.tasks.length > 0 ? (
          <ConversationTrace
            bundle={displayBundle}
            view={view}
            onViewChange={changeView}
            initialState={initialState}
            onStateChange={onStateChange}
            hasEarlier={Boolean(displayBundle.page.hasMore && displayBundle.page.beforeCursor)}
            loadingEarlier={loadingEarlier}
            earlierError={earlierError}
            onLoadEarlier={loadEarlier}
          />
        ) : (
          <div className="flex h-full flex-col items-center justify-center text-center">
            <Gauge className="size-6 text-ink-faint" aria-hidden="true" />
            <h2 className="mt-3 text-[0.9375rem] font-medium text-ink-soft">{t('diagnostics.emptyTitle')}</h2>
            <p className="mt-1 max-w-[25rem] text-[0.8125rem] leading-5 text-ink-muted">
              {t('diagnostics.emptyDescription')}
            </p>
          </div>
        )}
      </main>
    </div>
  )
}

function RefreshButton({ loading, onRefresh }: { loading: boolean; onRefresh: () => void }) {
  const { t } = useI18n()
  return (
    <button
      className="window-titlebar-control my-auto grid size-8 cursor-pointer place-items-center rounded-[8px] text-ink-muted outline-none transition-colors hover:bg-canvas-strong/65 hover:text-ink focus-visible:bg-canvas-strong/65 focus-visible:text-ink disabled:cursor-wait disabled:opacity-50"
      type="button"
      title={t('diagnostics.refresh')}
      aria-label={t('diagnostics.refresh')}
      disabled={loading}
      onClick={onRefresh}
    >
      <RefreshCw className={cn('size-4', loading && 'animate-spin')} aria-hidden="true" />
    </button>
  )
}

function PageState({ icon, children }: { icon: ReactNode; children: ReactNode }) {
  return (
    <div className="flex h-full items-center justify-center gap-2 text-[0.8125rem] text-ink-faint">
      {icon}
      {children}
    </div>
  )
}

function emptyTraceBundle(sessionID: string, selectedTaskID?: string): TraceBundle {
  return {
    version: 1,
    generatedAt: new Date().toISOString(),
    sessionId: sessionID,
    selectedTaskId: selectedTaskID ?? '',
    tasks: [],
    page: { hasMore: false },
  }
}

function startTraceRequest(slot: TraceRequestSlot): { revision: number; controller: AbortController } {
  slot.revision += 1
  slot.controller?.abort()
  const controller = new AbortController()
  slot.controller = controller
  return { revision: slot.revision, controller }
}

function cancelTraceRequest(slot: TraceRequestSlot) {
  slot.revision += 1
  slot.controller?.abort()
  slot.controller = undefined
}

function isCurrentTraceRequest(
  slot: TraceRequestSlot,
  request: { revision: number; controller: AbortController },
): boolean {
  return slot.revision === request.revision &&
    slot.controller === request.controller &&
    !request.controller.signal.aborted
}

function isAbortError(cause: unknown): boolean {
  return cause instanceof DOMException && cause.name === 'AbortError'
}

type TrajectoryKind = 'system' | 'user' | 'context' | 'assistant' | 'tool'

type TrajectoryItem = {
  id: string
  kind: TrajectoryKind
  task: TraceBundleTask
  taskNumber: number
  request?: TraceBundleRequest
  tool?: TraceBundleTool
  attachment?: RequestSnapshotAttachment
  message?: RequestSnapshotMessage
  preview: string
  thinking?: string
  thinkingOnly?: boolean
  raw: unknown
}

function ConversationTrace({
  bundle,
  view,
  onViewChange,
  initialState,
  onStateChange,
  hasEarlier,
  loadingEarlier,
  earlierError,
  onLoadEarlier,
}: {
  bundle: TraceBundle
  view: TraceView
  onViewChange: (view: TraceView) => void
  initialState?: DiagnosticsSessionState
  onStateChange?: (patch: DiagnosticsStatePatch) => void
  hasEarlier: boolean
  loadingEarlier: boolean
  earlierError: boolean
  onLoadEarlier: () => Promise<boolean>
}) {
  const { t } = useI18n()
  const rememberedStateRef = useRef<DiagnosticsSessionState>(initialState ?? {})
  const rememberState = (patch: DiagnosticsStatePatch) => {
    rememberedStateRef.current = { ...rememberedStateRef.current, ...patch }
    onStateChange?.(patch)
  }
  const rememberedState = rememberedStateRef.current
  const items = useMemo(() => buildTrajectoryItems(bundle.tasks, t), [bundle.tasks, t])
  const requests = useMemo(() => bundle.tasks.flatMap((task) => task.requests), [bundle.tasks])
  const initialRequest = selectedTask(bundle)?.requests.at(-1) ?? requests.at(-1)
  const initialItem = items.findLast(
    (item) => item.kind === 'assistant' && item.request?.id === initialRequest?.id,
  ) ?? items.findLast((item) => item.request?.id === initialRequest?.id) ?? items.at(-1)
  const [selectedItemID, setSelectedItemID] = useState(
    rememberedState.selectedItemID ?? initialItem?.id ?? '',
  )
  const [inspectorOpen, setInspectorOpen] = useState(rememberedState.inspectorOpen ?? true)
  const [mode, setMode] = useState<InspectorMode>(rememberedState.inspectorMode ?? 'summary')
  const [scrollTarget, setScrollTarget] = useState<TrajectoryScrollTarget>()
  const scrollRevisionRef = useRef(0)
  const selectedItem = items.find((item) => item.id === selectedItemID) ?? initialItem

  const selectItem = (item: TrajectoryItem, scroll = false) => {
    setSelectedItemID(item.id)
    setInspectorOpen(true)
    setMode('summary')
    rememberState({ selectedItemID: item.id, inspectorOpen: true, inspectorMode: 'summary' })
    if (scroll) {
      scrollRevisionRef.current += 1
      setScrollTarget({ itemID: item.id, revision: scrollRevisionRef.current })
    }
  }
  const selectRequest = (requestID: string) => {
    const item = items.find((candidate) => candidate.kind === 'assistant' && candidate.request?.id === requestID)
      ?? items.find((candidate) => candidate.request?.id === requestID)
    if (item) selectItem(item, true)
  }

  return (
    <section className="flex h-full min-h-0 flex-col overflow-hidden px-7 pb-3 max-lg:px-5 max-md:px-3" aria-label={t('diagnostics.runDetail')}>
      {view === 'overview' ? (
        <ConversationOverview
          bundle={bundle}
          hasEarlier={hasEarlier}
          loadingEarlier={loadingEarlier}
          earlierError={earlierError}
          onLoadEarlier={onLoadEarlier}
          onOpenRequest={(requestID) => {
            selectRequest(requestID)
            onViewChange('trajectory')
          }}
        />
      ) : items.length === 0 ? (
        <TraceEmpty title={t('diagnostics.noProviderRequests')} description={t('diagnostics.noProviderRequestsDescription')} />
      ) : (
        <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
          <TrajectoryTimeline
            items={items}
            selectedItemID={selectedItem?.id ?? ''}
            onSelectItem={(item) => selectItem(item, true)}
          />
          <div
            className={cn(
              'grid min-h-0 flex-1 overflow-hidden',
              inspectorOpen && selectedItem
                ? 'grid-cols-[minmax(0,1.6fr)_minmax(22rem,0.9fr)] max-md:grid-cols-1 max-md:grid-rows-[minmax(12rem,1fr)_minmax(12rem,0.9fr)]'
                : 'grid-cols-1',
            )}
          >
            <TrajectoryLedger
              items={items}
              selectedItemID={selectedItem?.id ?? ''}
              onSelect={selectItem}
              scrollTarget={scrollTarget}
              initialQuery={rememberedState.query}
              initialScrollTop={rememberedState.ledgerScrollTop}
              initialAnchorID={rememberedState.ledgerAnchorID}
              initialAnchorOffset={rememberedState.ledgerAnchorOffset}
              onQueryChange={(query) => rememberState({ query })}
              onScrollPositionChange={(position) => rememberState({
                ledgerScrollTop: position.scrollTop,
                ledgerAnchorID: position.anchorID,
                ledgerAnchorOffset: position.anchorOffset,
              })}
              hasEarlier={hasEarlier}
              loadingEarlier={loadingEarlier}
              earlierError={earlierError}
              onLoadEarlier={onLoadEarlier}
            />
            {inspectorOpen && selectedItem && (
              <TrajectoryInspector
                item={selectedItem}
                mode={mode}
                onModeChange={(nextMode) => {
                  setMode(nextMode)
                  rememberState({ inspectorMode: nextMode })
                }}
                onClose={() => {
                  setInspectorOpen(false)
                  rememberState({ inspectorOpen: false })
                }}
              />
            )}
          </div>
        </div>
      )}
    </section>
  )
}

function selectedTask(bundle: TraceBundle): TraceBundleTask | undefined {
  return bundle.tasks.find((task) => task.id === bundle.selectedTaskId) ?? bundle.tasks.at(-1)
}

function DetailTab({ active, label, onClick }: { active: boolean; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      className={cn(
        'window-titlebar-control relative self-stretch cursor-pointer px-2.5 text-[0.8125rem] font-medium outline-none transition-colors',
        active ? 'text-ink' : 'text-ink-muted hover:text-ink-soft',
      )}
      onClick={onClick}
    >
      {label}
      {active && <span className="absolute inset-x-0 bottom-0 h-0.5 bg-info" aria-hidden="true" />}
    </button>
  )
}

function TrajectoryTimeline({
  items,
  selectedItemID,
  onSelectItem,
}: {
  items: TrajectoryItem[]
  selectedItemID: string
  onSelectItem: (item: TrajectoryItem) => void
}) {
  const { t } = useI18n()
  const lanes: Array<{ id: string; kinds: TrajectoryKind[]; label: string }> = [
    { id: 'input', kinds: ['system', 'user', 'context'], label: t('diagnostics.timelineInput') },
    { id: 'model', kinds: ['assistant'], label: t('diagnostics.timelineModel') },
    { id: 'tools', kinds: ['tool'], label: t('diagnostics.timelineTools') },
  ]
  const virtualized = items.length > TRAJECTORY_VIRTUALIZATION_THRESHOLD
  const scrollRef = useRef<HTMLDivElement>(null)
  const columnVirtualizer = useVirtualizer({
    count: items.length,
    enabled: virtualized,
    getScrollElement: () => scrollRef.current,
    getItemKey: (index) => items[index]?.id ?? index,
    estimateSize: () => TIMELINE_COLUMN_WIDTH,
    horizontal: true,
    overscan: TRAJECTORY_OVERSCAN,
    useFlushSync: false,
  })
  const selectedIndex = items.findIndex((item) => item.id === selectedItemID)
  useEffect(() => {
    if (!virtualized || selectedIndex < 0) return
    const frame = window.requestAnimationFrame(() => {
      columnVirtualizer.scrollToIndex(selectedIndex, { align: 'auto' })
    })
    return () => { window.cancelAnimationFrame(frame) }
  }, [columnVirtualizer, selectedIndex, virtualized])

  if (virtualized) {
    return (
      <section className="shrink-0 py-2" aria-label={t('diagnostics.executionTimeline')}>
        <div className="grid grid-cols-[3.5rem_minmax(0,1fr)] gap-x-2 border-b border-edge-soft py-1">
          <div>
            {lanes.map((lane) => (
              <span key={lane.id} className="flex h-5 items-center text-[0.6875rem] text-ink-faint">
                {lane.label}
              </span>
            ))}
          </div>
          <div
            ref={scrollRef}
            className="code-scroll-area overflow-x-auto"
            data-testid="diagnostics-timeline-scroll"
            data-virtualized="true"
          >
            <div
              className="relative h-[3.75rem]"
              style={{ width: `${columnVirtualizer.getTotalSize()}px` }}
            >
              {columnVirtualizer.getVirtualItems().map((column) => {
                const item = items[column.index]!
                const laneIndex = lanes.findIndex((lane) => lane.kinds.includes(item.kind))
                return (
                  <div
                    key={column.key}
                    className="absolute top-0 flex h-5 items-center px-0.5"
                    style={{
                      left: `${column.start}px`,
                      top: `${laneIndex * 20}px`,
                      width: `${column.size}px`,
                    }}
                  >
                    <TimelineItemButton
                      item={item}
                      active={item.id === selectedItemID}
                      index={column.index}
                      onSelect={() => onSelectItem(item)}
                    />
                  </div>
                )
              })}
            </div>
          </div>
        </div>
      </section>
    )
  }

  const contentWidth = Math.max(640, items.length * TIMELINE_COLUMN_WIDTH)
  return (
    <section className="shrink-0 py-2" aria-label={t('diagnostics.executionTimeline')}>
      <div className="code-scroll-area overflow-x-auto border-b border-edge-soft py-1">
        <div className="grid grid-cols-[3.5rem_minmax(0,1fr)] gap-x-2" style={{ minWidth: `${contentWidth + 64}px` }}>
          {lanes.map((lane) => (
            <div key={lane.id} className="contents">
              <span className="flex h-5 items-center text-[0.6875rem] text-ink-faint">{lane.label}</span>
              <div
                className="grid h-5 items-center gap-1"
                style={{ gridTemplateColumns: `repeat(${items.length}, minmax(2.75rem, 1fr))` }}
              >
                {items.map((item, index) => lane.kinds.includes(item.kind) ? (
                  <TimelineItemButton
                    key={item.id}
                    item={item}
                    active={item.id === selectedItemID}
                    index={index}
                    style={{ gridColumnStart: index + 1 }}
                    onSelect={() => onSelectItem(item)}
                  />
                ) : null)}
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}

function TimelineItemButton({
  item,
  active,
  index,
  style,
  onSelect,
}: {
  item: TrajectoryItem
  active: boolean
  index: number
  style?: CSSProperties
  onSelect: () => void
}) {
  const { t } = useI18n()
  return (
    <button
      type="button"
      title={trajectoryItemTitle(item, t)}
      aria-label={trajectoryItemTitle(item, t)}
      aria-pressed={active}
      data-timeline-index={index}
      data-timeline-item-id={item.id}
      className={cn(
        'h-2.5 w-full cursor-pointer rounded-[2px] outline-none ring-offset-1 ring-offset-canvas transition-[height,opacity,box-shadow] hover:h-3.5 focus-visible:ring-2 focus-visible:ring-info',
        timelineTone(item.kind),
        active ? 'h-3.5 ring-2 ring-info' : 'opacity-90 hover:opacity-100',
      )}
      style={style}
      onClick={onSelect}
    />
  )
}

function TrajectoryLedger({
  items,
  selectedItemID,
  onSelect,
  scrollTarget,
  initialQuery = '',
  initialScrollTop = 0,
  initialAnchorID,
  initialAnchorOffset = 0,
  onQueryChange,
  onScrollPositionChange,
  hasEarlier,
  loadingEarlier,
  earlierError,
  onLoadEarlier,
}: {
  items: TrajectoryItem[]
  selectedItemID: string
  onSelect: (item: TrajectoryItem) => void
  scrollTarget?: TrajectoryScrollTarget
  initialQuery?: string
  initialScrollTop?: number
  initialAnchorID?: string
  initialAnchorOffset?: number
  onQueryChange?: (query: string) => void
  onScrollPositionChange?: (position: {
    scrollTop: number
    anchorID?: string
    anchorOffset?: number
  }) => void
  hasEarlier: boolean
  loadingEarlier: boolean
  earlierError: boolean
  onLoadEarlier: () => Promise<boolean>
}) {
  const { t } = useI18n()
  const [query, setQuery] = useState(initialQuery)
  const scrollRef = useRef<HTMLDivElement>(null)
  const prependAnchorRef = useRef<TrajectoryAnchor | undefined>(undefined)
  const normalizedQuery = query.trim().toLocaleLowerCase()
  const filtered = useMemo(() => normalizedQuery
    ? items.filter((item) => trajectorySearchText(item).toLocaleLowerCase().includes(normalizedQuery))
    : items, [items, normalizedQuery])
  const virtualized = filtered.length > TRAJECTORY_VIRTUALIZATION_THRESHOLD
  const rowVirtualizer = useVirtualizer({
    count: filtered.length,
    enabled: virtualized,
    getScrollElement: () => scrollRef.current,
    getItemKey: (index) => filtered[index]?.id ?? index,
    estimateSize: (index) => TRAJECTORY_ROW_ESTIMATE +
      (filtered[index]?.kind === 'user' ? TRAJECTORY_TASK_HEADER_HEIGHT : 0),
    overscan: TRAJECTORY_OVERSCAN,
    useFlushSync: false,
  })
  const virtualRows = rowVirtualizer.getVirtualItems()
  const visibleVirtualRow = virtualRows.find((row) =>
    row.end > (rowVirtualizer.scrollOffset ?? 0))
  const visibleItem = visibleVirtualRow ? filtered[visibleVirtualRow.index] : undefined
  const currentTaskNumber = visibleVirtualRow
    ? filtered.slice(0, visibleVirtualRow.index + 1)
        .findLast((item) => item.kind === 'user')?.taskNumber
    : undefined
  const overlayTaskNumber = visibleItem?.kind === 'user' ? undefined : currentTaskNumber

  const restoreAnchor = useCallback((anchor: TrajectoryAnchor) => {
    const scrollArea = scrollRef.current
    const index = filtered.findIndex((item) => item.id === anchor.itemID)
    if (!scrollArea || index < 0) return
    const adjustToMeasuredRow = () => {
      const row = [...scrollArea.querySelectorAll<HTMLElement>('[data-trajectory-item-id]')]
        .find((candidate) => candidate.dataset.trajectoryItemId === anchor.itemID)
      if (!row) return
      const rowTop = row.getBoundingClientRect().top
      const nextPosition = anchor.viewportTop === undefined
        ? rowTop - scrollArea.getBoundingClientRect().top
        : rowTop
      const desiredPosition = anchor.viewportTop ?? anchor.offset
      scrollArea.scrollTop += nextPosition - desiredPosition
    }
    if (!virtualized) {
      adjustToMeasuredRow()
      return
    }
    window.requestAnimationFrame(() => {
      const itemOffset = rowVirtualizer.getOffsetForIndex(index, 'start')?.[0]
      if (itemOffset === undefined) return
      const desiredOffset = anchor.viewportTop === undefined
        ? anchor.offset
        : anchor.viewportTop - scrollArea.getBoundingClientRect().top
      const rowInset = filtered[index]?.kind === 'user' ? TRAJECTORY_TASK_HEADER_HEIGHT : 0
      rowVirtualizer.scrollToOffset(itemOffset + rowInset - desiredOffset)
      window.requestAnimationFrame(() => {
        window.requestAnimationFrame(adjustToMeasuredRow)
      })
    })
  }, [filtered, rowVirtualizer, virtualized])

  const initialPositionRestoredRef = useRef(false)
  useLayoutEffect(() => {
    if (initialPositionRestoredRef.current || filtered.length === 0) return
    initialPositionRestoredRef.current = true
    if (initialAnchorID) {
      restoreAnchor({ itemID: initialAnchorID, offset: initialAnchorOffset })
    } else if (scrollRef.current) {
      scrollRef.current.scrollTop = initialScrollTop
    }
  }, [filtered.length, initialAnchorID, initialAnchorOffset, initialScrollTop, restoreAnchor])

  useLayoutEffect(() => {
    const anchor = prependAnchorRef.current
    if (!anchor || loadingEarlier) return
    prependAnchorRef.current = undefined
    restoreAnchor(anchor)
  }, [filtered, loadingEarlier, restoreAnchor])

  useEffect(() => {
    if (!scrollTarget) return
    const index = filtered.findIndex((item) => item.id === scrollTarget.itemID)
    if (index < 0) return
    if (virtualized) {
      const frame = window.requestAnimationFrame(() => {
        rowVirtualizer.scrollToIndex(index, { align: 'auto' })
      })
      return () => { window.cancelAnimationFrame(frame) }
    }
    const frame = window.requestAnimationFrame(() => {
      const scrollArea = scrollRef.current
      const row = scrollArea
        ? [...scrollArea.querySelectorAll<HTMLElement>('[data-trajectory-item-id]')]
            .find((candidate) => candidate.dataset.trajectoryItemId === scrollTarget.itemID)
        : undefined
      row?.scrollIntoView({ block: 'nearest' })
    })
    return () => { window.cancelAnimationFrame(frame) }
  }, [filtered, rowVirtualizer, scrollTarget, virtualized])

  const captureVisibleAnchor = () => {
    const scrollArea = scrollRef.current
    if (!scrollArea) return undefined
    const scrollBounds = scrollArea.getBoundingClientRect()
    const visibleTop = scrollBounds.top +
      (virtualized && currentTaskNumber !== undefined ? TRAJECTORY_TASK_HEADER_HEIGHT : 0)
    const row = [...scrollArea.querySelectorAll<HTMLElement>('[data-trajectory-item-id]')]
      .find((candidate) => candidate.getBoundingClientRect().bottom > visibleTop)
    if (!row?.dataset.trajectoryItemId) return undefined
    return {
      itemID: row.dataset.trajectoryItemId,
      offset: row.getBoundingClientRect().top - scrollBounds.top,
      viewportTop: row.getBoundingClientRect().top,
    }
  }

  const handleScroll = (scrollArea: HTMLDivElement) => {
    const anchor = captureVisibleAnchor()
    onScrollPositionChange?.({
      scrollTop: scrollArea.scrollTop,
      anchorID: anchor?.itemID,
      anchorOffset: anchor?.offset,
    })
  }

  const loadEarlier = () => {
    prependAnchorRef.current = captureVisibleAnchor()
    void onLoadEarlier().then((loaded) => {
      if (!loaded) prependAnchorRef.current = undefined
    })
  }

  return (
    <section className="flex min-h-0 flex-col overflow-hidden" aria-label={t('diagnostics.trajectoryRecords')}>
      <div className="flex h-10 shrink-0 items-center justify-between gap-4 border-b border-edge-soft px-3">
        <span className="text-[0.6875rem] text-ink-faint">{t('diagnostics.traceItemCount', { count: filtered.length })}</span>
        <label className="relative block w-60 max-w-[45%]">
          <Search className="pointer-events-none absolute left-2.5 top-2 size-3.5 text-ink-faint" aria-hidden="true" />
          <input
            type="search"
            value={query}
            placeholder={t('diagnostics.searchInput')}
            className="h-7 w-full rounded-[6px] border-0 bg-canvas-sunken pl-8 pr-2 text-[0.75rem] text-ink outline-none placeholder:text-ink-faint focus-visible:ring-1 focus-visible:ring-info"
            onChange={(event) => {
              setQuery(event.target.value)
              onQueryChange?.(event.target.value)
            }}
          />
        </label>
      </div>
      {hasEarlier && (
        <LoadEarlierControl loading={loadingEarlier} error={earlierError} onLoad={loadEarlier} />
      )}
      <div className="relative min-h-0 flex-1 overflow-hidden">
        <div
          ref={scrollRef}
          className="code-scroll-area h-full overflow-y-auto"
          data-testid="diagnostics-ledger-scroll"
          data-virtualized={virtualized ? 'true' : 'false'}
          onScroll={(event) => handleScroll(event.currentTarget)}
        >
          {filtered.length === 0 ? (
            <TraceEmpty title={t('diagnostics.noSearchResults')} description={t('diagnostics.searchInput')} />
          ) : virtualized ? (
            <div
              className="relative w-full"
              data-testid="diagnostics-virtual-spacer"
              style={{ height: `${rowVirtualizer.getTotalSize()}px` }}
            >
              {virtualRows.map((virtualRow) => {
                const item = filtered[virtualRow.index]!
                return (
                  <div
                    key={virtualRow.key}
                    ref={rowVirtualizer.measureElement}
                    data-index={virtualRow.index}
                    className="absolute left-0 top-0 w-full"
                    style={{ transform: `translateY(${virtualRow.start}px)` }}
                  >
                    <TrajectoryRow
                      item={item}
                      index={trajectorySequenceIndex(items, item)}
                      active={item.id === selectedItemID}
                      onSelect={() => onSelect(item)}
                    />
                  </div>
                )
              })}
            </div>
          ) : filtered.map((item) => (
            <TrajectoryRow
              key={item.id}
              item={item}
              index={trajectorySequenceIndex(items, item)}
              active={item.id === selectedItemID}
              onSelect={() => onSelect(item)}
            />
          ))}
        </div>
        {virtualized && overlayTaskNumber !== undefined && (
          <TaskHeader taskNumber={overlayTaskNumber} overlay />
        )}
      </div>
    </section>
  )
}

function TrajectoryRow({
  item,
  index,
  active,
  showTaskHeader = true,
  onSelect,
}: {
  item: TrajectoryItem
  index: number
  active: boolean
  showTaskHeader?: boolean
  onSelect: () => void
}) {
  const { t } = useI18n()
  const toolArguments = item.tool ? singleLine(JSON.stringify(item.tool.arguments ?? {})) : ''
  const toolResult = item.tool?.result ? singleLine(messageText(item.tool.result)) : statusText(item.tool?.status)
  return (
    <>
      {showTaskHeader && item.kind === 'user' && (
        <TaskHeader taskNumber={item.taskNumber} />
      )}
      <button
        id={trajectoryDOMID(item.id)}
        data-trajectory-item-id={item.id}
        type="button"
        aria-expanded={active}
        className={cn(
          'relative grid min-h-9 w-full cursor-pointer grid-cols-[2.75rem_7.25rem_minmax(0,1fr)] items-start gap-2 border-b border-edge-soft px-2 py-1.5 text-left outline-none transition-colors max-sm:grid-cols-[2.25rem_auto_minmax(0,1fr)]',
          active ? 'bg-surface-selected' : 'hover:bg-surface-hover focus-visible:bg-surface-hover',
        )}
        onClick={onSelect}
      >
        <span className="pt-0.5 text-right font-mono text-[0.75rem] font-normal text-ink-muted">
          {index >= 0 ? String(index + 1).padStart(2, '0') : ''}
        </span>
        <TraceBadge kind={item.kind}>{trajectoryKindLabel(item.kind, t)}</TraceBadge>
        <span className="flex min-w-0 items-baseline gap-3 text-[0.84375rem] leading-5 max-sm:block">
          {item.kind === 'system' ? (
            <span className="min-w-0 truncate text-ink">{t('diagnostics.initialSystemPrompt')}</span>
          ) : item.kind === 'tool' && item.tool ? (
            <>
              <span className="shrink-0 font-mono font-normal text-ink max-sm:mr-2">{item.tool.name || '—'}</span>
              <span className="grid min-w-0 flex-1 grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-baseline gap-3 text-ink-muted max-md:grid-cols-[minmax(0,1fr)_auto] max-md:[&>*:last-child]:col-span-2 max-sm:inline">
                <span className="truncate font-mono">{toolArguments}</span>
                <span className="text-ink-faint max-sm:px-1" aria-hidden="true">→</span>
                <span className={cn('truncate', item.tool.result?.isError && 'text-danger')}>{toolResult}</span>
              </span>
            </>
          ) : item.kind === 'context' && item.attachment ? (
            <>
              <span className="shrink-0 text-ink max-sm:mr-2">{attachmentLabel(item.attachment.kind, t)}</span>
              <span className="min-w-0 truncate text-ink-muted">{singleLine(item.preview) || '—'}</span>
            </>
          ) : (
            <span className={cn('min-w-0 truncate', item.thinkingOnly ? 'text-ink-muted' : 'text-ink')}>
              {item.thinkingOnly && <span className="text-ink-faint">{t('diagnostics.trace.thinking')} · </span>}
              {singleLine(item.preview) || '—'}
            </span>
          )}
        </span>
      </button>
    </>
  )
}

function TaskHeader({ taskNumber, overlay = false }: { taskNumber: number; overlay?: boolean }) {
  const { t } = useI18n()
  return (
    <div className={cn(
      'z-10 flex h-7 items-center gap-3 border-b border-edge-soft bg-canvas-raised/95 px-3 backdrop-blur-sm',
      overlay ? 'pointer-events-none absolute inset-x-0 top-0' : 'sticky top-0',
    )}>
      <span className="font-mono text-[0.71875rem] font-medium text-info">
        {t('diagnostics.taskLabel', { count: taskNumber })}
      </span>
      <span className="h-px flex-1 bg-edge-soft" aria-hidden="true" />
    </div>
  )
}

function TraceBadge({ kind, children }: { kind: TrajectoryKind; children: ReactNode }) {
  return (
    <span className={cn(
      'w-fit shrink-0 rounded-[5px] px-2 py-0.5 text-[0.6875rem] font-medium uppercase',
      kind === 'system' && 'bg-canvas-strong text-ink-soft',
      kind === 'user' && 'bg-info-surface text-info',
      kind === 'context' && 'bg-success/10 text-success',
      kind === 'assistant' && 'bg-violet-50 text-violet-600',
      kind === 'tool' && 'bg-warning-surface text-warning',
    )}>
      {children}
    </span>
  )
}

function TrajectoryInspector({
  item,
  mode,
  onModeChange,
  onClose,
}: {
  item: TrajectoryItem
  mode: InspectorMode
  onModeChange: (mode: InspectorMode) => void
  onClose: () => void
}) {
  const { t } = useI18n()
  const request = item.request
  const tabs: Array<{ mode: InspectorMode; label: string }> = item.kind === 'system' || item.kind === 'user' || item.kind === 'context'
    ? [
        { mode: 'content', label: t('diagnostics.content') },
        { mode: 'raw', label: t('diagnostics.raw') },
      ]
    : item.kind === 'tool'
      ? [
          { mode: 'summary', label: t('diagnostics.summary') },
          { mode: 'raw', label: t('diagnostics.raw') },
        ]
      : [
          { mode: 'summary', label: t('diagnostics.summary') },
          { mode: 'content', label: t('diagnostics.content') },
          { mode: 'input', label: t('diagnostics.inputItems') },
          { mode: 'system', label: t('diagnostics.systemPrompt') },
          { mode: 'tools', label: t('diagnostics.availableTools', { count: request?.input?.tools?.length ?? 0 }) },
          { mode: 'raw', label: t('diagnostics.raw') },
        ]
  const activeMode = tabs.some((tab) => tab.mode === mode) ? mode : tabs[0]!.mode
  return (
    <aside className="flex min-h-0 flex-col overflow-hidden border-l border-edge bg-canvas-raised/30 max-md:border-l-0 max-md:border-t" aria-label={trajectoryItemTitle(item, t)}>
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-edge-soft px-3">
        <TraceBadge kind={item.kind}>{trajectoryKindLabel(item.kind, t)}</TraceBadge>
        <span className="min-w-0 flex-1 truncate text-[0.75rem] text-ink-muted">
          {request ? t('diagnostics.requestNumber', { count: request.number }) : t('diagnostics.taskLabel', { count: item.taskNumber })}
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
      <div className="code-scroll-area flex h-10 shrink-0 items-end gap-4 overflow-x-auto border-b border-edge-soft px-3" role="tablist">
        {tabs.map((tab) => (
          <InspectorTab
            key={tab.mode}
            active={activeMode === tab.mode}
            label={tab.label}
            onClick={() => onModeChange(tab.mode)}
          />
        ))}
      </div>
      <div className="code-scroll-area min-h-0 flex-1 overflow-auto">
        {activeMode === 'raw' ? (
          <RawValue value={item.raw} />
        ) : item.kind === 'system' ? (
          <SystemDetail item={item} />
        ) : item.kind === 'user' ? (
          <UserDetail item={item} />
        ) : item.kind === 'context' ? (
          <ContextDetail item={item} />
        ) : item.kind === 'tool' && request && item.tool ? (
          <ToolDetail request={request} tool={item.tool} />
        ) : request && activeMode === 'input' ? (
          <RequestInput request={request} />
        ) : request && activeMode === 'system' ? (
          <SystemPrompt request={request} />
        ) : request && activeMode === 'tools' ? (
          <AvailableTools request={request} />
        ) : request && activeMode === 'content' ? (
          <ResponseContent request={request} />
        ) : request ? (
          <ResponseSummary request={request} />
        ) : null}
      </div>
    </aside>
  )
}

function InspectorTab({ active, label, onClick }: { active: boolean; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      className={cn(
        'relative h-10 shrink-0 cursor-pointer px-0.5 text-[0.75rem] font-medium outline-none transition-colors',
        active ? 'text-info' : 'text-ink-muted hover:text-ink',
      )}
      onClick={onClick}
    >
      {label}
      {active && <span className="absolute inset-x-0 bottom-0 h-0.5 bg-info" aria-hidden="true" />}
    </button>
  )
}

function UserDetail({ item }: { item: TrajectoryItem }) {
  const { t } = useI18n()
  return (
    <div className="px-5 py-5 max-sm:px-4">
      <InspectorSection title={t('diagnostics.userMessage')} first>
        <pre className="m-0 text-[0.84375rem] leading-6 whitespace-pre-wrap break-words text-ink">{item.preview}</pre>
      </InspectorSection>
    </div>
  )
}

function SystemDetail({ item }: { item: TrajectoryItem }) {
  const { t } = useI18n()
  return (
    <div className="px-5 py-5 max-sm:px-4">
      <InspectorSection title={t('diagnostics.initialSystemPrompt')} first>
        <pre className="m-0 font-mono text-[0.78125rem] leading-6 whitespace-pre-wrap break-words text-ink-soft">
          {item.preview || '—'}
        </pre>
      </InspectorSection>
    </div>
  )
}

function ContextDetail({ item }: { item: TrajectoryItem }) {
  const { t } = useI18n()
  return (
    <div className="px-5 py-5 max-sm:px-4">
      <InspectorSection title={item.attachment ? attachmentLabel(item.attachment.kind, t) : t('diagnostics.trace.context')} first>
        <pre className="m-0 font-mono text-[0.78125rem] leading-6 whitespace-pre-wrap break-words text-ink-soft">
          {item.message ? messageText(item.message) || '—' : item.preview || '—'}
        </pre>
      </InspectorSection>
    </div>
  )
}

function ResponseSummary({ request }: { request: TraceBundleRequest }) {
  const { t } = useI18n()
  const response = responseParts(request)
  const generationMS = request.durationMs !== undefined && request.timeToFirstOutputMs !== undefined
    ? Math.max(0, request.durationMs - request.timeToFirstOutputMs)
    : undefined
  const throughput = generationMS && request.outputTokens !== undefined
    ? request.outputTokens / (generationMS / 1000)
    : undefined
  const token = (value?: number) => value === undefined
    ? '—'
    : t('diagnostics.tokenValue', { count: formatCompactNumber(value) })
  return (
    <div className="px-5 py-5 max-sm:px-4">
      <dl className="grid grid-cols-[8.5rem_minmax(0,1fr)] gap-x-4 gap-y-2 text-[0.875rem] leading-5 max-sm:grid-cols-[7rem_minmax(0,1fr)]">
        <SummaryRow label={t('diagnostics.detailSource')} value={t('diagnostics.requestNumber', { count: request.number })} />
        <SummaryRow label={t('diagnostics.detailStatus')} value={requestStatusLabel(request, t)} />
        <SummaryRow label={t('diagnostics.tokens')} value={token(request.totalTokens)} />
        <SummaryRow nested label={t('diagnostics.tokenInput')} value={request.inputUnknown ? '—' : token(request.inputTokens)} />
        <SummaryRow nested label={t('diagnostics.tokenOutput')} value={token(request.outputTokens)} />
        <SummaryRow
          nested
          label={t('diagnostics.tokenReasoning')}
          value="—"
          title={t('diagnostics.tokenReasoningUnavailable')}
        />
        {(request.cacheReadTokens ?? 0) > 0 && (
          <SummaryRow nested label={t('diagnostics.tokenCacheRead')} value={token(request.cacheReadTokens)} />
        )}
        {(request.cacheWriteTokens ?? 0) > 0 && (
          <SummaryRow nested label={t('diagnostics.tokenCacheWrite')} value={token(request.cacheWriteTokens)} />
        )}
      </dl>
      <InspectorSection title={t('diagnostics.preview')}>
        {response.thinking && <ThinkingDetail value={response.thinking} />}
        <pre className={cn(
          'm-0 text-[0.84375rem] leading-6 whitespace-pre-wrap break-words text-ink',
          response.thinking && 'mt-5',
        )}>
          {response.text || '—'}
        </pre>
      </InspectorSection>
      <InspectorSection title={t('diagnostics.requestTiming')}>
        <DefinitionList rows={[
          [t('diagnostics.timingStarted'), formatTimestamp(request.startedAt)],
          [t('diagnostics.totalDuration'), formatDuration(request.durationMs)],
          [t('diagnostics.timingTTFT'), formatDuration(request.timeToFirstOutputMs)],
          [t('diagnostics.timingGeneration'), formatDuration(generationMS)],
          [t('diagnostics.timingThroughput'), throughput === undefined ? '—' : t('diagnostics.throughputValue', { count: throughput.toFixed(1) })],
        ]} />
      </InspectorSection>
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
      <dd className="m-0 min-w-0 break-words font-normal text-ink">{value}</dd>
    </>
  )
}

function ResponseContent({ request }: { request: TraceBundleRequest }) {
  const { t } = useI18n()
  const response = responseParts(request)
  return (
    <div className="px-5 py-5 max-sm:px-4">
      {response.thinking && <ThinkingDetail value={response.thinking} />}
      <InspectorSection title={t('diagnostics.assistantMessage')} first={!response.thinking}>
        <pre className="m-0 text-[0.84375rem] leading-6 whitespace-pre-wrap break-words text-ink">
          {response.text || '—'}
        </pre>
      </InspectorSection>
    </div>
  )
}

function RequestInput({ request }: { request: TraceBundleRequest }) {
  const { t } = useI18n()
  const attachments = new Map((request.attachments ?? []).map((attachment) => [attachment.messageIndex, attachment]))
  if (!request.input) {
    return <TraceEmpty title={t('diagnostics.snapshotUnavailable')} description={t('diagnostics.snapshotUnavailableDescription')} />
  }
  return (
    <div>
      {request.input.messages.map((message, index) => {
        const attachment = attachments.get(index)
        return (
          <div key={`${message.role}:${index}`} className="border-b border-edge-soft px-4 py-3">
            <div className="flex min-w-0 items-center justify-between gap-3">
              <span className="text-[0.6875rem] font-semibold text-ink-muted">
                {inputMessageLabel(message, attachment, t)}
              </span>
              {attachment?.path && <span className="max-w-48 truncate font-mono text-[0.6875rem] text-ink-faint">{attachment.path}</span>}
            </div>
            <pre className="m-0 mt-2 font-mono text-[0.78125rem] leading-5 whitespace-pre-wrap break-words text-ink-soft">
              {messageText(message) || '—'}
            </pre>
          </div>
        )
      })}
    </div>
  )
}

function SystemPrompt({ request }: { request: TraceBundleRequest }) {
  const { t } = useI18n()
  if (!request.input?.systemPrompt) {
    return <TraceEmpty title={t('diagnostics.systemPrompt')} description={t('diagnostics.snapshotUnavailableDescription')} />
  }
  return (
    <pre className="m-0 px-5 py-5 font-mono text-[0.78125rem] leading-6 whitespace-pre-wrap break-words text-ink-soft">
      {request.input.systemPrompt}
    </pre>
  )
}

function AvailableTools({ request }: { request: TraceBundleRequest }) {
  const { t } = useI18n()
  const tools = request.input?.tools ?? []
  if (tools.length === 0) {
    return <TraceEmpty title={t('diagnostics.availableTools', { count: 0 })} description={t('diagnostics.snapshotUnavailableDescription')} />
  }
  return (
    <div>
      {tools.map((tool) => (
        <details key={tool.name} className="group border-b border-edge-soft">
          <summary className="flex min-h-10 cursor-pointer list-none items-center gap-2 px-4 py-2 outline-none hover:bg-surface-hover focus-visible:bg-surface-hover">
            <ChevronRight className="size-3.5 shrink-0 text-ink-faint transition-transform group-open:rotate-90" aria-hidden="true" />
            <span className="shrink-0 font-mono text-[0.78125rem] font-semibold text-ink-soft">{tool.name}</span>
            <span className="min-w-0 truncate text-[0.75rem] text-ink-muted">{tool.description}</span>
          </summary>
          <div className="px-10 pb-4">
            {tool.description && <p className="mb-3 text-[0.78125rem] leading-5 text-ink-muted">{tool.description}</p>}
            <pre className="m-0 font-mono text-[0.75rem] leading-5 whitespace-pre-wrap break-words text-ink-soft">
              {JSON.stringify(tool.parameters, null, 2)}
            </pre>
          </div>
        </details>
      ))}
    </div>
  )
}

function ToolDetail({ request, tool }: { request: TraceBundleRequest; tool: TraceBundleTool }) {
  const { t } = useI18n()
  const schema = request.input?.tools?.find((candidate) => candidate.name === tool.name)
  return (
    <div className="px-5 py-5 max-sm:px-4">
      <CollapsibleInspectorSection title={t('diagnostics.toolInput')} first>
        <ToolArguments value={tool.arguments ?? {}} />
      </CollapsibleInspectorSection>
      <CollapsibleInspectorSection title={t('diagnostics.toolOutput')}>
        <pre className={cn(
          'm-0 rounded-[6px] bg-canvas-sunken/75 px-3 py-2.5 font-mono text-[0.78125rem] leading-5 whitespace-pre-wrap break-words text-ink-soft',
          tool.result?.isError && 'text-danger',
        )}>
          {tool.result ? messageText(tool.result) || '—' : '—'}
        </pre>
      </CollapsibleInspectorSection>
      <CollapsibleInspectorSection title={t('diagnostics.toolTiming')}>
        <DefinitionList rows={[
          [t('diagnostics.detailStatus'), statusText(tool.status)],
          [t('diagnostics.timingStarted'), formatTimestamp(tool.startedAt)],
          [t('diagnostics.totalDuration'), formatDuration(tool.durationMs)],
          [t('diagnostics.approvalWait'), formatDuration(tool.approvalDurationMs)],
          [t('diagnostics.toolExecution'), formatDuration(tool.executionDurationMs)],
          [t('diagnostics.timingSource'), toolTimingSource(tool, t)],
        ]} />
      </CollapsibleInspectorSection>
      {schema && (
        <CollapsibleInspectorSection title={t('diagnostics.toolSchema')} defaultOpen={false}>
          <p className="mb-3 text-[0.78125rem] leading-5 text-ink-muted">{schema.description}</p>
          <pre className="m-0 font-mono text-[0.75rem] leading-5 whitespace-pre-wrap break-words text-ink-soft">
            {JSON.stringify(schema.parameters, null, 2)}
          </pre>
        </CollapsibleInspectorSection>
      )}
    </div>
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
            {typeof entry === 'string' ? entry : JSON.stringify(entry, null, 2) ?? String(entry)}
          </dd>
        </div>
      ))}
    </dl>
  )
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

function InspectorSection({ title, children, first = false }: { title: string; children: ReactNode; first?: boolean }) {
  return (
    <section className={cn(!first && 'mt-6 border-t border-edge-soft pt-5')}>
      <h3 className="mb-3 text-[0.75rem] font-semibold text-ink-muted">{title}</h3>
      {children}
    </section>
  )
}

function CollapsibleInspectorSection({
  title,
  children,
  first = false,
  defaultOpen = true,
}: {
  title: string
  children: ReactNode
  first?: boolean
  defaultOpen?: boolean
}) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <section className={cn(!first && 'mt-6 border-t border-edge-soft pt-5')}>
      <h3 className="m-0">
        <button
          type="button"
          aria-expanded={open}
          className="flex w-full cursor-pointer items-center gap-1.5 text-left text-[0.75rem] font-semibold text-ink-muted outline-none hover:text-ink focus-visible:text-info"
          onClick={() => setOpen((value) => !value)}
        >
          <ChevronRight className={cn('size-3.5 shrink-0 transition-transform', open && 'rotate-90')} aria-hidden="true" />
          {title}
        </button>
      </h3>
      {open && <div className="mt-3">{children}</div>}
    </section>
  )
}

function DefinitionList({ rows }: { rows: Array<[string, string]> }) {
  return (
    <dl className="grid grid-cols-[8rem_minmax(0,1fr)] gap-x-4 gap-y-2 text-[0.8125rem] leading-5 max-sm:grid-cols-[6.5rem_minmax(0,1fr)]">
      {rows.map(([label, value]) => (
        <div key={label} className="contents">
          <dt className="text-ink-muted">{label}</dt>
          <dd className="m-0 min-w-0 break-words font-normal text-ink">{value}</dd>
        </div>
      ))}
    </dl>
  )
}

function RawValue({ value }: { value: unknown }) {
  return (
    <pre className="m-0 px-4 py-4 font-mono text-[0.75rem] leading-5 whitespace-pre-wrap break-words text-ink-soft">
      {JSON.stringify(value, null, 2)}
    </pre>
  )
}

function ConversationOverview({
  bundle,
  onOpenRequest,
  hasEarlier,
  loadingEarlier,
  earlierError,
  onLoadEarlier,
}: {
  bundle: TraceBundle
  onOpenRequest: (requestID: string) => void
  hasEarlier: boolean
  loadingEarlier: boolean
  earlierError: boolean
  onLoadEarlier: () => Promise<boolean>
}) {
  const { t } = useI18n()
  const tasks = bundle.tasks
  const requests = tasks.flatMap((task) => task.requests)
  const totalDuration = tasks.reduce((total, task) => total + (task.durationMs ?? 0), 0)
  const durations = [
    [t('diagnostics.modelTime'), requests.reduce((total, request) => total + (request.durationMs ?? 0), 0), 'bg-info'],
    [t('diagnostics.toolTime'), tasks.reduce((total, task) => total + (task.toolDurationMs ?? 0), 0), 'bg-warning'],
    [t('diagnostics.approvalWait'), tasks.reduce((total, task) => total + (task.approvalDurationMs ?? 0), 0), 'bg-warning/60'],
    [t('diagnostics.checkpoint'), tasks.reduce((total, task) => total + (task.checkpointDurationMs ?? 0), 0), 'bg-success'],
  ] as const
  return (
    <div className="code-scroll-area min-h-0 flex-1 overflow-y-auto py-6">
      {hasEarlier && (
        <div className="mb-5 max-w-[58rem]">
          <LoadEarlierControl
            loading={loadingEarlier}
            error={earlierError}
            onLoad={() => { void onLoadEarlier() }}
          />
        </div>
      )}
      <section className="max-w-[58rem]">
        <h3 className="text-[0.875rem] font-semibold text-ink-soft">{t('diagnostics.durationBreakdown')}</h3>
        <div className="mt-4 space-y-3">
          {durations.filter(([, value]) => value > 0).map(([label, value, tone]) => (
            <DurationBar key={label} label={label} value={value} total={totalDuration} tone={tone} />
          ))}
        </div>
      </section>
      <section className="mt-8 max-w-[58rem] border-t border-edge-soft pt-6">
        <div className="flex items-center justify-between gap-4">
          <h3 className="text-[0.875rem] font-semibold text-ink-soft">{t('diagnostics.modelRequests')}</h3>
          <span className="text-[0.75rem] text-ink-muted">{t('diagnostics.requestCount', { count: requests.length })}</span>
        </div>
        <div className="mt-3 border-y border-edge">
          {requests.map((request) => (
            <button
              key={request.id}
              type="button"
              className="grid min-h-10 w-full cursor-pointer grid-cols-[7rem_minmax(8rem,1fr)_6rem_6rem_6rem] items-center gap-3 border-b border-edge-soft px-3 text-left outline-none last:border-b-0 hover:bg-surface-hover focus-visible:bg-surface-hover max-md:grid-cols-[6rem_minmax(0,1fr)_5rem] max-md:[&>*:nth-child(4)]:hidden max-md:[&>*:nth-child(5)]:hidden"
              onClick={() => onOpenRequest(request.id)}
            >
              <span className="text-[0.75rem] font-semibold text-info">{t('diagnostics.requestNumber', { count: request.number })}</span>
              <span className="truncate font-mono text-[0.78125rem] text-ink-soft">{request.model || request.provider || '—'}</span>
              <span className="text-[0.75rem] text-ink-muted">{formatDuration(request.durationMs)}</span>
              <span className="text-[0.75rem] text-ink-muted">{t('diagnostics.firstTokenInline', { duration: formatDuration(request.timeToFirstOutputMs) })}</span>
              <span className="text-[0.75rem] text-ink-muted">{formatCompactNumber(request.totalTokens)}</span>
            </button>
          ))}
        </div>
      </section>
      <RawEvents events={tasks.flatMap((task) => task.rawEvents)} omitted={tasks.reduce((total, task) => total + (task.omittedEvents ?? 0), 0)} />
    </div>
  )
}

function LoadEarlierControl({
  loading,
  error,
  onLoad,
}: {
  loading: boolean
  error: boolean
  onLoad: () => void
}) {
  const { t } = useI18n()
  return (
    <div className="flex h-8 shrink-0 items-center justify-center bg-canvas">
      <button
        type="button"
        className="flex h-7 cursor-pointer items-center gap-1.5 rounded-[6px] px-2.5 text-[0.75rem] font-normal text-ink-muted outline-none hover:bg-surface-hover hover:text-ink focus-visible:bg-surface-hover focus-visible:text-ink disabled:cursor-wait disabled:opacity-60"
        disabled={loading}
        onClick={onLoad}
      >
        {loading && <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />}
        {t(error ? 'diagnostics.retryEarlier' : 'diagnostics.loadEarlier')}
      </button>
    </div>
  )
}

function DurationBar({ label, value, total, tone }: { label: string; value: number; total: number; tone: string }) {
  const width = total > 0 ? Math.max(1, Math.min(100, (value / total) * 100)) : 0
  return (
    <div className="grid grid-cols-[8rem_minmax(0,1fr)_5rem] items-center gap-3 text-[0.75rem]">
      <span className="text-ink-muted">{label}</span>
      <span className="h-1.5 overflow-hidden rounded-[2px] bg-canvas-sunken">
        <span className={cn('block h-full', tone)} style={{ width: `${width}%` }} />
      </span>
      <span className="text-right font-mono text-ink-soft">{formatDuration(value)}</span>
    </div>
  )
}

function RawEvents({ events, omitted }: { events: DiagnosticEvent[]; omitted?: number }) {
  const { t } = useI18n()
  return (
    <details className="group mt-8 max-w-[58rem] border-t border-edge-soft pt-5">
      <summary className="flex cursor-pointer list-none items-center gap-2 text-[0.8125rem] font-medium text-ink-muted outline-none hover:text-ink focus-visible:text-ink">
        <ChevronRight className="size-3.5 transition-transform group-open:rotate-90" aria-hidden="true" />
        {t('diagnostics.rawEvents')}
        <span className="text-[0.75rem] text-ink-faint">{t('diagnostics.eventCount', { count: events.length })}</span>
      </summary>
      {Boolean(omitted) && <p className="mt-2 text-[0.75rem] text-warning">{t('diagnostics.omittedEvents', { count: omitted ?? 0 })}</p>}
      <RawValue value={events} />
    </details>
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

function buildTrajectoryItems(tasks: TraceBundleTask[], t: Translate): TrajectoryItem[] {
  const items: TrajectoryItem[] = []
  const seenContext = new Set<string>()
  for (let taskIndex = 0; taskIndex < tasks.length; taskIndex++) {
    const task = tasks[taskIndex]!
    const request = task.requests.find((candidate) => candidate.input?.systemPrompt?.trim())
    const systemPrompt = request?.input?.systemPrompt?.trim()
    if (!request || !systemPrompt) continue
    items.push({
      id: `system:${request.id}`,
      kind: 'system',
      task,
      taskNumber: taskIndex + 1,
      request,
      preview: systemPrompt,
      raw: { systemPrompt, providerRequestId: request.id },
    })
    break
  }
  tasks.forEach((task, taskIndex) => {
    const firstRequest = task.requests[0]
    items.push({
      id: `task:${task.id}:user`,
      kind: 'user',
      task,
      taskNumber: taskIndex + 1,
      request: firstRequest,
      preview: task.prompt || t('diagnostics.taskPromptUnavailable'),
      raw: { taskId: task.id, prompt: task.prompt },
    })
    task.requests.forEach((request) => {
      request.attachments?.forEach((attachment) => {
        const message = request.input?.messages[attachment.messageIndex]
        if (!message || !shouldShowContextAttachment(attachment, message, seenContext)) return
        items.push({
          id: `request:${request.id}:context:${attachment.id || `${attachment.kind}:${attachment.messageIndex}`}`,
          kind: 'context',
          task,
          taskNumber: taskIndex + 1,
          request,
          attachment,
          message,
          preview: messageText(message),
          raw: { attachment, message },
        })
      })
      const response = responseParts(request)
      items.push({
        id: `request:${request.id}:response`,
        kind: 'assistant',
        task,
        taskNumber: taskIndex + 1,
        request,
        preview: response.text || response.thinking || t('diagnostics.noResponse'),
        thinking: response.thinking || undefined,
        thinkingOnly: !response.text && Boolean(response.thinking),
        raw: request.output ?? request,
      })
      request.tools.forEach((tool) => {
        items.push({
          id: `request:${request.id}:tool:${tool.id}`,
          kind: 'tool',
          task,
          taskNumber: taskIndex + 1,
          request,
          tool,
          preview: toolPreview(tool),
          raw: tool,
        })
      })
    })
  })
  return items
}

function trajectorySequenceIndex(items: TrajectoryItem[], item: TrajectoryItem): number {
  if (item.kind === 'system') return -1
  const itemIndex = items.indexOf(item)
  return items.slice(0, itemIndex + 1).filter((candidate) => candidate.kind !== 'system').length - 1
}

function shouldShowContextAttachment(
  attachment: RequestSnapshotAttachment,
  message: RequestSnapshotMessage,
  seenContext: Set<string>,
): boolean {
  const identity = attachment.kind === 'base' || attachment.kind === 'skill_listing'
    ? attachment.kind
    : attachment.id || [
        attachment.kind,
        attachment.revision ?? '',
        attachment.path ?? '',
        messageText(message),
      ].join(':')
  if (seenContext.has(identity)) return false
  seenContext.add(identity)
  return true
}

function responseParts(request: TraceBundleRequest): { thinking: string; text: string } {
  const content = request.output?.message.content ?? []
  return {
    thinking: content.filter((item) => item.type === 'thinking').map((item) => item.thinking ?? '').filter(Boolean).join('\n\n'),
    text: content.filter((item) => item.type !== 'thinking' && item.type !== 'toolCall').map(contentText).filter(Boolean).join('\n\n'),
  }
}

function toolPreview(tool: TraceBundleTool): string {
  const input = JSON.stringify(tool.arguments ?? {})
  const result = tool.result ? messageText(tool.result) : statusText(tool.status)
  return `${tool.name || '—'}  ${singleLine(input)}  →  ${singleLine(result)}`
}

function contentText(content: RequestSnapshotContent): string {
  if (content.type === 'text') return content.text ?? ''
  if (content.type === 'thinking') return content.thinking ?? ''
  if (content.type === 'toolCall') return `${content.toolName ?? 'tool'} ${JSON.stringify(content.arguments ?? {})}`
  if (content.type === 'image') return content.image?.mimeType ?? 'image'
  return content.type
}

function messageText(message: RequestSnapshotMessage): string {
  return message.content.map(contentText).filter(Boolean).join('\n\n')
}

function inputMessageLabel(
  message: RequestSnapshotMessage,
  attachment: RequestSnapshotAttachment | undefined,
  t: Translate,
): string {
  if (attachment) return attachmentLabel(attachment.kind, t)
  if (message.role === 'user') return t('diagnostics.userMessage')
  if (message.role === 'assistant') return t('diagnostics.assistantMessage')
  if (message.role === 'toolResult') return message.toolName || t('diagnostics.toolResult')
  return message.role
}

function attachmentLabel(kind: string, t: Translate): string {
  const labels: Record<string, string> = {
    base: t('diagnostics.context.base'),
    skill_listing: t('diagnostics.context.skillListing'),
    skills_update: t('diagnostics.context.skillsUpdate'),
    activated_skill: t('diagnostics.context.activatedSkill'),
    context_update: t('diagnostics.context.runtime'),
    task_status: t('diagnostics.context.taskStatus'),
  }
  return labels[kind] ?? kind
}

function timelineTone(kind: TrajectoryKind): string {
  if (kind === 'system') return 'bg-ink-muted'
  if (kind === 'user') return 'bg-info'
  if (kind === 'context') return 'bg-success'
  if (kind === 'assistant') return 'bg-violet-400'
  return 'bg-warning'
}

function trajectoryKindLabel(kind: TrajectoryKind, t: Translate): string {
  if (kind === 'system') return t('diagnostics.trace.system')
  if (kind === 'user') return t('diagnostics.trace.user')
  if (kind === 'context') return t('diagnostics.trace.context')
  if (kind === 'assistant') return t('diagnostics.trace.assistant')
  return t('diagnostics.trace.tool')
}

function trajectoryItemTitle(item: TrajectoryItem, t: Translate): string {
  const label = trajectoryKindLabel(item.kind, t)
  if (item.kind === 'system') return `${label} · ${t('diagnostics.initialSystemPrompt')}`
  if (item.kind === 'context' && item.attachment) return `${label} · ${attachmentLabel(item.attachment.kind, t)}`
  return item.request ? `${label} · ${t('diagnostics.requestNumber', { count: item.request.number })}` : label
}

function trajectorySearchText(item: TrajectoryItem): string {
  return [
    item.preview,
    item.thinking,
    item.attachment?.kind,
    item.tool?.name,
    item.request?.model,
    item.request?.provider,
  ].filter(Boolean).join(' ')
}

function trajectoryDOMID(itemID: string): string {
  return `trajectory-${itemID}`
}

function singleLine(value: string): string {
  return value.replace(/\s+/g, ' ').trim()
}

function statusText(value?: string): string {
  return value ? value.replaceAll('_', ' ') : '—'
}

function requestStatusLabel(request: TraceBundleRequest, t: Translate): string {
  const status = request.status || (request.lifecycle === 'in-progress' ? 'running' : 'completed')
  switch (status) {
    case 'running': return t('diagnostics.status.running')
    case 'completed': return t('diagnostics.status.completed')
    case 'success': return t('diagnostics.status.success')
    case 'failed': return t('diagnostics.status.failed')
    case 'cancelled': return t('diagnostics.status.cancelled')
    case 'waiting': return t('diagnostics.status.waiting')
    case 'allowed': return t('diagnostics.status.allowed')
    case 'denied': return t('diagnostics.status.denied')
    case 'discarded': return t('diagnostics.status.discarded')
    default: return statusText(status)
  }
}

function toolTimingSource(tool: TraceBundleTool, t: Translate): string {
  if (tool.lifecycle === 'missing-start') return t('diagnostics.timingSource.reconstructed')
  if ((tool.rawEvents?.length ?? 0) === 0) return t('diagnostics.timingSource.snapshot')
  return t('diagnostics.timingSource.events')
}

function formatDuration(value?: number): string {
  if (value === undefined || !Number.isFinite(value) || value < 0) return '—'
  if (value < 1000) return `${Math.round(value)} ms`
  if (value < 60_000) return `${(value / 1000).toFixed(value < 10_000 ? 2 : 1)} s`
  const minutes = Math.floor(value / 60_000)
  const seconds = Math.round((value % 60_000) / 1000)
  return `${minutes}m ${seconds}s`
}

function formatCompactNumber(value?: number): string {
  if (value === undefined || !Number.isFinite(value)) return '—'
  return new Intl.NumberFormat(undefined, { notation: value >= 10_000 ? 'compact' : 'standard', maximumFractionDigits: 1 }).format(value)
}

function formatTimestamp(value: string): string {
  const date = new Date(value)
  return Number.isFinite(date.getTime()) ? date.toLocaleString() : value
}

type Translate = ReturnType<typeof useI18n>['t']
