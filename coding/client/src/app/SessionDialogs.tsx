import { useEffect } from 'react'
import {
  CircleAlert,
  LoaderCircle,
  ShieldAlert,
  X,
} from 'lucide-react'
import type { SessionSummary } from '@/types'
import { useI18n } from '@/i18n'

export function DeleteSessionDialog({
  session,
  deleting,
  error,
  onCancel,
  onConfirm,
}: {
  session: SessionSummary
  deleting: boolean
  error: string
  onCancel: () => void
  onConfirm: () => void
}) {
  const blocked = session.running || session.hasApproval
  const { t } = useI18n()
  const title = session.title === 'New session' ? t('app.newSession') : session.title

  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !deleting) onCancel()
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [deleting, onCancel])

  return (
    <div
      className="fixed inset-0 z-[100] grid place-items-center bg-scrim/30 px-4 py-8 backdrop-blur-[3px]"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget && !deleting) onCancel()
      }}
    >
      <section
        className="relative w-full max-w-[29.25rem] animate-[fade-in_140ms_ease-out] rounded-[22px] border border-canvas/80 bg-canvas p-6 shadow-[0_30px_90px_-32px_rgba(28,25,23,0.62)] max-sm:rounded-[18px] max-sm:p-5"
        role="dialog"
        aria-modal="true"
        aria-labelledby="delete-session-title"
        aria-describedby="delete-session-description"
      >
        <button
          className="absolute top-4 right-4 grid size-8 cursor-pointer place-items-center rounded-full text-ink-faint transition-colors hover:bg-canvas-sunken hover:text-ink-soft disabled:cursor-wait disabled:opacity-40"
          type="button"
          aria-label={t('delete.close')}
          disabled={deleting}
          onClick={onCancel}
        >
          <X className="size-4" aria-hidden="true" />
        </button>

        <div className="pr-9">
          <h2
            id="delete-session-title"
            className="text-[1.1875rem] leading-6 font-semibold tracking-[-0.02em] text-ink"
          >
            {t('delete.title')}
          </h2>
          <p
            id="delete-session-description"
            className="mt-1.5 text-[0.875rem] leading-[1.55] text-ink-muted"
          >
            {t('delete.description')}
          </p>
        </div>

        <div className="mt-5 border-y border-edge/80 py-3.5">
          <div className="text-[0.6875rem] leading-4 font-medium tracking-[0.08em] text-ink-faint uppercase">
            {t('delete.session')}
          </div>
          <div className="mt-1 truncate text-[0.90625rem] leading-5 font-medium text-ink-soft">
            {title}
          </div>
        </div>

        {blocked && (
          <div className="mt-4 flex gap-2.5 rounded-xl border border-warning-edge/70 bg-warning-surface/70 px-3.5 py-3 text-[0.8125rem] leading-5 text-warning">
            <ShieldAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
            <span>{t('delete.blocked')}</span>
          </div>
        )}
        {error && (
          <div className="mt-4 flex gap-2.5 rounded-xl border border-danger-edge/70 bg-danger-surface/70 px-3.5 py-3 text-[0.8125rem] leading-5 text-danger">
            <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
            <span>{error}</span>
          </div>
        )}

        <div className="mt-6 flex justify-end gap-2.5">
          <button
            className="h-10 cursor-pointer rounded-xl border border-edge-strong bg-canvas px-4 text-[0.875rem] font-medium text-ink-soft transition-[border-color,background-color,color] hover:border-edge-stronger hover:bg-canvas-raised hover:text-ink disabled:cursor-wait disabled:opacity-50"
            type="button"
            disabled={deleting}
            onClick={onCancel}
          >
            {t('delete.cancel')}
          </button>
          <button
            className="flex h-10 min-w-[7.875rem] cursor-pointer items-center justify-center gap-2 rounded-xl bg-danger-solid px-4 text-[0.875rem] font-medium text-ink-inverse shadow-[0_5px_14px_-8px_rgba(180,35,24,0.85)] transition-[background-color,transform] hover:bg-danger-solid-hover active:translate-y-px disabled:cursor-not-allowed disabled:opacity-35"
            type="button"
            disabled={deleting || blocked}
            onClick={onConfirm}
          >
            {deleting && <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />}
            {deleting ? t('delete.deleting') : t('delete.confirm')}
          </button>
        </div>
      </section>
    </div>
  )
}

