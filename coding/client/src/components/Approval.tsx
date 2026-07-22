import { useState } from 'react'
import { Check, LoaderCircle, ShieldAlert, X } from 'lucide-react'
import type { ConfirmItem } from '@/types'
import { useI18n } from '@/i18n'

export function Approval({
  item,
  onResolve,
}: {
  item: ConfirmItem
  onResolve: (id: string, allow: boolean) => Promise<void>
}) {
  const { t } = useI18n()
  const [decision, setDecision] = useState<'allow' | 'deny'>()
  const [error, setError] = useState('')
  const busy = decision !== undefined

  const decide = async (allow: boolean) => {
    setDecision(allow ? 'allow' : 'deny')
    setError('')
    try {
      await onResolve(item.id, allow)
    } catch {
      setError(t('approval.couldNotSend'))
      setDecision(undefined)
    }
  }

  return (
    <section
      className="min-h-24 animate-[fade-in_160ms_ease-out] rounded-[28px] border border-stone-200 bg-white px-4 py-2.5 shadow-[0_10px_30px_-28px_rgba(28,25,23,0.55)] max-sm:px-3.5 max-sm:py-3"
      aria-live="polite"
      aria-busy={busy}
    >
      <div className="flex min-h-[4.5rem] items-center gap-4 max-sm:flex-col max-sm:items-stretch max-sm:justify-center max-sm:gap-3">
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <div className="grid size-9 shrink-0 place-items-center rounded-full bg-amber-50 text-amber-700 ring-1 ring-amber-200/70">
            <ShieldAlert className="size-4" aria-hidden="true" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="text-[0.875rem] leading-5 font-semibold text-stone-900">
              {t('approval.required')}
            </div>
            <code
              className="mt-0.5 block min-w-0 overflow-hidden font-mono text-[0.78125rem] leading-5 font-normal text-stone-500 text-ellipsis whitespace-nowrap"
              title={item.summary}
            >
              {item.summary || t('approval.noDetails')}
            </code>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2 max-sm:w-full">
          <button
            className="inline-flex h-9 min-w-[5rem] cursor-pointer items-center justify-center gap-1.5 rounded-xl border border-stone-200 bg-white px-3 text-[0.8125rem] font-medium text-stone-600 outline-none transition-[background-color,border-color,color] hover:border-stone-300 hover:bg-stone-50 hover:text-stone-950 focus-visible:ring-2 focus-visible:ring-stone-300 disabled:cursor-wait disabled:opacity-50 max-sm:flex-1"
            type="button"
            disabled={busy}
            onClick={() => decide(false)}
          >
            {decision === 'deny' ? (
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
            ) : (
              <X className="size-3.5" aria-hidden="true" />
            )}
            {t('approval.deny')}
          </button>
          <button
            className="inline-flex h-9 min-w-[7rem] cursor-pointer items-center justify-center gap-1.5 rounded-xl border border-stone-900 bg-stone-900 px-3.5 text-[0.8125rem] font-medium text-white outline-none transition-[background-color,border-color] hover:border-black hover:bg-black focus-visible:ring-2 focus-visible:ring-stone-400 focus-visible:ring-offset-2 disabled:cursor-wait disabled:opacity-50 max-sm:flex-1"
            type="button"
            disabled={busy}
            onClick={() => decide(true)}
          >
            {decision === 'allow' ? (
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
            ) : (
              <Check className="size-3.5" aria-hidden="true" />
            )}
            {t('approval.allowOnce')}
          </button>
        </div>
      </div>
      {error && (
        <div className="border-t border-red-100 pt-2 text-[0.75rem] leading-4 text-red-600" role="alert">
          {error}
        </div>
      )}
    </section>
  )
}
