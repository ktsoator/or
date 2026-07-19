import {
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from 'react'
import type { LucideIcon } from 'lucide-react'
import {
  CircleAlert,
  Clock3,
  Ellipsis,
  Files,
  Folder,
  FolderOpen,
  LoaderCircle,
  PanelLeft,
  Search,
  ShieldAlert,
  SquarePen,
  Trash2,
  Wrench,
  X,
} from 'lucide-react'
import { useSession } from './useSession'
import type { Item, SessionSummary } from './types'
import { cn } from './lib/utils'
import { Markdown } from './components/Markdown'
import { ToolCard } from './components/ToolCard'
import { Composer } from './components/Composer'
import { Thinking } from './components/Thinking'
import { ProfileMenu } from './components/ProfileMenu'
import { ResponseActions } from './components/ResponseActions'
import { SettingsPage } from './components/SettingsPage'
import { WorkspacePickerDialog } from './components/WorkspacePickerDialog'
import { useI18n } from './i18n'
import logoImage from './assets/logo.svg'

const DEFAULT_SIDEBAR_WIDTH = 256
const MIN_SIDEBAR_WIDTH = 220
const MAX_SIDEBAR_WIDTH = 360

function clampSidebarWidth(width: number) {
  return Math.min(MAX_SIDEBAR_WIDTH, Math.max(MIN_SIDEBAR_WIDTH, width))
}

export default function App() {
  const { t } = useI18n()
  const {
    sessions,
    workspaces,
    draft,
    activeSession,
    activeSessionID,
    items,
    queuedMessages,
    contextUsage,
    confirmation,
    running,
    loading,
    creating,
    updatingSettings,
    status,
    models,
    registerWorkspace,
    startDraft,
    updateDraftWorkspace,
    deleteSession,
    selectSession,
    updateSettings,
    send,
    removeQueuedMessage,
    stop,
    resolveConfirm,
  } = useSession()
  const logRef = useRef<HTMLDivElement>(null)
  const followLatestRef = useRef(true)
  const previousSessionIDRef = useRef<string | undefined>(undefined)
  const sidebarResizeRef = useRef<
    | {
        pointerID: number
        startX: number
        startWidth: number
      }
    | undefined
  >(undefined)
  const [mobileSessionsOpen, setMobileSessionsOpen] = useState(false)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [sidebarWidth, setSidebarWidth] = useState(DEFAULT_SIDEBAR_WIDTH)
  const [sidebarResizing, setSidebarResizing] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<SessionSummary>()
  const [deleting, setDeleting] = useState(false)
  const [deleteError, setDeleteError] = useState('')
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [workspacePickerOpen, setWorkspacePickerOpen] = useState(false)
  const [selectedWorkspacePath, setSelectedWorkspacePath] = useState<string>()

  const workspaceGroups = useMemo(() => {
    const groups = new Map<
      string,
      { path: string; name: string; sessions: SessionSummary[] }
    >()
    for (const workspace of workspaces) {
      groups.set(workspace.path, {
        path: workspace.path,
        name: workspace.name,
        sessions: [],
      })
    }
    for (const session of sessions) {
      if (session.scope !== 'project') continue
      const path = session.workspacePath || ''
      const existing = groups.get(path)
      if (existing) {
        existing.sessions.push(session)
      } else {
        groups.set(path, {
          path,
          name: session.workspaceName || path.split('/').filter(Boolean).pop() || t('app.workspace'),
          sessions: [session],
        })
      }
    }
    return [...groups.values()]
  }, [sessions, t, workspaces])
  const chatSessions = useMemo(
    () => sessions.filter((session) => session.scope === 'chat'),
    [sessions],
  )
  const workspacePickerPath =
    selectedWorkspacePath || draft?.workspacePath || activeSession?.workspacePath || workspaceGroups[0]?.path

  useEffect(() => {
    if (draft || selectedWorkspacePath) return
    const initialPath = activeSession?.scope === 'project'
      ? activeSession.workspacePath
      : workspaceGroups[0]?.path
    if (initialPath) setSelectedWorkspacePath(initialPath)
  }, [activeSession, draft, selectedWorkspacePath, workspaceGroups])

  useEffect(() => {
    const handleSettingsShortcut = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key === ',') {
        event.preventDefault()
        setSettingsOpen(true)
      } else if (event.key === 'Escape' && settingsOpen) {
        setSettingsOpen(false)
      }
    }
    window.addEventListener('keydown', handleSettingsShortcut)
    return () => window.removeEventListener('keydown', handleSettingsShortcut)
  }, [settingsOpen])

  useEffect(() => {
    if (!sidebarResizing) return
    const previousCursor = document.body.style.cursor
    const previousUserSelect = document.body.style.userSelect
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    return () => {
      document.body.style.cursor = previousCursor
      document.body.style.userSelect = previousUserSelect
    }
  }, [sidebarResizing])

  useLayoutEffect(() => {
    const el = logRef.current
    if (!el) return

    const sessionChanged = previousSessionIDRef.current !== activeSessionID
    previousSessionIDRef.current = activeSessionID
    if (sessionChanged) followLatestRef.current = true

    if (followLatestRef.current) {
      el.scrollTop = el.scrollHeight
    }
  }, [activeSessionID, items])

  const toggleSidebar = () => {
    if (mobileSessionsOpen) {
      setMobileSessionsOpen(false)
      return
    }
    setSidebarCollapsed((collapsed) => !collapsed)
  }

  const startSidebarResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (sidebarCollapsed) return
    event.preventDefault()
    sidebarResizeRef.current = {
      pointerID: event.pointerId,
      startX: event.clientX,
      startWidth: sidebarWidth,
    }
    event.currentTarget.setPointerCapture(event.pointerId)
    setSidebarResizing(true)
  }

  const resizeSidebar = (event: ReactPointerEvent<HTMLDivElement>) => {
    const resize = sidebarResizeRef.current
    if (!resize || resize.pointerID !== event.pointerId) return
    setSidebarWidth(clampSidebarWidth(resize.startWidth + event.clientX - resize.startX))
  }

  const stopSidebarResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    const resize = sidebarResizeRef.current
    if (!resize || resize.pointerID !== event.pointerId) return
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
    sidebarResizeRef.current = undefined
    setSidebarResizing(false)
  }

  const resizeSidebarWithKeyboard = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    let nextWidth: number | undefined
    if (event.key === 'ArrowLeft') nextWidth = sidebarWidth - 8
    if (event.key === 'ArrowRight') nextWidth = sidebarWidth + 8
    if (event.key === 'Home') nextWidth = MIN_SIDEBAR_WIDTH
    if (event.key === 'End') nextWidth = MAX_SIDEBAR_WIDTH
    if (nextWidth === undefined) return
    event.preventDefault()
    setSidebarWidth(clampSidebarWidth(nextWidth))
  }

  const trackScrollPosition = () => {
    const el = logRef.current
    if (!el) return
    followLatestRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 72
  }

  const chooseSession = (id: string) => {
    const session = sessions.find((candidate) => candidate.id === id)
    if (session) setSelectedWorkspacePath(session.scope === 'project' ? session.workspacePath : undefined)
    selectSession(id)
    setMobileSessionsOpen(false)
  }

  const addSession = (workspacePath?: string, projectScoped = false) => {
    setSelectedWorkspacePath(projectScoped ? workspacePath : undefined)
    startDraft(workspacePath, projectScoped)
    setMobileSessionsOpen(false)
  }

  const requestDelete = (session: SessionSummary) => {
    setDeleteError('')
    setDeleteTarget(session)
  }

  const confirmDelete = async () => {
    if (!deleteTarget || deleteTarget.running || deleteTarget.hasApproval) return
    setDeleting(true)
    setDeleteError('')
    try {
      await deleteSession(deleteTarget.id)
      setDeleteTarget(undefined)
      setMobileSessionsOpen(false)
    } catch (error) {
      setDeleteError(error instanceof Error ? error.message : t('app.couldNotDelete'))
    } finally {
      setDeleting(false)
    }
  }

  const emptySession = !loading && items.length === 0 && !confirmation

  const composer = (centered = false) => (
    <Composer
      key={draft?.id ?? activeSessionID ?? 'empty-session'}
      connected={status === 'ready' && !creating}
      running={running}
      confirmation={confirmation}
      queuedMessages={queuedMessages}
      contextUsage={contextUsage}
      centered={centered}
      projectPickerVisible={Boolean(draft)}
      workspaces={workspaces}
      workspacePath={draft?.projectScoped ? draft.workspacePath : undefined}
      models={models}
      modelProvider={draft?.modelProvider ?? activeSession?.modelProvider}
      modelID={draft?.modelID ?? activeSession?.modelId}
      thinkingLevel={draft?.thinkingLevel ?? activeSession?.thinkingLevel}
      updatingSettings={updatingSettings}
      onSend={send}
      onRemoveQueued={removeQueuedMessage}
      onStop={stop}
      onResolve={resolveConfirm}
      onSelectProject={(path) => {
        updateDraftWorkspace(path, Boolean(path))
        setSelectedWorkspacePath(path)
      }}
      onBrowseProjects={() => {
        setWorkspacePickerOpen(true)
      }}
      onSettingsChange={updateSettings}
    />
  )

  if (settingsOpen) {
    return <SettingsPage onBack={() => setSettingsOpen(false)} />
  }

  return (
    <div
      className={cn(
        'grid h-full grid-cols-[var(--sidebar-width)_minmax(0,1fr)] grid-rows-[minmax(0,1fr)] overflow-hidden bg-white motion-reduce:transition-none max-md:grid-cols-1',
        !sidebarResizing &&
          'transition-[grid-template-columns] duration-200 ease-[cubic-bezier(0.22,1,0.36,1)]',
      )}
      style={
        {
          '--sidebar-width': `${sidebarCollapsed ? 64 : sidebarWidth}px`,
        } as CSSProperties
      }
    >
      {mobileSessionsOpen && (
        <button
          className="fixed inset-0 z-40 bg-stone-950/15 backdrop-blur-[1px] md:hidden"
          type="button"
          aria-label={t('app.closeSessions')}
          onClick={() => setMobileSessionsOpen(false)}
        />
      )}
      <aside
        className={cn(
          'relative z-50 flex min-h-0 min-w-0 flex-col overflow-hidden border-r border-stone-200/75 bg-white text-stone-700 transition-transform duration-200 ease-out',
          'max-md:fixed max-md:inset-y-0 max-md:left-0 max-md:w-[280px] max-md:shadow-2xl',
          mobileSessionsOpen ? 'max-md:translate-x-0' : 'max-md:-translate-x-full',
        )}
        aria-label={t('app.sessions')}
      >
        <div className="relative h-16 w-full shrink-0 max-md:w-[280px]">
          <div
            className={cn(
              'absolute inset-y-0 left-3.5 flex items-center transition-opacity duration-100 ease-out motion-reduce:transition-none',
              sidebarCollapsed ? 'opacity-0' : 'opacity-100',
            )}
          >
            <img className="size-[25px] shrink-0" src={logoImage} alt="" aria-hidden="true" />
          </div>
          <button
            className={cn(
              'absolute top-4 right-14 grid size-8 cursor-pointer place-items-center rounded-lg text-stone-500 transition-[opacity,color,background-color,transform] duration-100 ease-out motion-reduce:transition-none hover:bg-stone-200/75 hover:text-stone-950 active:scale-95 focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-stone-400',
              sidebarCollapsed ? 'pointer-events-none opacity-0' : 'opacity-100',
            )}
            type="button"
            title={t('app.searchSessions')}
            aria-label={t('app.searchSessions')}
          >
            <Search className="size-[18px]" aria-hidden="true" />
          </button>
          <button
            className="absolute top-4 right-4 grid size-8 cursor-pointer place-items-center rounded-lg text-stone-500 transition-colors duration-200 ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none hover:bg-stone-200/75 hover:text-stone-950"
            type="button"
            title={sidebarCollapsed ? t('app.expandSidebar') : t('app.collapseSidebar')}
            aria-label={sidebarCollapsed ? t('app.expandSidebar') : t('app.collapseSidebar')}
            aria-expanded={!sidebarCollapsed}
            onClick={toggleSidebar}
          >
            <PanelLeft className="size-[18px]" aria-hidden="true" />
          </button>
        </div>

        <div className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto">
          <div className="w-full px-3 pb-3 max-md:w-[280px]">
            <button
              className={cn(
                'group flex h-8 w-full cursor-pointer items-center gap-2.5 rounded-[10px] px-2.5 text-left text-[14px] font-[540] text-stone-900 transition-colors duration-100 motion-reduce:transition-none disabled:cursor-wait disabled:opacity-50',
                !sidebarCollapsed &&
                  (draft
                    ? 'bg-[rgb(237,237,237)] text-stone-950'
                    : 'hover:bg-[rgb(246,246,246)] hover:text-stone-950'),
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
                    sidebarCollapsed &&
                      (draft
                        ? 'bg-[rgb(237,237,237)]'
                        : 'group-hover:bg-[rgb(246,246,246)]'),
                  )}
                  aria-hidden="true"
                />
                {creating ? (
                  <LoaderCircle
                    className="relative size-[18px] animate-spin"
                    aria-hidden="true"
                  />
                ) : (
                  <SquarePen className="relative size-[18px]" aria-hidden="true" />
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
                icon={Files}
                label={t('app.files')}
                collapsed={sidebarCollapsed}
              />
              <SidebarNavItem
                icon={Clock3}
                label={t('app.scheduled')}
                collapsed={sidebarCollapsed}
              />
              <SidebarNavItem
                icon={Wrench}
                label={t('app.tools')}
                collapsed={sidebarCollapsed}
              />
              <SidebarNavItem
                icon={Ellipsis}
                label={t('app.more')}
                collapsed={sidebarCollapsed}
              />
            </div>
          </div>

          <div
            className={cn(
              'w-full px-5 pt-2 pb-2 text-[13px] font-[620] tracking-[-0.01em] whitespace-nowrap text-stone-900 transition-opacity duration-100 ease-out motion-reduce:transition-none max-md:w-[280px]',
              sidebarCollapsed ? 'pointer-events-none opacity-0' : 'opacity-100',
            )}
          >
            {t('workspace.chats')}
          </div>
          <nav
            className={cn(
              'w-full px-3 pb-2 transition-opacity duration-100 ease-out motion-reduce:transition-none max-md:w-[280px]',
              sidebarCollapsed ? 'pointer-events-none opacity-0' : 'opacity-100',
            )}
            aria-hidden={sidebarCollapsed}
            aria-label={t('workspace.chats')}
          >
            <div className="space-y-1">
              {chatSessions.length === 0 ? (
                <div className="ml-7 flex h-8 items-center px-2.5 text-[13.5px] text-stone-400">
                  {t('workspace.noChats')}
                </div>
              ) : (
                chatSessions.map((session) => (
                  <SessionRow
                    key={session.id}
                    session={session}
                    active={session.id === activeSessionID}
                    onSelect={() => chooseSession(session.id)}
                    onDelete={() => requestDelete(session)}
                  />
                ))
              )}
            </div>
          </nav>

          <div
            className={cn(
              'w-full px-5 pt-2 pb-2 text-[13px] font-[620] tracking-[-0.01em] whitespace-nowrap text-stone-900 transition-opacity duration-100 ease-out motion-reduce:transition-none max-md:w-[280px]',
              sidebarCollapsed ? 'pointer-events-none opacity-0' : 'opacity-100',
            )}
          >
            {t('workspace.projects')}
          </div>
          <nav
            className={cn(
              'w-full px-3 pb-3 transition-opacity duration-100 ease-out motion-reduce:transition-none max-md:w-[280px]',
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
                  onSelectWorkspace={(path) => setSelectedWorkspacePath(path)}
                  onSelectSession={chooseSession}
                  onCreateSession={(path) => addSession(path, true)}
                  onDeleteSession={requestDelete}
                />
              ))}
            </div>
          </nav>
        </div>

        <ProfileMenu
          collapsed={sidebarCollapsed}
          onOpenSettings={() => setSettingsOpen(true)}
        />

        {!sidebarCollapsed && (
          <div
            className="group absolute inset-y-0 right-0 z-[60] w-1.5 touch-none cursor-col-resize outline-none max-md:hidden"
            role="separator"
            aria-label={t('app.resizeSidebar')}
            aria-orientation="vertical"
            aria-valuemin={MIN_SIDEBAR_WIDTH}
            aria-valuemax={MAX_SIDEBAR_WIDTH}
            aria-valuenow={sidebarWidth}
            tabIndex={0}
            onPointerDown={startSidebarResize}
            onPointerMove={resizeSidebar}
            onPointerUp={stopSidebarResize}
            onPointerCancel={stopSidebarResize}
            onKeyDown={resizeSidebarWithKeyboard}
          >
            <span
              className={cn(
                'absolute inset-y-0 right-0 w-px transition-colors group-hover:bg-stone-400/60 group-focus-visible:bg-stone-500/70',
                sidebarResizing && 'bg-stone-500/70',
              )}
              aria-hidden="true"
            />
          </div>
        )}
      </aside>

      <div className="relative flex h-full min-w-0 flex-col">
        <header className="z-20 flex h-13 shrink-0 items-center border-b border-stone-200/80 bg-white px-6 max-md:h-12 max-md:px-4">
          <div className="flex min-w-0 items-center gap-2.5">
            <button
              className="-ml-1 grid size-7 shrink-0 place-items-center rounded-md text-stone-500 transition-colors hover:bg-stone-100 hover:text-stone-900 md:hidden"
              type="button"
              title={t('app.sessions')}
              onClick={() => {
                setSidebarCollapsed(false)
                setMobileSessionsOpen(true)
              }}
            >
              <PanelLeft className="size-4" aria-hidden="true" />
              <span className="sr-only">{t('app.openSessions')}</span>
            </button>
            <span
              className="truncate text-[15px] font-[620] tracking-[-0.015em] text-stone-900"
              title={activeSession?.title}
            >
              {draft || activeSession?.title === 'New session'
                ? t('app.newSession')
                : (activeSession?.title ?? 'OR coding')}
            </span>
          </div>
        </header>

        <main
          ref={logRef}
          className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto px-3 md:px-6 md:[scrollbar-gutter:stable_both-edges]"
          onScroll={trackScrollPosition}
        >
          <div
            className={cn(
              'mx-auto min-h-full w-full max-w-[896px] py-8 pb-12 max-md:py-6',
              (loading || emptySession) && 'grid place-items-center',
            )}
          >
            {loading ? (
              <div className="flex items-center gap-2 pb-[8vh] text-xs text-stone-400">
                <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
                {t('app.loadingSession')}
              </div>
            ) : emptySession ? (
              <div className="flex w-full -translate-y-[3vh] flex-col items-center gap-9">
                <div className="max-w-lg text-center">
                  <h1 className="m-0 text-[28px] leading-tight font-[560] tracking-[-0.03em] text-stone-900 max-sm:text-2xl">
                    {t('app.emptyTitle')}
                  </h1>
                  <p className="mt-2.5 text-[15px] leading-6 text-stone-500">
                    {t('app.emptyDescription')}
                  </p>
                </div>
                {composer(true)}
              </div>
            ) : (
              items.map((item) => <ThreadItem key={item.id} item={item} />)
            )}
          </div>
        </main>

        {!loading && !emptySession && composer()}
      </div>

      {deleteTarget && (
        <DeleteSessionDialog
          session={deleteTarget}
          deleting={deleting}
          error={deleteError}
          onCancel={() => {
            if (!deleting) setDeleteTarget(undefined)
          }}
          onConfirm={() => void confirmDelete()}
        />
      )}

      {workspacePickerOpen && (
        <WorkspacePickerDialog
          initialPath={workspacePickerPath}
          onClose={() => {
            setWorkspacePickerOpen(false)
          }}
          onSelect={async (path) => {
            const workspace = await registerWorkspace(path)
            updateDraftWorkspace(workspace.path, true)
            setSelectedWorkspacePath(workspace.path)
            setWorkspacePickerOpen(false)
            setMobileSessionsOpen(false)
          }}
        />
      )}
    </div>
  )
}

