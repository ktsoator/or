import { ChevronDown, CornerUpLeft, GitFork } from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import type { SessionSummary } from '@/types'
import { useI18n } from '@/i18n'

export function ConversationBranchNavigation({
  parentSession,
  branches,
  onReturnToParent,
  onSelectSession,
}: {
  parentSession?: SessionSummary
  branches: SessionSummary[]
  onReturnToParent?: () => void
  onSelectSession: (id: string) => void
}) {
  const { t } = useI18n()
  const sessionTitle = (session: SessionSummary) =>
    session.title === 'New session' ? t('app.newSession') : session.title

  return (
    <div
      className="flex shrink-0 items-center gap-0.5 border-l border-edge/80 pl-2"
      data-testid="conversation-branch-navigation"
    >
      {parentSession && (
        <button
          className="window-titlebar-control flex h-7 min-w-0 max-w-[13rem] cursor-pointer items-center gap-1.5 rounded-md px-1.5 text-[0.75rem] outline-none transition-colors hover:bg-surface-active focus-visible:bg-surface-active max-sm:grid max-sm:size-7 max-sm:place-items-center max-sm:px-0"
          type="button"
          title={t('branchNav.returnToParent', { title: sessionTitle(parentSession) })}
          aria-label={t('branchNav.returnToParent', { title: sessionTitle(parentSession) })}
          onClick={onReturnToParent ?? (() => onSelectSession(parentSession.id))}
        >
          <CornerUpLeft className="size-3.5 shrink-0 text-ink-muted" aria-hidden="true" />
          <span className="shrink-0 text-ink-faint max-sm:hidden">{t('branchNav.from')}</span>
          <span className="truncate text-ink-soft max-sm:hidden">{sessionTitle(parentSession)}</span>
        </button>
      )}

      {branches.length > 0 && (
        <DropdownMenu.Root>
          <DropdownMenu.Trigger asChild>
            <button
              className="window-titlebar-control flex h-7 cursor-pointer items-center gap-1.5 rounded-md px-1.5 text-[0.75rem] text-ink-soft outline-none transition-colors hover:bg-surface-active hover:text-ink focus-visible:bg-surface-active focus-visible:text-ink data-[state=open]:bg-surface-selected data-[state=open]:text-ink max-sm:grid max-sm:size-7 max-sm:place-items-center max-sm:px-0"
              type="button"
              title={t('branchNav.openBranches', { count: branches.length })}
              aria-label={t('branchNav.openBranches', { count: branches.length })}
            >
              <GitFork className="size-3.5 shrink-0 text-ink-muted" aria-hidden="true" />
              <span className="tabular-nums max-sm:hidden">
                {t('branchNav.branchCount', { count: branches.length })}
              </span>
              <ChevronDown className="size-3 text-ink-faint max-sm:hidden" aria-hidden="true" />
            </button>
          </DropdownMenu.Trigger>
          <DropdownMenu.Portal>
            <DropdownMenu.Content
              side="bottom"
              align="start"
              sideOffset={7}
              collisionPadding={10}
              className="z-[120] flex max-h-[min(22rem,var(--radix-dropdown-menu-content-available-height))] w-[min(18rem,calc(100vw-1.25rem))] animate-[fade-in_100ms_ease-out] flex-col overflow-hidden rounded-[12px] border border-edge bg-canvas p-1 text-[0.84375rem] text-ink shadow-[0_16px_40px_-24px_rgba(28,25,23,0.46)] outline-none"
            >
              <DropdownMenu.Label className="flex h-8 items-center px-2.5 text-[0.75rem] font-medium text-ink-faint">
                <span>{t('branchNav.branches')}</span>
                <span className="ml-auto tabular-nums">{branches.length}</span>
              </DropdownMenu.Label>
              <DropdownMenu.Separator className="mx-1 mb-1 h-px bg-canvas-sunken" />
              <div className="code-scroll-area min-h-0 overflow-y-auto">
                {branches.map((branch) => (
                  <DropdownMenu.Item
                    key={branch.id}
                    className="flex h-9 cursor-default select-none items-center rounded-[9px] px-2.5 outline-none data-[highlighted]:bg-surface-active"
                    onSelect={() => onSelectSession(branch.id)}
                  >
                    <span className="min-w-0 truncate">{sessionTitle(branch)}</span>
                  </DropdownMenu.Item>
                ))}
              </div>
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>
      )}
    </div>
  )
}
