import { useEffect, useRef, useState } from 'react'
import { ArrowUp, Plus, Square, X } from 'lucide-react'
import type {
  ConfirmItem,
  MessageImage,
  ModelOption,
  PendingImage,
  ThinkingLevel,
} from '@/types'
import { cn } from '@/lib/utils'
import { Approval } from './Approval'
import { ModelSettingsMenu } from './ModelSettingsMenu'

export function Composer({
  connected,
  running,
  confirmation,
  centered = false,
  models,
  modelProvider,
  modelID,
  thinkingLevel,
  updatingSettings,
  onSend,
  onStop,
  onResolve,
  onSettingsChange,
}: {
  connected: boolean
  running: boolean
  confirmation?: ConfirmItem
  centered?: boolean
  models: ModelOption[]
  modelProvider?: string
  modelID?: string
  thinkingLevel?: ThinkingLevel
  updatingSettings: boolean
  onSend: (text: string, images: MessageImage[]) => void
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
  const [images, setImages] = useState<PendingImage[]>([])
  const awaitingApproval = Boolean(confirmation)
  const disabled = running || awaitingApproval || !connected || updatingSettings
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
    if (!disabled) ref.current?.focus()
  }, [disabled])

  useEffect(() => setSettingsError(''), [modelProvider, modelID, thinkingLevel])

  useEffect(() => {
    if (supportsImages) setAttachmentError('')
  }, [supportsImages])

  const submit = () => {
    const el = ref.current
    if (!el) return
    const text = el.value.trim()
    if ((!text && images.length === 0) || disabled) return
    if (images.length > 0 && !supportsImages) {
      setAttachmentError('The selected model does not support images')
      return
    }
    onSend(
      text,
      images.map(({ data, mimeType }) => ({ data, mimeType })),
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

        <div className="rounded-[28px] border border-stone-200 bg-white shadow-[0_10px_30px_-20px_rgba(28,25,23,0.38)] transition-[border-color,box-shadow] focus-within:border-stone-400 focus-within:shadow-[0_12px_32px_-20px_rgba(28,25,23,0.48)]">
          <div
            className="grid min-h-24 grid-cols-[40px_minmax(0,1fr)_auto] grid-rows-[auto_40px] items-center gap-x-3 gap-y-1 px-3 py-2.5 max-sm:gap-x-2"
          >
            <button
              className="group relative col-start-1 row-start-2 grid size-10 cursor-pointer place-items-center rounded-full text-stone-700 transition-colors hover:bg-stone-100 disabled:cursor-not-allowed disabled:opacity-30"
              type="button"
              aria-label={supportsImages ? 'Attach images' : 'Current model does not support images'}
              title={supportsImages ? 'Attach images' : 'Current model does not support images'}
              disabled={disabled || !supportsImages || images.length >= maxImages}
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
                disabled={disabled}
                className="block max-h-[240px] min-h-8 w-full min-w-0 resize-none overflow-y-auto border-0 bg-transparent px-1 py-1.5 text-[16.5px] leading-6 text-stone-900 outline-none placeholder:text-stone-400 disabled:cursor-not-allowed disabled:bg-transparent"
                placeholder={
                  awaitingApproval
                    ? 'Resolve the approval above to continue…'
                    : updatingSettings
                      ? 'Updating model settings…'
                    : connected
                      ? 'Ask anything'
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
                disabled={disabled}
                onChange={changeSettings}
              />
              {running && !awaitingApproval ? (
                <button
                  className="group relative grid size-10 cursor-pointer place-items-center rounded-full bg-stone-700 text-white transition-colors hover:bg-stone-800 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-400"
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
              ) : (
                <button
                  className="group relative grid size-10 cursor-pointer place-items-center rounded-full bg-black text-white transition-colors hover:bg-stone-800 disabled:cursor-not-allowed disabled:opacity-25 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-400"
                  type="button"
                  aria-label={
                    awaitingApproval
                      ? 'Resolve the approval first'
                      : connected
                        ? 'Send prompt'
                        : 'Waiting for coding API'
                  }
                  disabled={disabled}
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
                          ? 'Send prompt'
                          : 'Waiting for API'}
                    </span>
                    {connected && !awaitingApproval && (
                      <kbd className="font-mono text-[11px] font-normal text-stone-400">↵</kbd>
                    )}
                  </span>
                </button>
              )}
            </div>
          </div>
        </div>
        {(settingsError || attachmentError) && (
          <p className="px-4 text-[12px] leading-5 text-red-700" role="alert">
            {settingsError || attachmentError}
          </p>
        )}
      </div>
    </footer>
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
