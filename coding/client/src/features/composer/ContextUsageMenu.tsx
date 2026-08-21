import { LoaderCircle, Minimize2 } from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import type { ContextUsage } from '@/types'
import { cn } from '@/lib/utils'
import { useI18n } from '@/i18n'
import { ComposerControlTooltip } from './ComposerControlTooltip'

export function ContextUsageMenu({
  usage,
  contextWindow,
  disabled,
  compacting,
  compactDisabled,
  onCompact,
}: {
  usage?: ContextUsage
  contextWindow: number
  disabled: boolean
  compacting: boolean
  compactDisabled: boolean
  onCompact?: () => void
}) {
  const { t, formatNumber } = useI18n()
  const measured = Boolean(usage?.measured && usage.usedTokens > 0 && contextWindow > 0)
  const usedTokens = measured ? usage?.usedTokens ?? 0 : 0
  const percentage = measured ? Math.min((usedTokens / contextWindow) * 100, 100) : 0
  const breakdown = measured ? usage?.breakdown : undefined
  const rows = breakdown
    ? [
        {
          key: 'messages',
          label: t('model.contextMessages'),
          tokens: breakdown.messages,
          swatch: 'bg-blue-500',
        },
        {
          key: 'tools',
          label: t('model.contextSystemTools'),
          tokens: breakdown.systemTools,
          swatch: 'bg-sky-400',
        },
        {
          key: 'prompt',
          label: t('model.contextSystemPrompt'),
          tokens: breakdown.systemPrompt,
          swatch: 'bg-violet-400',
        },
        {
          key: 'skills',
          label: t('model.contextSkills'),
          tokens: breakdown.skills,
          swatch: 'bg-emerald-500',
        },
        {
          key: 'project',
          label: t('model.contextProject'),
          tokens: breakdown.projectContext,
          swatch: 'bg-amber-500',
        },
        {
          key: 'free',
          label: t('model.contextFree'),
          tokens: Math.max(contextWindow - usedTokens, 0),
          swatch: 'bg-canvas-strong',
        },
      ]
    : []
  const usageLabel = measured
    ? `${t('model.context')} · ${formatNumber(Math.round(percentage))}%`
    : t('model.context')

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          data-testid="context-window-trigger"
          type="button"
          className="group relative grid size-[30px] shrink-0 cursor-pointer place-items-center rounded-full text-ink-muted outline-none transition-colors hover:bg-surface-active focus-visible:bg-surface-active data-[state=open]:bg-surface-selected disabled:cursor-not-allowed disabled:opacity-40"
          aria-label={t('model.contextUsage')}
          disabled={disabled}
        >
          <ContextRing measured={measured} percentage={percentage} />
          <ComposerControlTooltip align="end">
            {usageLabel}
          </ComposerControlTooltip>
        </button>
      </DropdownMenu.Trigger>

      <DropdownMenu.Portal>
        <DropdownMenu.Content
          side="top"
          align="end"
          sideOffset={2}
          collisionPadding={10}
          className="z-[100] w-[19rem] max-w-[calc(100vw-1.25rem)] animate-[fade-in_110ms_ease-out] rounded-2xl border border-edge bg-canvas p-1 text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
          data-testid="context-window-panel"
        >
          <div className="px-2.5 pt-2 pb-2" aria-label={t('model.contextUsage')}>
            <div
              className="flex items-baseline gap-3 text-[0.875rem] leading-5 tabular-nums"
              data-testid="context-window-summary"
            >
              <span className="font-medium text-ink-muted">{t('model.context')}</span>
              <span className="ml-auto text-ink-soft">
                {measured ? formatTokens(usedTokens, formatNumber) : '—'} /{' '}
                {formatTokens(contextWindow, formatNumber)}
                {measured && <span className="text-ink-muted"> · {formatNumber(Math.round(percentage))}%</span>}
              </span>
            </div>
            <div className="mt-2 flex h-1 overflow-hidden rounded-full bg-canvas-sunken">
              {rows.length > 0
                ? rows.slice(0, -1).map((row) => row.tokens > 0 && (
                    <span
                      key={row.key}
                      className={cn('h-full', row.swatch)}
                      style={{ width: `${(row.tokens / contextWindow) * 100}%` }}
                      aria-hidden="true"
                    />
                  ))
                : measured && (
                    <span
                      className={cn(
                        'h-full rounded-full',
                        percentage >= 90
                          ? 'bg-danger-soft'
                          : percentage >= 75
                            ? 'bg-warning'
                            : 'bg-ink-muted',
                      )}
                      style={{ width: `${percentage}%` }}
                      aria-hidden="true"
                    />
                  )}
            </div>

            {rows.length > 0 ? (
              <div className="mt-3" data-testid="context-window-breakdown">
                <div className="mb-1 text-[0.75rem] leading-4 font-medium text-ink-faint">
                  {t('model.contextBreakdown')}
                </div>
                <div className="space-y-0.5">
                  {rows.map((row) => (
                    <div
                      key={row.key}
                      className="grid h-6 grid-cols-[minmax(0,1fr)_4rem_3.25rem] items-center gap-2 text-[0.875rem] leading-5 tabular-nums"
                      data-testid={`context-breakdown-${row.key}`}
                    >
                      <span className="flex min-w-0 items-center gap-2 text-ink">
                        <span
                          className={cn('size-2.5 shrink-0 rounded-[2px]', row.swatch)}
                          aria-hidden="true"
                        />
                        <span className="truncate">{row.label}</span>
                      </span>
                      <span className="text-right text-ink-faint">
                        {formatTokens(row.tokens, formatNumber)}
                      </span>
                      <span className="text-right text-ink-muted">
                        {formatContextPercentage(row.tokens, contextWindow, formatNumber)}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            ) : contextWindow > 0 ? (
              <p className="mt-2.5 mb-0 text-[0.6875rem] leading-4 text-ink-faint">
                {t('model.measureAfterResponse')}
              </p>
            ) : null}
          </div>

          {onCompact && (
            <>
              <DropdownMenu.Separator className="mx-1.5 my-0.5 h-px bg-canvas-sunken" />
              <DropdownMenu.Item
                className="flex h-[30px] cursor-default select-none items-center gap-2.5 rounded-[10px] px-2.5 text-[0.875rem] outline-none data-[highlighted]:bg-surface-active data-[disabled]:opacity-40"
                disabled={compacting || compactDisabled}
                onSelect={onCompact}
              >
                {compacting ? (
                  <LoaderCircle className="size-4 animate-spin text-ink-muted" aria-hidden="true" />
                ) : (
                  <Minimize2 className="size-4 text-ink-muted" aria-hidden="true" />
                )}
                <span>{compacting ? t('model.compacting') : t('model.compact')}</span>
              </DropdownMenu.Item>
            </>
          )}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

function ContextRing({ measured, percentage }: { measured: boolean; percentage: number }) {
  return (
    <svg
      viewBox="0 0 20 20"
      className="size-[18px] -rotate-90"
      aria-hidden="true"
    >
      <circle
        cx="10"
        cy="10"
        r="7.25"
        fill="none"
        stroke="currentColor"
        strokeWidth="2.5"
        className="text-canvas-strong"
      />
      {measured && (
        <circle
          cx="10"
          cy="10"
          r="7.25"
          pathLength="100"
          fill="none"
          stroke="currentColor"
          strokeWidth="2.5"
          strokeLinecap="round"
          strokeDasharray={`${Math.max(percentage, 1)} 100`}
          className={cn(
            'transition-[stroke-dasharray,color] duration-300 ease-out',
            percentage >= 90
              ? 'text-danger-soft'
              : percentage >= 75
                ? 'text-warning'
                : 'text-ink-muted',
          )}
          data-testid="context-window-ring"
        />
      )}
    </svg>
  )
}

type NumberFormatter = (value: number, options?: Intl.NumberFormatOptions) => string

function formatTokens(value: number, formatNumber: NumberFormatter): string {
  if (value <= 0) return '—'
  if (value >= 1_000_000) return `${formatTokenDecimal(value / 1_000_000, formatNumber)}m`
  if (value >= 1_000) return `${formatTokenDecimal(value / 1_000, formatNumber)}k`
  return formatNumber(Math.round(value))
}

function formatTokenDecimal(value: number, formatNumber: NumberFormatter): string {
  return formatNumber(value, { maximumFractionDigits: value >= 100 ? 0 : 1 })
}

function formatContextPercentage(
  tokens: number,
  contextWindow: number,
  formatNumber: NumberFormatter,
): string {
  if (contextWindow <= 0) return '—'
  return `${formatNumber((tokens / contextWindow) * 100, {
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
  })}%`
}
