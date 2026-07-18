import { useEffect, useRef, useState } from 'react'
import { ArrowUp, Check, ChevronDown, LoaderCircle, Plus, Square, X } from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import type {
  ConfirmItem,
  DeliveryMode,
  MessageImage,
  ModelOption,
  PendingImage,
  QueuedMessage,
  ThinkingLevel,
} from '@/types'
import { cn } from '@/lib/utils'
import { Approval } from './Approval'
import { ModelSettingsMenu } from './ModelSettingsMenu'

export function Composer({
  connected,
  running,
  confirmation,
  queuedMessages,
  centered = false,
  models,
  modelProvider,
  modelID,
  thinkingLevel,
  updatingSettings,
  onSend,
  onRemoveQueued,
  onStop,
  onResolve,
  onSettingsChange,
}: {
  connected: boolean
  running: boolean
  confirmation?: ConfirmItem
  queuedMessages: QueuedMessage[]
  centered?: boolean
  models: ModelOption[]
  modelProvider?: string
  modelID?: string
  thinkingLevel?: ThinkingLevel
  updatingSettings: boolean
  onSend: (text: string, images: MessageImage[], delivery?: DeliveryMode) => void
  onRemoveQueued: (id: string) => Promise<void>
  onStop: () => void
  onResolve: (id: string, allow: boolean) => Promise<void>
  onSettingsChange: (
    provider: string,
    model: string,
    thinkingLevel: ThinkingLevel,
  ) => Promise<void>
}) {
  const ref = useRef<HTMLTextAreaElement>(null)
  const fileRef = useRef<HTMLInputElement>(null)
  const [settingsError, setSettingsError] = useState('')
  const [attachmentError, setAttachmentError] = useState('')
  const [queueError, setQueueError] = useState('')
  const [images, setImages] = useState<PendingImage[]>([])
  const [delivery, setDelivery] = useState<DeliveryMode>('steer')
  const awaitingApproval = Boolean(confirmation)
  const inputDisabled = awaitingApproval || !connected || updatingSettings
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

  useEffect(() => setSettingsError(''), [modelProvider, modelID, thinkingLevel])

  useEffect(() => {
    if (supportsImages) setAttachmentError('')
  }, [supportsImages])

  const submit = () => {
    const el = ref.current
    if (!el) return
    const text = el.value.trim()
    if ((!text && images.length === 0) || inputDisabled) return
    if (images.length > 0 && !supportsImages) {
      setAttachmentError('The selected model does not support images')
      return
    }
    onSend(
      text,
      images.map(({ data, mimeType }) => ({ data, mimeType })),
      running ? delivery : undefined,
    )
    el.value = ''
    setImages([])
    setAttachmentError('')
    autosize()
  }

  const addImages = async (files: FileList | null) => {
    if (!files || files.length === 0 || !supportsImages) return
    setAttachmentError('')
    const selected = Array.from(files)
    if (images.length + selected.length > maxImages) {
      setAttachmentError(`You can attach up to ${maxImages} images`)
      return
    }
    const allowed = new Set(['image/gif', 'image/jpeg', 'image/png', 'image/webp'])
    if (selected.some((file) => !allowed.has(file.type))) {
      setAttachmentError('Use PNG, JPEG, WebP, or GIF images')
      return
    }
    if (selected.some((file) => file.size > maxImageBytes)) {
      setAttachmentError('Each image must be 10 MB or smaller')
      return
    }
    const totalBytes =
      images.reduce((total, image) => total + image.size, 0) +
      selected.reduce((total, file) => total + file.size, 0)
    if (totalBytes > maxImagesBytes) {
      setAttachmentError('Images must total 20 MB or less')
      return
    }
    try {
      const added = await Promise.all(selected.map(readImage))
      setImages((current) => [...current, ...added])
    } catch {
      setAttachmentError('Could not read the selected image')
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
      setSettingsError(error instanceof Error ? error.message : 'Could not update model settings')
    }
  }

  const removeQueued = async (id: string) => {
    setQueueError('')
    try {
      await onRemoveQueued(id)
    } catch (error) {
      setQueueError(error instanceof Error ? error.message : 'Could not remove queued message')
    }
  }

  return (
    <footer
      className={cn(
        'z-30 w-full',
        centered
          ? 'bg-transparent p-0'
          : 'shrink-0 bg-white px-6 pt-3 pb-4 max-md:px-3 max-md:pt-2',
      )}
    >
      <div className="mx-auto flex w-full max-w-[896px] flex-col gap-2">
        {confirmation && <Approval key={confirmation.id} item={confirmation} onResolve={onResolve} />}
        {queuedMessages.length > 0 && (
          <PendingQueue messages={queuedMessages} onRemove={(id) => void removeQueued(id)} />
        )}

        <div className="rounded-[28px] border border-stone-200 bg-white shadow-[0_10px_30px_-20px_rgba(28,25,23,0.38)] transition-[border-color,box-shadow] focus-within:border-stone-400 focus-within:shadow-[0_12px_32px_-20px_rgba(28,25,23,0.48)]">
          <div
            className="grid min-h-24 grid-cols-[40px_minmax(0,1fr)_auto] grid-rows-[auto_40px] items-center gap-x-3 gap-y-1 px-3 py-2.5 max-sm:gap-x-2"
          >
            <button
              className="group relative col-start-1 row-start-2 grid size-10 cursor-pointer place-items-center rounded-full text-stone-700 transition-colors hover:bg-stone-100 disabled:cursor-not-allowed disabled:opacity-30"
              type="button"
              aria-label={supportsImages ? 'Attach images' : 'Current model does not support images'}
              title={supportsImages ? 'Attach images' : 'Current model does not support images'}
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
                        aria-label={`Remove ${image.name}`}
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
                className="block max-h-[240px] min-h-8 w-full min-w-0 resize-none overflow-y-auto border-0 bg-transparent px-1 py-1.5 text-[16.5px] leading-6 text-stone-900 outline-none placeholder:text-stone-400 disabled:cursor-not-allowed disabled:bg-transparent"
                placeholder={
                  awaitingApproval
                    ? 'Resolve the approval above to continue…'
                    : updatingSettings
                      ? 'Updating model settings…'
                    : connected
                      ? running
                        ? delivery === 'steer'
                          ? 'Guide the current run…'
                          : 'Queue a follow-up…'
                        : 'Ask anything'
                      : 'Waiting for coding API…'
                }
                onInput={autosize}
                onKeyDown={(event) => {
                  if (event.key === 'Enter' && !event.shiftKey) {
                    event.preventDefault()
                    submit()
                  }
                }}
              />
            </div>
            <div
              className="col-start-3 row-start-2 flex shrink-0 items-center gap-2.5 max-sm:gap-1.5"
            >
              <ModelSettingsMenu
                models={models}
                modelProvider={modelProvider}
                modelID={modelID}
                thinkingLevel={thinkingLevel}
                disabled={settingsDisabled}
                onChange={changeSettings}
              />
              {running && !awaitingApproval && (
                <RunDeliveryMenu value={delivery} onValueChange={setDelivery} />
              )}
              {running && !awaitingApproval && (
                <button
                  className="group relative grid size-9 cursor-pointer place-items-center rounded-full bg-stone-200 text-stone-700 transition-colors hover:bg-stone-300 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-400"
                  type="button"
                  aria-label="Stop generating"
                  onClick={onStop}
                >
                  <Square className="size-3 fill-current" aria-hidden="true" />
                  <span
                    className="pointer-events-none absolute right-0 bottom-[calc(100%+9px)] z-50 translate-y-1 whitespace-nowrap rounded-md bg-stone-900 px-2.5 py-1.5 text-[12px] leading-4 font-medium text-white opacity-0 shadow-lg transition-[opacity,transform] duration-150 group-hover:translate-y-0 group-hover:opacity-100 group-focus-visible:translate-y-0 group-focus-visible:opacity-100"
                    aria-hidden="true"
                  >
                    Stop generating
                  </span>
                </button>
              )}
              <button
                className="group relative grid size-10 cursor-pointer place-items-center rounded-full bg-black text-white transition-colors hover:bg-stone-800 disabled:cursor-not-allowed disabled:opacity-25 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-400"
                type="button"
                aria-label={
                  awaitingApproval
                    ? 'Resolve the approval first'
                    : connected
                      ? running
                        ? delivery === 'steer'
                          ? 'Steer current run'
                          : 'Queue follow-up'
                        : 'Send prompt'
                      : 'Waiting for coding API'
                }
                disabled={inputDisabled}
                onClick={submit}
              >
                <ArrowUp className="size-4" aria-hidden="true" />
                <span
                  className="pointer-events-none absolute right-0 bottom-[calc(100%+9px)] z-50 flex translate-y-1 items-center gap-2 whitespace-nowrap rounded-md bg-stone-900 px-2.5 py-1.5 text-[12px] leading-4 font-medium text-white opacity-0 shadow-lg transition-[opacity,transform] duration-150 group-hover:translate-y-0 group-hover:opacity-100 group-focus-visible:translate-y-0 group-focus-visible:opacity-100"
                  aria-hidden="true"
                >
                  <span>
                    {awaitingApproval
                      ? 'Resolve approval first'
                      : connected
                        ? running
                          ? delivery === 'steer'
                            ? 'Steer current run'
                            : 'Queue follow-up'
                          : 'Send prompt'
                        : 'Waiting for API'}
                  </span>
                  {connected && !awaitingApproval && (
                    <kbd className="font-mono text-[11px] font-normal text-stone-400">↵</kbd>
                  )}
                </span>
              </button>
            </div>
          </div>
        </div>
        {(settingsError || attachmentError || queueError) && (
          <p className="px-4 text-[12px] leading-5 text-red-700" role="alert">
            {settingsError || attachmentError || queueError}
          </p>
        )}
      </div>
    </footer>
  )
}

