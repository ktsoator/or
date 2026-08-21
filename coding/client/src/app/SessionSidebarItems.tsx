import type { LucideIcon } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import {
  Archive,
  Bot,
  CircleAlert,
  Clock3,
  Ellipsis,
  Folder,
  FolderOpen,
  GitFork,
  LoaderCircle,
  MessageCircle,
  PanelRightOpen,
  Pin,
  PencilLine,
  Share2,
  SquarePen,
  Trash2,
  X,
} from 'lucide-react'
import { DropdownMenu, HoverCard } from 'radix-ui'
import type { SessionSummary } from '@/types'
import { useI18n } from '@/i18n'
import { formatMessageTime } from '@/lib/time'
import { cn } from '@/lib/utils'

export function WorkspaceSessions({
  path,
  name,
  sessions,
  activeSessionID,
  onSelectWorkspace,
  onSelectSession,
  onOpenSessionInWorkbench,
  onCreateSession,
  pinnedSessionIDs,
  onTogglePinnedSession,
  onDeleteSession,
  onRenameSession,
  onRevealWorkspace,
  onRemoveWorkspace,
  openHoverCardKey,
  onHoverCardOpenChange,
  onMenuOpenChange,
}: {
  path: string
  name: string
  sessions: SessionSummary[]
  activeSessionID?: string
  onSelectWorkspace: (path: string) => void
  onSelectSession: (id: string) => void
  onOpenSessionInWorkbench: (id: string) => void
  onCreateSession: (path: string) => void
  pinnedSessionIDs: Set<string>
  onTogglePinnedSession: (id: string) => void
  onDeleteSession: (session: SessionSummary) => void
  onRenameSession: (id: string, customTitle: string) => Promise<void>
  onRevealWorkspace: (path: string) => Promise<void>
  onRemoveWorkspace: (path: string, name: string) => void
  openHoverCardKey?: string
  onHoverCardOpenChange: (key: string, open: boolean) => void
  onMenuOpenChange: (key: string, open: boolean) => void
}) {
  const { t } = useI18n()
  const [expanded, setExpanded] = useState(true)
  const [menuOpen, setMenuOpen] = useState(false)
  const skipMenuFocusRestore = useRef(false)
  const hoverCardKey = `workspace:${path}`

  const handleMenuOpenChange = (open: boolean) => {
    setMenuOpen(open)
    onMenuOpenChange(hoverCardKey, open)
    if (open) onHoverCardOpenChange(hoverCardKey, false)
  }

  return (
    <section aria-label={name}>
      <div className="group/workspace relative flex h-8 items-center">
        <HoverCard.Root
          open={!menuOpen && openHoverCardKey === hoverCardKey}
          onOpenChange={(open) => {
            if (open && menuOpen) return
            onHoverCardOpenChange(hoverCardKey, open)
          }}
          openDelay={200}
          closeDelay={100}
        >
          <HoverCard.Trigger asChild>
            <button
              className="flex h-8 min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-[10px] py-0 pr-[4.125rem] pl-2.5 text-left text-[0.875rem] font-normal text-ink-soft transition-colors hover:bg-surface-hover hover:text-ink"
              type="button"
              aria-expanded={expanded}
              onClick={() => {
                onSelectWorkspace(path)
                setExpanded((current) => !current)
              }}
            >
              {expanded ? (
                <FolderOpen
                  className="size-4 shrink-0 text-ink-muted"
                  strokeWidth={1.8}
                  aria-hidden="true"
                />
              ) : (
                <Folder
                  className="size-4 shrink-0 text-ink-muted"
                  strokeWidth={1.8}
                  aria-hidden="true"
                />
              )}
              <span className="min-w-0 flex-1 truncate">{name}</span>
            </button>
          </HoverCard.Trigger>
          <HoverCard.Portal>
            <HoverCard.Content
              side="right"
              align="start"
              sideOffset={6}
              collisionPadding={10}
              className="z-[130] w-[18.25rem] animate-[fade-in_100ms_ease-out] rounded-[12px] border border-edge bg-canvas px-3 py-2.5 text-ink shadow-[0_16px_40px_-24px_rgba(28,25,23,0.46)] outline-none max-md:hidden"
              data-testid="workspace-hover-card"
            >
              <div className="grid min-w-0 grid-cols-[1rem_minmax(0,1fr)] items-start gap-x-2.5 gap-y-1">
                <FolderOpen
                  className="mt-0.5 size-4 shrink-0 text-ink-muted"
                  strokeWidth={1.8}
                  aria-hidden="true"
                />
                <h3 className="min-w-0 break-words text-[0.875rem] leading-5 font-medium text-ink">
                  {name}
                </h3>
                <MessageCircle
                  className="mt-0.5 size-4 shrink-0 text-ink-muted"
                  strokeWidth={1.8}
                  aria-hidden="true"
                />
                <span className="min-w-0 text-[0.8125rem] leading-5 text-ink-soft">
                  {t('workspace.sessionCount', { count: sessions.length })}
                </span>
                <div className="col-span-2 my-1.5 h-px bg-edge/75" aria-hidden="true" />
                <Folder
                  className="mt-0.5 size-4 shrink-0 text-ink-muted"
                  strokeWidth={1.8}
                  aria-hidden="true"
                />
                <span className="min-w-0 break-all text-[0.8125rem] leading-5 text-ink-soft">
                  {path}
                </span>
              </div>
            </HoverCard.Content>
          </HoverCard.Portal>
        </HoverCard.Root>
        <div
          className={cn(
            'absolute top-0.5 right-0.5 flex items-center opacity-0 transition-opacity duration-100 group-hover/workspace:opacity-100 group-focus-within/workspace:opacity-100 max-md:opacity-100',
            menuOpen && 'opacity-100',
          )}
        >
          <button
            className="grid size-7 cursor-pointer place-items-center rounded-[9px] text-ink-faint outline-none transition-colors hover:bg-canvas-sunken hover:text-ink focus-visible:bg-canvas-sunken focus-visible:text-ink"
            type="button"
            title={t('workspace.newSession', { name })}
            aria-label={t('workspace.newSession', { name })}
            onClick={() => onCreateSession(path)}
          >
            <SquarePen className="size-3.5" aria-hidden="true" />
          </button>
          <DropdownMenu.Root open={menuOpen} onOpenChange={handleMenuOpenChange}>
            <DropdownMenu.Trigger asChild>
              <button
                className="grid size-7 cursor-pointer place-items-center rounded-[9px] text-ink-faint outline-none transition-colors hover:bg-canvas-sunken hover:text-ink focus-visible:bg-canvas-sunken focus-visible:text-ink data-[state=open]:text-ink"
                type="button"
                title={t('workspace.projectActions')}
                aria-label={t('workspace.projectActionsNamed', { name })}
              >
                <Ellipsis className="size-4" aria-hidden="true" />
              </button>
            </DropdownMenu.Trigger>
            <DropdownMenu.Portal>
              <DropdownMenu.Content
                side="right"
                align="start"
                sideOffset={6}
                collisionPadding={10}
                className="z-[120] min-w-[13.75rem] animate-[fade-in_100ms_ease-out] rounded-[14px] border border-edge bg-canvas p-1 text-[0.84375rem] text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
                onCloseAutoFocus={(event) => {
                  if (!skipMenuFocusRestore.current) return
                  skipMenuFocusRestore.current = false
                  event.preventDefault()
                }}
              >
                <DropdownMenu.Item className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 outline-none data-[highlighted]:bg-surface-active">
                  <Pin className="size-4 text-ink-muted" aria-hidden="true" />
                  <span>{t('workspace.pinProject')}</span>
                </DropdownMenu.Item>
                <DropdownMenu.Item
                  className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 outline-none data-[highlighted]:bg-surface-active"
                  onSelect={() => {
                    skipMenuFocusRestore.current = true
                    void onRevealWorkspace(path)
                  }}
                >
                  <FolderOpen className="size-4 text-ink-muted" aria-hidden="true" />
                  <span>{t('workspace.revealInFinder')}</span>
                </DropdownMenu.Item>
                <DropdownMenu.Item className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 outline-none data-[highlighted]:bg-surface-active">
                  <GitFork className="size-4 text-ink-muted" aria-hidden="true" />
                  <span>{t('workspace.createWorktree')}</span>
                </DropdownMenu.Item>
                <DropdownMenu.Item className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 outline-none data-[highlighted]:bg-surface-active">
                  <PencilLine className="size-4 text-ink-muted" aria-hidden="true" />
                  <span>{t('workspace.renameProject')}</span>
                </DropdownMenu.Item>
                <DropdownMenu.Item
                  disabled
                  className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 text-ink-faint outline-none"
                >
                  <Archive className="size-4" aria-hidden="true" />
                  <span>{t('workspace.archiveChats')}</span>
                </DropdownMenu.Item>
                <DropdownMenu.Separator className="mx-1 my-1 h-px bg-canvas-sunken" />
                <DropdownMenu.Item
                  className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 text-danger outline-none data-[highlighted]:bg-danger-surface"
                  onSelect={() => onRemoveWorkspace(path, name)}
                >
                  <X className="size-4" aria-hidden="true" />
                  <span>{t('workspace.removeProject')}</span>
                </DropdownMenu.Item>
              </DropdownMenu.Content>
            </DropdownMenu.Portal>
          </DropdownMenu.Root>
        </div>
      </div>
      {expanded && (
        <div className="mt-1 space-y-1">
          {sessions.length === 0 ? (
            <div className="flex h-8 items-center pr-2.5 pl-[2.375rem] text-[0.84375rem] text-ink-faint">
              {t('workspace.noChats')}
            </div>
          ) : (
            sessions.map((session) => (
              <SessionRow
                key={session.id}
                session={session}
                active={session.id === activeSessionID}
                pinned={pinnedSessionIDs.has(session.id)}
                onSelect={() => onSelectSession(session.id)}
                onOpenInWorkbench={() => onOpenSessionInWorkbench(session.id)}
                onTogglePin={() => onTogglePinnedSession(session.id)}
                onDelete={() => onDeleteSession(session)}
                onRename={(title) => onRenameSession(session.id, title)}
                openHoverCardKey={openHoverCardKey}
                onHoverCardOpenChange={onHoverCardOpenChange}
                onMenuOpenChange={onMenuOpenChange}
                indented
              />
            ))
          )}
        </div>
      )}
    </section>
  )
}

