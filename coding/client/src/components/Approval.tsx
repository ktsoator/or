import { useState } from 'react'
import { Check, LoaderCircle, ShieldAlert, X } from 'lucide-react'
import type { ApprovalChoice, ApprovalItem } from '@/types'
import { useI18n } from '@/i18n'
import { cn } from '@/lib/utils'

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
      className="animate-[fade-in_160ms_ease-out] overflow-hidden rounded-[8px] border border-edge bg-canvas shadow-[0_8px_24px_-22px_rgba(28,25,23,0.5)]"
      data-testid="approval"
      aria-live="polite"
      aria-busy={busy}
    >
      <div className="flex min-h-16 items-center gap-3 px-3.5 py-2.5 max-sm:flex-wrap max-sm:gap-y-2.5">
        <ShieldAlert
          className="size-4 shrink-0 self-start text-warning max-sm:mt-0.5"
          aria-hidden="true"
        />
        <div className="flex min-w-0 flex-1 items-center gap-3 max-sm:basis-[calc(100%_-_1.75rem)]">
          <div className="min-w-0 flex-1">
            <div className="text-[0.8125rem] leading-5 font-semibold text-ink">
              {t('approval.required')}
            </div>
            <code
              className="block min-w-0 overflow-hidden font-mono text-[0.75rem] leading-5 font-normal text-ink-muted text-ellipsis whitespace-nowrap"
              title={item.summary}
            >
              {item.summary || t('approval.noDetails')}
            </code>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1.5 max-sm:ml-7 max-sm:w-[calc(100%_-_1.75rem)] max-sm:justify-end">
          <button
            className="inline-flex h-8 min-w-[4.5rem] cursor-pointer items-center justify-center gap-1.5 rounded-lg px-2.5 text-[0.78125rem] font-medium text-ink-muted outline-none transition-[background-color,color] hover:bg-surface-hover hover:text-ink focus-visible:ring-2 focus-visible:ring-edge-stronger focus-visible:ring-offset-1 focus-visible:ring-offset-canvas disabled:cursor-wait disabled:opacity-50"
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
            className="inline-flex h-8 min-w-[6.5rem] cursor-pointer items-center justify-center gap-1.5 rounded-lg bg-canvas-inverse px-3 text-[0.78125rem] font-medium text-ink-inverse outline-none transition-[opacity,transform] hover:opacity-90 focus-visible:ring-2 focus-visible:ring-edge-stronger focus-visible:ring-offset-2 focus-visible:ring-offset-canvas active:translate-y-px disabled:cursor-wait disabled:opacity-50 motion-reduce:transition-none"
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
        <div className="border-t border-edge bg-canvas-raised/60">
          {compound && (
            <div className="flex min-h-8 items-center gap-1.5 px-3.5 text-[0.71875rem] leading-4 font-medium text-ink-muted">
              <ShieldAlert className="size-3 shrink-0 text-warning" aria-hidden="true" />
              {t('approval.compoundCommand', { count: item.commandSegments })}
            </div>
          )}
          <pre
            className={cn(
              'code-scroll-area max-h-44 overflow-auto bg-canvas px-3.5 py-2.5 font-mono text-[0.75rem] leading-5 whitespace-pre text-ink-soft',
              compound && 'border-t border-edge/70',
            )}
          >
            {item.command}
          </pre>
        </div>
      )}
      {error && (
        <div className="border-t border-danger-edge bg-danger-surface/50 px-3.5 py-2 text-[0.75rem] leading-4 text-danger-soft" role="alert">
          {error}
        </div>
      )}
    </section>
  )
}
