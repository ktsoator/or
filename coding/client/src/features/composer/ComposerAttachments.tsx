import { FileCode2, X } from 'lucide-react'
import type { PendingFile, PendingImage } from '@/types'
import { formatFileSize } from '@/shared/attachments'
import { useI18n } from '@/i18n'

export function ComposerAttachments({
  files,
  images,
  onRemoveFile,
  onRemoveImage,
}: {
  files: PendingFile[]
  images: PendingImage[]
  onRemoveFile: (id: string) => void
  onRemoveImage: (id: string) => void
}) {
  const { t } = useI18n()

  return (
    <>
      {files.length > 0 && (
        <div className="flex flex-wrap gap-1.5 px-1 pt-1">
          {files.map((file) => (
            <div
              key={file.id}
              className="group/file flex h-8 max-w-[15rem] items-center gap-1.5 rounded-lg border border-edge bg-canvas-raised pr-1 pl-2 text-[0.75rem] text-ink-muted"
            >
              <FileCode2 className="size-3.5 shrink-0 text-ink-muted" aria-hidden="true" />
              <span className="min-w-0 truncate font-medium text-ink-soft">{file.name}</span>
              <span className="shrink-0 text-[0.6875rem] text-ink-faint">
                {formatFileSize(file.size)}
              </span>
              <button
                className="grid size-6 shrink-0 cursor-pointer place-items-center rounded-md text-ink-faint outline-none transition-colors hover:bg-canvas-strong/70 hover:text-ink-soft focus-visible:bg-canvas-strong/70 focus-visible:text-ink-soft"
                type="button"
                aria-label={t('composer.removeFile', { name: file.name })}
                title={t('composer.removeFile', { name: file.name })}
                onClick={() => onRemoveFile(file.id)}
              >
                <X className="size-3.5" aria-hidden="true" />
              </button>
            </div>
          ))}
        </div>
      )}
      {images.length > 0 && (
        <div className="flex flex-wrap gap-2 px-1 pt-1">
          {images.map((image) => (
            <div
              key={image.id}
              className="group/image relative size-16 overflow-hidden rounded-xl border border-edge bg-canvas-raised shadow-sm"
            >
              <img
                className="size-full object-cover"
                src={`data:${image.mimeType};base64,${image.data}`}
                alt={image.name}
              />
              <button
                className="absolute top-1 right-1 grid size-5 cursor-pointer place-items-center rounded-full bg-scrim/85 text-ink-inverse opacity-100 shadow-sm transition-opacity focus-visible:opacity-100 md:opacity-0 md:group-hover/image:opacity-100"
                type="button"
                aria-label={t('composer.removeImage', { name: image.name })}
                onClick={() => onRemoveImage(image.id)}
              >
                <X className="size-3" aria-hidden="true" />
              </button>
            </div>
          ))}
        </div>
      )}
    </>
  )
}
