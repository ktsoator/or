import { useState } from 'react'
import { Check, LoaderCircle, ShieldAlert, X } from 'lucide-react'
import type { ApprovalChoice, ApprovalItem } from '@/types'
import { useI18n } from '@/i18n'

export function Approval({
  item,
  onResolve,
}: {
  item: ApprovalItem
  onResolve: (id: string, choice: ApprovalChoice) => Promise<void>
}) {
  const { t } = useI18n()
  const [decision, setDecision] = useState<ApprovalChoice>()
  const [error, setError] = useState('')
  const busy = decision !== undefined
  // The summary is a single line. Show the command in full whenever that line
  // cannot carry all of it, so nothing runs that the user did not read.
  const compound = item.commandSegments > 1
  const showCommand = item.command !== '' && (compound || item.command.includes('\n'))

  const decide = async (choice: ApprovalChoice) => {
    setDecision(choice)
    setError('')
    try {
      await onResolve(item.id, choice)
    } catch {
      setError(t('approval.couldNotSend'))
      setDecision(undefined)
    }
  }

  return (
    <section
      className="min-h-24 animate-[fade-in_160ms_ease-out] rounded-[28px] border border-edge bg-canvas px-4 py-2.5 shadow-[0_10px_30px_-28px_rgba(28,25,23,0.55)] max-sm:px-3.5 max-sm:py-3"
      aria-live="polite"
      aria-busy={busy}
    >
      <div className="flex min-h-[4.5rem] items-center gap-4 max-sm:flex-col max-sm:items-stretch max-sm:justify-center max-sm:gap-3">
        <div className="flex min-w-0 flex-1 items-center gap-3">
          {/* A neutral chip rather than a tinted disc: the amber fill and ring
              muddy against a dark canvas, and the buttons already carry the
              row's weight. Colour stays on the glyph alone. */}
          <div className="grid size-9 shrink-0 place-items-center rounded-full bg-canvas-sunken text-warning">
            <ShieldAlert className="size-[1.0625rem]" aria-hidden="true" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="text-[0.875rem] leading-5 font-semibold text-ink">
              {t('approval.required')}
            </div>
            <code
              className="mt-0.5 block min-w-0 overflow-hidden font-mono text-[0.78125rem] leading-5 font-normal text-ink-muted text-ellipsis whitespace-nowrap"
              title={item.summary}
            >
              {item.summary || t('approval.noDetails')}
            </code>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2 max-sm:w-full">
          <button
            className="inline-flex h-9 min-w-[5rem] cursor-pointer items-center justify-center gap-1.5 rounded-xl border border-edge bg-canvas px-3 text-[0.8125rem] font-medium text-ink-muted outline-none transition-[background-color,border-color,color] hover:border-edge-strong hover:bg-canvas-raised hover:text-ink focus-visible:border-edge-strong focus-visible:bg-canvas-raised focus-visible:text-ink disabled:cursor-wait disabled:opacity-50 max-sm:flex-1"
            type="button"
            disabled={busy}
            onClick={() => decide('deny')}
          >
            {decision === 'deny' ? (
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
            ) : (
              <X className="size-3.5" aria-hidden="true" />
            )}
            {t('approval.deny')}
          </button>
          <button
            className="inline-flex h-9 min-w-[7rem] cursor-pointer items-center justify-center gap-1.5 rounded-xl border border-canvas-inverse bg-canvas-inverse px-3.5 text-[0.8125rem] font-medium text-ink-inverse outline-none transition-[background-color,border-color] hover:border-canvas-inverse hover:bg-canvas-inverse focus-visible:border-canvas-inverse focus-visible:bg-canvas-inverse disabled:cursor-wait disabled:opacity-50 max-sm:flex-1"
            type="button"
            disabled={busy}
            onClick={() => decide('allow_once')}
          >
            {decision === 'allow_once' ? (
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
            ) : (
              <Check className="size-3.5" aria-hidden="true" />
            )}
            {t('approval.allowOnce')}
          </button>
        </div>
      </div>
      {showCommand && (
        <div className="mt-1 mb-1.5 rounded-2xl border border-edge bg-canvas-raised">
          {compound && (
            <div className="flex items-center gap-1.5 border-b border-edge px-3 py-1.5 text-[0.75rem] leading-4 font-medium text-warning">
              <ShieldAlert className="size-3.5 shrink-0" aria-hidden="true" />
              {t('approval.compoundCommand', { count: item.commandSegments })}
            </div>
          )}
          <pre className="code-scroll-area max-h-[13rem] overflow-auto px-3 py-2 font-mono text-[0.78125rem] leading-5 whitespace-pre text-ink-soft">
            {item.command}
          </pre>
        </div>
      )}
      {error && (
        <div className="border-t border-danger-edge pt-2 text-[0.75rem] leading-4 text-danger-soft" role="alert">
          {error}
        </div>
      )}
    </section>
  )
}
