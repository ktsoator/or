import type {
  KeyboardEvent as ReactKeyboardEvent,
  PointerEvent as ReactPointerEvent,
} from 'react'
import { useCallback, useState } from 'react'
import {
  BookOpenText,
  Cable,
  LoaderCircle,
  SquarePen,
} from 'lucide-react'
import type { SessionSummary } from '@/types'
import type { WorkspaceSessionGroup } from './sessionSidebarLayout'
import { useI18n } from '@/i18n'
import { cn } from '@/lib/utils'
import { PanelResizeHandle } from '@/shared/ui/PanelResizeHandle'
import { SidebarToggleButton } from '@/shared/ui/SidebarToggleButton'
import { ProfileMenu } from './ProfileMenu'
import {
  SessionRow,
  SidebarNavItem,
  WorkspaceSessions,
} from './SessionSidebarItems'

type AppSidebarProps = {
  layout: {
    mobileSessionsOpen: boolean
    sidebarCollapsed: boolean
    persistentTitlebarControls: boolean
    sidebarWidth: number
    sidebarResizing: boolean
    sidebarMinimumWidth: number
    sidebarMaximumWidth: number
    pinnedSessionIDSet: Set<string>
  }
  content: {
    creating: boolean
    activeSessionID?: string
    chatSessions: SessionSummary[]
    workspaceGroups: WorkspaceSessionGroup[]
  }
  actions: {
    closeMobileSessions: () => void
    toggleSidebar: () => void
    addSession: (workspacePath?: string, projectScoped?: boolean) => void
    onOpenSkills: () => void
    onOpenMCP: () => void
    chooseSession: (id: string) => void
    openSessionInWorkbench: (id: string) => void
    togglePinnedSession: (id: string) => void
    requestDelete: (session: SessionSummary) => void
    handleRename: (id: string, customTitle: string) => Promise<void>
    onSelectWorkspace: (path: string) => void
    revealWorkspace: (path: string) => Promise<void>
    requestRemoveWorkspace: (path: string, name: string) => void
    onOpenSettings: () => void
    startSidebarResize: (event: ReactPointerEvent<HTMLDivElement>) => void
    resizeSidebar: (event: ReactPointerEvent<HTMLDivElement>) => void
    stopSidebarResize: (event: ReactPointerEvent<HTMLDivElement>) => void
    resizeSidebarWithKeyboard: (event: ReactKeyboardEvent<HTMLDivElement>) => void
  }
}