export function RemoveWorkspaceDialog({
  workspace,
  removing,
  error,
  onCancel,
  onConfirm,
}: {
  workspace: { path: string; name: string }
  removing: boolean
  error: string
  onCancel: () => void
  onConfirm: () => void
}) {
  const { t } = useI18n()

  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !removing) onCancel()
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [onCancel, removing])

  return (
    <div
      className="fixed inset-0 z-[100] grid place-items-center bg-scrim/25 px-4 py-8 backdrop-blur-[2px]"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget && !removing) onCancel()
      }}
    >
      <section
        className="relative w-full max-w-[28rem] animate-[fade-in_140ms_ease-out] rounded-[20px] border border-canvas/80 bg-canvas p-6 shadow-[0_28px_80px_-34px_rgba(28,25,23,0.58)] max-sm:rounded-[18px] max-sm:p-5"
        role="dialog"
        aria-modal="true"
        aria-labelledby="remove-workspace-title"
        aria-describedby="remove-workspace-description"
      >
        <button
          className="absolute top-4 right-4 grid size-8 cursor-pointer place-items-center rounded-full text-ink-faint transition-colors hover:bg-canvas-sunken hover:text-ink-soft disabled:cursor-wait disabled:opacity-40"
          type="button"
          aria-label={t('workspace.closeRemove')}
          disabled={removing}
          onClick={onCancel}
        >
          <X className="size-4" aria-hidden="true" />
        </button>

        <div className="pr-9">
          <h2
            id="remove-workspace-title"
            className="text-[1.125rem] leading-6 font-semibold tracking-[-0.02em] text-ink"
          >
            {t('workspace.removeTitle')}
          </h2>
          <p
            id="remove-workspace-description"
            className="mt-1.5 text-[0.875rem] leading-[1.55] text-ink-muted"
          >
            {t('workspace.removeDescription')}
          </p>
        </div>

        <div className="mt-5 rounded-xl border border-edge/80 px-3.5 py-3">
          <div className="truncate text-[0.90625rem] leading-5 font-medium text-ink-soft">
            {workspace.name}
          </div>
          <div
            className="mt-0.5 truncate font-mono text-[0.71875rem] leading-4 text-ink-faint"
            title={workspace.path}
          >
            {workspace.path}
          </div>
        </div>

        {error && (
          <div className="mt-4 flex gap-2.5 rounded-xl border border-danger-edge/70 bg-danger-surface/70 px-3.5 py-3 text-[0.8125rem] leading-5 text-danger">
            <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
            <span>{error}</span>
          </div>
        )}

        <div className="mt-6 flex justify-end gap-2.5">
          <button
            className="h-9 cursor-pointer rounded-[10px] border border-edge-strong bg-canvas px-4 text-[0.84375rem] font-medium text-ink-soft transition-colors hover:bg-canvas-raised hover:text-ink disabled:cursor-wait disabled:opacity-50"
            type="button"
            disabled={removing}
            onClick={onCancel}
          >
            {t('workspace.cancel')}
          </button>
          <button
            className="flex h-9 min-w-[7.5rem] cursor-pointer items-center justify-center gap-2 rounded-[10px] bg-danger-solid px-4 text-[0.84375rem] font-medium text-ink-inverse transition-colors hover:bg-danger-solid-hover disabled:cursor-wait disabled:opacity-40"
            type="button"
            disabled={removing}
            onClick={onConfirm}
          >
            {removing && <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />}
            {removing ? t('workspace.removing') : t('workspace.removeConfirm')}
          </button>
        </div>
      </section>
    </div>
  )
}
