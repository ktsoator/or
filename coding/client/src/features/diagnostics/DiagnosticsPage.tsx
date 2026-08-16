import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
} from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { Marked, type Token } from 'marked'
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
import { usePointerResize } from '@/shared/hooks/usePointerResize'
import { Markdown } from '@/shared/ui/Markdown'
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
type InspectorMode = 'summary' | 'content' | 'input' | 'raw' | 'tools'

const TRAJECTORY_VIRTUALIZATION_THRESHOLD = 100
const TRAJECTORY_OVERSCAN = 12
const TRAJECTORY_ROW_HEIGHT_REM = 2.25
const TRAJECTORY_TURN_HEADER_HEIGHT_REM = 2
const TRAJECTORY_STEP_HEADER_HEIGHT_REM = 2
const TIMELINE_COLUMN_WIDTH = 50
const TIMELINE_EDGE_PADDING = 8
const DEFAULT_INSPECTOR_RATIO = 0.36
const MIN_INSPECTOR_WIDTH = 320
const MIN_LEDGER_WIDTH = 360
const INSPECTOR_KEYBOARD_STEP = 16
const trajectoryMarkdownParser = new Marked()

export type DiagnosticsSessionState = {
  view?: TraceView
  selectedItemID?: string
  inspectorOpen?: boolean
  inspectorMode?: InspectorMode
  inspectorWidth?: number
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
    version: 2,
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
  request?: TraceBundleRequest
  tool?: TraceBundleTool
  attachment?: RequestSnapshotAttachment
  message?: RequestSnapshotMessage
  preview: string
  thinking?: string
  thinkingOnly?: boolean
  hierarchy?: TrajectoryHierarchy
  raw: unknown
}

type TrajectoryHierarchy = {
  turnKey: string
  turnID?: string
  turnNumber: number
  turnStepCount: number
  stepKey: string
  stepID?: string
  stepNumber: number
}

type TrajectoryHierarchyIndex = {
  tasks: Map<string, TrajectoryHierarchy>
  requests: Map<string, TrajectoryHierarchy>
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
  const rememberState = useCallback((patch: DiagnosticsStatePatch) => {
    rememberedStateRef.current = { ...rememberedStateRef.current, ...patch }
    onStateChange?.(patch)
  }, [onStateChange])
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
  const [inspectorWidth, setInspectorWidth] = useState(rememberedState.inspectorWidth)
  const [splitLayoutWidth, setSplitLayoutWidth] = useState(0)
  const [scrollTarget, setScrollTarget] = useState<TrajectoryScrollTarget>()
  const splitLayoutRef = useRef<HTMLDivElement>(null)
  const scrollRevisionRef = useRef(0)
  const selectedItem = items.find((item) => item.id === selectedItemID) ?? initialItem

  useLayoutEffect(() => {
    if (view !== 'trajectory') return
    const layout = splitLayoutRef.current
    if (!layout) return
    const updateWidth = () => setSplitLayoutWidth(layout.getBoundingClientRect().width)
    updateWidth()
    const observer = new ResizeObserver(updateWidth)
    observer.observe(layout)
    return () => observer.disconnect()
  }, [view])

  const currentLayoutWidth = useCallback(
    () => splitLayoutRef.current?.getBoundingClientRect().width ?? splitLayoutWidth,
    [splitLayoutWidth],
  )
  const inspectorBounds = diagnosticsInspectorWidthBounds(splitLayoutWidth)
  const effectiveInspectorWidth = clampDiagnosticsInspectorWidth(
    inspectorWidth ?? defaultDiagnosticsInspectorWidth(splitLayoutWidth),
    splitLayoutWidth,
  )
  const updateInspectorWidth = useCallback((nextWidth: number) => {
    setInspectorWidth(nextWidth)
    rememberState({ inspectorWidth: nextWidth })
  }, [rememberState])
  const beginInspectorResize = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    const layoutWidth = currentLayoutWidth()
    return {
      startX: event.clientX,
      startWidth: clampDiagnosticsInspectorWidth(
        inspectorWidth ?? defaultDiagnosticsInspectorWidth(layoutWidth),
        layoutWidth,
      ),
      layoutWidth,
    }
  }, [currentLayoutWidth, inspectorWidth])
  const moveInspectorResize = useCallback((
    resize: { startX: number; startWidth: number; layoutWidth: number },
    clientX: number,
  ) => {
    updateInspectorWidth(clampDiagnosticsInspectorWidth(
      resize.startWidth + resize.startX - clientX,
      resize.layoutWidth,
    ))
  }, [updateInspectorWidth])
  const {
    resizing: inspectorResizing,
    startResize: startInspectorResize,
    resize: resizeInspector,
    stopResize: stopInspectorResize,
  } = usePointerResize({ start: beginInspectorResize, move: moveInspectorResize })
  const resizeInspectorWithKeyboard = useCallback((event: ReactKeyboardEvent<HTMLDivElement>) => {
    const layoutWidth = currentLayoutWidth()
    const bounds = diagnosticsInspectorWidthBounds(layoutWidth)
    const currentWidth = clampDiagnosticsInspectorWidth(
      inspectorWidth ?? defaultDiagnosticsInspectorWidth(layoutWidth),
      layoutWidth,
    )
    let nextWidth: number | undefined
    switch (event.key) {
      case 'ArrowLeft':
        nextWidth = currentWidth + INSPECTOR_KEYBOARD_STEP
        break
      case 'ArrowRight':
        nextWidth = currentWidth - INSPECTOR_KEYBOARD_STEP
        break
      case 'Home':
        nextWidth = bounds.minimum
        break
      case 'End':
        nextWidth = bounds.maximum
        break
      default:
        return
    }
    event.preventDefault()
    updateInspectorWidth(clampDiagnosticsInspectorWidth(nextWidth, layoutWidth))
  }, [currentLayoutWidth, inspectorWidth, updateInspectorWidth])

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
            ref={splitLayoutRef}
            data-testid="diagnostics-split-layout"
            className={cn(
              'grid min-h-0 flex-1 overflow-hidden',
              inspectorOpen && selectedItem
                ? 'grid-cols-1 grid-rows-[minmax(12rem,1fr)_minmax(12rem,0.9fr)] md:grid-cols-[minmax(0,1fr)_var(--diagnostics-inspector-width)] md:grid-rows-[minmax(0,1fr)]'
                : 'grid-cols-1',
            )}
            style={inspectorOpen && selectedItem
              ? { '--diagnostics-inspector-width': `${effectiveInspectorWidth}px` } as CSSProperties
              : undefined}
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
              <div className="relative min-h-0 min-w-0">
                <div
                  className="group absolute inset-y-0 -left-1.5 z-20 hidden w-3 touch-none cursor-col-resize outline-none md:block"
                  data-testid="diagnostics-inspector-resize-handle"
                  role="separator"
                  aria-label={t('diagnostics.resizeInspector')}
                  aria-orientation="vertical"
                  aria-valuemin={Math.round(inspectorBounds.minimum)}
                  aria-valuemax={Math.round(inspectorBounds.maximum)}
                  aria-valuenow={Math.round(effectiveInspectorWidth)}
                  tabIndex={0}
                  onPointerDown={startInspectorResize}
                  onPointerMove={resizeInspector}
                  onPointerUp={stopInspectorResize}
                  onPointerCancel={stopInspectorResize}
                  onLostPointerCapture={stopInspectorResize}
                  onKeyDown={resizeInspectorWithKeyboard}
                >
                  <span
                    className={cn(
                      'absolute inset-y-0 left-1/2 w-px -translate-x-1/2 bg-edge transition-colors group-hover:bg-ink-muted/70 group-focus-visible:bg-ink-muted/80',
                      inspectorResizing && 'bg-ink-muted/80',
                    )}
                    aria-hidden="true"
                  />
                </div>
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
              </div>
            )}
          </div>
        </div>
      )}
    </section>
  )
}

