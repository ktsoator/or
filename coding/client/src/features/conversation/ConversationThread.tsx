import { type ClipboardEvent, type FormEvent, type KeyboardEvent, useEffect, useState } from 'react'
import {
  BookOpen,
  CircleAlert,
  CircleCheck,
  CircleStop,
  CircleX,
  FileCode2,
  LoaderCircle,
  Pencil,
} from 'lucide-react'
import { Tooltip } from 'radix-ui'
import { formatFileSize } from '@/shared/attachments'
import {
  parseSkillReference,
  serializeSkillReferenceCopy,
  type SkillReference,
} from '@/features/skills'
import type { Item } from '@/types'
import { useI18n } from '@/i18n'
import { formatMessageTime } from '@/lib/time'
import { cn } from '@/lib/utils'
import { Markdown } from '@/shared/ui/Markdown'
import { BranchSessionDialog } from './BranchSessionDialog'
import { CopyButton } from './CopyButton'
import { EditMessageDialog } from './EditMessageDialog'
import { ResponseActions } from './ResponseActions'
import { Thinking } from './Thinking'
import { ToolCard } from './ToolCard'

export function AwaitingResponse() {
  const { t } = useI18n()
  return (
    <div
      className="my-1 flex animate-[fade-in_160ms_ease-out] items-center gap-1.5 py-0.5 text-[0.8125rem] text-ink-faint"
      role="status"
    >
      <span className="size-1 animate-pulse rounded-full bg-info" />
      <span className="streaming-sheen">{t('thinking.working')}</span>
    </div>
  )
}

export function AutoCompactionStatus() {
  const { t } = useI18n()
  const [visible, setVisible] = useState(false)

  useEffect(() => {
    const timer = window.setTimeout(() => setVisible(true), 350)
    return () => window.clearTimeout(timer)
  }, [])

  if (!visible) return null
  return (
    <div
      className="my-1 flex animate-[fade-in_160ms_ease-out] items-center gap-1.5 py-0.5 text-[0.8125rem] text-ink-faint"
      role="status"
    >
      <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
      <span>{t('compaction.automatic')}</span>
    </div>
  )
}

