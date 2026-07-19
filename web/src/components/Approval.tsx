import { useState } from 'react'
import { ShieldAlert } from 'lucide-react'
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
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const decide = async (allow: boolean) => {
    setBusy(true)
    setError('')
    try {
      await onResolve(item.id, allow)
    } catch {
      setError(t('approval.couldNotSend'))
      setBusy(false)
    }
  }

  return (
    <section
      className="animate-[fade-in_160ms_ease-out] rounded-[16px] border border-stone-200 bg-[rgb(252,252,252)] px-3 py-2"
      aria-live="polite"
    >
      <div className="flex min-h-8 items-center gap-2.5">
        <ShieldAlert className="size-3.5 shrink-0 text-amber-700" aria-hidden="true" />
        <div className="flex min-w-0 flex-1 items-center gap-2.5 max-sm:flex-col max-sm:items-start max-sm:gap-0.5">
          <div className="shrink-0 text-[0.8125rem] font-medium text-stone-800">
            {t('approval.required')}
          </div>
          <code
            className="min-w-0 flex-1 overflow-hidden font-mono text-[0.75rem] leading-5 font-normal text-stone-500 text-ellipsis whitespace-nowrap"
            title={item.summary}
          >
            {item.summary || t('approval.noDetails')}
          </code>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <button
            className="h-7 cursor-pointer rounded-lg border-0 bg-transparent px-2.5 text-[0.75rem] font-medium text-stone-500 transition-colors hover:bg-stone-200/70 hover:text-stone-900 focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-stone-400 disabled:cursor-wait disabled:opacity-50"
            type="button"
            disabled={busy}
            onClick={() => decide(false)}
          >
            {t('approval.deny')}
          </button>
          <button
            className="h-7 cursor-pointer rounded-lg border-0 bg-stone-900 px-2.5 text-[0.75rem] font-medium text-white transition-colors hover:bg-black focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-stone-500 disabled:cursor-wait disabled:opacity-50"
            type="button"
            disabled={busy}
            onClick={() => decide(true)}
          >
            {t('approval.allowOnce')}
          </button>
        </div>
      </div>
      {error && <div className="mt-1 ml-6 text-[0.71875rem] leading-4 text-red-600">{error}</div>}
    </section>
  )
}
