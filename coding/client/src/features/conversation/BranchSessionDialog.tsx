import {
  CircleAlert,
  FolderOpen,
  GitFork,
  LoaderCircle,
  MessagesSquare,
} from 'lucide-react'
import { Dialog } from 'radix-ui'
import { useI18n } from '@/i18n'

export function BranchSessionDialog({
  open,
  creating,
  error,
  onOpenChange,
  onConfirm,
}: {
  open: boolean
  creating: boolean
  error: string
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}) {
  const { t } = useI18n()

  return (
    <Dialog.Root
      open={open}
      onOpenChange={(nextOpen) => {
        if (!creating) onOpenChange(nextOpen)
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-[160] animate-[fade-in_120ms_ease-out] bg-scrim/30 backdrop-blur-[1px]" />
        <Dialog.Content className="fixed top-1/2 left-1/2 z-[170] max-h-[calc(100vh-2rem)] w-[min(34rem,calc(100vw-2rem))] -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-[16px] border border-edge bg-canvas p-6 shadow-[0_28px_80px_-32px_rgba(28,25,23,0.55)] outline-none max-sm:p-5">
          <Dialog.Title className="flex items-center gap-2.5 text-[1.125rem] leading-6 font-medium text-ink">
            <GitFork className="size-5 shrink-0" strokeWidth={1.8} aria-hidden="true" />
            {t('actions.branchConfirmTitle')}
          </Dialog.Title>
          <Dialog.Description className="mt-4 text-[0.875rem] leading-6 text-ink-muted">
            {t('actions.branchConfirmDescription')}
          </Dialog.Description>

          <div
            data-testid="branch-session-details"
            className="mt-4 overflow-hidden rounded-[12px] bg-surface-hover px-4"
          >
            <div className="grid min-h-[58px] grid-cols-[1.25rem_minmax(0,1fr)] items-center gap-x-3 border-b border-edge-soft py-2.5">
              <MessagesSquare className="size-5 text-info" strokeWidth={1.8} aria-hidden="true" />
              <div className="min-w-0">
                <p className="text-[0.8125rem] leading-[1.125rem] font-medium text-ink">
                  {t('actions.branchHistoryTitle')}
                </p>
                <p className="text-[0.75rem] leading-4 text-ink-muted">
                  {t('actions.branchHistoryDescription')}
                </p>
              </div>
            </div>
            <div className="grid min-h-[58px] grid-cols-[1.25rem_minmax(0,1fr)] items-center gap-x-3 py-2.5">
              <FolderOpen className="size-5 text-ink-soft" strokeWidth={1.8} aria-hidden="true" />
              <div className="min-w-0">
                <p className="text-[0.8125rem] leading-[1.125rem] font-medium text-ink">
                  {t('actions.branchWorkspaceTitle')}
                </p>
                <p className="text-[0.75rem] leading-4 text-ink-muted">
                  {t('actions.branchWorkspaceDescription')}
                </p>
              </div>
            </div>
          </div>

          {error && (
            <div
              className="mt-4 flex gap-2.5 rounded-xl border border-danger-edge/70 bg-danger-surface/70 px-3.5 py-3 text-[0.8125rem] leading-5 text-danger"
              role="alert"
            >
              <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
              <span>{error}</span>
            </div>
          )}

          <div className="mt-5 flex justify-end gap-2.5">
            <button
              type="button"
              className="h-10 min-w-[7.25rem] cursor-pointer rounded-full bg-surface-hover px-4 text-[0.875rem] font-normal text-ink outline-none transition-colors hover:bg-surface-active focus-visible:bg-surface-active disabled:cursor-wait disabled:opacity-50"
              disabled={creating}
              onClick={() => onOpenChange(false)}
            >
              {t('actions.branchConfirmCancel')}
            </button>
            <button
              type="button"
              className="inline-flex h-10 min-w-[9.5rem] cursor-pointer items-center justify-center gap-2 rounded-full bg-ink px-4 text-[0.875rem] font-normal text-canvas outline-none transition-[opacity,transform] hover:opacity-90 active:translate-y-px focus-visible:ring-2 focus-visible:ring-edge-stronger disabled:cursor-wait disabled:opacity-60"
              disabled={creating}
              onClick={onConfirm}
            >
              {creating ? (
                <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
              ) : (
                <GitFork className="size-4" strokeWidth={1.8} aria-hidden="true" />
              )}
              {t(
                creating
                  ? 'actions.branchCreating'
                  : 'actions.branchConfirmAction',
              )}
            </button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