export function ThreadItem({
  item,
  cwd,
  highlighted = false,
  branchingDisabled = false,
  onForkMessage,
  onEditMessage,
  onOpenRunDiagnostics,
  editRequiresConfirmation = false,
}: {
  item: Item
  cwd?: string
  highlighted?: boolean
  branchingDisabled?: boolean
  onForkMessage?: (
    messageID: string,
    mode: 'before_user' | 'after_assistant',
    text?: string,
  ) => Promise<unknown>
  onEditMessage?: (messageID: string, text: string) => Promise<unknown>
  onOpenRunDiagnostics?: (runId: string) => void
  editRequiresConfirmation?: boolean
}) {
  const { locale, t } = useI18n()
  const [editing, setEditing] = useState(false)
  const [editText, setEditText] = useState('')
  const [branching, setBranching] = useState(false)
  const [branchError, setBranchError] = useState('')
  const [branchDialogOpen, setBranchDialogOpen] = useState(false)
  const [editDialogOpen, setEditDialogOpen] = useState(false)

  const startEditing = () => {
    if (item.kind !== 'user' || !item.messageID || branchingDisabled || branching) return
    setEditText(item.text)
    setBranchError('')
    setEditing(true)
  }

  const cancelEditing = () => {
    if (branching) return
    setEditing(false)
    setBranchError('')
  }

  const applyEdit = async () => {
    if (item.kind !== 'user' || !item.messageID || !onEditMessage || branchingDisabled || branching) {
      return
    }
    setBranching(true)
    setBranchError('')
    try {
      await onEditMessage(item.messageID, editText)
      setEditDialogOpen(false)
      setEditing(false)
    } catch {
      setBranchError(t('actions.editFailed'))
    } finally {
      setBranching(false)
    }
  }

  const submitEdit = (event?: FormEvent) => {
    event?.preventDefault()
    if (item.kind !== 'user' || !item.messageID || !onEditMessage || branchingDisabled || branching) {
      return
    }
    if (!editText.trim() && item.images.length === 0 && (item.files?.length ?? 0) === 0) return
    setBranchError('')
    if (editRequiresConfirmation) {
      setEditDialogOpen(true)
      return
    }
    void applyEdit()
  }

  const branchAssistant = async () => {
    if (item.kind !== 'assistant' || !item.messageID || !onForkMessage || branchingDisabled || branching) {
      return
    }
    setBranching(true)
    setBranchError('')
    try {
      await onForkMessage(item.messageID, 'after_assistant')
      setBranchDialogOpen(false)
    } catch {
      setBranchError(t('actions.branchFailed'))
    } finally {
      setBranching(false)
    }
  }

  switch (item.kind) {
    case 'user':
      return (
        <section
          className="group/message my-3.5 flex animate-[fade-in_160ms_ease-out] justify-end"
          data-testid="user-message"
          data-message-id={item.messageID}
          data-branch-point-message-id={item.messageID}
          data-branch-point-highlighted={highlighted || undefined}
        >
          <div
            className={cn(
              'flex flex-col items-end gap-2',
              editing
                ? 'w-full max-w-full'
                : 'max-w-[78%] max-md:max-w-[88%]',
              highlighted && 'bg-surface-hover',
            )}
          >
            {(item.files?.length ?? 0) > 0 && (
              <div className="flex max-w-full flex-wrap justify-end gap-1.5">
                {item.files?.map((file, index) => (
                  <div
                    key={`${file.name}-${file.size}-${index}`}
                    className="flex h-9 max-w-[15rem] items-center gap-1.5 rounded-lg border border-edge bg-canvas-raised px-2.5 text-[0.75rem] text-ink-muted"
                    title={file.name}
                  >
                    <FileCode2
                      className="size-3.5 shrink-0 text-ink-muted"
                      aria-hidden="true"
                    />
                    <span className="min-w-0 truncate font-medium text-ink-soft">
                      {file.name}
                    </span>
                    <span className="shrink-0 text-[0.6875rem] text-ink-faint">
                      {formatFileSize(file.size)}
                    </span>
                  </div>
                ))}
              </div>
            )}
            {item.images.length > 0 && (
              <div className="flex max-w-full flex-wrap justify-end gap-2">
                {item.images.map((image, index) => (
                  <img
                    key={`${image.mimeType}-${index}`}
                    className="size-[8.5rem] shrink-0 rounded-2xl border border-edge bg-canvas object-cover shadow-[0_7px_18px_-15px_rgba(28,25,23,0.55)] max-sm:size-28"
                    src={`data:${image.mimeType};base64,${image.data}`}
                    alt={t('app.uploadedImage', { index: index + 1 })}
                  />
                ))}
              </div>
            )}
            {editing ? (
              <form
                className="w-full"
                onSubmit={submitEdit}
              >
                <div className="flex h-[100px] flex-col rounded-[20px] bg-message-editor px-3 py-2.5">
                  <textarea
                    autoFocus
                    className="block min-h-0 w-full flex-1 resize-none overflow-y-auto bg-transparent px-1 py-0.5 text-[14px] leading-[22px] text-ink outline-none"
                    value={editText}
                    aria-label={t('actions.editMessage')}
                    disabled={branching}
                    onChange={(event) => setEditText(event.target.value)}
                    onKeyDown={(event: KeyboardEvent<HTMLTextAreaElement>) => {
                      if (event.key === 'Escape') {
                        event.preventDefault()
                        cancelEditing()
                      } else if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
                        event.preventDefault()
                        submitEdit()
                      }
                    }}
                  />
                  {branchError && (
                    <p className="mt-1 px-1 text-[0.75rem] leading-4 text-danger-soft" role="alert">
                      {branchError}
                    </p>
                  )}
                  <div className="mt-1.5 flex min-h-8 items-center justify-end gap-2">
                    <button
                      type="button"
                      className="inline-flex h-8 min-w-[4.5rem] cursor-pointer items-center justify-center rounded-full border border-edge bg-canvas px-3.5 text-[0.8125rem] font-medium text-ink-soft outline-none transition-[background-color,border-color,color] hover:border-edge-strong hover:bg-canvas-raised hover:text-ink focus-visible:ring-2 focus-visible:ring-edge-stronger focus-visible:ring-offset-1 focus-visible:ring-offset-message-editor disabled:cursor-wait disabled:opacity-50"
                      aria-label={t('actions.cancelEdit')}
                      disabled={branching}
                      onClick={cancelEditing}
                    >
                      {t('actions.cancelEditButton')}
                    </button>
                    <button
                      type="submit"
                      className="inline-flex h-8 min-w-[4.5rem] cursor-pointer items-center justify-center gap-1.5 rounded-full bg-canvas-inverse px-3.5 text-[0.8125rem] font-medium text-ink-inverse outline-none transition-[opacity,transform] hover:opacity-90 active:translate-y-px focus-visible:ring-2 focus-visible:ring-edge-stronger focus-visible:ring-offset-2 focus-visible:ring-offset-message-editor disabled:cursor-not-allowed disabled:opacity-30 motion-reduce:transition-none"
                      aria-label={t('actions.sendEditedMessage')}
                      disabled={
                        branching ||
                        branchingDisabled ||
                        (!editText.trim() && item.images.length === 0 && (item.files?.length ?? 0) === 0)
                      }
                    >
                      {branching && (
                        <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
                      )}
                      {t(
                        branching
                          ? 'actions.sendingEditedMessage'
                          : 'actions.sendEditedMessageButton',
                      )}
                    </button>
                  </div>
                </div>
              </form>
            ) : item.text ? (
              <div className="rounded-[10px] bg-canvas-sunken px-3 py-2 text-[14px] leading-[22px] whitespace-pre-wrap">
                <UserMessageText text={item.text} />
              </div>
            ) : null}
            {!editing && (item.text || item.sentAt || item.deliveryStatus === 'failed') && (
              <div className="-mt-1 flex h-7 items-center justify-end gap-2 px-0.5 text-[0.75rem] leading-4 tabular-nums">
                {item.deliveryStatus === 'failed' && (
                  <span className="text-danger-soft">{t('app.notSent')}</span>
                )}
                <div
                  className="pointer-events-none flex max-w-0 items-center gap-1 overflow-hidden opacity-0 group-focus-within/message:pointer-events-auto group-focus-within/message:max-w-48 group-focus-within/message:opacity-100 group-hover/message:pointer-events-auto group-hover/message:max-w-48 group-hover/message:opacity-100 max-md:pointer-events-auto max-md:max-w-48 max-md:opacity-100"
                  data-testid="user-message-actions"
                >
                  {item.sentAt && (
                    <time className="mr-1 shrink-0 text-ink-faint" dateTime={item.sentAt}>
                      {formatMessageTime(item.sentAt, locale)}
                    </time>
                  )}
                  {item.text && (
                    <CopyButton
                      value={item.text}
                      className="size-7 rounded-lg hover:bg-surface-active focus-visible:bg-surface-active"
                    />
                  )}
                  {onEditMessage && (
                    <Tooltip.Provider delayDuration={80}>
                      <MessageActionButton
                        icon={Pencil}
                        label={t('actions.editMessage')}
                        disabled={!item.messageID || branchingDisabled || branching}
                        onClick={startEditing}
                      />
                    </Tooltip.Provider>
                  )}
                </div>
              </div>
            )}
            <EditMessageDialog
              open={editDialogOpen}
              submitting={branching}
              error={branchError}
              onOpenChange={(open) => {
                setEditDialogOpen(open)
                if (!open) setBranchError('')
              }}
              onConfirm={() => void applyEdit()}
            />
          </div>
        </section>
      )
    case 'assistant':
      return (
        <section
          className={cn(
            'my-3 animate-[fade-in_160ms_ease-out]',
            highlighted && 'bg-surface-hover',
          )}
          data-testid="assistant-message"
          data-message-id={item.messageID}
          data-branch-point-highlighted={highlighted || undefined}
        >
          <Markdown source={item.markdown} />
          {item.complete && (
            <ResponseActions
              usage={item.usage}
              modelName={item.modelName || item.model}
              responseText={item.markdown}
              completedAt={item.completedAt}
              onOpenDiagnostics={item.runId && onOpenRunDiagnostics
                ? () => onOpenRunDiagnostics(item.runId as string)
                : undefined}
              onFork={onForkMessage ? () => {
                setBranchError('')
                setBranchDialogOpen(true)
              } : undefined}
              forkDisabled={!item.messageID || branchingDisabled}
              forking={branching}
            />
          )}
          <BranchSessionDialog
            open={branchDialogOpen}
            creating={branching}
            error={branchError}
            onOpenChange={(open) => {
              setBranchDialogOpen(open)
              if (!open) setBranchError('')
            }}
            onConfirm={() => void branchAssistant()}
          />
        </section>
      )
    case 'run':
      return <RunDuration item={item} />
    case 'thinking':
      return <Thinking item={item} />
    case 'tool':
      return <ToolCard item={item} cwd={cwd} />
    case 'task':
      return <TaskCompletion item={item} />
    case 'error':
      return (
        <div
          className="my-3 flex animate-[fade-in_160ms_ease-out] gap-2.5 border-l-2 border-danger-edge py-1 pl-3 text-danger"
          role="alert"
        >
          <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
          <div className="flex flex-col gap-0.5">
            <strong className="text-[0.8125rem] font-semibold">{t('app.somethingWentWrong')}</strong>
            <span className="text-[0.8125rem]">{item.text}</span>
          </div>
        </div>
      )
  }
}

