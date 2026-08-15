import { useEffect, useState } from 'react'
import {
  Check,
  FolderOpen,
  Globe2,
  LoaderCircle,
  SquareTerminal,
  TriangleAlert,
} from 'lucide-react'
import { Dialog, DropdownMenu } from 'radix-ui'
import type { PermissionMode } from '@/types'
import { useI18n } from '@/i18n'
import { cn } from '@/lib/utils'
import {
  composerControlTextClass,
  composerMenuTriggerClass,
} from '@/shared/ui/composerControlStyles'

type PermissionLabelKey =
  | 'permission.ask'
  | 'permission.autoEdit'
  | 'permission.fullAccess'

type ModeOption = {
  value: PermissionMode
  labelKey: PermissionLabelKey
  shortLabelKey:
    | 'permission.askShort'
    | 'permission.autoEditShort'
    | 'permission.fullAccessShort'
  descriptionKey:
    | 'permission.askDescription'
    | 'permission.autoEditDescription'
    | 'permission.fullAccessDescription'
}

const options: ModeOption[] = [
  {
    value: 'ask',
    labelKey: 'permission.ask',
    shortLabelKey: 'permission.askShort',
    descriptionKey: 'permission.askDescription',
  },
  {
    value: 'auto_edit',
    labelKey: 'permission.autoEdit',
    shortLabelKey: 'permission.autoEditShort',
    descriptionKey: 'permission.autoEditDescription',
  },
  {
    value: 'full_access',
    labelKey: 'permission.fullAccess',
    shortLabelKey: 'permission.fullAccessShort',
    descriptionKey: 'permission.fullAccessDescription',
  },
]