export function SessionRow({
  session,
  active,
  pinned,
  onSelect,
  onOpenInWorkbench,
  onTogglePin,
  onDelete,
  onRename,
  openHoverCardKey,
  onHoverCardOpenChange,
  onMenuOpenChange,
  indented = false,
}: {
  session: SessionSummary
  active: boolean
  pinned: boolean
  onSelect: () => void
  onOpenInWorkbench: () => void
  onTogglePin: () => void
  onDelete: () => void
  onRename: (customTitle: string) => Promise<void>
  openHoverCardKey?: string
  onHoverCardOpenChange: (key: string, open: boolean) => void
  onMenuOpenChange: (key: string, open: boolean) => void
  indented?: boolean
}) {
  const { locale, t } = useI18n()
  const title = session.title === 'New session' ? t('app.newSession') : session.title
  const [menuOpen, setMenuOpen] = useState(false)
  const [draftTitle, setDraftTitle] = useState<string | undefined>(undefined)
  const editing = draftTitle !== undefined
  const committing = useRef(false)
  const openingEditor = useRef(false)
  const suppressHoverCard = useRef(false)
  const renameInput = useRef<HTMLInputElement>(null)
  const hoverCardKey = `session:${session.id}`

  const handleMenuOpenChange = (open: boolean) => {
    setMenuOpen(open)
    onMenuOpenChange(hoverCardKey, open)
    if (open) onHoverCardOpenChange(hoverCardKey, false)
  }

  useEffect(() => {
    if (editing) renameInput.current?.select()
  }, [editing])

  const commitRename = async () => {
    if (committing.current) return
    committing.current = true
    try {
      const next = (draftTitle ?? '').trim()
      if (next !== '' && next !== title) await onRename(next)
      setDraftTitle(undefined)
    } catch {
      // Keep the editor open with the typed text so the rename can be retried.
    } finally {
      committing.current = false
    }
  }

  if (editing) {
    return (
      <div className="group/session relative">
        <input
          className={cn(
            'h-8 w-full rounded-[10px] border border-edge-strong bg-canvas pr-2.5 text-[0.875rem] font-normal leading-5 text-ink shadow-none outline-none focus:border-edge-stronger',
            indented ? 'pl-[2.375rem]' : 'pl-2.5',
          )}
          ref={renameInput}
          type="text"
          maxLength={120}
          aria-label={t('app.renameNamedSession', { title })}
          value={draftTitle}
          onChange={(event) => setDraftTitle(event.target.value)}
          onBlur={() => void commitRename()}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault()
              void commitRename()
            } else if (event.key === 'Escape') {
              event.preventDefault()
              setDraftTitle(undefined)
            }
          }}
        />
      </div>
    )
  }

  return (
    <div className="group/session relative">
      <HoverCard.Root
        open={!menuOpen && openHoverCardKey === hoverCardKey}
        onOpenChange={(open) => {
          if (open && (menuOpen || suppressHoverCard.current)) return
          onHoverCardOpenChange(hoverCardKey, open)
        }}
        openDelay={200}
        closeDelay={100}
      >
        <HoverCard.Trigger asChild>
          <button
            className={cn(
              'flex h-8 w-full cursor-pointer items-center rounded-[10px] pr-[4.125rem] text-left transition-colors',
              indented ? 'pl-[2.375rem]' : 'pl-2.5',
              active
                ? 'bg-surface-selected text-ink'
                : 'text-ink-soft hover:bg-surface-hover hover:text-ink',
            )}
            type="button"
            aria-current={active ? 'page' : undefined}
            onClick={(event) => {
              suppressHoverCard.current = event.detail > 0
              onHoverCardOpenChange(hoverCardKey, false)
              onSelect()
            }}
            onPointerLeave={() => {
              suppressHoverCard.current = false
            }}
          >
            <span className="min-w-0 flex-1 truncate text-[0.875rem] font-normal leading-5">
              {title}
            </span>
          </button>
        </HoverCard.Trigger>
        <HoverCard.Portal>
          <HoverCard.Content
            side="right"
            align="start"
            sideOffset={6}
            collisionPadding={10}
            className="z-[130] w-[18.25rem] animate-[fade-in_100ms_ease-out] rounded-[12px] border border-edge bg-canvas px-3 py-2.5 text-ink shadow-[0_16px_40px_-24px_rgba(28,25,23,0.46)] outline-none max-md:hidden"
            data-testid="session-hover-card"
          >
            <h3 className="break-words text-[0.875rem] leading-5 font-medium text-ink">
              {title}
            </h3>
            <div className="mt-2.5 space-y-1.5 border-t border-edge/75 pt-2.5 text-[0.8125rem] leading-5">
              <div className="flex min-w-0 items-center gap-2.5">
                <Bot className="size-4 shrink-0 text-ink-muted" strokeWidth={1.8} aria-hidden="true" />
                <span className="shrink-0 text-ink-faint">{t('app.sessionModel')}</span>
                <span className="ml-auto min-w-0 truncate text-right text-ink-soft">
                  {session.modelName || session.modelId}
                </span>
              </div>
              <div className="flex min-w-0 items-center gap-2.5">
                <Clock3 className="size-4 shrink-0 text-ink-muted" strokeWidth={1.8} aria-hidden="true" />
                <span className="shrink-0 text-ink-faint">{t('app.sessionUpdated')}</span>
                <time
                  className="ml-auto min-w-0 truncate text-right text-ink-soft tabular-nums"
                  dateTime={session.updatedAt}
                >
                  {formatMessageTime(session.updatedAt, locale)}
                </time>
              </div>
              {session.forkedFromSessionId && (
                <div className="flex min-w-0 items-center gap-2.5 text-ink-soft">
                  <GitFork className="size-4 shrink-0 text-ink-muted" strokeWidth={1.8} aria-hidden="true" />
                  <span>{t('app.sessionBranch')}</span>
                </div>
              )}
            </div>
          </HoverCard.Content>
        </HoverCard.Portal>
      </HoverCard.Root>
      {(session.hasApproval || session.running) && (
        <span
          className={cn(
            'pointer-events-none absolute top-1/2 right-3 grid size-4 -translate-y-1/2 place-items-center transition-opacity duration-100 group-hover/session:opacity-0 group-focus-within/session:opacity-0 max-md:opacity-0',
            menuOpen && 'opacity-0',
          )}
          title={session.hasApproval ? t('app.approvalNeeded') : t('app.working')}
        >
          {session.hasApproval ? (
            <CircleAlert className="size-3.5 text-warning" aria-hidden="true" />
          ) : (
            <LoaderCircle className="size-3.5 animate-spin text-ink-muted" aria-hidden="true" />
          )}
          <span className="sr-only">
            {session.hasApproval ? t('app.approvalNeeded') : t('app.working')}
          </span>
        </span>
      )}
      <div
        className={cn(
          'absolute top-0.5 right-0.5 flex items-center opacity-0 transition-opacity duration-100 group-hover/session:opacity-100 group-focus-within/session:opacity-100 max-md:opacity-100',
          menuOpen && 'opacity-100',
        )}
      >
        <button
          className={cn(
            'grid size-7 cursor-pointer place-items-center rounded-[9px] text-ink-faint outline-none transition-colors hover:bg-canvas-sunken hover:text-ink focus-visible:bg-canvas-sunken focus-visible:text-ink',
            pinned && 'text-ink-muted',
          )}
          type="button"
          title={pinned ? t('app.unpinSession') : t('app.pinSession')}
          aria-label={
            pinned
              ? t('app.unpinNamedSession', { title })
              : t('app.pinNamedSession', { title })
          }
          aria-pressed={pinned}
          onClick={onTogglePin}
        >
          <Pin className={cn('size-3.5', pinned && 'fill-current')} aria-hidden="true" />
        </button>
        <DropdownMenu.Root open={menuOpen} onOpenChange={handleMenuOpenChange}>
          <DropdownMenu.Trigger asChild>
            <button
              className="grid size-7 cursor-pointer place-items-center rounded-[9px] text-ink-faint outline-none transition-colors hover:bg-canvas-sunken hover:text-ink focus-visible:bg-canvas-sunken focus-visible:text-ink data-[state=open]:text-ink"
              type="button"
              title={t('app.sessionActions')}
              aria-label={t('app.sessionActionsNamed', { title })}
            >
              <Ellipsis className="size-4" aria-hidden="true" />
            </button>
          </DropdownMenu.Trigger>
          <DropdownMenu.Portal>
            <DropdownMenu.Content
              side="right"
              align="start"
              sideOffset={6}
              collisionPadding={10}
              className="z-[120] min-w-[11.75rem] animate-[fade-in_100ms_ease-out] rounded-[14px] border border-edge bg-canvas p-1 text-[0.84375rem] text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
              onCloseAutoFocus={(event) => {
                if (!openingEditor.current) return
                openingEditor.current = false
                event.preventDefault()
              }}
            >
              <DropdownMenu.Item className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 outline-none data-[highlighted]:bg-surface-active">
                <Share2 className="size-4 text-ink-muted" aria-hidden="true" />
                <span>{t('app.shareSession')}</span>
              </DropdownMenu.Item>
              <DropdownMenu.Item
                className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 outline-none data-[highlighted]:bg-surface-active"
                onSelect={() => {
                  openingEditor.current = true
                  setDraftTitle(title)
                }}
              >
                <PencilLine className="size-4 text-ink-muted" aria-hidden="true" />
                <span>{t('app.renameSession')}</span>
              </DropdownMenu.Item>
              <DropdownMenu.Item
                className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 outline-none data-[highlighted]:bg-surface-active"
                onSelect={onOpenInWorkbench}
              >
                <PanelRightOpen className="size-4 text-ink-muted" aria-hidden="true" />
                <span>{t('app.openInWorkbench')}</span>
              </DropdownMenu.Item>
              <DropdownMenu.Item
                className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 outline-none data-[highlighted]:bg-surface-active"
                onSelect={onTogglePin}
              >
                <Pin className="size-4 text-ink-muted" aria-hidden="true" />
                <span>{pinned ? t('app.unpinSession') : t('app.pinSession')}</span>
              </DropdownMenu.Item>
              <DropdownMenu.Item className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 outline-none data-[highlighted]:bg-surface-active">
                <Archive className="size-4 text-ink-muted" aria-hidden="true" />
                <span>{t('app.archiveSession')}</span>
              </DropdownMenu.Item>
              <DropdownMenu.Separator className="mx-1 my-1 h-px bg-canvas-sunken" />
              <DropdownMenu.Item
                className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 text-danger outline-none data-[highlighted]:bg-danger-surface"
                onSelect={onDelete}
              >
                <Trash2 className="size-4" aria-hidden="true" />
                <span>{t('app.deleteSession')}</span>
              </DropdownMenu.Item>
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>
      </div>
    </div>
  )
}

