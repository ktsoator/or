import { useEffect, useRef, useState } from 'react'
import { ArrowUp, Check, ChevronDown, Info, LoaderCircle, Plus, Square, X } from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import { isAPIError } from '@/api'
import type {
  ApprovalChoice,
  ApprovalItem,
  ContextUsage,
  DeliveryMode,
  MessageImage,
  ModelOption,
  PendingImage,
  PermissionMode,
  QueuedMessage,
  ThinkingLevel,
  WorkspaceSummary,
} from '@/types'
import { cn } from '@/lib/utils'
import { Approval } from './Approval'
import { ModelSettingsMenu } from './ModelSettingsMenu'
import { PermissionModeMenu } from './PermissionModeMenu'
import { ProjectPicker } from './ProjectPicker'
import { useI18n } from '@/i18n'

export function Composer({
  connected,
  running,
  approval,
  queuedMessages,
  contextUsage,
  centered = false,
  projectPickerVisible = false,
  workspaces,
  workspacePath,
  models,
  modelProvider,
  modelID,
  thinkingLevel,
  permissionMode,
  updatingSettings,
  compacting,
  onSend,
  onRemoveQueued,
  onStop,
  onResolve,
  onSelectProject,
  onBrowseProjects,
  onConfigureModel,
  onSettingsChange,
  onPermissionModeChange,
  onCompact,
}: {
  connected: boolean
  running: boolean
  approval?: ApprovalItem
  queuedMessages: QueuedMessage[]
  contextUsage?: ContextUsage
  centered?: boolean
  projectPickerVisible?: boolean
  workspaces: WorkspaceSummary[]
  workspacePath?: string
  models: ModelOption[]
  modelProvider?: string
  modelID?: string
  thinkingLevel?: ThinkingLevel
  permissionMode: PermissionMode
  updatingSettings: boolean
  compacting: boolean
  onSend: (text: string, images: MessageImage[], delivery?: DeliveryMode) => Promise<boolean>
  onRemoveQueued: (id: string) => Promise<void>
  onStop: () => void
  onResolve: (id: string, choice: ApprovalChoice) => Promise<void>
  onSelectProject: (path?: string) => void
  onBrowseProjects: () => void
  onConfigureModel: () => void
  onSettingsChange: (
    provider: string,
    model: string,
    thinkingLevel: ThinkingLevel,
  ) => Promise<void>
  onPermissionModeChange: (mode: PermissionMode) => Promise<void>
  onCompact?: () => Promise<unknown>
}) {
  const { t } = useI18n()
  const ref = useRef<HTMLTextAreaElement>(null)
  const fileRef = useRef<HTMLInputElement>(null)
  const composingRef = useRef(false)
  const submittingRef = useRef(false)
  const compactFeedbackTimerRef = useRef<number | undefined>(undefined)
  const [settingsError, setSettingsError] = useState('')
  const [attachmentError, setAttachmentError] = useState('')
  const [queueError, setQueueError] = useState('')
  const [sendError, setSendError] = useState('')
  const [compactFeedback, setCompactFeedback] = useState<CompactFeedback>()
  const [images, setImages] = useState<PendingImage[]>([])
  const [delivery, setDelivery] = useState<DeliveryMode>('steer')
  const awaitingApproval = Boolean(approval)
  const modelConfigured = Boolean(modelProvider && modelID && thinkingLevel)
  const inputDisabled =
    awaitingApproval || !connected || updatingSettings || compacting || !modelConfigured
  const settingsDisabled = running || inputDisabled
  const supportsImages = Boolean(
    models.find((model) => model.provider === modelProvider && model.id === modelID)
      ?.supportsImages,
  )

  const autosize = () => {
    const el = ref.current
    if (!el) return
    el.style.height = '0px'
    const contentHeight = el.scrollHeight
    el.style.height = Math.min(contentHeight, 240) + 'px'
  }

  useEffect(() => {
    if (!inputDisabled) ref.current?.focus()
  }, [inputDisabled])

  useEffect(() => {
    if (!running) setDelivery('steer')
  }, [running])

  useEffect(() => setSettingsError(''), [modelProvider, modelID, thinkingLevel, permissionMode])

  useEffect(() => {
    if (supportsImages) setAttachmentError('')
  }, [supportsImages])

  useEffect(
    () => () => {
      if (compactFeedbackTimerRef.current !== undefined) {
        window.clearTimeout(compactFeedbackTimerRef.current)
      }
    },
    [],
  )

  const submit = async () => {
    const el = ref.current
    if (!el || submittingRef.current) return
    const text = el.value.trim()
    if ((!text && images.length === 0) || inputDisabled) return
    if (images.length > 0 && !supportsImages) {
      setAttachmentError(t('composer.modelNoImages'))
      return
    }
    submittingRef.current = true
    setSendError('')
    try {
      const accepted = await onSend(
        text,
        images.map(({ data, mimeType }) => ({ data, mimeType })),
        running ? delivery : undefined,
      )
      if (!accepted) return
      el.value = ''
      setImages([])
      setAttachmentError('')
      autosize()
    } catch (error) {
      setSendError(error instanceof Error ? error.message : t('composer.couldNotSend'))
    } finally {
      submittingRef.current = false
    }
  }

  const addImages = async (files: FileList | null) => {
    if (!files || files.length === 0 || !supportsImages) return
    setAttachmentError('')
    const selected = Array.from(files)
    if (images.length + selected.length > maxImages) {
      setAttachmentError(t('composer.maxImages', { count: maxImages }))
      return
    }
    const allowed = new Set(['image/gif', 'image/jpeg', 'image/png', 'image/webp'])
    if (selected.some((file) => !allowed.has(file.type))) {
      setAttachmentError(t('composer.imageTypes'))
      return
    }
    if (selected.some((file) => file.size > maxImageBytes)) {
      setAttachmentError(t('composer.imageTooLarge'))
      return
    }
    const totalBytes =
      images.reduce((total, image) => total + image.size, 0) +
      selected.reduce((total, file) => total + file.size, 0)
    if (totalBytes > maxImagesBytes) {
      setAttachmentError(t('composer.imagesTooLarge'))
      return
    }
    try {
      const added = await Promise.all(selected.map(readImage))
      setImages((current) => [...current, ...added])
    } catch {
      setAttachmentError(t('composer.couldNotReadImage'))
    }
  }

  const changeSettings = async (
    provider: string,
    model: string,
    nextThinking: ThinkingLevel,
  ) => {
    setSettingsError('')
    try {
      await onSettingsChange(provider, model, nextThinking)
    } catch (error) {
      setSettingsError(error instanceof Error ? error.message : t('composer.couldNotUpdateSettings'))
    }
  }

  const changePermissionMode = async (mode: PermissionMode) => {
    setSettingsError('')
    try {
      await onPermissionModeChange(mode)
    } catch (error) {
      setSettingsError(error instanceof Error ? error.message : t('permission.couldNotUpdate'))
    }
  }

  const removeQueued = async (id: string) => {
    setQueueError('')
    try {
      await onRemoveQueued(id)
    } catch (error) {
      setQueueError(error instanceof Error ? error.message : t('composer.couldNotRemoveQueued'))
    }
  }

  const compactContext = async () => {
    if (!onCompact) return
    dismissCompactFeedback()
    try {
      await onCompact()
    } catch (error) {
      showCompactFeedback({
        kind: isAPIError(error, 'nothing_to_compact') ? 'notice' : 'error',
        message: isAPIError(error, 'nothing_to_compact')
          ? t('model.nothingToCompact')
          : t('model.compactFailed'),
      })
    }
  }

  const dismissCompactFeedback = () => {
    if (compactFeedbackTimerRef.current !== undefined) {
      window.clearTimeout(compactFeedbackTimerRef.current)
      compactFeedbackTimerRef.current = undefined
    }
    setCompactFeedback(undefined)
  }

  const showCompactFeedback = (feedback: CompactFeedback) => {
    dismissCompactFeedback()
    setCompactFeedback(feedback)
    compactFeedbackTimerRef.current = window.setTimeout(() => {
      compactFeedbackTimerRef.current = undefined
      setCompactFeedback(undefined)
    }, 4000)
  }

  return (
    <footer
      className={cn(
        'z-30 w-full',
        centered
          ? 'bg-transparent p-0'
          : 'shrink-0 bg-white px-3 pt-3 pb-4 md:px-8 max-md:pt-2',
      )}
    >
      <div className="relative mx-auto flex w-full max-w-[750px] flex-col gap-2">
        {queuedMessages.length > 0 && (
          <PendingQueue messages={queuedMessages} onRemove={(id) => void removeQueued(id)} />
        )}
        {approval && <Approval key={approval.id} item={approval} onResolve={onResolve} />}

        <div
          hidden={awaitingApproval}
          className={cn(
            'rounded-[28px] border border-stone-200 bg-white',
            !centered &&
              'transition-colors focus-within:border-stone-300',
          )}
        >
          <div
            className="grid min-h-24 grid-cols-[2.5rem_minmax(0,1fr)_auto] grid-rows-[auto_2.5rem] items-center gap-x-3 gap-y-1 px-3 py-2.5 max-sm:gap-x-2"
          >
            <button
              className="group relative col-start-1 row-start-2 grid size-10 cursor-pointer place-items-center rounded-full text-stone-700 transition-colors hover:bg-stone-100 disabled:cursor-not-allowed disabled:opacity-30"
              type="button"
              aria-label={
                supportsImages ? t('composer.attachImages') : t('composer.currentModelNoImages')
              }
              title={
                supportsImages ? t('composer.attachImages') : t('composer.currentModelNoImages')
              }
              disabled={inputDisabled || !supportsImages || images.length >= maxImages}
              onClick={() => fileRef.current?.click()}
            >
              <Plus className="size-5" aria-hidden="true" />
            </button>
            <input
              ref={fileRef}
              className="sr-only"
              type="file"
              accept="image/png,image/jpeg,image/webp,image/gif"
              multiple
              tabIndex={-1}
              onChange={(event) => {
                void addImages(event.target.files)
                event.target.value = ''
              }}
            />
            <div className="col-start-2 row-start-2 flex min-w-0 items-center gap-1">
              <PermissionModeMenu
                value={permissionMode}
                disabled={settingsDisabled}
                onChange={changePermissionMode}
              />
              {projectPickerVisible && (
                <ProjectPicker
                  workspaces={workspaces}
                  selectedPath={workspacePath}
                  disabled={inputDisabled}
                  onSelect={onSelectProject}
                  onBrowse={onBrowseProjects}
                />
              )}
            </div>
            <div className="col-span-3 col-start-1 row-start-1 flex min-w-0 flex-col gap-2">
              {images.length > 0 && (
                <div className="flex flex-wrap gap-2 px-1 pt-1">
                  {images.map((image) => (
                    <div
                      key={image.id}
                      className="group/image relative size-16 overflow-hidden rounded-xl border border-stone-200 bg-stone-50 shadow-sm"
                    >
                      <img
                        className="size-full object-cover"
                        src={`data:${image.mimeType};base64,${image.data}`}
                        alt={image.name}
                      />
                      <button
                        className="absolute top-1 right-1 grid size-5 cursor-pointer place-items-center rounded-full bg-stone-900/85 text-white opacity-100 shadow-sm transition-opacity focus-visible:opacity-100 md:opacity-0 md:group-hover/image:opacity-100"
                        type="button"
                        aria-label={t('composer.removeImage', { name: image.name })}
                        onClick={() => {
                          setImages((current) =>
                            current.filter((item) => item.id !== image.id),
                          )
                          setAttachmentError('')
                        }}
                      >
                        <X className="size-3" aria-hidden="true" />
                      </button>
                    </div>
                  ))}
                </div>
              )}
              <textarea
                ref={ref}
                rows={1}
                disabled={inputDisabled}
                className="block max-h-[15rem] min-h-8 w-full min-w-0 resize-none overflow-y-auto border-0 bg-transparent px-1 py-1.5 text-[var(--chat-font-size)] leading-6 text-stone-900 outline-none placeholder:text-stone-400 disabled:cursor-not-allowed disabled:bg-transparent"
                placeholder={
                  awaitingApproval
                    ? t('composer.resolveApprovalPlaceholder')
                    : compacting
                      ? t('composer.compactingContext')
                      : updatingSettings
                        ? t('composer.updatingSettings')
                    : !modelConfigured
                      ? t('composer.configureModelPlaceholder')
                    : connected
                      ? running
                        ? delivery === 'steer'
                          ? t('composer.guideRun')
                          : t('composer.queueFollowUpPlaceholder')
                        : t('composer.askAnything')
                      : t('composer.waitingForAPI')
                }
                onInput={autosize}
                onCompositionStart={() => {
                  composingRef.current = true
                }}
                onCompositionEnd={() => {
                  composingRef.current = false
                }}
                onKeyDown={(event) => {
                  if (
                    composingRef.current ||
                    event.nativeEvent.isComposing ||
                    event.nativeEvent.keyCode === 229
                  ) {
                    return
                  }
                  if (event.key === 'Enter' && !event.shiftKey) {
                    event.preventDefault()
                    void submit()
                  }
                }}
              />
            </div>
            <div
              className="col-start-3 row-start-2 flex shrink-0 items-center gap-2.5 max-sm:gap-1.5"
            >
              {modelConfigured ? (
                <ModelSettingsMenu
                  models={models}
                  modelProvider={modelProvider}
                  modelID={modelID}
                  thinkingLevel={thinkingLevel}
                  contextUsage={contextUsage}
                  disabled={settingsDisabled}
                  onChange={changeSettings}
                  compacting={compacting}
                  onCompact={onCompact ? compactContext : undefined}
                />
              ) : (
                <button
                  type="button"
                  onClick={onConfigureModel}
                  className="inline-flex h-9 cursor-pointer items-center rounded-full px-3 text-[0.8125rem] font-medium text-stone-500 outline-none transition-colors hover:bg-[rgb(241,241,241)] hover:text-stone-950 focus-visible:bg-[rgb(241,241,241)]"
                >
                  {t('composer.configureModel')}
                </button>
              )}
              {running && !awaitingApproval && (
                <RunDeliveryMenu value={delivery} onValueChange={setDelivery} />
              )}
              {running && !awaitingApproval && (
                <button
                  className="group relative grid size-9 cursor-pointer place-items-center rounded-full bg-stone-200 text-stone-700 transition-colors hover:bg-stone-300 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-400"
                  type="button"
                  aria-label={t('composer.stopGenerating')}
                  onClick={onStop}
                >
                  <Square className="size-3 fill-current" aria-hidden="true" />
                  <span
                    className="pointer-events-none absolute right-0 bottom-[calc(100%+0.5625rem)] z-50 translate-y-1 whitespace-nowrap rounded-md bg-stone-900 px-2.5 py-1.5 text-[0.75rem] leading-4 font-medium text-white opacity-0 shadow-lg transition-[opacity,transform] duration-150 group-hover:translate-y-0 group-hover:opacity-100 group-focus-visible:translate-y-0 group-focus-visible:opacity-100"
                    aria-hidden="true"
                  >
                    {t('composer.stopGenerating')}
                  </span>
                </button>
              )}
              <button
                className="group relative grid size-10 cursor-pointer place-items-center rounded-full bg-black text-white transition-colors hover:bg-stone-800 disabled:cursor-not-allowed disabled:opacity-25 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-400"
                type="button"
                aria-label={
                  awaitingApproval
                    ? t('composer.resolveApprovalFirst')
                    : connected
                      ? running
                        ? delivery === 'steer'
                          ? t('composer.steerRun')
                          : t('composer.queueFollowUp')
                        : t('composer.sendPrompt')
                      : t('composer.waitingForCodingAPI')
                }
                disabled={inputDisabled}
                onClick={() => void submit()}
              >
                <ArrowUp className="size-4" aria-hidden="true" />
                <span
                  className="pointer-events-none absolute right-0 bottom-[calc(100%+0.5625rem)] z-50 flex translate-y-1 items-center gap-2 whitespace-nowrap rounded-md bg-stone-900 px-2.5 py-1.5 text-[0.75rem] leading-4 font-medium text-white opacity-0 shadow-lg transition-[opacity,transform] duration-150 group-hover:translate-y-0 group-hover:opacity-100 group-focus-visible:translate-y-0 group-focus-visible:opacity-100"
                  aria-hidden="true"
                >
                  <span>
                    {awaitingApproval
                      ? t('composer.resolveApprovalFirst')
                      : connected
                        ? running
                          ? delivery === 'steer'
                            ? t('composer.steerRun')
                            : t('composer.queueFollowUp')
                          : t('composer.sendPrompt')
                        : t('composer.waitingForAPIShort')}
                  </span>
                  {connected && !awaitingApproval && (
                    <kbd className="font-mono text-[0.6875rem] font-normal text-stone-400">↵</kbd>
                  )}
                </span>
              </button>
            </div>
          </div>
        </div>
        {compactFeedback && (
          <div
            className={cn(
              'absolute right-2 bottom-[calc(100%+0.625rem)] z-50 flex max-w-[calc(100vw-2rem)] animate-[fade-in_140ms_ease-out] items-center gap-2 border px-2.5 py-2 text-[0.8125rem] leading-5 shadow-[0_12px_32px_-18px_rgba(28,25,23,0.45)]',
              compactFeedback.kind === 'notice'
                ? 'rounded-lg border-stone-200 bg-white text-stone-700'
                : 'rounded-lg border-red-200 bg-red-50 text-red-800',
            )}
            role={compactFeedback.kind === 'error' ? 'alert' : 'status'}
          >
            <Info
              className={cn(
                'size-4 shrink-0',
                compactFeedback.kind === 'notice' ? 'text-stone-500' : 'text-red-600',
              )}
              aria-hidden="true"
            />
            <span>{compactFeedback.message}</span>
            <button
              type="button"
              className="grid size-6 shrink-0 cursor-pointer place-items-center rounded-md text-current opacity-55 outline-none transition-[background-color,opacity] hover:bg-black/5 hover:opacity-100 focus-visible:bg-black/5 focus-visible:opacity-100"
              aria-label={t('model.dismissCompactFeedback')}
              title={t('model.dismissCompactFeedback')}
              onClick={dismissCompactFeedback}
            >
              <X className="size-3.5" aria-hidden="true" />
            </button>
          </div>
        )}
        {(settingsError || attachmentError || queueError || sendError) && (
          <p className="px-4 text-[0.75rem] leading-5 text-red-700" role="alert">
            {settingsError || attachmentError || queueError || sendError}
          </p>
        )}
      </div>
    </footer>
  )
}