export function PermissionModeMenu({
  value,
  disabled,
  confirmationBlocked,
  onChange,
}: {
  value: PermissionMode
  disabled: boolean
  confirmationBlocked: boolean
  onChange: (mode: PermissionMode) => Promise<void>
}) {
  const { t } = useI18n()
  const [open, setOpen] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const selected = options.find((option) => option.value === value) ?? options[0]
  const menuOpen = open && !disabled
  const fullAccessSelected = selected.value === 'full_access'

  useEffect(() => {
    if (disabled) setOpen(false)
  }, [disabled])

  useEffect(() => {
    if (confirmationBlocked) setConfirmOpen(false)
  }, [confirmationBlocked])

  const chooseMode = (next: string) => {
    const mode = next as PermissionMode
    if (mode === value) return
    if (mode === 'full_access') {
      setOpen(false)
      setConfirmOpen(true)
      return
    }
    void onChange(mode).catch(() => undefined)
  }

  const enableFullAccess = async () => {
    setConfirming(true)
    try {
      await onChange('full_access')
      setConfirmOpen(false)
    } catch {
      // The composer owns the visible error message; keep this dialog open so
      // the user can retry or cancel after a failed update.
    } finally {
      setConfirming(false)
    }
  }

  return (
    <>
      <DropdownMenu.Root
        open={menuOpen}
        onOpenChange={(nextOpen) => setOpen(!disabled && nextOpen)}
      >
        <DropdownMenu.Trigger asChild>
          <button
            data-testid="permission-mode-trigger"
            type="button"
            className={cn(
              composerMenuTriggerClass,
              composerControlTextClass,
              'h-[30px] max-w-[10rem] rounded-[10px] px-2',
              fullAccessSelected && 'text-warning hover:text-warning focus-visible:text-warning',
            )}
            aria-label={t('permission.choose')}
            title={t(selected.labelKey)}
            disabled={disabled}
          >
            <span
              data-testid="permission-mode-label"
              className="min-w-0 truncate"
            >
              {t(selected.shortLabelKey)}
            </span>
          </button>
        </DropdownMenu.Trigger>

        <DropdownMenu.Portal>
          <DropdownMenu.Content
            side="top"
            align="start"
            sideOffset={2}
            collisionPadding={10}
            className="z-[110] w-[30rem] max-w-[calc(100vw-1.25rem)] animate-[fade-in_110ms_ease-out] rounded-2xl border border-edge bg-canvas p-1.5 text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
          >
            <DropdownMenu.Label className="px-3 pt-1.5 pb-2 text-[0.75rem] font-normal text-ink-faint">
              {t('permission.choose')}
            </DropdownMenu.Label>
            <DropdownMenu.RadioGroup
              className="flex flex-col gap-0.5"
              value={value}
              onValueChange={chooseMode}
            >
              {options.map((option) => {
                const fullAccess = option.value === 'full_access'
                return (
                  <DropdownMenu.RadioItem
                    key={option.value}
                    value={option.value}
                    className={cn(
                      'relative flex min-h-12 cursor-default select-none items-center rounded-[12px] px-3 py-1.5 pr-9 outline-none data-[highlighted]:bg-surface-active',
                      fullAccess && 'text-warning',
                    )}
                  >
                    <span className="min-w-0 flex-1">
                      <span
                        className={cn(
                          'block text-[0.875rem] leading-5 font-normal text-ink',
                          fullAccess && 'text-warning',
                        )}
                      >
                        {t(option.labelKey)}
                      </span>
                      <span
                        className={cn(
                          'block text-[0.75rem] leading-4 text-ink-muted',
                          fullAccess && 'text-warning',
                        )}
                      >
                        {t(option.descriptionKey)}
                      </span>
                    </span>
                    <DropdownMenu.ItemIndicator
                      className={cn(
                        'absolute right-2.5 grid size-5 place-items-center text-ink-soft',
                        fullAccess && 'text-warning',
                      )}
                    >
                      <Check className="size-4" aria-hidden="true" />
                    </DropdownMenu.ItemIndicator>
                  </DropdownMenu.RadioItem>
                )
              })}
            </DropdownMenu.RadioGroup>
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>

      <Dialog.Root
        open={confirmOpen}
        onOpenChange={(nextOpen) => {
          if (!confirming) setConfirmOpen(nextOpen)
        }}
      >
        <Dialog.Portal>
          <Dialog.Overlay className="fixed inset-0 z-[160] animate-[fade-in_120ms_ease-out] bg-scrim/30 backdrop-blur-[1px]" />
          <Dialog.Content className="fixed top-1/2 left-1/2 z-[170] max-h-[calc(100vh-2rem)] w-[min(34rem,calc(100vw-2rem))] -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-[16px] border border-edge bg-canvas p-6 shadow-[0_28px_80px_-32px_rgba(28,25,23,0.55)] outline-none max-sm:p-5">
            <Dialog.Title className="flex items-center gap-2.5 text-[1.125rem] leading-6 font-medium text-ink">
              <TriangleAlert className="size-5 shrink-0" strokeWidth={1.8} aria-hidden="true" />
              {t('permission.fullAccessConfirmTitle')}
            </Dialog.Title>
            <Dialog.Description className="mt-4 text-[0.875rem] leading-6 text-ink-muted">
              {t('permission.fullAccessConfirmDescription')}
            </Dialog.Description>

            <div
              data-testid="full-access-capabilities"
              className="mt-4 overflow-hidden rounded-[12px] bg-surface-hover px-4"
            >
              <div className="grid min-h-[58px] grid-cols-[1.25rem_minmax(0,1fr)] items-center gap-x-3 border-b border-edge-soft py-2.5">
                <FolderOpen className="size-5 text-info" strokeWidth={1.8} aria-hidden="true" />
                <div className="min-w-0">
                  <p className="text-[0.8125rem] leading-[1.125rem] font-medium text-ink">
                    {t('permission.fullAccessFilesTitle')}
                  </p>
                  <p className="text-[0.75rem] leading-4 text-ink-muted">
                    {t('permission.fullAccessFilesDescription')}
                  </p>
                </div>
              </div>
              <div className="grid min-h-[58px] grid-cols-[1.25rem_minmax(0,1fr)] items-center gap-x-3 border-b border-edge-soft py-2.5">
                <SquareTerminal
                  className="size-5 text-ink-soft"
                  strokeWidth={1.8}
                  aria-hidden="true"
                />
                <div className="min-w-0">
                  <p className="text-[0.8125rem] leading-[1.125rem] font-medium text-ink">
                    {t('permission.fullAccessCommandsTitle')}
                  </p>
                  <p className="text-[0.75rem] leading-4 text-ink-muted">
                    {t('permission.fullAccessCommandsDescription')}
                  </p>
                </div>
              </div>
              <div className="grid min-h-[58px] grid-cols-[1.25rem_minmax(0,1fr)] items-center gap-x-3 py-2.5">
                <Globe2 className="size-5 text-info" strokeWidth={1.8} aria-hidden="true" />
                <div className="min-w-0">
                  <p className="text-[0.8125rem] leading-[1.125rem] font-medium text-ink">
                    {t('permission.fullAccessInternetTitle')}
                  </p>
                  <p className="text-[0.75rem] leading-4 text-ink-muted">
                    {t('permission.fullAccessInternetDescription')}
                  </p>
                </div>
              </div>
            </div>

            <p className="mt-4 text-[0.8125rem] leading-5 text-ink-muted">
              {t('permission.fullAccessRisk')}
            </p>

            <div className="mt-5 flex justify-end gap-2.5">
              <button
                type="button"
                className="h-10 min-w-[7.25rem] cursor-pointer rounded-full bg-surface-hover px-4 text-[0.875rem] font-normal text-ink outline-none transition-colors hover:bg-surface-active focus-visible:bg-surface-active disabled:cursor-wait disabled:opacity-50"
                disabled={confirming}
                onClick={() => setConfirmOpen(false)}
              >
                {t('permission.fullAccessConfirmCancel')}
              </button>
              <button
                type="button"
                className="inline-flex h-10 min-w-[9.5rem] cursor-pointer items-center justify-center gap-2 rounded-full bg-danger-surface px-4 text-[0.875rem] font-normal text-danger outline-none transition-colors hover:bg-danger-edge/55 focus-visible:bg-danger-edge/55 disabled:cursor-wait disabled:opacity-60"
                disabled={confirming}
                onClick={() => void enableFullAccess()}
              >
                {confirming ? (
                  <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
                ) : (
                  <TriangleAlert className="size-4" strokeWidth={1.8} aria-hidden="true" />
                )}
                {t(
                  confirming
                    ? 'permission.fullAccessEnabling'
                    : 'permission.fullAccessConfirmAction',
                )}
              </button>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </>
  )
}
