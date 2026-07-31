import { useState } from 'react'
import { ArrowLeft, ArrowRight, ArrowUp, Check, LoaderCircle } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { QuestionAnswer, QuestionItem } from '@/types'
import { useI18n } from '@/i18n'

// The free-text choice is always offered by the product surface, so the agent
// never sends an "other" option of its own.
const OTHER = '__other__'

type Selection = {
  labels: string[]
  other: string
}

function emptySelection(): Selection {
  return { labels: [], other: '' }
}

function answerValues(selection: Selection): string[] {
  const values = selection.labels.filter((label) => label !== OTHER)
  const other = selection.other.trim()
  if (selection.labels.includes(OTHER) && other) values.push(other)
  return values
}

export function Question({
  item,
  onResolve,
}: {
  item: QuestionItem
  onResolve: (id: string, answers: QuestionAnswer[]) => Promise<void>
}) {
  const { t } = useI18n()
  const [selections, setSelections] = useState<Selection[]>(() =>
    item.questions.map(() => emptySelection()),
  )
  // Questions are asked one at a time. The agent still receives every answer in
  // a single reply, so stepping is purely how this surface paces the form.
  const [step, setStep] = useState(0)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const total = item.questions.length
  const question = item.questions[step]
  const selection = selections[step]
  const last = step === total - 1
  const answered = answerValues(selection).length > 0
  const allAnswered = selections.every((entry) => answerValues(entry).length > 0)

  const select = (label: string) => {
    setSelections((current) =>
      current.map((entry, index) => {
        if (index !== step) return entry
        if (!question.multiSelect) return { ...entry, labels: [label] }
        return {
          ...entry,
          labels: entry.labels.includes(label)
            ? entry.labels.filter((existing) => existing !== label)
            : [...entry.labels, label],
        }
      }),
    )
    // A single-select answer is complete the moment it is picked, so move on.
    // The final question never advances on its own: submitting is deliberate.
    if (!question.multiSelect && !last) setStep((current) => current + 1)
  }

  const setOther = (value: string) => {
    setSelections((current) =>
      current.map((entry, index) =>
        index === step
          ? {
              ...entry,
              other: value,
              labels: entry.labels.includes(OTHER) ? entry.labels : [...entry.labels, OTHER],
            }
          : entry,
      ),
    )
  }

  const submit = async () => {
    // Every question must carry a value: a half-filled form would reach the
    // agent as an unanswered question, which it treats as no answer at all.
    if (!allAnswered || busy) return
    setBusy(true)
    setError('')
    try {
      await onResolve(
        item.id,
        item.questions.map((entry, index) => ({
          question: entry.question,
          values: answerValues(selections[index]),
        })),
      )
    } catch {
      setError(t('question.couldNotSend'))
      setBusy(false)
    }
  }

  const advance = () => {
    if (last) return void submit()
    if (answered) setStep((current) => current + 1)
  }

  const indicator = (checked: boolean) => (
    <span
      className={cn(
        'grid size-4 shrink-0 place-items-center border transition-colors',
        question.multiSelect ? 'rounded-[4px]' : 'rounded-full',
        checked ? 'border-canvas-inverse bg-canvas-inverse text-ink-inverse' : 'border-edge-strong bg-canvas',
      )}
      aria-hidden="true"
    >
      {checked && <Check className="size-2.5" strokeWidth={3} />}
    </span>
  )

  const primary = (
    <button
      type="button"
      disabled={(last ? !allAnswered : !answered) || busy}
      onClick={advance}
      aria-label={last ? t('question.submit') : t('question.next')}
      className="grid size-8 shrink-0 cursor-pointer place-items-center rounded-full bg-canvas-inverse text-ink-inverse outline-none transition-colors hover:bg-canvas-inverse focus-visible:bg-canvas-inverse disabled:cursor-not-allowed disabled:opacity-25"
    >
      {busy ? (
        <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
      ) : last ? (
        <ArrowUp className="size-3.5" aria-hidden="true" />
      ) : (
        <ArrowRight className="size-3.5" aria-hidden="true" />
      )}
    </button>
  )

  return (
    <section
      className="animate-[fade-in_160ms_ease-out] overflow-hidden rounded-[28px] border border-edge bg-canvas [container-type:inline-size]"
      aria-live="polite"
      aria-busy={busy}
    >
      <fieldset key={question.question} className="min-w-0">
        <legend className="flex w-full flex-wrap items-baseline gap-2 px-3.5 pt-3 pb-2">
          <span className="rounded bg-canvas-sunken px-1.5 py-0.5 font-mono text-[0.6875rem] leading-4 text-ink-muted">
            {question.header}
          </span>
          <span className="text-[0.875rem] leading-5 font-medium text-ink">
            {question.question}
          </span>
        </legend>

        {question.options.map((option) => {
          const selected = selection.labels.includes(option.label)
          return (
            <button
              key={option.label}
              type="button"
              disabled={busy}
              aria-pressed={selected}
              onClick={() => select(option.label)}
              className="flex w-full cursor-pointer items-start gap-2.5 border-t border-edge/70 px-3.5 py-2.5 text-left transition-colors hover:bg-canvas-raised focus-visible:bg-canvas-raised focus-visible:outline-none disabled:cursor-wait disabled:opacity-60"
            >
              <span className="mt-[0.1875rem]">{indicator(selected)}</span>
              <span className="min-w-0">
                <span
                  className={cn(
                    'block text-[0.8125rem] leading-5',
                    selected ? 'font-medium text-ink' : 'text-ink-soft',
                  )}
                >
                  {option.label}
                </span>
                {option.description && (
                  <span className="block text-[0.78125rem] leading-5 text-ink-muted">
                    {option.description}
                  </span>
                )}
              </span>
            </button>
          )
        })}

        <div className="flex items-center gap-2.5 border-t border-edge/70 px-3.5">
          {indicator(selection.labels.includes(OTHER) && Boolean(selection.other.trim()))}
          <input
            type="text"
            disabled={busy}
            value={selection.other}
            placeholder={t('question.otherPlaceholder')}
            aria-label={t('question.otherPlaceholder')}
            onChange={(event) => setOther(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                event.preventDefault()
                advance()
              }
            }}
            className="w-full min-w-0 border-0 bg-transparent py-2.5 text-[0.8125rem] leading-5 text-ink outline-none placeholder:text-ink-faint disabled:cursor-wait disabled:opacity-60"
          />
          {total === 1 && <span className="my-1.5">{primary}</span>}
        </div>
      </fieldset>

      {total > 1 && (
        <div className="flex items-center gap-2 border-t border-edge/80 px-3.5 py-2">
          <button
            type="button"
            disabled={step === 0 || busy}
            onClick={() => setStep((current) => Math.max(0, current - 1))}
            aria-label={t('question.back')}
            className="grid size-8 shrink-0 cursor-pointer place-items-center rounded-full text-ink-muted outline-none transition-colors hover:bg-canvas-sunken hover:text-ink focus-visible:bg-canvas-sunken focus-visible:text-ink disabled:cursor-not-allowed disabled:opacity-25"
          >
            <ArrowLeft className="size-3.5" aria-hidden="true" />
          </button>
          <span className="font-mono text-[0.6875rem] leading-4 text-ink-faint tabular-nums">
            {step + 1}/{total}
          </span>
          {error && (
            <span
              className="min-w-0 flex-1 truncate text-[0.75rem] leading-4 text-danger-soft"
              role="alert"
            >
              {error}
            </span>
          )}
          <span className={cn('ml-auto', error && 'ml-0')}>{primary}</span>
        </div>
      )}

      {total === 1 && error && (
        <div
          className="border-t border-edge/80 px-3.5 py-2 text-[0.75rem] leading-4 text-danger-soft"
          role="alert"
        >
          {error}
        </div>
      )}
    </section>
  )
}