function MessageActionButton({
  icon: Icon,
  label,
  disabled,
  iconClassName,
  type = 'button',
  onClick,
}: {
  icon: typeof Pencil
  label: string
  disabled?: boolean
  iconClassName?: string
  type?: 'button' | 'submit'
  onClick: () => void
}) {
  return (
    <Tooltip.Root>
      <Tooltip.Trigger asChild>
        <button
          className="grid size-7 shrink-0 cursor-pointer place-items-center rounded-lg text-ink-faint outline-none transition-colors hover:bg-surface-active hover:text-ink-soft focus-visible:bg-surface-active focus-visible:text-ink-soft disabled:cursor-not-allowed disabled:opacity-30"
          type={type}
          aria-label={label}
          disabled={disabled}
          onClick={onClick}
        >
          <Icon className={`size-[0.9375rem] ${iconClassName ?? ''}`} aria-hidden="true" />
        </button>
      </Tooltip.Trigger>
      <Tooltip.Portal>
        <Tooltip.Content
          side="bottom"
          sideOffset={6}
          collisionPadding={8}
          className="z-[150] animate-[fade-in_100ms_ease-out] rounded-md bg-canvas-inverse px-2 py-1 text-[0.6875rem] leading-4 font-medium whitespace-nowrap text-ink-inverse shadow-lg"
        >
          {label}
        </Tooltip.Content>
      </Tooltip.Portal>
    </Tooltip.Root>
  )
}