export function AppSidebar({ layout, content, actions }: AppSidebarProps) {
  const { t } = useI18n()
  const [openHoverCardKey, setOpenHoverCardKey] = useState<string>()
  const [openMenuKey, setOpenMenuKey] = useState<string>()
  const handleHoverCardOpenChange = useCallback((key: string, open: boolean) => {
    if (open && openMenuKey !== undefined) return
    setOpenHoverCardKey((current) => open ? key : current === key ? undefined : current)
  }, [openMenuKey])
  const handleMenuOpenChange = useCallback((key: string, open: boolean) => {
    setOpenMenuKey((current) => open ? key : current === key ? undefined : current)
    if (open) setOpenHoverCardKey(undefined)
  }, [])
  const {
    mobileSessionsOpen,
    sidebarCollapsed,
    persistentTitlebarControls,
    sidebarWidth,
    sidebarResizing,
    sidebarMinimumWidth,
    sidebarMaximumWidth,
    pinnedSessionIDSet,
  } = layout
  const {
    creating,
    activeSessionID,
    chatSessions,
    workspaceGroups,
  } = content
  const {
    closeMobileSessions,
    toggleSidebar,
    addSession,
    onOpenSkills,
    onOpenMCP,
    chooseSession,
    openSessionInWorkbench,
    togglePinnedSession,
    requestDelete,
    handleRename,
    onSelectWorkspace,
    revealWorkspace,
    requestRemoveWorkspace,
    onOpenSettings,
    startSidebarResize,
    resizeSidebar,
    stopSidebarResize,
    resizeSidebarWithKeyboard,
  } = actions

  return (
    <>
      {mobileSessionsOpen && (
        <button
          className="fixed inset-0 z-40 bg-scrim/15 backdrop-blur-[1px] md:hidden"
          type="button"
          aria-label={t('app.closeSessions')}
          onClick={closeMobileSessions}
        />
      )}
      <div
        className={cn(
          'sidebar-viewport relative z-50 min-h-0 min-w-0 max-md:contents',
          sidebarCollapsed ? 'overflow-hidden' : 'overflow-visible',
        )}
        data-testid="sidebar-viewport"
      >
        <aside
          className={cn(
            'app-sidebar relative flex h-full w-[var(--sidebar-expanded-width)] min-h-0 min-w-0 flex-col overflow-hidden bg-canvas text-ink-soft transition-transform duration-200 ease-out',
            persistentTitlebarControls ? 'border-r-0' : 'border-r border-edge/75',
            'max-md:fixed max-md:inset-y-0 max-md:left-0 max-md:z-50 max-md:w-[17.5rem] max-md:shadow-2xl',
            mobileSessionsOpen ? 'max-md:translate-x-0' : 'max-md:-translate-x-full',
          )}
          aria-label={t('app.sessions')}
          aria-hidden={sidebarCollapsed && !mobileSessionsOpen ? true : undefined}
          inert={sidebarCollapsed && !mobileSessionsOpen}
        >
          <div className="app-sidebar-header window-titlebar relative h-16 w-full shrink-0 max-md:w-[17.5rem]">
            <div className="window-titlebar-controls">
              {!sidebarCollapsed && (!persistentTitlebarControls || mobileSessionsOpen) && (
                <SidebarToggleButton
                  expanded
                  className="sidebar-header-action sidebar-collapse-action absolute top-4 right-4 motion-reduce:transition-none"
                  onToggle={toggleSidebar}
                />
              )}
            </div>
          </div>

        <div className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto">
          <div className="w-full px-3 pb-3 max-md:w-[17.5rem]">
            <button
              className={cn(
                'group flex h-8 w-full cursor-pointer items-center gap-2.5 rounded-[10px] px-2.5 text-left text-[0.875rem] font-normal text-ink transition-colors duration-100 motion-reduce:transition-none disabled:cursor-wait disabled:opacity-50',
                !sidebarCollapsed &&
                  'hover:bg-surface-hover hover:text-ink',
              )}
              type="button"
              title={t('app.newSession')}
              disabled={creating}
              onClick={() => addSession(undefined, false)}
            >
              <span className="relative shrink-0">
                <span
                  className={cn(
                    'pointer-events-none absolute -inset-1.5 rounded-[9px] transition-colors duration-100',
                    sidebarCollapsed && 'group-hover:bg-surface-hover',
                  )}
                  aria-hidden="true"
                />
                {creating ? (
                  <LoaderCircle
                    className="relative size-4 animate-spin"
                    aria-hidden="true"
                  />
                ) : (
                  <SquarePen className="relative size-4" aria-hidden="true" />
                )}
              </span>
              <span
                className={cn(
                  'whitespace-nowrap transition-opacity duration-100 ease-out motion-reduce:transition-none',
                  sidebarCollapsed ? 'opacity-0' : 'opacity-100',
                )}
              >
                {t('app.newSession')}
              </span>
            </button>

            <div className="mt-1 space-y-1" aria-label={t('app.workspaceShortcuts')}>
              <SidebarNavItem
                icon={BookOpenText}
                label={t('app.skills')}
                collapsed={sidebarCollapsed}
                onClick={() => onOpenSkills()}
              />
              <SidebarNavItem
                icon={Cable}
                label={t('app.mcp')}
                collapsed={sidebarCollapsed}
                onClick={() => onOpenMCP()}
              />
            </div>
          </div>

          <div
            className={cn(
              'w-full px-5 pt-2 pb-2 text-[0.8125rem] font-medium tracking-[-0.01em] whitespace-nowrap text-ink-faint transition-opacity duration-100 ease-out motion-reduce:transition-none max-md:w-[17.5rem]',
              sidebarCollapsed ? 'pointer-events-none opacity-0' : 'opacity-100',
            )}
          >
            {t('workspace.chats')}
          </div>
          <nav
            className={cn(
              'w-full px-3 pb-2 transition-opacity duration-100 ease-out motion-reduce:transition-none max-md:w-[17.5rem]',
              sidebarCollapsed ? 'pointer-events-none opacity-0' : 'opacity-100',
            )}
            aria-hidden={sidebarCollapsed}
            aria-label={t('workspace.chats')}
          >
            <div className="space-y-1">
              {chatSessions.length === 0 ? (
                <div className="ml-7 flex h-8 items-center px-2.5 text-[0.84375rem] text-ink-faint">
                  {t('workspace.noChats')}
                </div>
              ) : (
                chatSessions.map((session) => (
                  <SessionRow
                    key={session.id}
                    session={session}
                    active={session.id === activeSessionID}
                    pinned={pinnedSessionIDSet.has(session.id)}
                    onSelect={() => chooseSession(session.id)}
                    onOpenInWorkbench={() => openSessionInWorkbench(session.id)}
                    onTogglePin={() => togglePinnedSession(session.id)}
                    onDelete={() => requestDelete(session)}
                    onRename={(title) => handleRename(session.id, title)}
                    openHoverCardKey={openHoverCardKey}
                    onHoverCardOpenChange={handleHoverCardOpenChange}
                    onMenuOpenChange={handleMenuOpenChange}
                  />
                ))
              )}
            </div>
          </nav>

          <div
            className={cn(
              'w-full px-5 pt-2 pb-2 text-[0.8125rem] font-medium tracking-[-0.01em] whitespace-nowrap text-ink-faint transition-opacity duration-100 ease-out motion-reduce:transition-none max-md:w-[17.5rem]',
              sidebarCollapsed ? 'pointer-events-none opacity-0' : 'opacity-100',
            )}
          >
            {t('workspace.projects')}
          </div>
          <nav
            className={cn(
              'w-full px-3 pb-3 transition-opacity duration-100 ease-out motion-reduce:transition-none max-md:w-[17.5rem]',
              sidebarCollapsed ? 'pointer-events-none opacity-0' : 'opacity-100',
            )}
            aria-hidden={sidebarCollapsed}
            aria-label={t('app.codingSessions')}
          >
            <div className="space-y-2">
              {workspaceGroups.map((workspace) => (
                <WorkspaceSessions
                  key={workspace.path}
                  path={workspace.path}
                  name={workspace.name}
                  sessions={workspace.sessions}
                  activeSessionID={activeSessionID}
                  onSelectWorkspace={(path) => onSelectWorkspace(path)}
                  onSelectSession={chooseSession}
                  onOpenSessionInWorkbench={openSessionInWorkbench}
                  onCreateSession={(path) => addSession(path, true)}
                  pinnedSessionIDs={pinnedSessionIDSet}
                  onTogglePinnedSession={togglePinnedSession}
                  onDeleteSession={requestDelete}
                  onRenameSession={handleRename}
                  onRevealWorkspace={revealWorkspace}
                  onRemoveWorkspace={requestRemoveWorkspace}
                  openHoverCardKey={openHoverCardKey}
                  onHoverCardOpenChange={handleHoverCardOpenChange}
                  onMenuOpenChange={handleMenuOpenChange}
                />
              ))}
            </div>
          </nav>
        </div>

        <ProfileMenu
          collapsed={sidebarCollapsed}
          onOpenSettings={() => {
            onOpenSettings()
          }}
        />

        </aside>
        {!sidebarCollapsed && (
          <PanelResizeHandle
            edge="right"
            label={t('app.resizeSidebar')}
            minimum={sidebarMinimumWidth}
            maximum={sidebarMaximumWidth}
            value={sidebarWidth}
            resizing={sidebarResizing}
            testID="sidebar-resize-handle"
            dividerTestID="sidebar-resize-divider"
            onPointerDown={startSidebarResize}
            onPointerMove={resizeSidebar}
            onPointerUp={stopSidebarResize}
            onPointerCancel={stopSidebarResize}
            onLostPointerCapture={stopSidebarResize}
            onKeyDown={resizeSidebarWithKeyboard}
          />
        )}
        {persistentTitlebarControls && (
          <span
            className="sidebar-transition-divider pointer-events-none absolute inset-y-0 right-0 hidden w-px bg-edge/75 md:block"
            data-testid="sidebar-divider-line"
            aria-hidden="true"
          />
        )}
      </div>
    </>
  )
}
