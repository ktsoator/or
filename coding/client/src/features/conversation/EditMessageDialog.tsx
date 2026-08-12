import { LoaderCircle } from 'lucide-react'
import { Dialog } from 'radix-ui'
import { useI18n } from '@/i18n'

export function EditMessageDialog({
  open,
  submitting,
  error,
  onOpenChange,
  onConfirm,
}: {
  open: boolean
  submitting: boolean
  error: string
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}) {
  const { t } = useI18n()

  return (
    <Dialog.Root
      open={open}
      onOpenChange={(nextOpen) => {
        if (!submitting) onOpenChange(nextOpen)
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-[160] bg-scrim/30 backdrop-blur-[1px]" />
        <Dialog.Content className="fixed top-1/2 left-1/2 z-[170] w-[min(28rem,calc(100vw-2rem))] -translate-x-1/2 -translate-y-1/2 rounded-[16px] border border-edge bg-canvas p-5 shadow-[0_24px_64px_-32px_rgba(28,25,23,0.5)] outline-none">
          <Dialog.Title className="text-[1.0625rem] leading-6 font-medium text-ink">
            {t('actions.editConfirmTitle')}
          </Dialog.Title>
          <Dialog.Description className="mt-2 text-[0.875rem] leading-5 text-ink-muted">
            {t('actions.editConfirmDescription')}
          </Dialog.Description>
          <p className="mt-3 border-l-2 border-edge-strong pl-3 text-[0.8125rem] leading-5 text-ink-soft">
            {t('actions.editWorkspaceUnchanged')}
          </p>

          {error && (
            <p className="mt-3 text-[0.8125rem] leading-5 text-danger" role="alert">
              {error}
            </p>
          )}

          <div className="mt-5 flex justify-end gap-2.5">
            <button
              type="button"
              className="h-9 min-w-[5.5rem] cursor-pointer rounded-full bg-surface-hover px-4 text-[0.8125rem] text-ink outline-none transition-colors hover:bg-surface-active focus-visible:bg-surface-active disabled:cursor-wait disabled:opacity-50"
              disabled={submitting}
              onClick={() => onOpenChange(false)}
            >
              {t('actions.editConfirmCancel')}
            </button>
            <button
              type="button"
              className="inline-flex h-9 min-w-[8rem] cursor-pointer items-center justify-center gap-2 rounded-full bg-ink px-4 text-[0.8125rem] text-canvas outline-none transition-opacity hover:opacity-90 focus-visible:ring-2 focus-visible:ring-edge-stronger disabled:cursor-wait disabled:opacity-60"
              disabled={submitting}
              onClick={onConfirm}
            >
              {submitting && <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />}
              {t(submitting ? 'actions.editSubmitting' : 'actions.editConfirmAction')}
            </button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
