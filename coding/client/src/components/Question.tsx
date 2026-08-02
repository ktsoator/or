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

  const activateOther = () => {
    setSelections((current) =>
      current.map((entry, index) => {
        if (index !== step || entry.labels.includes(OTHER)) return entry
        return {
          ...entry,
          labels: question.multiSelect ? [...entry.labels, OTHER] : [OTHER],
        }
      }),
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
        'grid size-[1.125rem] shrink-0 place-items-center border transition-[background-color,border-color,box-shadow]',
        question.multiSelect ? 'rounded-[5px]' : 'rounded-full',
        checked
          ? 'border-canvas-inverse bg-canvas-inverse text-ink-inverse shadow-[0_2px_6px_-3px_rgba(28,25,23,0.65)]'
          : 'border-edge-strong bg-canvas shadow-[inset_0_1px_1px_rgba(28,25,23,0.04)]',
      )}
      aria-hidden="true"
    >
      {checked &&
        (question.multiSelect ? (
          <Check className="size-2.5" strokeWidth={3} />
        ) : (
          <span className="size-1.5 rounded-full bg-current" />
        ))}
    </span>
  )

  const primary = (
    <button
      type="button"
      disabled={(last ? !allAnswered : !answered) || busy}
      onClick={advance}
      aria-label={last ? t('question.submit') : t('question.next')}
      className="grid size-9 shrink-0 cursor-pointer place-items-center rounded-full bg-canvas-inverse text-ink-inverse shadow-[0_5px_12px_-7px_rgba(28,25,23,0.75)] outline-none transition-[opacity,transform,box-shadow] hover:-translate-y-px hover:shadow-[0_7px_16px_-8px_rgba(28,25,23,0.8)] focus-visible:ring-2 focus-visible:ring-edge-stronger focus-visible:ring-offset-2 focus-visible:ring-offset-canvas active:translate-y-0 disabled:cursor-not-allowed disabled:opacity-25 disabled:shadow-none motion-reduce:transition-none"
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
      data-testid="question"
      className="animate-[fade-in_160ms_ease-out] overflow-hidden rounded-[24px] border border-edge-strong/80 bg-canvas shadow-[0_18px_48px_-34px_rgba(28,25,23,0.5),0_2px_8px_-6px_rgba(28,25,23,0.28)] [container-type:inline-size]"
      aria-live="polite"
      aria-busy={busy}
    >
      <fieldset key={question.question} className="min-w-0">
        <legend className="flex w-full flex-wrap items-center gap-x-2.5 gap-y-1 border-b border-edge/80 bg-canvas-raised/45 px-4 py-3.5 max-sm:px-3.5">
          <span className="rounded-md border border-edge/70 bg-canvas-sunken/80 px-2 py-0.5 text-[0.6875rem] leading-4 font-medium text-ink-muted">
            {question.header}
          </span>
          <span className="text-[0.875rem] leading-5 font-semibold text-ink">
            {question.question}
          </span>
        </legend>

        <div className="space-y-0.5 p-1.5">
          {question.options.map((option) => {
            const selected = selection.labels.includes(option.label)
            return (
              <button
                key={option.label}
                type="button"
                disabled={busy}
                aria-pressed={selected}
                onClick={() => select(option.label)}
                className={cn(
                  'flex min-h-[3.375rem] w-full cursor-pointer items-start gap-3 rounded-[12px] px-3 py-2.5 text-left outline-none transition-[background-color,box-shadow] disabled:cursor-wait disabled:opacity-60',
                  selected
                    ? 'bg-surface-selected shadow-[inset_0_0_0_1px_var(--edge)]'
                    : 'hover:bg-surface-hover focus-visible:bg-surface-active',
                )}
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
                    <span className="mt-0.5 block text-[0.75rem] leading-[1.125rem] text-ink-muted">
                      {option.description}
                    </span>
                  )}
                </span>
              </button>
            )
          })}
        </div>

        <div className="border-t border-edge/80 p-1.5">
          <label
            className={cn(
              'flex min-h-10 items-center gap-3 rounded-[12px] px-3 transition-[background-color,box-shadow]',
              selection.labels.includes(OTHER)
                ? 'bg-surface-selected shadow-[inset_0_0_0_1px_var(--edge)]'
                : 'hover:bg-surface-hover focus-within:bg-surface-active',
            )}
          >
            {indicator(selection.labels.includes(OTHER) && Boolean(selection.other.trim()))}
            <input
              type="text"
              disabled={busy}
              value={selection.other}
              placeholder={t('question.otherPlaceholder')}
              aria-label={t('question.otherPlaceholder')}
              onFocus={activateOther}
              onChange={(event) => setOther(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault()
                  advance()
                }
              }}
              className="w-full min-w-0 border-0 bg-transparent py-2 text-[0.8125rem] leading-5 text-ink outline-none placeholder:text-ink-faint disabled:cursor-wait disabled:opacity-60"
            />
          </label>
        </div>
      </fieldset>

      <div className="flex min-h-[3.375rem] items-center gap-2.5 border-t border-edge/80 bg-canvas-raised/45 px-3.5 py-2 max-sm:px-3">
        {total > 1 && (
          <>
            <button
              type="button"
              disabled={step === 0 || busy}
              onClick={() => setStep((current) => Math.max(0, current - 1))}
              aria-label={t('question.back')}
              className="grid size-8 shrink-0 cursor-pointer place-items-center rounded-full text-ink-muted outline-none transition-colors hover:bg-surface-active hover:text-ink focus-visible:bg-surface-active focus-visible:text-ink disabled:cursor-not-allowed disabled:opacity-25"
            >
              <ArrowLeft className="size-3.5" aria-hidden="true" />
            </button>
            <span className="font-mono text-[0.6875rem] leading-4 text-ink-muted tabular-nums">
              {step + 1} / {total}
            </span>
            <span
              className="h-1 w-14 overflow-hidden rounded-full bg-canvas-strong"
              aria-hidden="true"
            >
              <span
                className="block h-full rounded-full bg-ink-muted transition-[width] duration-200"
                style={{ width: `${((step + 1) / total) * 100}%` }}
              />
            </span>
          </>
        )}
        {error && (
          <span
            className="min-w-0 flex-1 truncate text-[0.75rem] leading-4 text-danger-soft"
            role="alert"
          >
            {error}
          </span>
        )}
        <span className={cn('ml-auto', error && total === 1 && 'ml-0')}>{primary}</span>
      </div>
    </section>
  )
}