function UserMessageText({ text }: Pick<Extract<Item, { kind: 'user' }>, 'text'>) {
  const reference = parseSkillReference(text)
  if (!reference) return text

  return (
    <span
      className="flex min-w-0 items-center gap-1.5"
      onCopy={(event) => copySkillReference(event, reference)}
    >
      <span
        className="inline-flex h-6 max-w-[16rem] shrink-0 items-center gap-1.5 rounded-md bg-info-surface px-1.5 font-mono text-[13px] font-medium text-info"
        data-testid="skill-reference"
        title={reference.path}
      >
        <BookOpen className="size-3.5 shrink-0" strokeWidth={1.9} aria-hidden="true" />
        <span className="truncate">{reference.name}</span>
      </span>
      {reference.argumentsText && (
        <span className="min-w-0 whitespace-pre-wrap break-words">
          {reference.argumentsText}
        </span>
      )}
    </span>
  )
}

function copySkillReference(
  event: ClipboardEvent<HTMLSpanElement>,
  reference: SkillReference,
) {
  const selectedText = window.getSelection()?.toString() ?? ''
  const serialized = serializeSkillReferenceCopy(reference, selectedText)
  if (!selectedText || serialized === selectedText) return
  event.preventDefault()
  event.clipboardData.setData('text/plain', serialized)
}

function TaskCompletion({ item }: { item: Extract<Item, { kind: 'task' }> }) {
  const { t } = useI18n()
  const Icon =
    item.status === 'succeeded' ? CircleCheck : item.status === 'stopped' ? CircleStop : CircleX
  const label =
    item.status === 'succeeded'
      ? t('task.succeeded')
      : item.status === 'stopped'
        ? t('task.stopped')
        : t('task.failed', { code: item.exitCode })

  return (
    <div
      className="my-1 flex min-w-0 animate-[fade-in_160ms_ease-out] items-center gap-2 py-0.5 text-[0.8125rem] leading-5 text-ink-muted"
      title={item.outputPath}
    >
      <Icon
        className={
          item.status === 'failed' ? 'size-3.5 shrink-0 text-danger-soft' : 'size-3.5 shrink-0 text-ink-faint'
        }
        aria-hidden="true"
      />
      <span className="shrink-0">{label}</span>
      <code className="min-w-0 overflow-hidden font-mono text-[0.75rem] text-ink-faint text-ellipsis whitespace-nowrap">
    {item.description || item.command || item.taskID}
      </code>
    </div>
  )
}

function RunDuration({ item }: { item: Extract<Item, { kind: 'run' }> }) {
  const { locale, t } = useI18n()
  const [now, setNow] = useState(() => Date.now())
  const running = item.durationMs === undefined

  useEffect(() => {
    if (!running) return
    setNow(Date.now())
    const interval = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(interval)
  }, [item.startedAt, running])

  const startedAt = new Date(item.startedAt).getTime()
  const durationMs =
    item.durationMs ?? (Number.isFinite(startedAt) ? Math.max(0, now - startedAt) : 0)
  const duration = formatRunDuration(durationMs, locale)

  return (
    <div className="mt-3.5 mb-2.5 animate-[fade-in_160ms_ease-out]">
      <div className="text-[0.8125rem] leading-5 text-ink-faint tabular-nums">
        {t(running ? 'run.working' : 'run.completed', { duration })}
      </div>
    </div>
  )
}

function formatRunDuration(durationMs: number, locale: 'en' | 'zh-CN'): string {
  const totalSeconds = Math.max(0, Math.floor(durationMs / 1000))
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60

  if (locale === 'zh-CN') {
    if (hours > 0) return `${hours} 小时 ${minutes} 分 ${seconds} 秒`
    if (minutes > 0) return `${minutes} 分 ${seconds} 秒`
    return `${seconds} 秒`
  }
  if (hours > 0) return `${hours}h ${minutes}m ${seconds}s`
  if (minutes > 0) return `${minutes}m ${seconds}s`
  return `${seconds}s`
}