function WorkspaceSessions({
  path,
  name,
  sessions,
  activeSessionID,
  onSelectWorkspace,
  onSelectSession,
  onCreateSession,
  onDeleteSession,
}: {
  path: string
  name: string
  sessions: SessionSummary[]
  activeSessionID?: string
  onSelectWorkspace: (path: string) => void
  onSelectSession: (id: string) => void
  onCreateSession: (path: string) => void
  onDeleteSession: (session: SessionSummary) => void
}) {
  const { t } = useI18n()
  const [expanded, setExpanded] = useState(true)
  return (
    <section aria-label={name}>
      <div className="group/workspace relative flex h-8 items-center">
        <button
          className="flex h-8 min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-[10px] px-2.5 text-left text-[14px] font-[570] text-stone-800 transition-colors hover:bg-[rgb(246,246,246)] hover:text-stone-950"
          type="button"
          title={path}
          aria-expanded={expanded}
          onClick={() => {
            onSelectWorkspace(path)
            setExpanded((current) => !current)
          }}
        >
          {expanded ? (
            <FolderOpen
              className="size-[17px] shrink-0 text-stone-600"
              strokeWidth={1.8}
              aria-hidden="true"
            />
          ) : (
            <Folder
              className="size-[17px] shrink-0 text-stone-600"
              strokeWidth={1.8}
              aria-hidden="true"
            />
          )}
          <span className="min-w-0 flex-1 truncate">{name}</span>
        </button>
        <button
          className="absolute right-1 grid size-7 cursor-pointer place-items-center rounded-md text-stone-400 opacity-0 transition-[opacity,color,background-color] group-hover/workspace:opacity-100 hover:bg-stone-200/70 hover:text-stone-900 focus-visible:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-stone-400 max-md:opacity-100"
          type="button"
          title={t('workspace.newSession', { name })}
          aria-label={t('workspace.newSession', { name })}
          onClick={() => onCreateSession(path)}
        >
          <SquarePen className="size-3.5" aria-hidden="true" />
        </button>
      </div>
      {expanded && (
        <div className="mt-1 ml-7 space-y-1">
          {sessions.length === 0 ? (
            <div className="flex h-8 items-center px-2.5 text-[13.5px] text-stone-400">
              {t('workspace.noChats')}
            </div>
          ) : (
            sessions.map((session) => (
              <SessionRow
                key={session.id}
                session={session}
                active={session.id === activeSessionID}
                onSelect={() => onSelectSession(session.id)}
                onDelete={() => onDeleteSession(session)}
              />
            ))
          )}
        </div>
      )}
    </section>
  )
}

