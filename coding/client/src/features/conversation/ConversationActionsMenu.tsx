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
import { cn } from '@/lib/utils'
import { useI18n } from '@/i18n'
import type { BackgroundTask } from '@/types'
import { HeaderControlTooltip } from '@/shared/ui/HeaderControlTooltip'

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
            className="window-titlebar-control relative grid size-[30px] shrink-0 cursor-pointer place-items-center rounded-md text-ink-muted outline-none transition-colors hover:bg-surface-active hover:text-ink focus-visible:bg-surface-active focus-visible:text-ink data-[state=open]:bg-surface-selected data-[state=open]:text-ink"
            data-testid="conversation-actions-trigger"
            type="button"
            aria-label={t('conversation.actions')}
          >
            <Ellipsis className="size-4" aria-hidden="true" />
            {runningCount > 0 && (
              <span
                className="absolute top-1 right-1 size-1.5 rounded-full bg-success ring-2 ring-canvas"
                aria-hidden="true"
              />
            )}
            <HeaderControlTooltip align="end">
              {t('conversation.actions')}
            </HeaderControlTooltip>
          </button>
        </DropdownMenu.Trigger>

        <DropdownMenu.Portal>
          <DropdownMenu.Content
            side="bottom"
            align="end"
            sideOffset={7}
            collisionPadding={10}
            className="z-[120] min-w-[15.5rem] animate-[fade-in_110ms_ease-out] rounded-2xl border border-edge bg-canvas p-1 text-[0.875rem] text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
          >
            <DropdownMenu.Sub>
              <DropdownMenu.SubTrigger className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[10px] px-2.5 outline-none data-[highlighted]:bg-surface-active data-[state=open]:bg-surface-active">
                <Activity
                  className={cn('size-4 text-ink-muted', runningCount > 0 && 'text-success')}
                  aria-hidden="true"
                />
                <span>{t('tasks.title')}</span>
                <span className="ml-auto flex items-center gap-1.5 text-ink-faint">
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
                  className="z-[130] flex max-h-[min(24rem,var(--radix-dropdown-menu-content-available-height))] w-[min(21rem,calc(100vw-1.25rem))] animate-[fade-in_110ms_ease-out] flex-col overflow-hidden rounded-2xl border border-edge bg-canvas p-1 text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
                >
                  <DropdownMenu.Label className="flex h-8 items-center gap-2 px-2.5 text-[0.75rem] font-medium text-ink-faint">
                    <span>{t('tasks.title')}</span>
                    <span className="ml-auto tabular-nums">
                      {runningCount > 0
                        ? t('tasks.runningCount', { count: runningCount })
                        : t('tasks.recent')}
                    </span>
                  </DropdownMenu.Label>
                  <DropdownMenu.Separator className="mx-1 mb-1 h-px bg-canvas-sunken" />

                  {orderedTasks.length === 0 ? (
                    <div className="px-3 py-7 text-center text-[0.8125rem] text-ink-faint">
                      {t('tasks.none')}
                    </div>
                  ) : (
                    <div className="code-scroll-area min-h-0 overflow-y-auto">
                      {orderedTasks.map((task) => (
                        <DropdownMenu.Item
                          key={task.id}
                          className="grid min-h-14 cursor-default select-none grid-cols-[1rem_minmax(0,1fr)_auto] items-start gap-x-2.5 rounded-[10px] px-2.5 py-2 outline-none data-[highlighted]:bg-surface-active"
                          onSelect={() => onSelectTask(task.id)}
                        >
                          <TaskStatusIcon status={task.status} />
                          <span className="min-w-0">
                            <span className="block truncate text-[0.78125rem] font-medium text-ink-soft">
                              {task.description || task.command}
                            </span>
                            {task.description && (
                              <code className="mt-0.5 block truncate font-mono text-[0.65625rem] leading-4 text-ink-faint">
                                {task.command}
                              </code>
                            )}
                          </span>
                          <span className="flex items-center gap-1 pt-0.5 text-[0.65625rem] tabular-nums text-ink-faint">
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
      <LoaderCircle className="mt-0.5 size-3.5 animate-spin text-success" aria-hidden="true" />
    )
  }
  if (status === 'succeeded') {
    return <Check className="mt-0.5 size-3.5 text-ink-faint" aria-hidden="true" />
  }
  if (status === 'stopped') {
    return <CircleStop className="mt-0.5 size-3.5 text-ink-faint" aria-hidden="true" />
  }
  return <CircleX className="mt-0.5 size-3.5 text-danger-soft" aria-hidden="true" />
}
