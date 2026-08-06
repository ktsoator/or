import { useState } from 'react'
import {
  LoaderCircle,
  ShieldCheck,
  SquareTerminal,
} from 'lucide-react'
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
  const compound = item.commandSegments > 1
  const command = item.command.trim()
  const summary = item.summary.trim()
  const TypeIcon = command ? SquareTerminal : ShieldCheck

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
      className="animate-[fade-in_160ms_ease-out] rounded-[24px] border border-edge-strong bg-canvas px-4 py-3.5 shadow-[0_12px_32px_-28px_rgba(28,25,23,0.48)] max-sm:rounded-[20px] max-sm:px-3.5"
      data-testid="approval"
      aria-live="polite"
      aria-busy={busy}
    >
      <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-[0.8125rem] leading-5 text-ink-muted">
        <TypeIcon className="size-4 shrink-0" aria-hidden="true" />
        <span className="font-medium">
          {t(command ? 'approval.terminal' : 'approval.permission')}
        </span>
        {compound && (
          <span className="text-[0.75rem] text-ink-faint">
            {t('approval.compoundCommand', { count: item.commandSegments })}
          </span>
        )}
      </div>
      <h2 className="m-0 mt-3 text-[0.9375rem] leading-6 font-semibold text-ink">
        {t(command ? 'approval.allowCommandQuestion' : 'approval.allowActionQuestion')}
      </h2>
      {command ? (
        <pre
          className="code-scroll-area mt-3 mb-0 max-h-36 overflow-auto font-mono text-[0.875rem] leading-6 whitespace-pre-wrap text-ink-soft"
          tabIndex={0}
        >
          {command}
        </pre>
      ) : (
        <div className="mt-3">
          <code className="font-mono text-[0.875rem] leading-6 break-words text-ink-soft">
            {summary || t('approval.noDetails')}
          </code>
        </div>
      )}
      {error && (
        <div className="mt-3 rounded-md bg-danger-surface/60 px-3 py-1.5 text-[0.75rem] leading-4 text-danger-soft" role="alert">
          {error}
        </div>
      )}
      <div className="mt-4 flex min-h-9 items-center justify-end gap-2">
        <button
          className="inline-flex h-9 min-w-[4.5rem] cursor-pointer items-center justify-center gap-1.5 rounded-full border border-edge-strong bg-canvas px-3.5 text-[0.8125rem] font-medium text-ink-soft outline-none transition-[background-color,border-color,color] hover:border-edge-stronger hover:bg-surface-hover hover:text-ink focus-visible:ring-2 focus-visible:ring-edge-stronger focus-visible:ring-offset-1 focus-visible:ring-offset-canvas disabled:cursor-wait disabled:opacity-50"
          type="button"
          disabled={busy}
          onClick={() => decide('deny')}
        >
          {decision === 'deny' && (
            <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
          )}
          {t('approval.deny')}
        </button>
        <button
          className="inline-flex h-9 min-w-[6.5rem] cursor-pointer items-center justify-center gap-1.5 rounded-full bg-canvas-inverse px-4 text-[0.8125rem] font-medium text-ink-inverse outline-none transition-[opacity,transform] hover:opacity-90 focus-visible:ring-2 focus-visible:ring-edge-stronger focus-visible:ring-offset-2 focus-visible:ring-offset-canvas active:translate-y-px disabled:cursor-wait disabled:opacity-50 motion-reduce:transition-none"
          type="button"
          disabled={busy}
          onClick={() => decide('allow_once')}
        >
          {decision === 'allow_once' && (
            <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
          )}
          {t('approval.allowOnce')}
        </button>
      </div>
    </section>
  )
}