function SessionRow({
  session,
  active,
  onSelect,
  onDelete,
}: {
  session: SessionSummary
  active: boolean
  onSelect: () => void
  onDelete: () => void
}) {
  const { t } = useI18n()
  const title = session.title === 'New session' ? t('app.newSession') : session.title
  return (
    <div className="group relative">
      <button
        className={cn(
          'flex h-8 w-full cursor-pointer items-center rounded-[10px] px-2.5 pr-9 text-left transition-colors',
          active
            ? 'bg-[rgb(237,237,237)] text-stone-950'
            : 'text-stone-700 hover:bg-[rgb(246,246,246)] hover:text-stone-950',
        )}
        type="button"
        aria-current={active ? 'page' : undefined}
        onClick={onSelect}
      >
        <span className="min-w-0 flex-1 truncate text-[14px] font-[510] leading-5" title={title}>
          {title}
        </span>
        {(session.hasApproval || session.running) && (
          <span className="ml-2 flex shrink-0 items-center gap-1.5 text-[11.5px] leading-4 text-stone-500 transition-opacity group-hover:opacity-0">
            {session.hasApproval ? (
              <>
                <ShieldAlert className="size-3 text-amber-700" aria-hidden="true" />
                {t('app.approvalNeeded')}
              </>
            ) : (
              <>
                <LoaderCircle className="size-3 animate-spin text-stone-500" aria-hidden="true" />
                {t('app.working')}
              </>
            )}
          </span>
        )}
      </button>
      <button
        className="absolute top-0.5 right-0.5 grid size-7 cursor-pointer place-items-center rounded-[9px] text-stone-400 opacity-0 transition-[opacity,color,background-color] group-hover:opacity-100 hover:bg-stone-200 hover:text-red-700 focus-visible:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-stone-400 max-md:opacity-100"
        type="button"
        title={t('app.deleteNamedSession', { title })}
        aria-label={t('app.deleteNamedSession', { title })}
        onClick={onDelete}
      >
        <Trash2 className="size-3.5" aria-hidden="true" />
      </button>
    </div>
  )
}

