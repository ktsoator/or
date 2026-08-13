import { useCallback, useEffect, useState } from 'react'
import {
  ArrowLeft,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  CircleAlert,
  CircleDot,
  Gauge,
  LoaderCircle,
  RefreshCw,
  RotateCcw,
} from 'lucide-react'
import { useI18n, type Locale } from '@/i18n'
import { cn } from '@/lib/utils'
import { SidebarToggleButton } from '@/shared/ui/SidebarToggleButton'
import {
  fetchDiagnosticRuns,
  type DiagnosticEvent,
  type DiagnosticRun,
  type DiagnosticReport,
} from './catalog'
import { buildDiagnosticTurns, type DiagnosticStep, type DiagnosticTurn } from './viewModel'

type Scope = 'session' | 'all'

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
  const { locale, t, formatNumber } = useI18n()
  const [scope, setScope] = useState<Scope>(sessionID ? 'session' : 'all')
  const [report, setReport] = useState<DiagnosticReport>()
  const [selectedRunID, setSelectedRunID] = useState(initialRunID ?? '')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const scopedSessionID = scope === 'session' ? sessionID : undefined

  const load = useCallback(async (signal?: AbortSignal, quiet = false) => {
    if (!quiet) setLoading(true)
    setError(false)
    try {
      const next = await fetchDiagnosticRuns(scopedSessionID, signal)
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
  }, [scopedSessionID])

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
        <div className="mx-auto flex h-full w-full max-w-[76rem] min-w-0 flex-col px-8 pt-8 pb-7 max-lg:px-5 max-md:px-3 max-md:pt-5">
          <div className="flex shrink-0 flex-wrap items-end justify-between gap-4 border-b border-edge/80 pb-5">
            <div>
              <h1 className="text-[1.5rem] leading-8 font-semibold tracking-normal text-ink">
                {t('diagnostics.title')}
              </h1>
              {report && (
                <p className="mt-0.5 text-[0.8125rem] leading-5 text-ink-muted">
                  {t('diagnostics.updatedAt', { time: formatTimestamp(report.generatedAt, locale) })}
                </p>
              )}
            </div>
            {sessionID && (
              <div className="flex h-8 items-center rounded-[8px] bg-canvas-sunken p-0.5" role="group" aria-label={t('diagnostics.scope')}>
                <ScopeButton
                  active={scope === 'session'}
                  label={t('diagnostics.currentSession')}
                  onClick={() => setScope('session')}
                />
                <ScopeButton
                  active={scope === 'all'}
                  label={t('diagnostics.allSessions')}
                  onClick={() => setScope('all')}
                />
              </div>
            )}
          </div>

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
            <div className="grid min-h-0 flex-1 grid-cols-[19rem_minmax(0,1fr)] max-lg:grid-cols-[15rem_minmax(0,1fr)] max-md:grid-cols-1 max-md:grid-rows-[12rem_minmax(0,1fr)]">
              <aside className="min-h-0 overflow-y-auto border-r border-edge/80 pr-3 pt-3 max-md:border-r-0 max-md:border-b max-md:pr-0">
                <div className="mb-2 flex h-7 items-center justify-between px-2">
                  <span className="text-[0.71875rem] font-medium text-ink-faint">
                    {t('diagnostics.recentRuns')}
                  </span>
                  <span className="font-mono text-[0.6875rem] text-ink-faint">
                    {formatNumber(report?.runs.length ?? 0)}
                  </span>
                </div>
                <div className="space-y-0.5 pb-3 max-md:grid max-md:grid-cols-2 max-md:gap-1 max-sm:grid-cols-1">
                  {report?.runs.map((run) => (
                    <RunRow
                      key={run.id}
                      run={run}
                      active={run.id === selectedRun?.id}
                      locale={locale}
                      onSelect={() => setSelectedRunID(run.id)}
                    />
                  ))}
                </div>
              </aside>

              {selectedRun && (
                <RunDetail
                  run={selectedRun}
                  locale={locale}
                  formatNumber={formatNumber}
                />
              )}
            </div>
          )}
        </div>
      </main>
    </div>
  )
}

