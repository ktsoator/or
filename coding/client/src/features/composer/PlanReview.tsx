import { useState } from 'react'
import { Check, ListChecks, LoaderCircle, MessageSquareText } from 'lucide-react'
import type { QuestionAnswer, QuestionItem } from '@/types'
import { useI18n } from '@/i18n'
import { Markdown } from '@/shared/ui/Markdown'

const APPROVE = 'Approve'
const KEEP_PLANNING = 'Keep planning'

export function PlanReview({
  item,
  onResolve,
}: {
  item: QuestionItem
  onResolve: (id: string, answers: QuestionAnswer[]) => Promise<void>
}) {
  const { t } = useI18n()
  const question = item.questions[0]
  const [feedback, setFeedback] = useState('')
  const [decision, setDecision] = useState<'approve' | 'continue'>()
  const [error, setError] = useState('')
  const busy = decision !== undefined

  if (!question) return null

  const resolve = async (next: 'approve' | 'continue') => {
    if (busy) return
    setDecision(next)
    setError('')
    try {
      await onResolve(item.id, [{
        question: question.question,
        values: [next === 'approve' ? APPROVE : feedback.trim() || KEEP_PLANNING],
      }])
    } catch {
      setError(t('planReview.couldNotSend'))
      setDecision(undefined)
    }
  }

  return (
    <section
      data-testid="plan-review"
      className="animate-[fade-in_160ms_ease-out] overflow-hidden rounded-[24px] border border-edge-strong bg-canvas shadow-[0_18px_48px_-34px_rgba(28,25,23,0.5),0_2px_8px_-6px_rgba(28,25,23,0.28)]"
      aria-live="polite"
      aria-busy={busy}
    >
      <header className="flex items-center gap-2.5 border-b border-edge/80 bg-canvas-raised/45 px-4 py-3.5 max-sm:px-3.5">
        <ListChecks className="size-4 shrink-0 text-ink-muted" aria-hidden="true" />
        <div className="min-w-0">
          <h2 className="m-0 text-[0.875rem] leading-5 font-semibold text-ink">
            {t('planReview.title')}
          </h2>
          <p className="m-0 text-[0.75rem] leading-4 text-ink-muted">
            {t('planReview.description')}
          </p>
        </div>
      </header>

      <div className="max-h-[min(42vh,28rem)] overflow-y-auto px-4 py-3.5 max-sm:px-3.5">
        <Markdown source={question.detail ?? ''} />
      </div>

      <div className="border-t border-edge/80 px-3.5 py-3">
        <label className="flex items-start gap-2.5 rounded-lg border border-edge bg-canvas-sunken/45 px-3 py-2.5 focus-within:border-edge-strong">
          <MessageSquareText className="mt-1 size-3.5 shrink-0 text-ink-faint" aria-hidden="true" />
          <textarea
            rows={2}
            value={feedback}
            disabled={busy}
            onChange={(event) => setFeedback(event.target.value)}
            placeholder={t('planReview.feedbackPlaceholder')}
            aria-label={t('planReview.feedbackPlaceholder')}
            className="max-h-28 min-h-10 w-full resize-y border-0 bg-transparent text-[0.8125rem] leading-5 text-ink outline-none placeholder:text-ink-faint disabled:cursor-wait disabled:opacity-60"
          />
        </label>
        {error && (
          <p className="m-0 mt-2 text-[0.75rem] leading-4 text-danger-soft" role="alert">
            {error}
          </p>
        )}
        <div className="mt-3 flex items-center justify-end gap-2">
          <button
            type="button"
            disabled={busy}
            onClick={() => void resolve('continue')}
            className="inline-flex h-9 cursor-pointer items-center justify-center gap-1.5 rounded-full border border-edge-strong bg-canvas px-3.5 text-[0.8125rem] font-medium text-ink-soft outline-none transition-colors hover:bg-surface-hover hover:text-ink focus-visible:ring-2 focus-visible:ring-edge-stronger disabled:cursor-wait disabled:opacity-50"
          >
            {decision === 'continue' && (
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
            )}
            {t('planReview.keepPlanning')}
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={() => void resolve('approve')}
            className="inline-flex h-9 cursor-pointer items-center justify-center gap-1.5 rounded-full bg-canvas-inverse px-4 text-[0.8125rem] font-medium text-ink-inverse outline-none transition-opacity hover:opacity-90 focus-visible:ring-2 focus-visible:ring-edge-stronger focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:cursor-wait disabled:opacity-50"
          >
            {decision === 'approve' ? (
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
            ) : (
              <Check className="size-3.5" aria-hidden="true" />
            )}
            {t('planReview.approve')}
          </button>
        </div>
      </div>
    </section>
  )
}
