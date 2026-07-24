import { useState } from 'react'
import { ArrowUp, Check, LoaderCircle } from 'lucide-react'
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
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const select = (index: number, label: string, multiSelect: boolean) => {
    setSelections((current) =>
      current.map((selection, i) => {
        if (i !== index) return selection
        if (!multiSelect) return { ...selection, labels: [label] }
        return {
          ...selection,
          labels: selection.labels.includes(label)
            ? selection.labels.filter((existing) => existing !== label)
            : [...selection.labels, label],
        }
      }),
    )
  }

  const setOther = (index: number, value: string) => {
    setSelections((current) =>
      current.map((selection, i) =>
        i === index
          ? {
              ...selection,
              other: value,
              labels: selection.labels.includes(OTHER)
                ? selection.labels
                : [...selection.labels, OTHER],
            }
          : selection,
      ),
    )
  }

  // Every question must carry a value: a half-filled form would reach the agent
  // as an unanswered question, which it is told to treat as no answer at all.
  const complete = selections.every((selection) => answerValues(selection).length > 0)

  const submit = async () => {
    if (!complete || busy) return
    setBusy(true)
    setError('')
    try {
      await onResolve(
        item.id,
        item.questions.map((question, index) => ({
          question: question.question,
          values: answerValues(selections[index]),
        })),
      )
    } catch {
      setError(t('question.couldNotSend'))
      setBusy(false)
    }
  }

  return (
    <section
      className="animate-[fade-in_160ms_ease-out] overflow-hidden rounded-[28px] border border-stone-200 bg-white [container-type:inline-size]"
      aria-live="polite"
      aria-busy={busy}
    >
      {item.questions.map((question, index) => {
        const selection = selections[index]
        return (
          <fieldset
            key={question.question}
            className={cn('min-w-0', index > 0 && 'border-t border-stone-200/80')}
          >
            <legend
              className={cn(
                'flex w-full flex-wrap items-baseline gap-2 px-3.5 pb-2',
                index > 0 ? 'pt-4' : 'pt-3',
              )}
            >
              <span className="rounded bg-stone-100 px-1.5 py-0.5 font-mono text-[0.6875rem] leading-4 text-stone-500">
                {question.header}
              </span>
              <span className="text-[0.875rem] leading-5 font-medium text-stone-900">
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
                  onClick={() => select(index, option.label, Boolean(question.multiSelect))}
                  className="flex w-full cursor-pointer items-start gap-2.5 border-t border-stone-200/70 px-3.5 py-2.5 text-left transition-colors hover:bg-stone-50 focus-visible:bg-stone-50 focus-visible:outline-none disabled:cursor-wait disabled:opacity-60"
                >
                  <span
                    className={cn(
                      'mt-[0.1875rem] grid size-4 shrink-0 place-items-center border transition-colors',
                      question.multiSelect ? 'rounded-[4px]' : 'rounded-full',
                      selected
                        ? 'border-stone-900 bg-stone-900 text-white'
                        : 'border-stone-300 bg-white',
                    )}
                    aria-hidden="true"
                  >
                    {selected && <Check className="size-2.5" strokeWidth={3} />}
                  </span>
                  <span className="min-w-0">
                    <span
                      className={cn(
                        'block text-[0.8125rem] leading-5',
                        selected ? 'font-medium text-stone-900' : 'text-stone-800',
                      )}
                    >
                      {option.label}
                    </span>
                    {option.description && (
                      <span className="block text-[0.78125rem] leading-5 text-stone-500">
                        {option.description}
                      </span>
                    )}
                  </span>
                </button>
              )
            })}

            <div className="flex items-center gap-2.5 border-t border-stone-200/70 px-3.5">
              <span
                className={cn(
                  'grid size-4 shrink-0 place-items-center border transition-colors',
                  question.multiSelect ? 'rounded-[4px]' : 'rounded-full',
                  selection.labels.includes(OTHER) && selection.other.trim()
                    ? 'border-stone-900 bg-stone-900 text-white'
                    : 'border-stone-300 bg-white',
                )}
                aria-hidden="true"
              >
                {selection.labels.includes(OTHER) && selection.other.trim() && (
                  <Check className="size-2.5" strokeWidth={3} />
                )}
              </span>
              <input
                type="text"
                disabled={busy}
                value={selection.other}
                placeholder={t('question.otherPlaceholder')}
                aria-label={t('question.otherPlaceholder')}
                onChange={(event) => setOther(index, event.target.value)}
                className="w-full min-w-0 border-0 bg-transparent py-2.5 text-[0.8125rem] leading-5 text-stone-900 outline-none placeholder:text-stone-400 disabled:cursor-wait disabled:opacity-60"
              />
              {index === item.questions.length - 1 && (
                <button
                  type="button"
                  disabled={!complete || busy}
                  onClick={() => void submit()}
                  aria-label={t('question.submit')}
                  className="my-1.5 grid size-8 shrink-0 cursor-pointer place-items-center rounded-full bg-black text-white transition-colors hover:bg-stone-800 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-400 disabled:cursor-not-allowed disabled:opacity-25"
                >
                  {busy ? (
                    <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
                  ) : (
                    <ArrowUp className="size-3.5" aria-hidden="true" />
                  )}
                </button>
              )}
            </div>
          </fieldset>
        )
      })}

      {error && (
        <div
          className="border-t border-stone-200/80 px-3.5 py-2 text-[0.75rem] leading-4 text-red-600"
          role="alert"
        >
          {error}
        </div>
      )}
    </section>
  )
}
