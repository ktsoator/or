import { useEffect, useRef, useState } from 'react'
import { Check, Copy, ThumbsDown, ThumbsUp, type LucideIcon } from 'lucide-react'
import { Tooltip } from 'radix-ui'
import type { Usage } from '@/types'
import { cn } from '@/lib/utils'
import { useI18n } from '@/i18n'

export function ResponseActions({
  usage,
  responseText,
}: {
  usage?: Usage
  responseText: string
}) {
  const { t, formatNumber } = useI18n()
  const [copied, setCopied] = useState(false)
  const [feedback, setFeedback] = useState<'up' | 'down'>()
  const resetCopyRef = useRef<number>(undefined)
  const totalTokens = usage
    ? usage.totalTokens || usage.input + usage.output + usage.cacheRead + usage.cacheWrite
    : 0

  useEffect(
    () => () => {
      if (resetCopyRef.current) window.clearTimeout(resetCopyRef.current)
    },
    [],
  )

  const copyResponse = async () => {
    if (!responseText) return
    try {
      await navigator.clipboard.writeText(responseText)
      setCopied(true)
      if (resetCopyRef.current) window.clearTimeout(resetCopyRef.current)
      resetCopyRef.current = window.setTimeout(() => setCopied(false), 1600)
    } catch {
      setCopied(false)
    }
  }

  return (
    <div className="mt-1 flex h-7 animate-[fade-in_160ms_ease-out] items-center gap-0.5">
      <Tooltip.Provider delayDuration={80} skipDelayDuration={100}>
        <ActionButton
          icon={copied ? Check : Copy}
          label={copied ? t('actions.copied') : t('actions.copyResponse')}
          disabled={!responseText}
          onClick={() => void copyResponse()}
        />
        <ActionButton
          icon={ThumbsUp}
          label={t('actions.goodResponse')}
          pressed={feedback === 'up'}
          onClick={() => setFeedback((current) => (current === 'up' ? undefined : 'up'))}
        />
        <ActionButton
          icon={ThumbsDown}
          label={t('actions.badResponse')}
          pressed={feedback === 'down'}
          onClick={() => setFeedback((current) => (current === 'down' ? undefined : 'down'))}
        />

        {usage && hasUsage(usage) && (
          <>
            <span className="mx-1 h-3 w-px bg-stone-200" aria-hidden="true" />
            <Tooltip.Root>
              <Tooltip.Trigger asChild>
                <button
                  className="group inline-flex h-7 cursor-pointer items-center gap-1.5 rounded-lg px-2 text-[12px] leading-5 text-stone-400 tabular-nums outline-none transition-colors hover:bg-[rgb(241,241,241)] hover:text-stone-600 focus-visible:bg-[rgb(241,241,241)] focus-visible:text-stone-600 focus-visible:ring-2 focus-visible:ring-stone-300 data-[state=delayed-open]:bg-[rgb(241,241,241)] data-[state=delayed-open]:text-stone-600"
                  type="button"
                  aria-label={t('actions.showUsage')}
                >
                  <span className="font-[560] text-stone-500">{t('actions.usage')}</span>
                  <span className="text-stone-300">·</span>
                  <span>
                    {formatCompactNumber(totalTokens, formatNumber)} {t('actions.tokens')}
                  </span>
                  {usage.cost.total > 0 && (
                    <>
                      <span className="text-stone-300">·</span>
                      <span>{formatSummaryCost(usage.cost.total)}</span>
                    </>
                  )}
                </button>
              </Tooltip.Trigger>

              <Tooltip.Portal>
                <Tooltip.Content
                  side="bottom"
                  align="start"
                  sideOffset={7}
                  collisionPadding={10}
                  className="z-[150] animate-[fade-in_110ms_ease-out] rounded-[10px] border border-stone-200 bg-white px-3 py-2 text-[12px] leading-5 text-stone-700 tabular-nums shadow-[0_14px_38px_-20px_rgba(28,25,23,0.5)] outline-none"
                >
                  <div className="flex items-center gap-3 whitespace-nowrap">
                    <Metric label={t('actions.input')} value={formatNumber(usage.input)} />
                    <Metric label={t('actions.output')} value={formatNumber(usage.output)} />
                    {usage.cacheRead > 0 && (
                      <Metric label={t('actions.cacheRead')} value={formatNumber(usage.cacheRead)} />
                    )}
                    {usage.cacheWrite > 0 && (
                      <Metric label={t('actions.cacheWrite')} value={formatNumber(usage.cacheWrite)} />
                    )}
                    {usage.cost.total > 0 && (
                      <Metric label={t('actions.cost')} value={formatExactCost(usage.cost.total)} />
                    )}
                  </div>
                </Tooltip.Content>
              </Tooltip.Portal>
            </Tooltip.Root>
          </>
        )}
      </Tooltip.Provider>
    </div>
  )
}

function ActionButton({
  icon: Icon,
  label,
  pressed,
  disabled,
  onClick,
}: {
  icon: LucideIcon
  label: string
  pressed?: boolean
  disabled?: boolean
  onClick: () => void
}) {
  return (
    <Tooltip.Root>
      <Tooltip.Trigger asChild>
        <button
          className={cn(
            'grid size-7 cursor-pointer place-items-center rounded-lg text-stone-400 outline-none transition-colors hover:bg-[rgb(241,241,241)] hover:text-stone-700 focus-visible:ring-2 focus-visible:ring-stone-300 disabled:cursor-not-allowed disabled:opacity-30',
            pressed && 'bg-[rgb(241,241,241)] text-stone-800',
          )}
          type="button"
          aria-label={label}
          aria-pressed={pressed}
          disabled={disabled}
          onClick={onClick}
        >
          <Icon className="size-[15px]" aria-hidden="true" />
        </button>
      </Tooltip.Trigger>
      <Tooltip.Portal>
        <Tooltip.Content
          side="bottom"
          sideOffset={6}
          collisionPadding={8}
          className="z-[150] animate-[fade-in_100ms_ease-out] rounded-md bg-stone-900 px-2 py-1 text-[11px] leading-4 font-medium whitespace-nowrap text-white shadow-lg"
        >
          {label}
        </Tooltip.Content>
      </Tooltip.Portal>
    </Tooltip.Root>
  )
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <span className="flex items-baseline gap-1">
      <span className="text-stone-400">{label}</span>
      <span className="font-[540] text-stone-700">{value}</span>
    </span>
  )
}

function hasUsage(usage: Usage): boolean {
  return (
    usage.input !== 0 ||
    usage.output !== 0 ||
    usage.cacheRead !== 0 ||
    usage.cacheWrite !== 0 ||
    usage.totalTokens !== 0 ||
    usage.cost.total !== 0
  )
}

type NumberFormatter = (value: number, options?: Intl.NumberFormatOptions) => string

function formatCompactNumber(value: number, formatNumber: NumberFormatter): string {
  if (value >= 1_000_000) return `${formatDecimal(value / 1_000_000, formatNumber)}m`
  if (value >= 1_000) return `${formatDecimal(value / 1_000, formatNumber)}k`
  return formatNumber(Math.round(value))
}

function formatDecimal(value: number, formatNumber: NumberFormatter): string {
  return formatNumber(value, { maximumFractionDigits: value >= 100 ? 0 : 1 })
}

function formatSummaryCost(value: number): string {
  if (value < 0.0001) return '<$0.0001'
  return formatExactCost(value)
}

function formatExactCost(value: number): string {
  const digits = value >= 1 ? 2 : value >= 0.01 ? 3 : value >= 0.0001 ? 4 : 6
  return `$${value.toFixed(digits)}`
}
