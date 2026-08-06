import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  RefreshCw,
} from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import { apiURL } from '@/api'
import { useI18n } from '@/i18n'
import { ProviderIcon } from '@/shared/ui/ProviderIdentity'
import { providerName } from '@/lib/provider'
import type {
  ModelUsageSummary,
  UsageCost,
  UsageEvent,
  UsageEventPage,
  UsageReport,
  UsageTotals,
} from '@/types'

type ProviderUsageGroup = UsageTotals & {
  provider: string
  models: ModelUsageSummary[]
}

type UsageRange = '7d' | '30d' | 'all'

const requestPageSize = 10

export function UsageSettings() {
  const { locale, t, formatNumber } = useI18n()
  const [report, setReport] = useState<UsageReport>()
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [range, setRange] = useState<UsageRange>('30d')
  const since = useMemo(() => usageRangeSince(range), [range])

  const load = useCallback(async (signal?: AbortSignal) => {
    setLoading(true)
    setError('')
    try {
      const query = new URLSearchParams()
      if (since) query.set('since', since)
      const suffix = query.toString()
      const response = await fetch(apiURL(`/usage${suffix ? `?${suffix}` : ''}`), {
        signal,
        cache: 'no-store',
      })
      if (!response.ok) throw new Error(`HTTP ${response.status}`)
      setReport((await response.json()) as UsageReport)
    } catch (cause) {
      if (cause instanceof DOMException && cause.name === 'AbortError') return
      setError(t('usage.loadFailed'))
    } finally {
      if (!signal?.aborted) setLoading(false)
    }
  }, [since, t])

  useEffect(() => {
    const controller = new AbortController()
    void load(controller.signal)
    return () => controller.abort()
  }, [load])

  const total = report?.total
  const hasUsage = Boolean(total?.requests)
  const providerGroups = useMemo(() => groupByProvider(report?.models ?? []), [report?.models])
  const [selectedProvider, setSelectedProvider] = useState('')
  const [selectedModelKey, setSelectedModelKey] = useState('')
  const activeProvider =
    providerGroups.find((group) => group.provider === selectedProvider) ?? providerGroups[0]
  const selectedModel = activeProvider?.models.find(
    (model) => modelUsageKey(model) === selectedModelKey,
  )

  return (
    <div>
      {error ? (
        <div className="flex items-center justify-between rounded-lg border border-danger-edge bg-danger-surface/60 px-4 py-3 text-[0.8125rem] text-danger">
          <span>{error}</span>
          <button
            className="cursor-pointer font-medium underline decoration-danger/40 underline-offset-4 hover:decoration-danger"
            type="button"
            onClick={() => void load()}
          >
            {t('usage.tryAgain')}
          </button>
        </div>
      ) : loading && !report ? (
        <div className="h-28 animate-pulse rounded-lg border border-edge/85 bg-canvas-raised/60" />
      ) : (
        <>
          <section aria-labelledby="usage-overview-title">
            <div className="mb-3 flex h-9 items-center justify-between gap-4">
              <h2 id="usage-overview-title" className="text-[0.84375rem] font-medium text-ink-soft">
                {t('usage.overview')}
              </h2>
              <div className="flex items-center gap-1.5">
                <UsageRangeSelect value={range} onChange={setRange} />
                <button
                  className="grid size-9 cursor-pointer place-items-center rounded-[10px] bg-surface-hover text-ink-muted outline-none transition-colors hover:bg-surface-active hover:text-ink focus-visible:bg-surface-active focus-visible:text-ink disabled:cursor-wait disabled:opacity-50"
                  type="button"
                  title={t('usage.refresh')}
                  aria-label={t('usage.refresh')}
                  disabled={loading}
                  onClick={() => void load()}
                >
                  <RefreshCw
                    className={`size-4 ${loading ? 'animate-spin' : ''}`}
                    aria-hidden="true"
                  />
                </button>
              </div>
            </div>
            {hasUsage ? (
              <div className="grid grid-cols-3 overflow-hidden rounded-[12px] border border-edge/85 bg-canvas max-sm:grid-cols-1">
                <Metric
                  label={t('usage.totalTokens')}
                  value={formatNumber(total?.totalTokens ?? 0)}
                />
                <Metric
                  label={t('usage.requests')}
                  value={formatNumber(total?.requests ?? 0)}
                />
                <Metric
                  label={t('usage.estimatedCost')}
                  value={formatCost(total?.cost.total ?? 0)}
                />
              </div>
            ) : (
              <div className="rounded-[12px] border border-edge/85 bg-canvas px-5 py-9">
                <div className="text-[0.9375rem] font-medium text-ink-soft">
                  {t('usage.emptyTitle')}
                </div>
                <p className="mt-1.5 text-[0.84375rem] leading-6 text-ink-muted">
                  {t('usage.emptyDescription')}
                </p>
              </div>
            )}
          </section>

          {hasUsage && (
            <section className="mt-9" aria-labelledby="usage-models-title">
              <div className="mb-3 flex h-9 items-center justify-between gap-5">
                <h2
                  id="usage-models-title"
                  className="text-[0.84375rem] font-medium text-ink-soft"
                >
                  {t('usage.byModel')}
                </h2>
                {activeProvider && (
                  <ProviderSelect
                    groups={providerGroups}
                    value={activeProvider.provider}
                    onChange={(provider) => {
                      setSelectedProvider(provider)
                      setSelectedModelKey('')
                    }}
                  />
                )}
              </div>

              {activeProvider && (
                <>
                  <div className="min-w-0 overflow-hidden rounded-[12px] border border-edge/85 bg-canvas">
                    <div className="overflow-x-auto">
                      <table className="w-full min-w-[38.75rem] border-collapse text-left text-[0.8125rem]">
                        <thead className="border-b border-edge/75 bg-canvas-raised/45 text-[0.71875rem] font-medium text-ink-muted">
                          <tr>
                            <th className="px-4 py-2.5">{t('usage.model')}</th>
                            <th className="px-3 py-2.5 text-right">{t('usage.requests')}</th>
                            <th className="px-3 py-2.5 text-right">{t('usage.input')}</th>
                            <th className="px-3 py-2.5 text-right">{t('usage.cacheRead')}</th>
                            <th className="px-3 py-2.5 text-right">{t('usage.output')}</th>
                            <th className="px-3 py-2.5 text-right">{t('usage.total')}</th>
                            <th className="px-4 py-2.5 text-right">{t('usage.cost')}</th>
                          </tr>
                        </thead>
                        <tbody>
                          {activeProvider.models.map((model) => (
                            <ModelRow
                              key={`${model.provider}/${model.model}`}
                              model={model}
                              formatNumber={formatNumber}
                              selected={modelUsageKey(model) === selectedModelKey}
                              onToggle={() => {
                                const key = modelUsageKey(model)
                                setSelectedModelKey((current) => (current === key ? '' : key))
                              }}
                            />
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                  {selectedModel && (
                    <RequestDetails
                      key={modelUsageKey(selectedModel)}
                      model={selectedModel}
                      locale={locale}
                      formatNumber={formatNumber}
                      since={since}
                    />
                  )}
                </>
              )}
            </section>
          )}
        </>
      )}
    </div>
  )
}

function UsageRangeSelect({
  value,
  onChange,
}: {
  value: UsageRange
  onChange: (value: UsageRange) => void
}) {
  const { t } = useI18n()
  const options: Array<{ value: UsageRange; label: string }> = [
    { value: '7d', label: t('usage.last7Days') },
    { value: '30d', label: t('usage.last30Days') },
    { value: 'all', label: t('usage.allTime') },
  ]
  const selected = options.find((option) => option.value === value) ?? options[1]

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          className="group flex h-9 min-w-[6rem] cursor-pointer items-center justify-between gap-2 rounded-[10px] bg-surface-hover px-2.5 text-[0.8125rem] text-ink-soft outline-none transition-colors hover:bg-surface-active focus-visible:bg-surface-active data-[state=open]:bg-surface-selected"
          type="button"
          aria-label={t('usage.timeRange')}
        >
          <span>{selected.label}</span>
          <ChevronDown
            className="size-3.5 text-ink-faint transition-transform duration-150 group-data-[state=open]:rotate-180"
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
          className="z-[100] min-w-[8.25rem] animate-[fade-in_110ms_ease-out] rounded-[14px] border border-edge bg-canvas p-1 text-[0.8125rem] text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
        >
          <DropdownMenu.RadioGroup
            className="flex flex-col gap-0.5"
            value={value}
            onValueChange={(next) => onChange(next as UsageRange)}
          >
            {options.map((option) => (
              <DropdownMenu.RadioItem
                key={option.value}
                value={option.value}
                className="relative flex h-9 cursor-default select-none items-center rounded-[9px] px-2.5 pr-8 outline-none data-[highlighted]:bg-surface-active data-[state=checked]:bg-surface-selected"
              >
                <span>{option.label}</span>
                <DropdownMenu.ItemIndicator className="absolute right-2 grid size-4 place-items-center text-ink-soft">
                  <Check className="size-3.5" aria-hidden="true" />
                </DropdownMenu.ItemIndicator>
              </DropdownMenu.RadioItem>
            ))}
          </DropdownMenu.RadioGroup>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

function ProviderSelect({
  groups,
  value,
  onChange,
}: {
  groups: ProviderUsageGroup[]
  value: string
  onChange: (provider: string) => void
}) {
  const { t } = useI18n()
  const selected = groups.find((group) => group.provider === value) ?? groups[0]
  if (!selected) return null

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          className="group flex h-9 min-w-[10.5rem] max-w-[15rem] shrink-0 cursor-pointer items-center gap-2 rounded-[10px] bg-surface-hover px-2.5 text-left text-[0.8125rem] outline-none transition-colors hover:bg-surface-active focus-visible:bg-surface-active data-[state=open]:bg-surface-selected max-sm:min-w-0"
          type="button"
          aria-label={t('usage.providers')}
        >
          <ProviderIcon provider={selected.provider} />
          <span className="min-w-0 flex-1 truncate font-medium text-ink-soft">
            {providerName(selected.provider)}
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
          className="z-[100] min-w-[15rem] animate-[fade-in_110ms_ease-out] rounded-[14px] border border-edge bg-canvas p-1 text-[0.8125rem] text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
        >
          <DropdownMenu.RadioGroup
            className="flex flex-col gap-0.5"
            value={value}
            onValueChange={onChange}
          >
            {groups.map((group) => (
              <DropdownMenu.RadioItem
                key={group.provider}
                value={group.provider}
                className="relative flex h-9 cursor-default select-none items-center gap-2 rounded-[9px] px-2.5 pr-8 outline-none data-[highlighted]:bg-surface-active data-[state=checked]:bg-surface-selected"
              >
                <ProviderIcon provider={group.provider} />
                <span className="min-w-0 flex-1 truncate">{providerName(group.provider)}</span>
                <DropdownMenu.ItemIndicator className="absolute right-2 grid size-4 place-items-center text-ink-soft">
                  <Check className="size-3.5" aria-hidden="true" />
                </DropdownMenu.ItemIndicator>
              </DropdownMenu.RadioItem>
            ))}
          </DropdownMenu.RadioGroup>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="border-r border-edge/75 px-5 py-4 last:border-r-0 max-sm:border-r-0 max-sm:border-b max-sm:last:border-b-0">
      <div className="text-[0.78125rem] text-ink-muted">{label}</div>
      <div className="mt-1 text-[1.375rem] leading-7 font-medium tracking-normal text-ink tabular-nums">
        {value}
      </div>
    </div>
  )
}

function ModelRow({
  model,
  formatNumber,
  selected,
  onToggle,
}: {
  model: ModelUsageSummary
  formatNumber: (value: number, options?: Intl.NumberFormatOptions) => string
  selected: boolean
  onToggle: () => void
}) {
  return (
    <tr className={`border-b border-edge-soft transition-colors last:border-b-0 hover:bg-surface-hover ${selected ? 'bg-canvas-raised' : ''}`}>
      <td className="px-4 py-3">
        <button
          className="group flex max-w-full cursor-pointer items-center gap-2 text-left outline-none focus-visible:underline focus-visible:decoration-ink-ghost focus-visible:underline-offset-4"
          type="button"
          aria-expanded={selected}
          onClick={onToggle}
        >
          <ChevronRight
            className={`size-3.5 shrink-0 transition-transform duration-150 ${selected ? 'rotate-90 text-ink-soft' : 'text-ink-faint'}`}
            aria-hidden="true"
          />
          <span className={`min-w-0 truncate font-medium ${selected ? 'text-ink' : 'text-ink-soft'}`}>
            {model.name || model.model}
          </span>
        </button>
      </td>
      <NumberCell value={formatNumber(model.requests)} />
      <NumberCell value={model.inputUnknown ? '--' : formatNumber(model.input)} />
      <NumberCell value={formatNumber(model.cacheRead)} />
      <NumberCell value={formatNumber(model.output)} />
      <NumberCell value={formatNumber(model.totalTokens)} strong />
      <td className="px-4 py-3 text-right text-[0.78125rem] font-normal text-ink-muted tabular-nums">
        {formatCost(model.cost.total)}
      </td>
    </tr>
  )
}

function RequestDetails({
  model,
  locale,
  formatNumber,
  since,
}: {
  model: ModelUsageSummary
  locale: string
  formatNumber: (value: number, options?: Intl.NumberFormatOptions) => string
  since: string
}) {
  const { t } = useI18n()
  const [events, setEvents] = useState<UsageEvent[]>([])
  const [total, setTotal] = useState(0)
  const [pageIndex, setPageIndex] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    setPageIndex(0)
  }, [since])

  const load = useCallback(async (page: number, signal?: AbortSignal) => {
    setLoading(true)
    setError('')
    const query = new URLSearchParams({
      provider: model.provider,
      model: model.model,
      offset: String(page * requestPageSize),
      limit: String(requestPageSize),
    })
    if (since) query.set('since', since)
    try {
      const response = await fetch(apiURL(`/usage/events?${query}`), {
        signal,
        cache: 'no-store',
      })
      if (!response.ok) throw new Error(`HTTP ${response.status}`)
      const result = (await response.json()) as UsageEventPage
      setEvents(result.events)
      setTotal(result.total)
    } catch (cause) {
      if (cause instanceof DOMException && cause.name === 'AbortError') return
      setError(t('usage.requestsLoadFailed'))
    } finally {
      if (!signal?.aborted) {
        setLoading(false)
      }
    }
  }, [model.model, model.provider, since, t])

  useEffect(() => {
    const controller = new AbortController()
    void load(pageIndex, controller.signal)
    return () => controller.abort()
  }, [load, pageIndex])

  const pageCount = Math.max(1, Math.ceil(total / requestPageSize))
  const rangeStart = total === 0 ? 0 : pageIndex * requestPageSize + 1
  const rangeEnd = Math.min(total, rangeStart + events.length - 1)

  return (
    <div
      className="mt-5 min-w-0 animate-[fade-in_120ms_ease-out]"
      data-testid="usage-request-details"
    >
      <div className="mb-2.5 flex min-w-0 items-center gap-2 px-1 text-[0.8125rem]">
        <h3 className="shrink-0 font-medium text-ink-soft">{t('usage.requestDetails')}</h3>
        <span className="min-w-0 truncate text-ink-faint">{model.name || model.model}</span>
      </div>

      <div className="overflow-hidden rounded-[12px] border border-edge/85 bg-canvas shadow-[0_10px_28px_-28px_rgba(28,25,23,0.5)]">
        {loading && events.length === 0 ? (
          <div className="m-4 h-16 animate-pulse rounded-lg bg-canvas-sunken" />
        ) : error ? (
          <div className="m-4 flex items-center justify-between rounded-lg border border-danger-edge bg-danger-surface/60 px-3 py-3 text-[0.78125rem] text-danger">
            <span>{error}</span>
            <button
              className="cursor-pointer font-medium text-danger hover:underline"
              type="button"
              onClick={() => void load(pageIndex)}
            >
              {t('usage.tryAgain')}
            </button>
          </div>
        ) : events.length === 0 ? (
          <div className="px-4 py-4 text-[0.78125rem] text-ink-faint">{t('usage.noRequests')}</div>
        ) : (
          <>
            <div
              className={`max-h-[27rem] overflow-auto transition-opacity duration-150 [scrollbar-gutter:stable] ${loading ? 'opacity-70' : 'opacity-100'}`}
              aria-busy={loading}
            >
              <table className="w-full min-w-[38.125rem] table-fixed border-collapse text-left text-[0.78125rem]">
                <colgroup>
                  <col className="w-[31%]" />
                  <col className="w-[18%]" />
                  <col className="w-[17%]" />
                  <col className="w-[12%]" />
                  <col className="w-[12%]" />
                  <col className="w-[10%]" />
                </colgroup>
                <thead className="sticky top-0 z-10 border-b border-edge/75 bg-canvas-raised text-[0.71875rem] font-medium text-ink-muted">
                  <tr>
                    <th className="bg-canvas-raised px-4 py-2.5">{t('usage.time')}</th>
                    <th className="bg-canvas-raised px-3 py-2.5 text-right">{t('usage.input')}</th>
                    <th className="bg-canvas-raised px-3 py-2.5 text-right">{t('usage.cacheRead')}</th>
                    <th className="bg-canvas-raised px-3 py-2.5 text-right">{t('usage.output')}</th>
                    <th className="bg-canvas-raised px-3 py-2.5 text-right">{t('usage.total')}</th>
                    <th className="bg-canvas-raised px-4 py-2.5 text-right">{t('usage.cost')}</th>
                  </tr>
                </thead>
                <tbody>
                  {events.map((event) => (
                    <RequestRow
                      key={event.id}
                      event={event}
                      locale={locale}
                      formatNumber={formatNumber}
                    />
                  ))}
                </tbody>
              </table>
            </div>
            {pageCount > 1 && (
              <div className="flex h-10 items-center justify-between border-t border-edge/75 px-4 text-[0.71875rem] text-ink-faint tabular-nums">
                <span>
                  {t('usage.pageRange', {
                    start: rangeStart,
                    end: rangeEnd,
                    total,
                  })}
                </span>
                <div className="flex items-center gap-1">
                  <button
                    className="grid size-7 cursor-pointer place-items-center rounded-[8px] text-ink-muted transition-colors hover:bg-surface-active hover:text-ink disabled:cursor-default disabled:opacity-25"
                    type="button"
                    aria-label={t('usage.previousPage')}
                    disabled={loading || pageIndex === 0}
                    onClick={() => setPageIndex((current) => Math.max(0, current - 1))}
                  >
                    <ChevronLeft className="size-3.5" aria-hidden="true" />
                  </button>
                  <span className="min-w-10 text-center text-ink-muted">
                    {t('usage.pageStatus', { page: pageIndex + 1, pages: pageCount })}
                  </span>
                  <button
                    className="grid size-7 cursor-pointer place-items-center rounded-[8px] text-ink-muted transition-colors hover:bg-surface-active hover:text-ink disabled:cursor-default disabled:opacity-25"
                    type="button"
                    aria-label={t('usage.nextPage')}
                    disabled={loading || pageIndex + 1 >= pageCount}
                    onClick={() => setPageIndex((current) => Math.min(pageCount - 1, current + 1))}
                  >
                    <ChevronRight className="size-3.5" aria-hidden="true" />
                  </button>
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}

function RequestRow({
  event,
  locale,
  formatNumber,
}: {
  event: UsageEvent
  locale: string
  formatNumber: (value: number, options?: Intl.NumberFormatOptions) => string
}) {
  const timestamp = new Intl.DateTimeFormat(locale === 'zh-CN' ? 'zh-CN' : 'en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(new Date(event.timestamp))
  const totalTokens = event.usage.totalTokens ||
    event.usage.input + event.usage.output + event.usage.cacheRead + event.usage.cacheWrite
  return (
    <tr className="border-b border-edge-soft transition-colors last:border-b-0 hover:bg-surface-hover">
      <td className="px-4 py-2.5">
        <div className="text-[0.78125rem] text-ink-muted tabular-nums">{timestamp}</div>
      </td>
      <NumberCell value={event.usage.inputUnknown ? '--' : formatNumber(event.usage.input)} />
      <NumberCell value={formatNumber(event.usage.cacheRead)} />
      <NumberCell value={formatNumber(event.usage.output)} />
      <NumberCell value={formatNumber(totalTokens)} strong />
      <td className="px-4 py-2.5 text-right text-[0.78125rem] font-normal text-ink-muted tabular-nums">
        {formatCost(event.usage.cost.total)}
      </td>
    </tr>
  )
}

function modelUsageKey(model: ModelUsageSummary): string {
  return `${model.provider}\u0000${model.model}`
}

function usageRangeSince(range: UsageRange): string {
  if (range === 'all') return ''
  const days = range === '7d' ? 7 : 30
  return new Date(Date.now() - days * 24 * 60 * 60 * 1000).toISOString()
}

function NumberCell({ value, strong = false }: { value: string; strong?: boolean }) {
  return (
    <td
      className={`px-3 py-3 text-right text-[0.78125rem] tabular-nums ${strong ? 'font-medium text-ink-soft' : 'font-normal text-ink-muted'}`}
    >
      {value}
    </td>
  )
}

function formatCost(value: number): string {
  if (value > 0 && value < 0.0001) return '<$0.0001'
  if (value < 1) return `$${value.toFixed(4)}`
  return `$${value.toFixed(2)}`
}

function groupByProvider(models: ModelUsageSummary[]): ProviderUsageGroup[] {
  const groups = new Map<string, ProviderUsageGroup>()
  for (const model of models) {
    let group = groups.get(model.provider)
    if (!group) {
      group = {
        provider: model.provider,
        models: [],
        requests: 0,
        input: 0,
        inputUnknown: false,
        output: 0,
        cacheRead: 0,
        cacheWrite: 0,
        totalTokens: 0,
        cost: emptyCost(),
      }
      groups.set(model.provider, group)
    }
    group.models.push(model)
    group.requests += model.requests
    group.input += model.input
    group.inputUnknown = Boolean(group.inputUnknown || model.inputUnknown)
    group.output += model.output
    group.cacheRead += model.cacheRead
    group.cacheWrite += model.cacheWrite
    group.totalTokens += model.totalTokens
    group.cost.input += model.cost.input
    group.cost.output += model.cost.output
    group.cost.cacheRead += model.cost.cacheRead
    group.cost.cacheWrite += model.cost.cacheWrite
    group.cost.total += model.cost.total
  }
  return [...groups.values()]
    .map((group) => ({
      ...group,
      models: group.models.toSorted((a, b) => b.totalTokens - a.totalTokens),
    }))
    .toSorted((a, b) => b.totalTokens - a.totalTokens)
}

function emptyCost(): UsageCost {
  return { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 }
}