function PendingQueue({
  messages,
  onRemove,
}: {
  messages: QueuedMessage[]
  onRemove: (id: string) => void
}) {
  return (
    <section
      className="overflow-hidden rounded-[18px] border border-stone-200/90 bg-[rgb(252,252,252)] text-stone-700 shadow-[0_8px_24px_-22px_rgba(28,25,23,0.5)]"
      aria-label="Pending messages"
      aria-live="polite"
    >
      <div className="flex h-8 items-center justify-between px-3.5 text-[11.5px] leading-none text-stone-500">
        <span className="font-[580] text-stone-600">Up next</span>
        <span>{messages.length}</span>
      </div>
      <div className="max-h-[132px] overflow-y-auto border-t border-stone-200/80">
        {messages.map((message, index) => (
          <div
            key={message.id}
            className={cn(
              'group/queue flex min-h-11 items-center gap-2.5 py-2 pr-2 pl-3.5 text-[13px]',
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
            <span className="shrink-0 font-[590] text-stone-700">
              {message.delivery === 'steer' ? 'Steer' : 'Follow-up'}
            </span>
            <span className="min-w-0 flex-1 truncate text-stone-500">
              {message.text || imageLabel(message.images.length)}
            </span>
            {message.text && message.images.length > 0 && (
              <span className="shrink-0 text-[11.5px] text-stone-400">
                +{message.images.length} {message.images.length === 1 ? 'image' : 'images'}
              </span>
            )}
            <span
              className={cn(
                'shrink-0 text-[11.5px]',
                message.status === 'failed' ? 'text-red-600' : 'text-stone-400',
              )}
            >
              {message.status === 'failed'
                ? 'Not sent'
                : message.status === 'removing'
                  ? 'Removing\u2026'
                  : 'Waiting'}
            </span>
            <button
              className="grid size-7 shrink-0 cursor-pointer place-items-center rounded-lg text-stone-400 outline-none transition-colors hover:bg-stone-200/80 hover:text-stone-700 focus-visible:ring-2 focus-visible:ring-stone-300 disabled:cursor-wait disabled:opacity-55"
              type="button"
              aria-label={`Remove queued ${message.delivery === 'steer' ? 'steer' : 'follow-up'} message`}
              title="Remove from queue"
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

function imageLabel(count: number): string {
  return count === 1 ? '1 image' : `${count} images`
}

function RunDeliveryMenu({
  value,
  onValueChange,
}: {
  value: DeliveryMode
  onValueChange: (value: DeliveryMode) => void
}) {
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          className="group inline-flex h-9 cursor-pointer items-center gap-1 rounded-full px-2.5 text-[13px] font-medium text-stone-600 outline-none transition-colors hover:bg-[rgb(241,241,241)] focus-visible:ring-2 focus-visible:ring-stone-300 data-[state=open]:bg-[rgb(241,241,241)]"
          type="button"
          aria-label="Choose how this message is delivered"
        >
          <span>{value === 'steer' ? 'Steer' : 'Follow-up'}</span>
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
          className="z-[110] min-w-[236px] animate-[fade-in_110ms_ease-out] rounded-2xl border border-stone-200 bg-white p-1 text-[13px] text-stone-900 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
        >
          <DropdownMenu.RadioGroup
            value={value}
            onValueChange={(next) => onValueChange(next as DeliveryMode)}
          >
            <DeliveryOption value="steer" label="Steer current run" hint="Apply on the next turn" />
            <DeliveryOption value="followup" label="Queue follow-up" hint="Run after this reply" />
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
      className="relative flex h-10 cursor-default select-none items-center gap-2 rounded-[10px] px-2.5 pr-8 outline-none data-[highlighted]:bg-[rgb(241,241,241)] data-[state=checked]:bg-[rgb(241,241,241)]"
    >
      <span className="font-medium">{label}</span>
      <span className="ml-auto text-[11.5px] text-stone-400">{hint}</span>
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
