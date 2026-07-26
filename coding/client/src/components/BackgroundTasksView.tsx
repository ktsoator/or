import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Check,
  CircleStop,
  CircleX,
  ListChecks,
  LoaderCircle,
  RefreshCw,
  Square,
  Terminal,
} from 'lucide-react'
import { useI18n } from '@/i18n'
import { cn } from '@/lib/utils'
import type { BackgroundTask, TaskOutputResponse } from '@/types'

const outputRefreshInterval = 2000

export type WorkbenchTaskSource = {
  sessionID: string
  tasks: BackgroundTask[]
  onStopTask: (id: string) => Promise<void>
  onReadTaskOutput: (id: string) => Promise<TaskOutputResponse>
}

export function BackgroundTasksView({
  tasks,
  selectedTaskID,
  onSelectTask,
  onStopTask,
  onReadTaskOutput,
}: WorkbenchTaskSource & {
  selectedTaskID?: string
  onSelectTask: (id: string) => void
}) {
  const { t } = useI18n()
  const orderedTasks = useMemo(() => orderTasks(tasks), [tasks])
  const selectedTask =
    orderedTasks.find((task) => task.id === selectedTaskID) ?? orderedTasks[0]

  useEffect(() => {
    if (selectedTask && selectedTask.id !== selectedTaskID) onSelectTask(selectedTask.id)
  }, [onSelectTask, selectedTask, selectedTaskID])

  return (
    <section
      className="background-tasks-view min-h-0 flex-1 bg-white"
      aria-label={t('tasks.title')}
      data-testid="background-tasks-view"
    >
      {orderedTasks.length === 0 ? (
        <div className="flex h-full min-h-0 flex-col">
          <TaskListHeader count={0} />
          <div className="grid min-h-0 flex-1 place-items-center px-6 text-center text-[0.8125rem] text-stone-400">
            {t('tasks.none')}
          </div>
        </div>
      ) : (
        <div className="background-tasks-layout grid h-full min-h-0">
          <aside className="background-tasks-list flex min-h-0 flex-col border-stone-200 bg-stone-50/40">
            <TaskListHeader count={tasks.length} />
            <div className="code-scroll-area min-h-0 flex-1 overflow-y-auto p-1.5">
              {orderedTasks.map((task) => (
                <TaskListItem
                  key={task.id}
                  task={task}
                  active={task.id === selectedTask?.id}
                  onSelect={() => onSelectTask(task.id)}
                />
              ))}
            </div>
          </aside>

          {selectedTask && (
            <TaskOutputView
              task={selectedTask}
              onStopTask={onStopTask}
              onReadTaskOutput={onReadTaskOutput}
            />
          )}
        </div>
      )}
    </section>
  )
}

function TaskListHeader({ count }: { count: number }) {
  const { t } = useI18n()
  return (
    <div className="flex h-10 shrink-0 items-center gap-2 border-b border-stone-200 px-3">
      <ListChecks className="size-3.5 shrink-0 text-stone-400" aria-hidden="true" />
      <span className="text-[0.75rem] font-medium text-stone-600">{t('tasks.recent')}</span>
      <span className="ml-auto text-[0.6875rem] tabular-nums text-stone-400">{count}</span>
    </div>
  )
}

function TaskListItem({
  task,
  active,
  onSelect,
}: {
  task: BackgroundTask
  active: boolean
  onSelect: () => void
}) {
  const { t } = useI18n()
  const statusLabels: Record<BackgroundTask['status'], string> = {
    running: t('tasks.statusRunning'),
    succeeded: t('tasks.statusSucceeded'),
    failed: t('tasks.statusFailed'),
    stopped: t('tasks.statusStopped'),
  }

  return (
    <button
      className={cn(
        'mb-0.5 grid w-full cursor-pointer grid-cols-[1rem_minmax(0,1fr)] gap-x-2 rounded-md px-2 py-2 text-left outline-none transition-colors last:mb-0 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-stone-300',
        active
          ? 'bg-white text-stone-900 shadow-[0_1px_2px_rgba(28,25,23,0.08)] ring-1 ring-stone-200'
          : 'text-stone-600 hover:bg-stone-100',
      )}
      type="button"
      aria-current={active ? 'page' : undefined}
      onClick={onSelect}
    >
      <TaskStatusIcon status={task.status} className="mt-0.5" />
      <span className="min-w-0">
        <span className="block truncate text-[0.75rem] font-medium">
          {task.description || task.command}
        </span>
        <span className="mt-0.5 flex min-w-0 items-center gap-1.5 text-[0.65625rem] text-stone-400">
          <span className="shrink-0">{statusLabels[task.status]}</span>
          <span aria-hidden="true">·</span>
          <code className="min-w-0 truncate font-mono">{task.command}</code>
        </span>
      </span>
    </button>
  )
}