type CompactFeedback = {
  kind: 'notice' | 'error'
  message: string
}

function PendingQueue({
  messages,
  onRemove,
}: {
  messages: QueuedMessage[]
  onRemove: (id: string) => void
}) {
  const { t } = useI18n()
  return (
    <section
      className="overflow-hidden rounded-[18px] border border-stone-200/90 bg-[rgb(252,252,252)] text-stone-700 shadow-[0_8px_24px_-22px_rgba(28,25,23,0.5)]"
      aria-label={t('queue.pendingMessages')}
      aria-live="polite"
    >
      <div className="flex h-8 items-center justify-between px-3.5 text-[0.71875rem] leading-none text-stone-500">
        <span className="font-medium text-stone-600">{t('queue.upNext')}</span>
        <span>{messages.length}</span>
      </div>
      <div className="max-h-[8.25rem] overflow-y-auto border-t border-stone-200/80">
        {messages.map((message, index) => (
          <div
            key={message.id}
            className={cn(
              'group/queue flex min-h-11 items-center gap-2.5 py-2 pr-2 pl-3.5 text-[0.8125rem]',
              index > 0 && 'border-t border-stone-200/70',
            )}
          >
            <span
              className={cn(
                'size-1.5 shrink-0 rounded-full',
                message.status === 'failed' ? 'bg-red-500' : 'bg-stone-400',
              )}
              aria-hidden="true"
            />
            <span className="shrink-0 font-medium text-stone-700">
              {message.delivery === 'steer' ? t('queue.steer') : t('queue.followUp')}
            </span>
            <span className="min-w-0 flex-1 truncate text-stone-500">
              {message.text ||
                `${message.images.length} ${
                  message.images.length === 1 ? t('queue.image') : t('queue.images')
                }`}
            </span>
            {message.text && message.images.length > 0 && (
              <span className="shrink-0 text-[0.71875rem] text-stone-400">
                +{message.images.length}{' '}
                {message.images.length === 1 ? t('queue.image') : t('queue.images')}
              </span>
            )}
            <span
              className={cn(
                'shrink-0 text-[0.71875rem]',
                message.status === 'failed' ? 'text-red-600' : 'text-stone-400',
              )}
            >
              {message.status === 'failed'
                ? t('app.notSent')
                : message.status === 'removing'
                  ? t('queue.removing')
                  : t('queue.waiting')}
            </span>
            <button
              className="grid size-7 shrink-0 cursor-pointer place-items-center rounded-lg text-stone-400 outline-none transition-colors hover:bg-stone-200/80 hover:text-stone-700 focus-visible:ring-2 focus-visible:ring-stone-300 disabled:cursor-wait disabled:opacity-55"
              type="button"
              aria-label={
                message.delivery === 'steer'
                  ? t('queue.removeSteer')
                  : t('queue.removeFollowUp')
              }
              title={t('queue.remove')}
              disabled={message.status === 'removing'}
              onClick={() => onRemove(message.id)}
            >
              {message.status === 'removing' ? (
                <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
              ) : (
                <X className="size-3.5" aria-hidden="true" />
              )}
            </button>
          </div>
        ))}
      </div>
    </section>
  )
}