export function SidebarNavItem({
  icon: Icon,
  label,
  collapsed = false,
  onClick,
}: {
  icon: LucideIcon
  label: string
  collapsed?: boolean
  onClick?: () => void
}) {
  return (
    <button
      className={cn(
        'group flex h-8 w-full cursor-pointer items-center gap-2.5 rounded-[10px] px-2.5 text-left text-[0.875rem] font-normal text-ink-soft outline-none transition-[background-color,color,transform] duration-100 active:scale-[0.985] focus-visible:bg-canvas-strong/60 focus-visible:text-ink',
        !collapsed && 'hover:bg-surface-hover hover:text-ink',
      )}
      type="button"
      title={label}
      onClick={onClick}
    >
      <span className="relative shrink-0">
        <span
          className={cn(
            'pointer-events-none absolute -inset-1.5 rounded-[9px] transition-colors duration-100',
            collapsed && 'group-hover:bg-surface-hover',
          )}
          aria-hidden="true"
        />
        <Icon
          className="relative size-4 text-ink-soft"
          strokeWidth={1.85}
          aria-hidden="true"
        />
      </span>
      <span
        className={cn(
          'whitespace-nowrap transition-opacity duration-100 ease-out motion-reduce:transition-none',
          collapsed ? 'opacity-0' : 'opacity-100',
        )}
      >
        {label}
      </span>
    </button>
  )
}