function diagnosticsInspectorWidthBounds(layoutWidth: number): { minimum: number; maximum: number } {
  if (!Number.isFinite(layoutWidth) || layoutWidth <= 0) {
    return { minimum: MIN_INSPECTOR_WIDTH, maximum: MIN_INSPECTOR_WIDTH * 2 }
  }
  const minimum = Math.min(MIN_INSPECTOR_WIDTH, layoutWidth)
  return {
    minimum,
    maximum: Math.max(minimum, layoutWidth - MIN_LEDGER_WIDTH),
  }
}

function defaultDiagnosticsInspectorWidth(layoutWidth: number): number {
  const basis = layoutWidth > 0 ? layoutWidth * DEFAULT_INSPECTOR_RATIO : 420
  return clampDiagnosticsInspectorWidth(basis, layoutWidth)
}

function clampDiagnosticsInspectorWidth(width: number, layoutWidth: number): number {
  const bounds = diagnosticsInspectorWidthBounds(layoutWidth)
  return Math.min(bounds.maximum, Math.max(bounds.minimum, width))
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
    paddingEnd: TIMELINE_EDGE_PADDING,
    scrollPaddingEnd: TIMELINE_EDGE_PADDING,
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
        <div
          className="grid grid-cols-[3.5rem_minmax(0,1fr)] gap-x-2"
          style={{
            minWidth: `${contentWidth + 64}px`,
            paddingRight: `${TIMELINE_EDGE_PADDING}px`,
          }}
        >
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
  const modelTiming = timelineModelTiming(item)
  const ttftPercentage = modelTiming
    ? Math.round(modelTiming.ttftFraction * 100_000) / 1000
    : undefined
  const itemTitle = trajectoryItemTitle(item, t)
  const timingTitle = modelTiming
    ? t('diagnostics.timelineModelTiming', {
        ttft: formatDuration(modelTiming.ttftMs),
        generation: formatDuration(modelTiming.generationMs),
        total: formatDuration(modelTiming.totalMs),
      })
    : undefined
  return (
    <button
      type="button"
      title={timingTitle ? `${itemTitle}\n${timingTitle}` : itemTitle}
      aria-label={itemTitle}
      aria-description={timingTitle}
      aria-pressed={active}
      data-timeline-index={index}
      data-timeline-item-id={item.id}
      data-model-timing={modelTiming ? 'true' : undefined}
      className={cn(
        'flex h-2.5 w-full cursor-pointer overflow-hidden rounded-[2px] outline-none ring-offset-1 ring-offset-canvas transition-[height,opacity,box-shadow] hover:h-3.5 focus-visible:ring-2 focus-visible:ring-info',
        modelTiming ? 'bg-transparent' : timelineTone(item.kind),
        active ? 'h-3.5 ring-2 ring-info' : 'opacity-90 hover:opacity-100',
      )}
      style={style}
      onClick={onSelect}
    >
      {modelTiming && (
        <>
          <span
            className="h-full bg-violet-300"
            data-timeline-segment="ttft"
            style={{ width: `${ttftPercentage}%` }}
            aria-hidden="true"
          />
          <span
            className="h-full bg-violet-500"
            data-timeline-segment="generation"
            style={{ width: `${100 - (ttftPercentage ?? 0)}%` }}
            aria-hidden="true"
          />
        </>
      )}
    </button>
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
  const rootFontSize = useMemo(() => {
    if (typeof window === 'undefined') return 16
    return Number.parseFloat(window.getComputedStyle(document.documentElement).fontSize) || 16
  }, [])
  const rowEstimate = rootFontSize * TRAJECTORY_ROW_HEIGHT_REM
  const turnHeaderHeight = rootFontSize * TRAJECTORY_TURN_HEADER_HEIGHT_REM
  const stepHeaderHeight = rootFontSize * TRAJECTORY_STEP_HEADER_HEIGHT_REM
  const rowVirtualizer = useVirtualizer({
    count: filtered.length,
    enabled: virtualized,
    getScrollElement: () => scrollRef.current,
    getItemKey: (index) => filtered[index]?.id ?? index,
    estimateSize: (index) => rowEstimate + trajectoryHeaderHeight(
      trajectoryHeaderFlags(filtered, index),
      turnHeaderHeight,
      stepHeaderHeight,
    ),
    overscan: TRAJECTORY_OVERSCAN,
    useFlushSync: false,
  })
  const virtualRows = rowVirtualizer.getVirtualItems()
  const visibleVirtualRow = virtualRows.find((row) =>
    row.end > (rowVirtualizer.scrollOffset ?? 0))
  const visibleItem = visibleVirtualRow ? filtered[visibleVirtualRow.index] : undefined
  const currentTurn = visibleItem?.kind === 'system' ? undefined : visibleItem?.hierarchy
  const overlayTurn = visibleVirtualRow && trajectoryHeaderFlags(filtered, visibleVirtualRow.index).turn
    ? undefined
    : currentTurn

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
      const rowInset = trajectoryHeaderHeight(
        trajectoryHeaderFlags(filtered, index),
        turnHeaderHeight,
        stepHeaderHeight,
      )
      rowVirtualizer.scrollToOffset(itemOffset + rowInset - desiredOffset)
      window.requestAnimationFrame(() => {
        window.requestAnimationFrame(adjustToMeasuredRow)
      })
    })
  }, [filtered, rowVirtualizer, stepHeaderHeight, turnHeaderHeight, virtualized])

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
      (virtualized && currentTurn ? turnHeaderHeight : 0)
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
                const headers = trajectoryHeaderFlags(filtered, virtualRow.index)
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
                      headers={headers}
                      onSelect={() => onSelect(item)}
                    />
                  </div>
                )
              })}
            </div>
          ) : filtered.map((item, index) => (
            <TrajectoryRow
              key={item.id}
              item={item}
              index={trajectorySequenceIndex(items, item)}
              active={item.id === selectedItemID}
              headers={trajectoryHeaderFlags(filtered, index)}
              stickyTurn
              onSelect={() => onSelect(item)}
            />
          ))}
        </div>
        {virtualized && overlayTurn && (
          <TurnHeader hierarchy={overlayTurn} overlay />
        )}
      </div>
    </section>
  )
}