function ScopeButton({ active, label, onClick }: { active: boolean; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      className={cn(
        'h-7 cursor-pointer rounded-[6px] px-2.5 text-[0.75rem] outline-none transition-colors',
        active ? 'bg-canvas text-ink shadow-sm' : 'text-ink-muted hover:text-ink',
      )}
      aria-pressed={active}
      onClick={onClick}
    >
      {label}
    </button>
  )
}

function RunRow({
  run,
  active,
  locale,
  onSelect,
}: {
  run: DiagnosticRun
  active: boolean
  locale: Locale
  onSelect: () => void
}) {
  const { t } = useI18n()
  return (
    <button
      className={cn(
        'group flex w-full cursor-pointer items-center gap-2 rounded-[7px] px-2 py-2 text-left outline-none transition-colors',
        active ? 'bg-surface-selected' : 'hover:bg-surface-hover focus-visible:bg-surface-hover',
      )}
      type="button"
      onClick={onSelect}
    >
      <StatusDot status={run.status} />
      <div className="min-w-0 flex-1">
        <div className="flex items-center justify-between gap-3">
          <span className="truncate font-mono text-[0.75rem] font-medium text-ink-soft">
            {shortID(run.id)}
          </span>
          <span className="shrink-0 text-[0.6875rem] text-ink-faint">
            {formatClock(run.updatedAt, locale)}
          </span>
        </div>
        <div className="mt-0.5 flex items-center gap-2 text-[0.6875rem] text-ink-muted">
          <span>{runStatusLabel(run.status, t)}</span>
          <span aria-hidden="true">·</span>
          <span>{formatDuration(run.durationMs)}</span>
          {run.toolCalls > 0 && (
            <>
              <span aria-hidden="true">·</span>
              <span>{t('diagnostics.toolsShort', { count: run.toolCalls })}</span>
            </>
          )}
        </div>
      </div>
      <ChevronRight className={cn('size-3.5 shrink-0 text-ink-faint', active && 'text-ink-muted')} aria-hidden="true" />
    </button>
  )
}