function RunDeliveryMenu({
  value,
  onValueChange,
}: {
  value: DeliveryMode
  onValueChange: (value: DeliveryMode) => void
}) {
  const { t } = useI18n()
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          className="group inline-flex h-9 cursor-pointer items-center gap-1 rounded-full px-2.5 text-[0.8125rem] font-medium text-stone-600 outline-none transition-colors hover:bg-[rgb(241,241,241)] focus-visible:ring-2 focus-visible:ring-stone-300 data-[state=open]:bg-[rgb(237,237,237)]"
          type="button"
          aria-label={t('delivery.choose')}
        >
          <span>{value === 'steer' ? t('queue.steer') : t('queue.followUp')}</span>
          <ChevronDown
            className="size-3.5 text-stone-400 transition-transform group-data-[state=open]:rotate-180"
            aria-hidden="true"
          />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          side="top"
          align="end"
          sideOffset={7}
          collisionPadding={10}
          className="z-[110] min-w-[14.75rem] animate-[fade-in_110ms_ease-out] rounded-2xl border border-stone-200 bg-white p-1 text-[0.8125rem] text-stone-900 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
        >
          <DropdownMenu.RadioGroup
            className="flex flex-col gap-0.5"
            value={value}
            onValueChange={(next) => onValueChange(next as DeliveryMode)}
          >
            <DeliveryOption
              value="steer"
              label={t('composer.steerRun')}
              hint={t('delivery.steerHint')}
            />
            <DeliveryOption
              value="followup"
              label={t('composer.queueFollowUp')}
              hint={t('delivery.followUpHint')}
            />
          </DropdownMenu.RadioGroup>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

function DeliveryOption({ value, label, hint }: { value: DeliveryMode; label: string; hint: string }) {
  return (
    <DropdownMenu.RadioItem
      value={value}
      className="relative flex h-10 cursor-default select-none items-center gap-2 rounded-[10px] px-2.5 pr-8 outline-none data-[highlighted]:bg-[rgb(241,241,241)] data-[state=checked]:bg-[rgb(237,237,237)]"
    >
      <span className="font-medium">{label}</span>
      <span className="ml-auto text-[0.71875rem] text-stone-400">{hint}</span>
      <DropdownMenu.ItemIndicator className="absolute right-2 grid size-4 place-items-center text-stone-700">
        <Check className="size-3.5" aria-hidden="true" />
      </DropdownMenu.ItemIndicator>
    </DropdownMenu.RadioItem>
  )
}

const maxImages = 4
const maxImageBytes = 10 * 1024 * 1024
const maxImagesBytes = 20 * 1024 * 1024

function readImage(file: File): Promise<PendingImage> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(reader.error)
    reader.onload = () => {
      const result = typeof reader.result === 'string' ? reader.result : ''
      const comma = result.indexOf(',')
      if (comma < 0) {
        reject(new Error('invalid image data'))
        return
      }
      resolve({
        id: `${file.name}-${file.lastModified}-${crypto.randomUUID()}`,
        name: file.name,
        size: file.size,
        mimeType: file.type,
        data: result.slice(comma + 1),
      })
    }
    reader.readAsDataURL(file)
  })
}