function TrajectoryRow({
  item,
  index,
  active,
  headers,
  stickyTurn = false,
  onSelect,
}: {
  item: TrajectoryItem
  index: number
  active: boolean
  headers: TrajectoryHeaderFlags
  stickyTurn?: boolean
  onSelect: () => void
}) {
  const { t } = useI18n()
  const toolArguments = item.tool ? singleLine(JSON.stringify(item.tool.arguments ?? {})) : ''
  const toolResult = item.tool?.result ? singleLine(messageText(item.tool.result)) : statusText(item.tool?.status)
  const rowPreview = item.kind === 'assistant'
    ? markdownPlainText(item.preview)
    : singleLine(item.preview)
  return (
    <>
      {headers.turn && item.hierarchy && <TurnHeader hierarchy={item.hierarchy} sticky={stickyTurn} />}
      {headers.step && item.hierarchy && item.request && (
        <StepHeader hierarchy={item.hierarchy} request={item.request} />
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
              {rowPreview || '—'}
            </span>
          )}
        </span>
      </button>
    </>
  )
}

type TrajectoryHeaderFlags = {
  turn: boolean
  step: boolean
}

function trajectoryHeaderFlags(items: TrajectoryItem[], index: number): TrajectoryHeaderFlags {
  const item = items[index]
  if (!item || item.kind === 'system') return { turn: false, step: false }
  const previous = index > 0 && items[index - 1]?.kind !== 'system' ? items[index - 1] : undefined
  if (!item.hierarchy) return { turn: false, step: false }
  const turn = !previous?.hierarchy || previous.hierarchy.turnKey !== item.hierarchy.turnKey
  const step = Boolean(item.request) && (
    turn || !previous?.hierarchy || previous.hierarchy.stepKey !== item.hierarchy.stepKey
  )
  return { turn, step }
}

function trajectoryHeaderHeight(
  headers: TrajectoryHeaderFlags,
  turnHeight: number,
  stepHeight: number,
): number {
  return (headers.turn ? turnHeight : 0) +
    (headers.step ? stepHeight : 0)
}

function TurnHeader({
  hierarchy,
  sticky = false,
  overlay = false,
}: {
  hierarchy: TrajectoryHierarchy
  sticky?: boolean
  overlay?: boolean
}) {
  const { t } = useI18n()
  return (
    <div
      className={cn(
        'z-10 flex h-8 items-center gap-3 border-b border-edge-soft bg-canvas-raised/95 px-3 backdrop-blur-sm',
        sticky && 'sticky top-0',
        overlay && 'pointer-events-none absolute inset-x-0 top-0',
      )}
      data-testid="trajectory-turn-header"
      data-trajectory-turn-id={hierarchy.turnID}
      title={hierarchy.turnID}
    >
      <span className="font-mono text-[0.75rem] font-semibold text-ink">
        {t('diagnostics.turnLabel', { count: hierarchy.turnNumber })}
      </span>
      <span className="text-[0.71875rem] text-ink-muted">
        {hierarchy.turnStepCount === 1
          ? t('diagnostics.stepCountSingle')
          : t('diagnostics.stepCount', { count: hierarchy.turnStepCount })}
      </span>
      <span className="h-px flex-1 bg-edge-soft" aria-hidden="true" />
    </div>
  )
}