function TaskOutputView({
  task,
  onStopTask,
  onReadTaskOutput,
}: {
  task: BackgroundTask
  onStopTask: (id: string) => Promise<void>
  onReadTaskOutput: (id: string) => Promise<TaskOutputResponse>
}) {
  const { t } = useI18n()
  const [output, setOutput] = useState<TaskOutputResponse>({ content: '', truncated: false })
  const [loading, setLoading] = useState(false)
  const [outputLoaded, setOutputLoaded] = useState(false)
  const [outputError, setOutputError] = useState('')
  const [stopping, setStopping] = useState(false)
  const [stopError, setStopError] = useState('')
  const [now, setNow] = useState(() => Date.now())
  const requestSequenceRef = useRef(0)
  const readingTaskRef = useRef<string | undefined>(undefined)
  const onReadTaskOutputRef = useRef(onReadTaskOutput)
  onReadTaskOutputRef.current = onReadTaskOutput

  const statusLabels: Record<BackgroundTask['status'], string> = {
    running: t('tasks.statusRunning'),
    succeeded: t('tasks.statusSucceeded'),
    failed: t('tasks.statusFailed'),
    stopped: t('tasks.statusStopped'),
  }

  const refreshOutput = useCallback(async () => {
    if (readingTaskRef.current === task.id) return
    const requestSequence = ++requestSequenceRef.current
    readingTaskRef.current = task.id
    setLoading(true)
    setOutputError('')
    try {
      const nextOutput = await onReadTaskOutputRef.current(task.id)
      if (requestSequence === requestSequenceRef.current) {
        setOutput(nextOutput)
        setOutputLoaded(true)
      }
    } catch (error) {
      if (requestSequence === requestSequenceRef.current) {
        setOutputError(error instanceof Error ? error.message : t('tasks.logFailed'))
      }
    } finally {
      if (requestSequence === requestSequenceRef.current) {
        readingTaskRef.current = undefined
        setLoading(false)
      }
    }
  }, [t, task.id])

  useEffect(() => {
    requestSequenceRef.current += 1
    readingTaskRef.current = undefined
    setOutput({ content: '', truncated: false })
    setOutputLoaded(false)
    setOutputError('')
    setStopError('')
    void refreshOutput()
  }, [refreshOutput, task.id])

  useEffect(() => {
    if (task.status !== 'running') return
    const timer = window.setInterval(() => void refreshOutput(), outputRefreshInterval)
    return () => window.clearInterval(timer)
  }, [refreshOutput, task.status])

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [])

  const stopTask = async () => {
    if (stopping) return
    setStopping(true)
    setStopError('')
    try {
      await onStopTask(task.id)
    } catch (error) {
      setStopError(error instanceof Error ? error.message : t('tasks.stopFailed'))
    } finally {
      setStopping(false)
    }
  }

  return (
    <div className="flex min-h-0 min-w-0 flex-col bg-white">
      <header className="flex min-h-14 shrink-0 items-center gap-2.5 border-b border-stone-200 px-3">
        <TaskStatusIcon status={task.status} />
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-[0.8125rem] font-semibold text-stone-900">
            {task.description || task.command}
          </h2>
          <div className="mt-0.5 flex items-center gap-1.5 text-[0.6875rem] tabular-nums text-stone-400">
            <span>{statusLabels[task.status]}</span>
            <span aria-hidden="true">·</span>
            <span>{formatRuntime(task, now)}</span>
          </div>
        </div>
        <button
          className="grid size-7 shrink-0 cursor-pointer place-items-center rounded-md text-stone-400 transition-colors hover:bg-stone-100 hover:text-stone-800 disabled:cursor-wait disabled:opacity-40"
          type="button"
          aria-label={t('tasks.refreshLog')}
          title={t('tasks.refreshLog')}
          disabled={loading}
          onClick={() => void refreshOutput()}
        >
          <RefreshCw className={cn('size-3.5', loading && 'animate-spin')} aria-hidden="true" />
        </button>
        {task.status === 'running' && (
          <button
            className="grid size-7 shrink-0 cursor-pointer place-items-center rounded-md text-stone-400 transition-colors hover:bg-red-50 hover:text-red-700 disabled:cursor-wait disabled:opacity-40"
            type="button"
            aria-label={t('tasks.stop')}
            title={t('tasks.stop')}
            disabled={stopping}
            onClick={() => void stopTask()}
          >
            {stopping ? (
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
            ) : (
              <Square className="size-2.5 fill-current" aria-hidden="true" />
            )}
          </button>
        )}
      </header>

      <div className="flex min-h-9 shrink-0 items-center gap-2 border-b border-stone-100 px-3">
        <Terminal className="size-3.5 shrink-0 text-stone-400" aria-hidden="true" />
        <code className="min-w-0 flex-1 truncate font-mono text-[0.6875rem] text-stone-500">
          {task.command}
        </code>
      </div>

      {(output.truncated || stopError) && (
        <div
          className={cn(
            'shrink-0 border-b border-stone-100 px-3 py-1.5 text-[0.6875rem]',
            stopError ? 'text-red-700' : 'text-stone-400',
          )}
          role={stopError ? 'alert' : undefined}
        >
          {stopError || t('tasks.truncated')}
        </div>
      )}

      <pre
        className={cn(
          'code-scroll-area min-h-0 flex-1 select-text overflow-auto bg-stone-50/70 p-3 font-mono text-[0.6875rem] leading-[1.125rem] whitespace-pre-wrap text-stone-700',
          outputError && 'text-red-700',
        )}
      >
        {outputError
          ? outputError
          : loading && !outputLoaded
            ? t('tasks.loadingLog')
            : output.content || t('tasks.emptyLog')}
      </pre>
    </div>
  )
}