function SidebarNavItem({
  icon: Icon,
  label,
  collapsed = false,
}: {
  icon: LucideIcon
  label: string
  collapsed?: boolean
}) {
  return (
    <button
      className={cn(
        'group flex h-8 w-full cursor-pointer items-center gap-2.5 rounded-[10px] px-2.5 text-left text-[14px] font-[500] text-stone-800 transition-[background-color,color,transform] duration-100 active:scale-[0.985] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-stone-400',
        !collapsed && 'hover:bg-[rgb(246,246,246)] hover:text-stone-950',
      )}
      type="button"
      title={label}
    >
      <span className="relative shrink-0">
        <span
          className={cn(
            'pointer-events-none absolute -inset-1.5 rounded-[9px] transition-colors duration-100',
            collapsed && 'group-hover:bg-[rgb(246,246,246)]',
          )}
          aria-hidden="true"
        />
        <Icon
          className="relative size-[18px] text-stone-700"
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

function DeleteSessionDialog({
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
      className="fixed inset-0 z-[100] grid place-items-center bg-stone-950/30 px-4 py-8 backdrop-blur-[3px]"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget && !deleting) onCancel()
      }}
    >
      <section
        className="relative w-full max-w-[468px] animate-[fade-in_140ms_ease-out] rounded-[22px] border border-white/80 bg-white p-6 shadow-[0_30px_90px_-32px_rgba(28,25,23,0.62)] max-sm:rounded-[18px] max-sm:p-5"
        role="dialog"
        aria-modal="true"
        aria-labelledby="delete-session-title"
        aria-describedby="delete-session-description"
      >
        <button
          className="absolute top-4 right-4 grid size-8 cursor-pointer place-items-center rounded-full text-stone-400 transition-colors hover:bg-stone-100 hover:text-stone-700 disabled:cursor-wait disabled:opacity-40"
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
            className="text-[19px] leading-6 font-[650] tracking-[-0.02em] text-stone-950"
          >
            {t('delete.title')}
          </h2>
          <p
            id="delete-session-description"
            className="mt-1.5 text-[14px] leading-[1.55] text-stone-500"
          >
            {t('delete.description')}
          </p>
        </div>

        <div className="mt-5 border-y border-stone-200/80 py-3.5">
          <div className="text-[11px] leading-4 font-semibold tracking-[0.08em] text-stone-400 uppercase">
            {t('delete.session')}
          </div>
          <div className="mt-1 truncate text-[14.5px] leading-5 font-[560] text-stone-800">
            {title}
          </div>
        </div>

        {blocked && (
          <div className="mt-4 flex gap-2.5 rounded-xl border border-amber-200/70 bg-amber-50/70 px-3.5 py-3 text-[13px] leading-5 text-amber-900">
            <ShieldAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
            <span>{t('delete.blocked')}</span>
          </div>
        )}
        {error && (
          <div className="mt-4 flex gap-2.5 rounded-xl border border-red-200/70 bg-red-50/70 px-3.5 py-3 text-[13px] leading-5 text-red-800">
            <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
            <span>{error}</span>
          </div>
        )}

        <div className="mt-6 flex justify-end gap-2.5">
          <button
            className="h-10 cursor-pointer rounded-xl border border-stone-300 bg-white px-4 text-[14px] font-[560] text-stone-700 transition-[border-color,background-color,color] hover:border-stone-400 hover:bg-stone-50 hover:text-stone-950 disabled:cursor-wait disabled:opacity-50"
            type="button"
            disabled={deleting}
            onClick={onCancel}
          >
            {t('delete.cancel')}
          </button>
          <button
            className="flex h-10 min-w-[126px] cursor-pointer items-center justify-center gap-2 rounded-xl bg-[#b42318] px-4 text-[14px] font-[600] text-white shadow-[0_5px_14px_-8px_rgba(180,35,24,0.85)] transition-[background-color,transform] hover:bg-[#991b1b] active:translate-y-px disabled:cursor-not-allowed disabled:opacity-35"
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

function ThreadItem({ item }: { item: Item }) {
  const { t } = useI18n()
  switch (item.kind) {
    case 'user':
      return (
        <section className="my-5 flex animate-[fade-in_160ms_ease-out] justify-end">
          <div className="flex max-w-[78%] flex-col items-end gap-3 max-md:max-w-[88%]">
            {item.images.length > 0 && (
              <div className="flex max-w-full flex-wrap justify-end gap-3">
                {item.images.map((image, index) => (
                  <img
                    key={`${image.mimeType}-${index}`}
                    className="size-[136px] shrink-0 rounded-2xl border border-stone-200 bg-white object-cover shadow-[0_7px_18px_-15px_rgba(28,25,23,0.55)] max-sm:size-28"
                    src={`data:${image.mimeType};base64,${image.data}`}
                    alt={t('app.uploadedImage', { index: index + 1 })}
                  />
                ))}
              </div>
            )}
            {item.text && (
              <div className="rounded-xl bg-stone-100 px-3.5 py-2.5 text-[16.5px] leading-[1.58] whitespace-pre-wrap">
                {item.text}
              </div>
            )}
            {item.deliveryStatus === 'failed' && (
              <span className="-mt-1 inline-flex items-center gap-1.5 px-1 text-[11.5px] leading-4 text-red-600">
                {t('app.notSent')}
              </span>
            )}
          </div>
        </section>
      )
    case 'assistant':
      return (
        <section className="my-4 animate-[fade-in_160ms_ease-out]">
          <Markdown source={item.markdown} />
          {item.complete && (
            <ResponseActions usage={item.usage} responseText={item.markdown} />
          )}
        </section>
      )
    case 'thinking':
      return <Thinking item={item} />
    case 'tool':
      return <ToolCard item={item} />
    case 'confirm':
      return null
    case 'error':
      return (
        <div
          className="my-4 flex animate-[fade-in_160ms_ease-out] gap-2.5 border-l-2 border-red-300 py-1 pl-3 text-red-700"
          role="alert"
        >
          <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
          <div className="flex flex-col gap-0.5">
            <strong className="text-[13px] font-semibold">{t('app.somethingWentWrong')}</strong>
            <span className="text-[13px]">{item.text}</span>
          </div>
        </div>
      )
  }
}
