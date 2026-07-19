import { useCallback, useEffect, useState } from 'react'
import { ArrowUp, Folder, LoaderCircle, X } from 'lucide-react'
import { apiURL } from '@/api'
import type { DirectoryListing } from '@/types'
import { useI18n } from '@/i18n'

export function WorkspacePickerDialog({
  initialPath,
  onClose,
  onSelect,
}: {
  initialPath?: string
  onClose: () => void
  onSelect: (path: string) => Promise<void>
}) {
  const { t } = useI18n()
  const [listing, setListing] = useState<DirectoryListing>()
  const [draftPath, setDraftPath] = useState(initialPath ?? '')
  const [loading, setLoading] = useState(true)
  const [selecting, setSelecting] = useState(false)
  const [error, setError] = useState('')

  const loadDirectory = useCallback(async (path?: string) => {
    setLoading(true)
    setError('')
    try {
      const suffix = path?.trim() ? `?path=${encodeURIComponent(path.trim())}` : ''
      const response = await fetch(apiURL(`/directories${suffix}`), { cache: 'no-store' })
      if (!response.ok) {
        let message = t('workspace.loadFailed')
        try {
          const body = (await response.json()) as { error?: string }
          if (body.error) message = body.error
        } catch {
          // Use the localized fallback for non-JSON responses.
        }
        throw new Error(message)
      }
      const next = (await response.json()) as DirectoryListing
      setListing(next)
      setDraftPath(next.path)
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : t('workspace.loadFailed'))
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => {
    void loadDirectory(initialPath)
  }, [initialPath, loadDirectory])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !selecting) onClose()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [onClose, selecting])

  const chooseCurrent = async () => {
    if (!listing || selecting) return
    setSelecting(true)
    setError('')
    try {
      await onSelect(listing.path)
    } catch (selectError) {
      setError(selectError instanceof Error ? selectError.message : t('workspace.openFailed'))
      setSelecting(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-[140] grid place-items-center bg-stone-950/20 px-4"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget && !selecting) onClose()
      }}
    >
      <section
        className="w-full max-w-[37.5rem] overflow-hidden rounded-[16px] border border-stone-300/80 bg-white shadow-[0_24px_64px_-28px_rgba(28,25,23,0.42)] animate-[fade-in_100ms_ease-out]"
        role="dialog"
        aria-modal="true"
        aria-labelledby="workspace-picker-title"
      >
        <header className="flex items-start gap-3 border-b border-stone-200/70 px-4 py-3.5">
          <div className="min-w-0 flex-1">
            <h2
              id="workspace-picker-title"
              className="text-[0.9375rem] leading-5 font-semibold tracking-[-0.01em] text-stone-950"
            >
              {t('workspace.chooseFolder')}
            </h2>
            <p className="mt-0.5 text-[0.75rem] leading-4.5 text-stone-500">
              {t('workspace.chooseDescription')}
            </p>
          </div>
          <button
            className="-mt-0.5 grid size-7 shrink-0 cursor-pointer place-items-center rounded-[8px] text-stone-400 transition-colors hover:bg-[rgb(246,246,246)] hover:text-stone-800 disabled:cursor-not-allowed disabled:opacity-40"
            type="button"
            aria-label={t('workspace.closePicker')}
            disabled={selecting}
            onClick={onClose}
          >
            <X className="size-3.5" aria-hidden="true" />
          </button>
        </header>

        <div className="px-4 pt-3">
          <form
            className="flex h-9 items-center overflow-hidden rounded-[10px] border border-stone-200 bg-white transition-[border-color,box-shadow] focus-within:border-stone-400 focus-within:ring-2 focus-within:ring-stone-200/70"
            onSubmit={(event) => {
              event.preventDefault()
              void loadDirectory(draftPath)
            }}
          >
            <button
              className="grid h-full w-9 shrink-0 cursor-pointer place-items-center border-r border-stone-200 text-stone-500 transition-colors hover:bg-[rgb(246,246,246)] hover:text-stone-900 disabled:cursor-not-allowed disabled:opacity-35"
              type="button"
              title={t('workspace.parentFolder')}
              aria-label={t('workspace.parentFolder')}
              disabled={!listing?.parent || loading}
              onClick={() => void loadDirectory(listing?.parent)}
            >
              <ArrowUp className="size-3.5" aria-hidden="true" />
            </button>
            <label className="min-w-0 flex-1">
              <span className="sr-only">{t('workspace.path')}</span>
              <input
                className="h-8 w-full border-0 bg-transparent px-2.5 font-mono text-[0.75rem] text-stone-800 outline-none"
                value={draftPath}
                aria-label={t('workspace.path')}
                spellCheck={false}
                onChange={(event) => setDraftPath(event.target.value)}
              />
            </label>
            <button
              className="mr-1 h-7 shrink-0 cursor-pointer rounded-[8px] px-2.5 text-[0.75rem] font-medium text-stone-600 transition-colors hover:bg-[rgb(241,241,241)] hover:text-stone-950 disabled:cursor-wait disabled:opacity-40"
              type="submit"
              disabled={loading || !draftPath.trim()}
            >
              {t('workspace.go')}
            </button>
          </form>
          {error && (
            <p className="mt-2 text-[0.75rem] leading-4.5 text-red-700" role="alert">
              {error}
            </p>
          )}
        </div>

        <div className="mx-4 mt-3 min-h-[16.25rem] overflow-hidden rounded-[11px] border border-stone-200 bg-white">
          <div className="flex h-8 items-center border-b border-stone-200/80 bg-stone-50/60 px-2.5 font-mono text-[0.71875rem] text-stone-500">
            {listing?.path ?? (draftPath || t('workspace.folders'))}
          </div>
          <div className="code-scroll-area max-h-[18.75rem] overflow-y-auto p-1">
            {loading ? (
              <div className="flex h-[13.75rem] items-center justify-center gap-2 text-[0.75rem] text-stone-400">
                <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
                {t('workspace.loadingFolders')}
              </div>
            ) : listing && listing.directories.length > 0 ? (
              <div className="flex flex-col">
                {listing.directories.map((directory) => (
                  <button
                    key={directory.path}
                    className="group flex h-8 w-full cursor-pointer items-center gap-2 rounded-[8px] px-2 text-left text-[0.8125rem] text-stone-700 outline-none transition-colors hover:bg-[rgb(246,246,246)] hover:text-stone-950 focus-visible:bg-[rgb(237,237,237)] focus-visible:text-stone-950"
                    type="button"
                    title={directory.path}
                    onClick={() => void loadDirectory(directory.path)}
                  >
                    <Folder
                      className="size-[0.9375rem] shrink-0 text-stone-400 transition-colors group-hover:text-stone-600"
                      strokeWidth={1.8}
                      aria-hidden="true"
                    />
                    <span className="min-w-0 flex-1 truncate">{directory.name}</span>
                  </button>
                ))}
              </div>
            ) : (
              <div className="flex h-[13.75rem] items-center justify-center text-[0.75rem] text-stone-400">
                {t('workspace.noFolders')}
              </div>
            )}
          </div>
        </div>

        <footer className="mt-3 flex items-center justify-end gap-1.5 border-t border-stone-200/70 px-4 py-3">
          <button
            className="h-8 cursor-pointer rounded-[8px] px-3 text-[0.78125rem] font-normal text-stone-600 transition-colors hover:bg-[rgb(246,246,246)] hover:text-stone-900 disabled:cursor-not-allowed disabled:opacity-40"
            type="button"
            disabled={selecting}
            onClick={onClose}
          >
            {t('workspace.cancel')}
          </button>
          <button
            className="inline-flex h-8 min-w-[6.5rem] cursor-pointer items-center justify-center gap-1.5 rounded-[8px] bg-stone-900 px-3.5 text-[0.78125rem] font-medium text-white transition-colors hover:bg-stone-700 disabled:cursor-wait disabled:opacity-45"
            type="button"
            disabled={!listing || loading || selecting}
            onClick={() => void chooseCurrent()}
          >
            {selecting && <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />}
            {selecting ? t('workspace.opening') : t('workspace.useFolder')}
          </button>
        </footer>
      </section>
    </div>
  )
}