function TaskStatusIcon({
  status,
  className,
}: {
  status: BackgroundTask['status']
  className?: string
}) {
  if (status === 'running') {
    return (
      <LoaderCircle
        className={cn('size-3.5 shrink-0 animate-spin text-emerald-600', className)}
        aria-hidden="true"
      />
    )
  }
  if (status === 'succeeded') {
    return <Check className={cn('size-3.5 shrink-0 text-stone-400', className)} aria-hidden="true" />
  }
  if (status === 'stopped') {
    return <CircleStop className={cn('size-3.5 shrink-0 text-stone-400', className)} aria-hidden="true" />
  }
  return <CircleX className={cn('size-3.5 shrink-0 text-red-500', className)} aria-hidden="true" />
}

function orderTasks(tasks: BackgroundTask[]): BackgroundTask[] {
  return [...tasks].sort((left, right) => {
    if (left.status === 'running' && right.status !== 'running') return -1
    if (left.status !== 'running' && right.status === 'running') return 1
    return Date.parse(right.startedAt) - Date.parse(left.startedAt)
  })
}

function formatRuntime(task: BackgroundTask, now: number): string {
  const startedAt = Date.parse(task.startedAt)
  const completedAt = task.completedAt ? Date.parse(task.completedAt) : now
  if (!Number.isFinite(startedAt) || !Number.isFinite(completedAt)) return ''
  const seconds = Math.max(0, Math.floor((completedAt - startedAt) / 1000))
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const remainder = seconds % 60
  if (minutes < 60) return `${minutes}m ${remainder}s`
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`
}