function RunDetail({
  run,
  locale,
  formatNumber,
}: {
  run: DiagnosticRun
  locale: Locale
  formatNumber: (value: number, options?: Intl.NumberFormatOptions) => string
}) {
  const { t } = useI18n()
  const turns = buildDiagnosticTurns(run.events)
  const providerDurationMs = run.events.reduce((total, event) =>
    event.name === 'provider.request.completed' || event.name === 'provider.request.failed'
      ? total + (event.durationMs ?? 0)
      : total, 0)
  const metrics: Array<{ label: string; value: string }> = [
    { label: t('diagnostics.totalDuration'), value: formatDuration(run.durationMs) },
    { label: t('diagnostics.firstToken'), value: formatDuration(run.timeToFirstOutputMs) },
    { label: t('diagnostics.tokens'), value: t('diagnostics.tokenCount', { count: formatNumber(run.totalTokens ?? 0) }) },
    { label: t('diagnostics.cost'), value: formatCost(run.costTotalUsd ?? 0) },
  ]
  const durations = [
    { label: t('diagnostics.modelTime'), value: providerDurationMs, tone: 'bg-info' },
    { label: t('diagnostics.toolTime'), value: run.toolDurationMs ?? 0, tone: 'bg-ink-muted' },
    { label: t('diagnostics.approvalWait'), value: run.approvalDurationMs ?? 0, tone: 'bg-warning' },
    { label: t('diagnostics.checkpoint'), value: run.checkpointDurationMs ?? 0, tone: 'bg-success' },
  ].filter((duration) => duration.value > 0)
  return (
    <section className="min-h-0 overflow-y-auto pl-8 pt-7 max-lg:pl-6 max-md:pl-0 max-md:pt-5" aria-label={t('diagnostics.runDetail')}>
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex items-center gap-2.5">
            <StatusDot status={run.status} />
            <h2 className="truncate text-[1.125rem] font-semibold leading-6 text-ink" title={run.id}>
              {t('diagnostics.runLabel', { id: shortID(run.id).replace(/^run_/, '') })}
            </h2>
            <span className="text-[0.8125rem] text-ink-muted">{runStatusLabel(run.status, t)}</span>
          </div>
          <div className="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1 pl-6 text-[0.8125rem] text-ink-muted">
            <span>{formatTimestamp(run.startedAt, locale)}</span>
            <span aria-hidden="true" className="text-ink-faint">·</span>
            <span>{t('diagnostics.turnCount', { count: turns.length })}</span>
            {run.errorCode && <span className="font-mono text-danger">{run.errorCode}</span>}
          </div>
        </div>
        <div className="flex items-center gap-2 text-[0.75rem] text-ink-muted">
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
      </div>

      <div className="mt-7 grid grid-cols-4 gap-x-8 gap-y-5 rounded-[8px] bg-canvas-sunken/65 px-5 py-4 max-lg:grid-cols-2 max-sm:gap-x-4 max-sm:px-4">
        {metrics.map((metric) => (
          <Metric key={metric.label} {...metric} />
        ))}
      </div>

      {durations.length > 0 && (
        <section className="mt-8">
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

      <div className="mt-8 border-t border-edge-soft pt-6">
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
    </section>
  )
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <div className="truncate text-[0.75rem] font-medium text-ink-muted">{label}</div>
      <div className="mt-1 truncate font-mono text-[1.125rem] font-semibold leading-6 text-ink">{value}</div>
    </div>
  )
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

function TurnSection({ turn, index }: { turn: DiagnosticTurn; index: number }) {
  const { t } = useI18n()
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
      {turn.steps.length > 0 ? (
        <div className="mt-3 space-y-1.5">
          {turn.steps.map((step, stepIndex) => (
            <StepRow key={`${step.timestamp}-${step.kind}-${stepIndex}`} step={step} />
          ))}
        </div>
      ) : (
        <p className="mt-2 text-[0.8125rem] text-ink-muted">{t('diagnostics.noMeasuredSteps')}</p>
      )}
    </section>
  )
}

function StepRow({ step }: { step: DiagnosticStep }) {
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
          {step.timeToFirstOutputMs ? <span>{t('diagnostics.outputStartedAfter', { duration: formatDuration(step.timeToFirstOutputMs) })}</span> : null}
          {step.approvalDurationMs ? <span>{t('diagnostics.approvalInline', { duration: formatDuration(step.approvalDurationMs) })}</span> : null}
          {step.attempts && step.attempts > 1 ? <span className="text-warning">{t('diagnostics.attemptCount', { count: step.attempts })}</span> : null}
          {step.totalTokens ? <span>{t('diagnostics.requestTokenUsage', { count: formatNumber(step.totalTokens) })}</span> : null}
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

function StatusDot({ status }: { status: string }) {
  const Icon = status === 'completed' || status === 'success' || status === 'allowed'
    ? CheckCircle2
    : status === 'failed' || status === 'cancelled' || status === 'denied'
      ? CircleAlert
      : CircleDot
  return <Icon className={cn('size-3.5 shrink-0', eventTone({ status } as DiagnosticEvent).text)} aria-hidden="true" />
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

function runStatusLabel(status: string, t: Translate): string {
  return statusLabel(status, t)
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

function formatCost(value: number): string {
  if (value === 0) return '$0.00'
  if (value < 1) return `$${value.toFixed(4)}`
  return `$${value.toFixed(2)}`
}

function formatClock(value: string, locale: Locale): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return new Intl.DateTimeFormat(locale === 'zh-CN' ? 'zh-CN' : 'en-US', {
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

function formatTimestamp(value: string, locale: Locale): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return new Intl.DateTimeFormat(locale === 'zh-CN' ? 'zh-CN' : 'en-US', {
    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit',
  }).format(date)
}
