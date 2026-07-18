import { useState } from 'react'
import { ShieldAlert } from 'lucide-react'
import type { ConfirmItem } from '@/types'

export function Approval({
  item,
  onResolve,
}: {
  item: ConfirmItem
  onResolve: (id: string, allow: boolean) => Promise<void>
}) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const decide = async (allow: boolean) => {
    setBusy(true)
    setError('')
    try {
      await onResolve(item.id, allow)
    } catch {
      setError('Could not send decision')
      setBusy(false)
    }
  }

  return (
    <section
      className="animate-[fade-in_160ms_ease-out] rounded-xl border border-stone-300 bg-[#fffefa] px-3.5 py-3 shadow-[0_8px_24px_-20px_rgba(28,25,23,0.65)]"
      aria-live="polite"
    >
      <div className="flex items-center gap-3 max-sm:flex-wrap">
        <ShieldAlert className="size-4 shrink-0 text-amber-700" aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <div className="text-[15px] font-semibold text-stone-900">Approval required</div>
          <code
            className="mt-1 block overflow-hidden font-mono text-[13.5px] leading-5.5 text-stone-500 text-ellipsis whitespace-nowrap"
            title={item.summary}
          >
            {item.summary || 'No details provided'}
          </code>
        </div>
        <div className="flex shrink-0 items-center gap-2 max-sm:ml-7">
          <button
            className="h-8 cursor-pointer rounded-md border border-stone-300 bg-white px-3 text-[13px] font-semibold text-stone-700 transition-colors hover:border-stone-400 hover:bg-stone-50 disabled:cursor-wait disabled:opacity-50"
            type="button"
            disabled={busy}
            onClick={() => decide(false)}
          >
            Deny
          </button>
          <button
            className="h-8 cursor-pointer rounded-md bg-stone-800 px-3 text-[13px] font-semibold text-white transition-colors hover:bg-stone-950 disabled:cursor-wait disabled:opacity-50"
            type="button"
            disabled={busy}
            onClick={() => decide(true)}
          >
            Allow once
          </button>
        </div>
      </div>
      {error && <div className="mt-2 ml-7 text-[13px] text-red-600">{error}</div>}
    </section>
  )
}
