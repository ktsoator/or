import { useEffect, useMemo, useState } from 'react'
import {
  Activity,
  Check,
  ChevronRight,
  CircleStop,
  CircleX,
  Ellipsis,
  LoaderCircle,
} from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import { useI18n } from '@/i18n'
import { cn } from '@/lib/utils'
import type { BackgroundTask } from '@/types'

const visibleTaskLimit = 20

export function ConversationActionsMenu({
  sessionID,
  tasks,
  onSelectTask,
}: {
  sessionID: string
  tasks: BackgroundTask[]
  onSelectTask: (id: string) => void
}) {
  const { t } = useI18n()
  const [menuOpen, setMenuOpen] = useState(false)
  const runningCount = tasks.filter((task) => task.status === 'running').length
  const statusLabels: Record<BackgroundTask['status'], string> = {
    running: t('tasks.statusRunning'),
    succeeded: t('tasks.statusSucceeded'),
    failed: t('tasks.statusFailed'),
    stopped: t('tasks.statusStopped'),
  }
  const orderedTasks = useMemo(
    () =>
      [...tasks]
        .sort((left, right) => {
          if (left.status === 'running' && right.status !== 'running') return -1
          if (left.status !== 'running' && right.status === 'running') return 1
          return Date.parse(right.startedAt) - Date.parse(left.startedAt)
        })
        .slice(0, visibleTaskLimit),
    [tasks],
  )

  useEffect(() => setMenuOpen(false), [sessionID])

  return (
    <DropdownMenu.Root open={menuOpen} onOpenChange={setMenuOpen}>
        <DropdownMenu.Trigger asChild>
          <button
            className="window-titlebar-control relative grid size-7 shrink-0 cursor-pointer place-items-center rounded-md text-stone-500 outline-none transition-colors hover:bg-[rgb(241,241,241)] hover:text-stone-900 focus-visible:ring-2 focus-visible:ring-stone-300 data-[state=open]:bg-[rgb(237,237,237)] data-[state=open]:text-stone-900"
            data-testid="conversation-actions-trigger"
            type="button"
            title={t('conversation.actions')}
            aria-label={t('conversation.actions')}
          >
            <Ellipsis className="size-4" aria-hidden="true" />
            {runningCount > 0 && (
              <span
                className="absolute top-1 right-1 size-1.5 rounded-full bg-emerald-500 ring-2 ring-white"
                aria-hidden="true"
              />
            )}
          </button>
        </DropdownMenu.Trigger>

        <DropdownMenu.Portal>
          <DropdownMenu.Content
            side="bottom"
            align="end"
            sideOffset={7}
            collisionPadding={10}
            className="z-[120] min-w-[15.5rem] animate-[fade-in_110ms_ease-out] rounded-2xl border border-stone-200 bg-white p-1 text-[0.875rem] text-stone-900 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
          >
            <DropdownMenu.Sub>
              <DropdownMenu.SubTrigger className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[10px] px-2.5 outline-none data-[highlighted]:bg-[rgb(241,241,241)] data-[state=open]:bg-[rgb(241,241,241)]">
                <Activity
                  className={cn('size-4 text-stone-600', runningCount > 0 && 'text-emerald-600')}
                  aria-hidden="true"
                />
                <span>{t('tasks.title')}</span>
                <span className="ml-auto flex items-center gap-1.5 text-stone-400">
                  <span className="tabular-nums">{runningCount || tasks.length}</span>
                  <ChevronRight className="size-3.5" aria-hidden="true" />
                </span>
              </DropdownMenu.SubTrigger>

              <DropdownMenu.Portal>
                <DropdownMenu.SubContent
                  align="start"
                  sideOffset={6}
                  alignOffset={-4}
                  collisionPadding={10}
                  className="z-[130] flex max-h-[min(24rem,var(--radix-dropdown-menu-content-available-height))] w-[min(21rem,calc(100vw-1.25rem))] animate-[fade-in_110ms_ease-out] flex-col overflow-hidden rounded-2xl border border-stone-200 bg-white p-1 text-stone-900 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
                >
                  <DropdownMenu.Label className="flex h-8 items-center gap-2 px-2.5 text-[0.75rem] font-medium text-stone-400">
                    <span>{t('tasks.title')}</span>
                    <span className="ml-auto tabular-nums">
                      {runningCount > 0
                        ? t('tasks.runningCount', { count: runningCount })
                        : t('tasks.recent')}
                    </span>
                  </DropdownMenu.Label>
                  <DropdownMenu.Separator className="mx-1 mb-1 h-px bg-stone-100" />

                  {orderedTasks.length === 0 ? (
                    <div className="px-3 py-7 text-center text-[0.8125rem] text-stone-400">
                      {t('tasks.none')}
                    </div>
                  ) : (
                    <div className="code-scroll-area min-h-0 overflow-y-auto">
                      {orderedTasks.map((task) => (
                        <DropdownMenu.Item
                          key={task.id}
                          className="grid min-h-14 cursor-default select-none grid-cols-[1rem_minmax(0,1fr)_auto] items-start gap-x-2.5 rounded-[10px] px-2.5 py-2 outline-none data-[highlighted]:bg-[rgb(241,241,241)]"
                          onSelect={() => onSelectTask(task.id)}
                        >
                          <TaskStatusIcon status={task.status} />
                          <span className="min-w-0">
                            <span className="block truncate text-[0.78125rem] font-medium text-stone-800">
                              {task.description || task.command}
                            </span>
                            {task.description && (
                              <code className="mt-0.5 block truncate font-mono text-[0.65625rem] leading-4 text-stone-400">
                                {task.command}
                              </code>
                            )}
                          </span>
                          <span className="flex items-center gap-1 pt-0.5 text-[0.65625rem] tabular-nums text-stone-400">
                            <span>{statusLabels[task.status]}</span>
                            <ChevronRight className="size-3.5" aria-hidden="true" />
                          </span>
                        </DropdownMenu.Item>
                      ))}
                    </div>
                  )}
                </DropdownMenu.SubContent>
              </DropdownMenu.Portal>
            </DropdownMenu.Sub>
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

function TaskStatusIcon({ status }: { status: BackgroundTask['status'] }) {
  if (status === 'running') {
    return (
      <LoaderCircle className="mt-0.5 size-3.5 animate-spin text-emerald-600" aria-hidden="true" />
    )
  }
  if (status === 'succeeded') {
    return <Check className="mt-0.5 size-3.5 text-stone-400" aria-hidden="true" />
  }
  if (status === 'stopped') {
    return <CircleStop className="mt-0.5 size-3.5 text-stone-400" aria-hidden="true" />
  }
  return <CircleX className="mt-0.5 size-3.5 text-red-500" aria-hidden="true" />
}
