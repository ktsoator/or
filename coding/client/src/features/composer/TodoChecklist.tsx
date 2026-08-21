import { useId, useState } from 'react'
import {
  ChevronDown,
  CircleCheck,
  CircleDashed,
  ListTodo,
  LoaderCircle,
} from 'lucide-react'
import type { TodoItem } from '@/types'
import { useI18n } from '@/i18n'
import { cn } from '@/lib/utils'

type TodoStatus = 'pending' | 'in_progress' | 'completed'

function todoStatus(status: string): TodoStatus {
  if (status === 'completed' || status === 'in_progress') return status
  return 'pending'
}

function TodoStatusIcon({ status }: { status: TodoStatus }) {
  if (status === 'completed') {
    return <CircleCheck className="size-4 text-success" strokeWidth={1.8} aria-hidden="true" />
  }
  if (status === 'in_progress') {
    return (
      <LoaderCircle
        className="size-4 animate-spin text-info motion-reduce:animate-none"
        strokeWidth={1.8}
        aria-hidden="true"
      />
    )
  }
  return <CircleDashed className="size-4 text-ink-faint" strokeWidth={1.7} aria-hidden="true" />
}

export function TodoChecklist({ todos }: { todos: readonly TodoItem[] }) {
  const { t } = useI18n()
  const [expanded, setExpanded] = useState(false)
  const listID = `todo-checklist-${useId()}`
  if (todos.length === 0) return null

  const counts = { completed: 0, in_progress: 0, pending: 0 }
  for (const todo of todos) counts[todoStatus(todo.status)] += 1
  const summary = [
    ...(counts.completed > 0 ? [t('todo.done', { count: counts.completed })] : []),
    ...(counts.in_progress > 0 ? [t('todo.active', { count: counts.in_progress })] : []),
    ...(counts.pending > 0 ? [t('todo.pending', { count: counts.pending })] : []),
  ].join(' · ')

  return (
    <section
      className="overflow-hidden rounded-xl border border-edge/90 bg-canvas-raised text-ink-soft shadow-[0_8px_24px_-22px_rgba(28,25,23,0.5)]"
      data-testid="todo-checklist"
      aria-label={t('todo.title')}
      aria-live="polite"
    >
      <button
        type="button"
        className="flex h-9 w-full cursor-pointer items-center gap-2.5 px-3 text-left outline-none transition-colors hover:bg-surface-hover focus-visible:bg-surface-hover"
        aria-controls={listID}
        aria-expanded={expanded}
        onClick={() => setExpanded((current) => !current)}
      >
        <ListTodo className="size-4 shrink-0 text-ink-faint" strokeWidth={1.8} aria-hidden="true" />
        <span className="shrink-0 text-[0.8125rem] font-medium text-ink-soft">
          {t('todo.title')}
        </span>
        <span className="min-w-0 flex-1 truncate text-[0.75rem] text-ink-faint">
          {summary}
        </span>
        <ChevronDown
          className={cn(
            'size-3.5 shrink-0 text-ink-faint transition-transform duration-150 motion-reduce:transition-none',
            expanded && 'rotate-180',
          )}
          aria-hidden="true"
        />
      </button>
      {expanded && (
        <ul
          id={listID}
          className="m-0 flex max-h-44 list-none flex-col overflow-y-auto border-t border-edge/80 px-3 py-2"
        >
          {todos.map((todo, index) => {
            const status = todoStatus(todo.status)
            const statusLabel =
              status === 'completed'
                ? t('todo.status.completed')
                : status === 'in_progress'
                  ? t('todo.status.inProgress')
                  : t('todo.status.pending')
            return (
              <li
                key={`${todo.content}:${index}`}
                className="flex min-h-8 min-w-0 items-center gap-2.5 py-1 text-[0.8125rem] leading-5"
                data-status={status}
              >
                <span className="grid size-4 shrink-0 place-items-center">
                  <TodoStatusIcon status={status} />
                </span>
                <span
                  className={cn(
                    'min-w-0 flex-1 break-words text-ink-muted',
                    status === 'completed' && 'text-ink-faint line-through',
                    status === 'in_progress' && 'text-ink-soft',
                  )}
                >
                  {todo.content}
                </span>
                <span className="sr-only">{statusLabel}</span>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