function StepHeader({
  hierarchy,
  request,
}: {
  hierarchy: TrajectoryHierarchy
  request: TraceBundleRequest
}) {
  const { t } = useI18n()
  const model = [request.provider, request.model].filter(Boolean).join(' / ')
  const needsAttention = requestNeedsAttention(request)
  return (
    <div
      className="grid h-8 grid-cols-[2.75rem_7.25rem_minmax(0,1fr)] items-center gap-2 border-b border-edge-soft bg-canvas-sunken/45 px-2 max-sm:grid-cols-[2.25rem_auto_minmax(0,1fr)]"
      data-testid="trajectory-step-header"
      data-trajectory-step-id={hierarchy.stepID}
      data-trajectory-request-id={request.id}
      title={hierarchy.stepID}
    >
      <span className="flex justify-end" aria-hidden="true">
        <span className="h-px w-3 bg-edge" />
      </span>
      <span className="font-mono text-[0.71875rem] font-medium text-ink-soft">
        {t('diagnostics.stepLabel', { count: hierarchy.stepNumber })}
      </span>
      <span className="flex min-w-0 items-center gap-3 text-[0.75rem]">
        <span className="shrink-0 text-ink-soft">
          {t('diagnostics.requestNumber', { count: request.number })}
        </span>
        {model && <span className="min-w-0 truncate text-ink-muted max-sm:hidden">{model}</span>}
        <span className="flex-1" aria-hidden="true" />
        <span className="shrink-0 font-mono text-ink-muted">{formatDuration(request.durationMs)}</span>
        <span className={cn('shrink-0 text-ink-muted', needsAttention && 'text-danger')}>
          {requestStatusLabel(request, t)}
        </span>
      </span>
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
  const toolCount = request?.input?.tools?.length ?? 0
  const tabs: Array<{ mode: InspectorMode; label: string }> = item.kind === 'system'
    ? [
        { mode: 'content', label: t('diagnostics.content') },
        { mode: 'raw', label: t('diagnostics.raw') },
        ...(toolCount > 0
          ? [{ mode: 'tools' as const, label: t('diagnostics.availableTools', { count: toolCount }) }]
          : []),
      ]
    : item.kind === 'user' || item.kind === 'context'
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
          { mode: 'raw', label: t('diagnostics.raw') },
        ]
  const activeMode = tabs.some((tab) => tab.mode === mode) ? mode : tabs[0]!.mode
  return (
    <aside className="flex h-full min-h-0 flex-col overflow-hidden border-l border-edge bg-canvas-raised/30 max-md:border-l-0 max-md:border-t" aria-label={trajectoryItemTitle(item, t)}>
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-edge-soft px-3">
        <TraceBadge kind={item.kind}>{trajectoryKindLabel(item.kind, t)}</TraceBadge>
        <span className="min-w-0 flex-1 truncate text-[0.75rem] text-ink-muted">
          {trajectoryHierarchyLabel(item, t)}
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
        {activeMode === 'tools' && request ? (
          <AvailableTools request={request} />
        ) : activeMode === 'raw' ? (
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
        ) : request && activeMode === 'content' ? (
          <ResponseContent request={request} />
        ) : request ? (
          <ResponseSummary request={request} runID={item.task.id} />
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
  const content = item.message ? messageText(item.message) : item.preview
  const skillMarkdown = item.attachment?.kind === 'activated_skill'
    ? activatedSkillMarkdown(content)
    : undefined
  return (
    <div className="px-5 py-5 max-sm:px-4">
      <InspectorSection title={item.attachment ? attachmentLabel(item.attachment.kind, t) : t('diagnostics.trace.context')} first>
        {skillMarkdown ? (
          <Markdown source={skillMarkdown} />
        ) : (
          <pre className="m-0 font-mono text-[0.78125rem] leading-6 whitespace-pre-wrap break-words text-ink-soft">
            {content || '—'}
          </pre>
        )}
      </InspectorSection>
    </div>
  )
}

function ResponseSummary({ request, runID }: { request: TraceBundleRequest; runID: string }) {
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
        <SummaryRow label={t('diagnostics.detailRun')} value={runID} />
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
        {response.text ? (
          <Markdown source={response.text} className={cn(response.thinking && 'mt-5')} />
        ) : request.tools.length === 0 ? (
          <p className={cn('m-0 text-[0.84375rem] leading-6 text-ink', response.thinking && 'mt-5')}>—</p>
        ) : null}
        {request.tools.length > 0 && (
          <ResponseToolCalls
            tools={request.tools}
            className={cn((response.thinking || response.text) && 'mt-5')}
          />
        )}
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

function ResponseToolCalls({ tools, className }: { tools: TraceBundleTool[]; className?: string }) {
  const { t } = useI18n()
  return (
    <div className={className} data-testid="diagnostics-response-tool-calls">
      <div className="mb-2 flex items-baseline gap-2">
        <span className="text-[0.75rem] font-medium text-ink-muted">{t('diagnostics.toolCalls')}</span>
        <span className="font-mono text-[0.6875rem] text-ink-faint">{tools.length}</span>
      </div>
      <div className="border-y border-edge-soft">
        {tools.map((tool, index) => {
          const argumentsPreview = singleLine(JSON.stringify(tool.arguments ?? {}))
          const duration = formatDuration(tool.durationMs)
          return (
            <div
              key={tool.id}
              className={cn('py-2.5', index > 0 && 'border-t border-edge-soft')}
            >
              <div className="flex min-w-0 items-center gap-2">
                <TraceBadge kind="tool">{trajectoryKindLabel('tool', t)}</TraceBadge>
                <span className="min-w-0 flex-1 truncate font-mono text-[0.78125rem] font-medium text-ink-soft">
                  {tool.name || '—'}
                </span>
                <span className="shrink-0 text-[0.71875rem] text-ink-muted">
                  {toolStatusLabel(tool, t)}{duration === '—' ? '' : ` · ${duration}`}
                </span>
              </div>
              <div
                className="mt-1.5 truncate font-mono text-[0.75rem] leading-5 text-ink-muted"
                title={argumentsPreview}
              >
                {argumentsPreview || '{}'}
              </div>
            </div>
          )
        })}
      </div>
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
        {response.text ? (
          <Markdown source={response.text} />
        ) : (
          <p className="m-0 text-[0.84375rem] leading-6 text-ink">—</p>
        )}
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
      <Markdown
        source={value}
        className="mt-2 text-[0.8125rem] leading-6 [--tw-prose-body:var(--ink-muted)] [--tw-prose-bold:var(--ink-soft)] prose-headings:text-[0.9375rem] prose-headings:leading-5 prose-headings:text-ink-soft"
      />
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
  const modelDuration = requests.reduce((total, request) => total + (request.durationMs ?? 0), 0)
  const toolExecutionDuration = tasks.reduce((total, task) => total + taskToolExecutionDuration(task), 0)
  const approvalDuration = tasks.reduce((total, task) => total + taskApprovalDuration(task), 0)
  const checkpointDuration = tasks.reduce((total, task) => total + taskCheckpointDuration(task), 0)
  const classifiedDuration = modelDuration + toolExecutionDuration + approvalDuration + checkpointDuration
  const reportedDuration = tasks.reduce((total, task) => total + (task.durationMs ?? 0), 0)
  const totalDuration = reportedDuration || classifiedDuration
  const otherDuration = Math.max(0, totalDuration - classifiedDuration)
  const breakdownTotal = Math.max(1, totalDuration, classifiedDuration)
  const durationBreakdown = [
    { label: t('diagnostics.modelTime'), value: modelDuration, tone: 'bg-info' },
    { label: t('diagnostics.toolExecutionTime'), value: toolExecutionDuration, tone: 'bg-warning' },
    { label: t('diagnostics.approvalWait'), value: approvalDuration, tone: 'bg-warning/55' },
    { label: t('diagnostics.checkpoint'), value: checkpointDuration, tone: 'bg-success' },
    { label: t('diagnostics.otherTime'), value: otherDuration, tone: 'bg-ink-ghost' },
  ].filter((item) => item.value > 0)
  const inputTokens = overviewMetricTotal(tasks, requests, 'inputTokens')
  const outputTokens = overviewMetricTotal(tasks, requests, 'outputTokens')
  const cacheReadTokens = overviewMetricTotal(tasks, requests, 'cacheReadTokens')
  const cacheWriteTokens = overviewMetricTotal(tasks, requests, 'cacheWriteTokens')
  const totalTokens = overviewMetricTotal(tasks, requests, 'totalTokens')
  const totalCost = overviewMetricTotal(tasks, requests, 'costTotalUsd')
  const toolCount = requests.reduce((total, request) => total + request.tools.length, 0)
  const firstTokenValues = requests
    .map((request) => request.timeToFirstOutputMs)
    .filter((value): value is number => value !== undefined && Number.isFinite(value) && value >= 0)
  const medianFirstToken = median(firstTokenValues)
  const slowestRequest = maxRequestBy(requests, (request) => request.durationMs)
  const slowestFirstToken = maxRequestBy(requests, (request) => request.timeToFirstOutputMs)
  const highestTokenRequest = maxRequestBy(requests, (request) => request.totalTokens)
  const retries = tasks.reduce((total, task) => total + task.retries, 0)
  const recoveries = tasks.reduce((total, task) => total + task.contextRecoveries, 0)
  const problemRequests = requests.filter(requestNeedsAttention)
  const overviewMetrics = [
    {
      id: 'duration',
      label: t('diagnostics.totalDuration'),
      value: formatDuration(totalDuration),
      detail: t('diagnostics.taskCount', { count: tasks.length }),
    },
    {
      id: 'requests',
      label: t('diagnostics.modelRequests'),
      value: formatCompactNumber(requests.length),
      detail: problemRequests.length > 0
        ? t('diagnostics.requestIssues', { count: problemRequests.length })
        : t('diagnostics.allRequestsCompleted'),
      danger: problemRequests.length > 0,
    },
    {
      id: 'first-token',
      label: t('diagnostics.medianFirstToken'),
      value: formatDuration(medianFirstToken),
      detail: slowestFirstToken
        ? t('diagnostics.slowestInline', { duration: formatDuration(slowestFirstToken.timeToFirstOutputMs) })
        : t('diagnostics.notReported'),
    },
    {
      id: 'tokens',
      label: t('diagnostics.tokens'),
      value: formatCompactNumber(totalTokens),
      detail: formatTokenBreakdown({
        inputTokens,
        outputTokens,
        cacheReadTokens,
        cacheWriteTokens,
        totalTokens,
      }, t),
    },
    {
      id: 'cost',
      label: t('diagnostics.cost'),
      value: formatUSD(totalCost),
      detail: totalCost === undefined ? t('diagnostics.notReported') : t('diagnostics.providerReported'),
    },
    {
      id: 'tools',
      label: t('diagnostics.toolCalls'),
      value: formatCompactNumber(toolCount),
      detail: approvalDuration > 0
        ? t('diagnostics.approvalTotal', { duration: formatDuration(approvalDuration) })
        : t('diagnostics.noApprovalWait'),
    },
  ]
  return (
    <div
      className="code-scroll-area -mr-7 min-h-0 flex-1 overflow-y-auto py-5 pr-7 max-lg:-mr-5 max-lg:pr-5 max-md:-mr-3 max-md:pr-3"
      data-testid="diagnostics-overview-scroll"
    >
      {hasEarlier && (
        <div className="mb-5 w-full">
          <LoadEarlierControl
            loading={loadingEarlier}
            error={earlierError}
            onLoad={() => { void onLoadEarlier() }}
          />
        </div>
      )}
      <div
        className="grid w-full grid-cols-[minmax(0,1.65fr)_minmax(20rem,0.65fr)] border-b border-edge-soft pb-5 max-2xl:grid-cols-1"
        data-testid="diagnostics-overview-summary-grid"
      >
        <div className="min-w-0 pr-6 max-2xl:pr-0">
          <section className="w-full" aria-label={t('diagnostics.performanceSummary')}>
            <h3 className="text-[0.875rem] font-medium text-ink">{t('diagnostics.performanceSummary')}</h3>
            <dl className="mt-3 grid grid-cols-3 gap-px border-y border-edge bg-edge-soft max-sm:grid-cols-2">
              {overviewMetrics.map((metric) => (
                <div
                  key={metric.id}
                  data-overview-metric={metric.id}
                  className="min-w-0 bg-canvas px-3 py-2.5"
                >
                  <dt className="truncate text-[0.6875rem] font-normal text-ink-muted">{metric.label}</dt>
                  <dd className={cn(
                    'm-0 mt-0.5 truncate text-[1.0625rem] leading-6 font-medium tabular-nums text-ink',
                    metric.danger && 'text-danger',
                  )}>
                    {metric.value}
                  </dd>
                  <dd className={cn(
                    'block truncate text-[0.6875rem] tabular-nums text-ink-faint',
                    metric.danger && 'text-danger',
                  )}>
                    {metric.detail}
                  </dd>
                </div>
              ))}
            </dl>
          </section>

          <section className="mt-5 w-full" data-testid="diagnostics-duration-breakdown">
            <div className="flex items-end justify-between gap-4">
              <h3 className="text-[0.8125rem] font-medium text-ink-soft">{t('diagnostics.durationBreakdown')}</h3>
              <span className="text-[0.6875rem] tabular-nums text-ink-faint">{formatDuration(totalDuration)}</span>
            </div>
            <div
              className="mt-2.5 flex h-2 overflow-hidden rounded-[2px] bg-canvas-sunken"
              aria-label={t('diagnostics.durationBreakdown')}
            >
              {durationBreakdown.map((item) => (
                <span
                  key={item.label}
                  className={cn('h-full min-w-px', item.tone)}
                  style={{ width: `${(item.value / breakdownTotal) * 100}%` }}
                  title={`${item.label}: ${formatDuration(item.value)}`}
                />
              ))}
            </div>
            <dl className="mt-2.5 grid grid-cols-3 gap-x-5 gap-y-2 max-sm:grid-cols-2">
              {durationBreakdown.map((item) => (
                <div key={item.label} className="min-w-0">
                  <dt className="flex items-center gap-1.5 truncate text-[0.6875rem] text-ink-muted">
                    <span className={cn('size-1.5 shrink-0 rounded-[1px]', item.tone)} aria-hidden="true" />
                    {item.label}
                  </dt>
                  <dd className="m-0 mt-0.5 flex items-baseline gap-1.5 tabular-nums">
                    <span className="text-[0.75rem] text-ink-soft">{formatDuration(item.value)}</span>
                    <span className="text-[0.65625rem] text-ink-faint">{formatPercentage(item.value, breakdownTotal)}</span>
                  </dd>
                </div>
              ))}
            </dl>
          </section>
        </div>

        <section
          className="min-w-0 border-l border-edge-soft pl-6 max-2xl:mt-5 max-2xl:border-t max-2xl:border-l-0 max-2xl:pt-5 max-2xl:pl-0"
          data-testid="diagnostics-key-signals"
        >
          <h3 className="text-[0.8125rem] font-medium text-ink-soft">{t('diagnostics.keySignals')}</h3>
          <div className="mt-3 border-y border-edge">
            <OverviewSignal
              label={t('diagnostics.longestRequest')}
              request={slowestRequest}
              value={formatDuration(slowestRequest?.durationMs)}
              detail={slowestRequest?.durationMs === undefined
                ? undefined
                : t('diagnostics.modelTimeShare', {
                    percentage: formatPercentage(slowestRequest.durationMs, modelDuration),
                  })}
              onOpenRequest={onOpenRequest}
            />
            <OverviewSignal
              label={t('diagnostics.slowestFirstToken')}
              request={slowestFirstToken}
              value={formatDuration(slowestFirstToken?.timeToFirstOutputMs)}
              detail={slowestFirstToken?.timeToFirstOutputMs === undefined || !medianFirstToken
                ? undefined
                : t('diagnostics.medianMultiple', {
                    multiple: formatRatio(slowestFirstToken.timeToFirstOutputMs, medianFirstToken),
                  })}
              onOpenRequest={onOpenRequest}
            />
            <OverviewSignal
              label={t('diagnostics.highestTokenRequest')}
              request={highestTokenRequest}
              value={t('diagnostics.tokenValue', { count: formatCompactNumber(highestTokenRequest?.totalTokens) })}
              detail={highestTokenRequest?.totalTokens === undefined || totalTokens === undefined
                ? undefined
                : t('diagnostics.totalTokenShare', {
                    percentage: formatPercentage(highestTokenRequest.totalTokens, totalTokens),
                  })}
              onOpenRequest={onOpenRequest}
            />
            <div className="min-w-0 px-3 py-2.5">
              <span className="block text-[0.6875rem] text-ink-muted">{t('diagnostics.reliability')}</span>
              <span className={cn(
                'mt-0.5 block truncate text-[0.8125rem] font-medium text-ink-soft',
                problemRequests.length > 0 && 'text-danger',
                problemRequests.length === 0 && (retries > 0 || recoveries > 0) && 'text-warning',
              )}>
                {problemRequests.length > 0
                  ? t('diagnostics.requestIssues', { count: problemRequests.length })
                  : t('diagnostics.noFailedRequests')}
              </span>
              <span className="mt-0.5 block truncate text-[0.6875rem] text-ink-faint">
                {t('diagnostics.retryRecoverySummary', { retries, recoveries })}
              </span>
            </div>
          </div>
        </section>
      </div>

      <RawEvents events={tasks.flatMap((task) => task.rawEvents)} omitted={tasks.reduce((total, task) => total + (task.omittedEvents ?? 0), 0)} />
    </div>
  )
}

function OverviewSignal({
  label,
  request,
  value,
  detail,
  onOpenRequest,
}: {
  label: string
  request?: TraceBundleRequest
  value: string
  detail?: string
  onOpenRequest: (requestID: string) => void
}) {
  const { t } = useI18n()
  if (!request) {
    return (
      <div className="min-w-0 border-b border-edge-soft px-3 py-2.5">
        <span className="block text-[0.6875rem] text-ink-muted">{label}</span>
        <span className="mt-0.5 block text-[0.8125rem] text-ink-faint">—</span>
      </div>
    )
  }
  return (
    <button
      type="button"
      className="group grid w-full min-w-0 cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-center gap-3 border-b border-edge-soft px-3 py-2.5 text-left outline-none hover:bg-surface-hover focus-visible:bg-surface-hover"
      aria-label={`${label}: ${t('diagnostics.requestNumber', { count: request.number })}, ${value}${detail ? `, ${detail}` : ''}`}
      onClick={() => onOpenRequest(request.id)}
    >
      <span className="min-w-0">
        <span className="block text-[0.6875rem] text-ink-muted">{label}</span>
        <span className="mt-0.5 block truncate text-[0.8125rem] font-medium text-ink-soft">
          {t('diagnostics.requestNumber', { count: request.number })}
        </span>
        <span className="mt-0.5 block truncate font-mono text-[0.6875rem] text-ink-faint">
          {request.model || request.provider || '—'}
        </span>
      </span>
      <span className="flex items-center gap-1.5">
        <span className="min-w-0 text-right tabular-nums">
          <span className="block text-[0.75rem] text-ink-muted">{value}</span>
          {detail && <span className="mt-0.5 block text-[0.65625rem] text-ink-faint">{detail}</span>}
        </span>
        <ChevronRight className="size-3.5 shrink-0 text-ink-ghost transition-transform group-hover:translate-x-0.5 group-hover:text-ink-muted" aria-hidden="true" />
      </span>
    </button>
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

function RawEvents({ events, omitted }: { events: DiagnosticEvent[]; omitted?: number }) {
  const { t } = useI18n()
  return (
    <details className="group mt-7 w-full border-t border-edge-soft pt-5">
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
  const hierarchy = buildTrajectoryHierarchy(tasks)
  for (let taskIndex = 0; taskIndex < tasks.length; taskIndex++) {
    const task = tasks[taskIndex]!
    const request = task.requests.find((candidate) => candidate.input?.systemPrompt?.trim())
    const systemPrompt = request?.input?.systemPrompt?.trim()
    if (!request || !systemPrompt) continue
    items.push({
      id: `system:${request.id}`,
      kind: 'system',
      task,
      request,
      preview: systemPrompt,
      raw: { systemPrompt, providerRequestId: request.id },
    })
    break
  }
  tasks.forEach((task) => {
    const firstRequest = task.requests[0]
    items.push({
      id: `task:${task.id}:user`,
      kind: 'user',
      task,
      request: firstRequest,
      hierarchy: hierarchy.tasks.get(task.id),
      preview: task.prompt || t('diagnostics.taskPromptUnavailable'),
      raw: { runId: task.id, prompt: task.prompt },
    })
    task.requests.forEach((request) => {
      const requestHierarchy = hierarchy.requests.get(request.id)
      request.attachments?.forEach((attachment) => {
        const message = request.input?.messages[attachment.messageIndex]
        if (!message || !shouldShowContextAttachment(attachment, message, seenContext)) return
        items.push({
          id: `request:${request.id}:context:${attachment.id || `${attachment.kind}:${attachment.messageIndex}`}`,
          kind: 'context',
          task,
          request,
          hierarchy: requestHierarchy,
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
        request,
        hierarchy: requestHierarchy,
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
          request,
          hierarchy: requestHierarchy,
          tool,
          preview: toolPreview(tool),
          raw: tool,
        })
      })
    })
  })
  return items
}

function buildTrajectoryHierarchy(tasks: TraceBundleTask[]): TrajectoryHierarchyIndex {
  const turnNumbers = new Map<string, number>()
  const stepNumbers = new Map<string, Map<string, number>>()
  const taskTurns = new Map<string, { turnKey: string; turnID?: string }>()
  const pending: Array<{
    request: TraceBundleRequest
    turnKey: string
    turnID?: string
    stepKey: string
    stepID?: string
  }> = []

  const ensureTurn = (turnKey: string) => {
    if (!turnNumbers.has(turnKey)) turnNumbers.set(turnKey, turnNumbers.size + 1)
    if (!stepNumbers.has(turnKey)) stepNumbers.set(turnKey, new Map())
  }

  for (const task of tasks) {
    const semanticHierarchy = task.requests.some((request) => Boolean(request.stepId))
    let activeTurnKey = semanticHierarchy ? `turn:${task.id}` : `legacy-turn:${task.id}`
    let activeTurnID: string | undefined
    if (task.requests.length === 0) {
      ensureTurn(activeTurnKey)
      taskTurns.set(task.id, { turnKey: activeTurnKey })
      continue
    }

    for (const request of task.requests) {
      if (semanticHierarchy && request.turnId) {
        activeTurnKey = `turn:${request.turnId}`
        activeTurnID = request.turnId
      }
      const turnKey = semanticHierarchy ? activeTurnKey : `legacy-turn:${task.id}`
      const turnID = semanticHierarchy ? activeTurnID : undefined
      ensureTurn(turnKey)
      if (!taskTurns.has(task.id)) taskTurns.set(task.id, { turnKey, turnID })
      const steps = stepNumbers.get(turnKey)!
      const stepKey = request.stepId ? `step:${request.stepId}` : `request:${request.id}`
      if (!steps.has(stepKey)) steps.set(stepKey, steps.size + 1)
      pending.push({ request, turnKey, turnID, stepKey, stepID: request.stepId })
    }
  }

  const requests = new Map(pending.map((entry) => [entry.request.id, {
    turnKey: entry.turnKey,
    turnID: entry.turnID,
    turnNumber: turnNumbers.get(entry.turnKey)!,
    turnStepCount: stepNumbers.get(entry.turnKey)?.size ?? 0,
    stepKey: entry.stepKey,
    stepID: entry.stepID,
    stepNumber: stepNumbers.get(entry.turnKey)?.get(entry.stepKey) ?? 0,
  }]))
  const taskHierarchy = new Map(tasks.map((task) => {
    const requestHierarchy = requests.get(task.requests[0]?.id ?? '')
    if (requestHierarchy) return [task.id, requestHierarchy]
    const turn = taskTurns.get(task.id)!
    return [task.id, {
      turnKey: turn.turnKey,
      turnID: turn.turnID,
      turnNumber: turnNumbers.get(turn.turnKey)!,
      turnStepCount: 0,
      stepKey: `empty-step:${task.id}`,
      stepNumber: 0,
    }]
  }))
  return { tasks: taskHierarchy, requests }
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

function activatedSkillMarkdown(value: string): string {
  const loadedSkill = /<loaded_skill\b[^>]*>/i.exec(value)
  const content = loadedSkill
    ? value.slice((loadedSkill.index ?? 0) + loadedSkill[0].length)
    : value.replace(/^\s*<or-context\b[^>]*>/i, '')
  return content
    .replace(/<\/loaded_skill>\s*(?:<\/or-context>)?\s*$/i, '')
    .replace(/<\/or-context>\s*$/i, '')
    .trim()
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

function timelineModelTiming(item: TrajectoryItem): {
  ttftMs: number
  generationMs: number
  totalMs: number
  ttftFraction: number
} | undefined {
  const totalMs = item.request?.durationMs
  const ttftMs = item.request?.timeToFirstOutputMs
  if (
    item.kind !== 'assistant' ||
    totalMs === undefined ||
    ttftMs === undefined ||
    !Number.isFinite(totalMs) ||
    !Number.isFinite(ttftMs) ||
    totalMs <= 0 ||
    ttftMs < 0 ||
    ttftMs > totalMs
  ) return undefined
  return {
    ttftMs,
    generationMs: totalMs - ttftMs,
    totalMs,
    ttftFraction: ttftMs / totalMs,
  }
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

function trajectoryHierarchyLabel(item: TrajectoryItem, t: Translate): string {
  if (!item.hierarchy || !item.request) {
    return item.request
      ? t('diagnostics.requestNumber', { count: item.request.number })
      : item.hierarchy
        ? t('diagnostics.turnLabel', { count: item.hierarchy.turnNumber })
        : ''
  }
  return [
    t('diagnostics.turnLabel', { count: item.hierarchy.turnNumber }),
    t('diagnostics.stepLabel', { count: item.hierarchy.stepNumber }),
    t('diagnostics.requestNumber', { count: item.request.number }),
  ].join(' · ')
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

function markdownPlainText(value: string): string {
  try {
    return singleLine(trajectoryMarkdownParser.lexer(value).map(markdownTokenText).join(' '))
  } catch {
    return singleLine(value)
  }
}

function markdownTokenText(token: Token): string {
  if (token.type === 'list') return token.items.map(markdownTokenText).join(' ')
  if (token.type === 'table') {
    return [...token.header, ...token.rows.flat()]
      .map((cell) => cell.tokens.map(markdownTokenText).join(' '))
      .join(' ')
  }
  if (token.type === 'html' || token.type === 'space' || token.type === 'hr' || token.type === 'br') {
    return ''
  }
  if ('tokens' in token && Array.isArray(token.tokens)) {
    const separator = token.type === 'blockquote' || token.type === 'list_item' ? ' ' : ''
    return token.tokens.map(markdownTokenText).join(separator)
  }
  return 'text' in token && typeof token.text === 'string' ? token.text : ''
}

function statusText(value?: string): string {
  return value ? value.replaceAll('_', ' ') : '—'
}

type OverviewMetricKey =
  | 'inputTokens'
  | 'outputTokens'
  | 'cacheReadTokens'
  | 'cacheWriteTokens'
  | 'totalTokens'
  | 'costTotalUsd'

function overviewMetricTotal(
  tasks: TraceBundleTask[],
  requests: TraceBundleRequest[],
  key: OverviewMetricKey,
): number | undefined {
  const taskValues = tasks
    .map((task) => task[key])
    .filter((value): value is number => value !== undefined && Number.isFinite(value))
  if (taskValues.length > 0) return taskValues.reduce((total, value) => total + value, 0)
  const requestValues = requests
    .map((request) => request[key])
    .filter((value): value is number => value !== undefined && Number.isFinite(value))
  return requestValues.length > 0
    ? requestValues.reduce((total, value) => total + value, 0)
    : undefined
}

function taskApprovalDuration(task: TraceBundleTask): number {
  if ((task.approvalDurationMs ?? 0) > 0) return task.approvalDurationMs ?? 0
  return task.requests.flatMap((request) => request.tools)
    .reduce((total, tool) => total + (tool.approvalDurationMs ?? 0), 0)
}

function taskCheckpointDuration(task: TraceBundleTask): number {
  if ((task.checkpointDurationMs ?? 0) > 0) return task.checkpointDurationMs ?? 0
  return task.requests.reduce((total, request) => total + (request.checkpointDurationMs ?? 0), 0)
}

function taskToolExecutionDuration(task: TraceBundleTask): number {
  const tools = task.requests.flatMap((request) => request.tools)
  const measuredTools = tools.filter((tool) =>
    tool.executionDurationMs !== undefined || tool.durationMs !== undefined)
  if (measuredTools.length > 0) {
    return measuredTools.reduce((total, tool) => total + (
      tool.executionDurationMs ?? Math.max(0, (tool.durationMs ?? 0) - (tool.approvalDurationMs ?? 0))
    ), 0)
  }
  return Math.max(0, (task.toolDurationMs ?? 0) - taskApprovalDuration(task))
}

function median(values: number[]): number | undefined {
  if (values.length === 0) return undefined
  const sorted = [...values].sort((left, right) => left - right)
  const midpoint = Math.floor(sorted.length / 2)
  return sorted.length % 2 === 0
    ? ((sorted[midpoint - 1] ?? 0) + (sorted[midpoint] ?? 0)) / 2
    : sorted[midpoint]
}

function formatTokenBreakdown(
  values: {
    inputTokens?: number
    outputTokens?: number
    cacheReadTokens?: number
    cacheWriteTokens?: number
    totalTokens?: number
  },
  t: Translate,
): string {
  const { inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens, totalTokens } = values
  if (
    inputTokens === undefined &&
    outputTokens === undefined &&
    cacheReadTokens === undefined &&
    cacheWriteTokens === undefined
  ) return t('diagnostics.notReported')

  const detailKey = (cacheWriteTokens ?? 0) > 0
    ? 'diagnostics.tokenSplitWithCacheWrite'
    : (cacheReadTokens ?? 0) > 0
      ? 'diagnostics.tokenSplitWithCache'
      : 'diagnostics.tokenSplit'
  const detail = t(detailKey, {
    input: formatCompactNumber(inputTokens),
    output: formatCompactNumber(outputTokens),
    cacheRead: formatCompactNumber(cacheReadTokens),
    cacheWrite: formatCompactNumber(cacheWriteTokens),
  })
  const accountedTokens = (inputTokens ?? 0) + (outputTokens ?? 0) +
    (cacheReadTokens ?? 0) + (cacheWriteTokens ?? 0)
  return totalTokens !== undefined && accountedTokens < totalTokens
    ? t('diagnostics.tokenBreakdownPartial', { detail })
    : detail
}

function maxRequestBy(
  requests: TraceBundleRequest[],
  valueFor: (request: TraceBundleRequest) => number | undefined,
): TraceBundleRequest | undefined {
  return requests.reduce<TraceBundleRequest | undefined>((current, request) => {
    const value = valueFor(request)
    if (value === undefined || !Number.isFinite(value)) return current
    const currentValue = current ? valueFor(current) : undefined
    return currentValue === undefined || value > currentValue ? request : current
  }, undefined)
}

function requestNeedsAttention(request: TraceBundleRequest): boolean {
  const status = request.status?.toLocaleLowerCase()
  return Boolean(
    request.errorCode ||
    status === 'failed' ||
    status === 'cancelled' ||
    status === 'denied' ||
    status === 'discarded',
  )
}

function requestStatusLabel(request: TraceBundleRequest, t: Translate): string {
  const status = request.status || (request.lifecycle === 'in-progress' ? 'running' : 'completed')
  return localizedStatusLabel(status, t)
}

function toolStatusLabel(tool: TraceBundleTool, t: Translate): string {
  const status = tool.status || (tool.lifecycle === 'in-progress' ? 'running' : 'completed')
  return localizedStatusLabel(status, t)
}

function localizedStatusLabel(status: string, t: Translate): string {
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

function formatPercentage(value: number, total: number): string {
  if (!Number.isFinite(value) || !Number.isFinite(total) || total <= 0) return '—'
  return new Intl.NumberFormat(undefined, {
    style: 'percent',
    maximumFractionDigits: value / total < 0.01 ? 1 : 0,
  }).format(value / total)
}

function formatRatio(value: number, baseline: number): string {
  if (!Number.isFinite(value) || !Number.isFinite(baseline) || baseline <= 0) return '—'
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 }).format(value / baseline)
}

function formatUSD(value?: number): string {
  if (value === undefined || !Number.isFinite(value) || value < 0) return '—'
  const fractionDigits = value > 0 && value < 0.01 ? 4 : 2
  return new Intl.NumberFormat(undefined, {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  }).format(value)
}

function formatTimestamp(value: string): string {
  const date = new Date(value)
  return Number.isFinite(date.getTime()) ? date.toLocaleString() : value
}

type Translate = ReturnType<typeof useI18n>['t']
