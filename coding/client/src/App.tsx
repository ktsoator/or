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
  Archive,
  BookOpenText,
  CircleAlert,
  Clock3,
  Ellipsis,
  Files,
  Folder,
  FolderOpen,
  GitFork,
  LoaderCircle,
  PanelLeft,
  Pin,
  PencilLine,
  Search,
  Share2,
  ShieldAlert,
  SquarePen,
  Trash2,
  Wrench,
  X,
} from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import { useSession } from './useSession'
import type { Item, SessionSummary } from './types'
import { cn } from './lib/utils'
import { Markdown } from './components/Markdown'
import { ToolCard } from './components/ToolCard'
import { Composer } from './components/Composer'
import { Thinking } from './components/Thinking'
import { StepGroup } from './components/StepGroup'
import { groupItems } from './lib/steps'
import { ProfileMenu } from './components/ProfileMenu'
import { ResponseActions } from './components/ResponseActions'
import { formatMessageTime } from './lib/time'
import { SettingsPage, type SettingsSection } from './components/SettingsPage'
import { SkillsPage } from './components/SkillsPage'
import { WorkspacePickerDialog } from './components/WorkspacePickerDialog'
import { useI18n } from './i18n'
import logoImage from './assets/logo.svg'

const DEFAULT_SIDEBAR_WIDTH = 240
const MIN_SIDEBAR_WIDTH = 206
const MAX_SIDEBAR_WIDTH = 338
const PINNED_SESSIONS_KEY = 'coding.pinned-session-ids'

function clampSidebarWidth(width: number) {
  return Math.min(MAX_SIDEBAR_WIDTH, Math.max(MIN_SIDEBAR_WIDTH, width))
}

function wheelDeltaInPixels(event: WheelEvent, pageHeight: number) {
  if (event.deltaMode === WheelEvent.DOM_DELTA_LINE) return event.deltaY * 16
  if (event.deltaMode === WheelEvent.DOM_DELTA_PAGE) return event.deltaY * pageHeight
  return event.deltaY
}

function readPinnedSessionIDs(): string[] {
  try {
    const value = JSON.parse(localStorage.getItem(PINNED_SESSIONS_KEY) ?? '[]')
    return Array.isArray(value) ? value.filter((id): id is string => typeof id === 'string') : []
  } catch {
    return []
  }
}

function pinnedFirst(items: SessionSummary[], pinned: Set<string>): SessionSummary[] {
  return [...items].sort(
    (left, right) => Number(pinned.has(right.id)) - Number(pinned.has(left.id)),
  )
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
    approval,
    running,
    autoCompacting,
    loading,
    creating,
    updatingSettings,
    compacting,
    status,
    models,
    refreshModels,
    registerWorkspace,
    removeWorkspace,
    startDraft,
    updateDraftWorkspace,
    deleteSession,
    renameSession,
    selectSession,
    updateSettings,
    updatePermissionMode,
    compactContext,
    send,
    removeQueuedMessage,
    stop,
    resolveApproval,
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
  const [removeWorkspaceTarget, setRemoveWorkspaceTarget] = useState<{
    path: string
    name: string
  }>()
  const [removingWorkspace, setRemovingWorkspace] = useState(false)
  const [removeWorkspaceError, setRemoveWorkspaceError] = useState('')
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [settingsSection, setSettingsSection] = useState<SettingsSection>('general')
  const [skillsOpen, setSkillsOpen] = useState(false)
  const [workspacePickerOpen, setWorkspacePickerOpen] = useState(false)
  const [selectedWorkspacePath, setSelectedWorkspacePath] = useState<string>()
  const [pinnedSessionIDs, setPinnedSessionIDs] = useState(readPinnedSessionIDs)
  const pinnedSessionIDSet = useMemo(() => new Set(pinnedSessionIDs), [pinnedSessionIDs])

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
      if (existing) existing.sessions.push(session)
    }
    return [...groups.values()].map((group) => ({
      ...group,
      sessions: pinnedFirst(group.sessions, pinnedSessionIDSet),
    }))
  }, [pinnedSessionIDSet, sessions, t, workspaces])
  const chatSessions = useMemo(
    () => pinnedFirst(sessions.filter((session) => session.scope === 'chat'), pinnedSessionIDSet),
    [pinnedSessionIDSet, sessions],
  )
  const workspacePickerPath =
    selectedWorkspacePath || draft?.workspacePath || activeSession?.workspacePath || workspaceGroups[0]?.path

  useEffect(() => {
    localStorage.setItem(PINNED_SESSIONS_KEY, JSON.stringify(pinnedSessionIDs))
  }, [pinnedSessionIDs])

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
        setSettingsSection('general')
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

  useEffect(() => {
    const transcript = logRef.current
    if (!transcript) return

    const routeWheelToCode = (event: WheelEvent) => {
      if (event.ctrlKey || event.defaultPrevented) return
      if (Math.abs(event.deltaY) <= Math.abs(event.deltaX)) return

      const hitTarget = document.elementFromPoint(event.clientX, event.clientY)
      const codeArea = hitTarget?.closest<HTMLElement>('.code-scroll-area')
      if (!codeArea || !transcript.contains(codeArea)) return

      const maxScrollTop = codeArea.scrollHeight - codeArea.clientHeight
      if (maxScrollTop <= 1) return

      const deltaY = wheelDeltaInPixels(event, codeArea.clientHeight)
      const canScrollUp = deltaY < 0 && codeArea.scrollTop > 0
      const canScrollDown = deltaY > 0 && codeArea.scrollTop < maxScrollTop
      if (!canScrollUp && !canScrollDown) return

      event.preventDefault()
      event.stopPropagation()
      codeArea.scrollTop = Math.min(maxScrollTop, Math.max(0, codeArea.scrollTop + deltaY))
    }

    // A native non-passive capture listener bypasses the browser's wheel target
    // latching and gives the code under the pointer the first scroll gesture.
    transcript.addEventListener('wheel', routeWheelToCode, { capture: true, passive: false })
    return () => transcript.removeEventListener('wheel', routeWheelToCode, { capture: true })
  }, [])

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

  const requestRemoveWorkspace = (path: string, name: string) => {
    setRemoveWorkspaceError('')
    setRemoveWorkspaceTarget({ path, name })
  }

  const togglePinnedSession = (id: string) => {
    setPinnedSessionIDs((current) =>
      current.includes(id) ? current.filter((sessionID) => sessionID !== id) : [...current, id],
    )
  }

  // Rejections propagate so the inline editor stays open with the typed text.
  const handleRename = async (id: string, customTitle: string) => {
    await renameSession(id, customTitle)
  }

  const confirmDelete = async () => {
    if (!deleteTarget || deleteTarget.running || deleteTarget.hasApproval) return
    setDeleting(true)
    setDeleteError('')
    try {
      await deleteSession(deleteTarget.id)
      setPinnedSessionIDs((current) => current.filter((id) => id !== deleteTarget.id))
      setDeleteTarget(undefined)
      setMobileSessionsOpen(false)
    } catch (error) {
      setDeleteError(error instanceof Error ? error.message : t('app.couldNotDelete'))
    } finally {
      setDeleting(false)
    }
  }

  const confirmRemoveWorkspace = async () => {
    if (!removeWorkspaceTarget) return
    const target = removeWorkspaceTarget
    setRemovingWorkspace(true)
    setRemoveWorkspaceError('')
    try {
      await removeWorkspace(target.path)
      if (
        (draft?.projectScoped && draft.workspacePath === target.path) ||
        (activeSession?.scope === 'project' && activeSession.workspacePath === target.path)
      ) {
        addSession(undefined, false)
      }
      if (selectedWorkspacePath === target.path) setSelectedWorkspacePath(undefined)
      setRemoveWorkspaceTarget(undefined)
      setMobileSessionsOpen(false)
    } catch (error) {
      setRemoveWorkspaceError(
        error instanceof Error ? error.message : t('workspace.removeFailed'),
      )
    } finally {
      setRemovingWorkspace(false)
    }
  }

  const emptySession = !loading && items.length === 0 && !approval
  // Nothing renders between sending a prompt and the first assistant event, so
  // a slow model looks like a dead thread. The placeholder fills that gap and
  // is replaced by the real item as soon as anything streams in.
  const awaitingFirstOutput = running && items.at(-1)?.kind === 'user'

  const composer = (centered = false) => (
    <Composer
      key={draft?.id ?? activeSessionID ?? 'empty-session'}
      connected={status === 'ready' && !creating}
      running={running}
      approval={approval}
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
      permissionMode={draft?.permissionMode ?? activeSession?.permissionMode ?? 'ask'}
      updatingSettings={updatingSettings}
      compacting={compacting}
      onSend={send}
      onRemoveQueued={removeQueuedMessage}
      onStop={stop}
      onResolve={resolveApproval}
      onSelectProject={(path) => {
        updateDraftWorkspace(path, Boolean(path))
        setSelectedWorkspacePath(path)
      }}
      onBrowseProjects={() => {
        setWorkspacePickerOpen(true)
      }}
      onConfigureModel={() => {
        setSettingsSection('models')
        setSettingsOpen(true)
      }}
      onSettingsChange={updateSettings}
      onPermissionModeChange={updatePermissionMode}
      onCompact={draft ? undefined : compactContext}
    />
  )

  if (settingsOpen) {
    return (
      <SettingsPage
        initialSection={settingsSection}
        onBack={() => setSettingsOpen(false)}
        onProvidersChanged={refreshModels}
      />
    )
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
          '--sidebar-width': `${sidebarCollapsed ? 60 : sidebarWidth}px`,
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
          'max-md:fixed max-md:inset-y-0 max-md:left-0 max-md:w-[17.5rem] max-md:shadow-2xl',
          mobileSessionsOpen ? 'max-md:translate-x-0' : 'max-md:-translate-x-full',
        )}
        aria-label={t('app.sessions')}
      >
        <div className="relative h-16 w-full shrink-0 max-md:w-[17.5rem]">
          <div
            className={cn(
              'absolute inset-y-0 left-3.5 flex items-center transition-opacity duration-100 ease-out motion-reduce:transition-none',
              sidebarCollapsed ? 'opacity-0' : 'opacity-100',
            )}
          >
            <img className="size-[1.5625rem] shrink-0" src={logoImage} alt="" aria-hidden="true" />
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
            <Search className="size-[1.125rem]" aria-hidden="true" />
          </button>
          <button
            className="absolute top-4 right-4 grid size-8 cursor-pointer place-items-center rounded-lg text-stone-500 transition-colors duration-200 ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none hover:bg-stone-200/75 hover:text-stone-950"
            type="button"
            title={sidebarCollapsed ? t('app.expandSidebar') : t('app.collapseSidebar')}
            aria-label={sidebarCollapsed ? t('app.expandSidebar') : t('app.collapseSidebar')}
            aria-expanded={!sidebarCollapsed}
            onClick={toggleSidebar}
          >
            <PanelLeft className="size-[1.125rem]" aria-hidden="true" />
          </button>
        </div>

        <div className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto">
          <div className="w-full px-3 pb-3 max-md:w-[17.5rem]">
            <button
              className={cn(
                'group flex h-8 w-full cursor-pointer items-center gap-2.5 rounded-[10px] px-2.5 text-left text-[0.875rem] font-normal text-stone-900 transition-colors duration-100 motion-reduce:transition-none disabled:cursor-wait disabled:opacity-50',
                !sidebarCollapsed &&
                  'hover:bg-[rgb(246,246,246)] hover:text-stone-950',
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
                    sidebarCollapsed && 'group-hover:bg-[rgb(246,246,246)]',
                  )}
                  aria-hidden="true"
                />
                {creating ? (
                  <LoaderCircle
                    className="relative size-[1.125rem] animate-spin"
                    aria-hidden="true"
                  />
                ) : (
                  <SquarePen className="relative size-[1.125rem]" aria-hidden="true" />
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
                icon={BookOpenText}
                label={t('app.skills')}
                collapsed={sidebarCollapsed}
                onClick={() => setSkillsOpen(true)}
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
              'w-full px-5 pt-2 pb-2 text-[0.8125rem] font-medium tracking-[-0.01em] whitespace-nowrap text-stone-400 transition-opacity duration-100 ease-out motion-reduce:transition-none max-md:w-[17.5rem]',
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
                <div className="ml-7 flex h-8 items-center px-2.5 text-[0.84375rem] text-stone-400">
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
                    onTogglePin={() => togglePinnedSession(session.id)}
                    onDelete={() => requestDelete(session)}
                    onRename={(title) => handleRename(session.id, title)}
                  />
                ))
              )}
            </div>
          </nav>

          <div
            className={cn(
              'w-full px-5 pt-2 pb-2 text-[0.8125rem] font-medium tracking-[-0.01em] whitespace-nowrap text-stone-400 transition-opacity duration-100 ease-out motion-reduce:transition-none max-md:w-[17.5rem]',
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
                  onSelectWorkspace={(path) => setSelectedWorkspacePath(path)}
                  onSelectSession={chooseSession}
                  onCreateSession={(path) => addSession(path, true)}
                  pinnedSessionIDs={pinnedSessionIDSet}
                  onTogglePinnedSession={togglePinnedSession}
                  onDeleteSession={requestDelete}
                  onRenameSession={handleRename}
                  onRemoveWorkspace={requestRemoveWorkspace}
                />
              ))}
            </div>
          </nav>
        </div>

        <ProfileMenu
          collapsed={sidebarCollapsed}
          onOpenUsage={() => {
            setSettingsSection('usage')
            setSettingsOpen(true)
          }}
          onOpenSettings={() => {
            setSettingsSection('general')
            setSettingsOpen(true)
          }}
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

      {skillsOpen ? (
        <SkillsPage
          onBack={() => setSkillsOpen(false)}
          workspacePath={activeSession?.workspacePath}
          workspaceName={activeSession?.workspaceName}
        />
      ) : (
      <div className="relative flex h-full min-w-0 flex-col">
        <header className="z-20 flex h-13 shrink-0 items-center gap-3 border-b border-stone-200/80 bg-white px-6 max-md:h-12 max-md:px-4">
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
            <div className="flex min-w-0 items-center gap-1.5">
              {!draft && activeSession?.scope === 'project' && activeSession.workspaceName && (
                <>
                  <span
                    className="shrink-0 text-[0.9375rem] text-stone-400 max-sm:hidden"
                    title={activeSession.workspacePath}
                  >
                    {activeSession.workspaceName}
                  </span>
                  <span className="shrink-0 text-stone-300 max-sm:hidden" aria-hidden="true">
                    /
                  </span>
                </>
              )}
              <span
                className="truncate text-[0.9375rem] font-medium tracking-[-0.015em] text-stone-900"
                title={activeSession?.title}
              >
                {draft || activeSession?.title === 'New session'
                  ? t('app.newSession')
                  : (activeSession?.title ?? 'OR coding')}
              </span>
            </div>
          </div>
        </header>

        <main
          ref={logRef}
          className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto px-3 md:px-6 md:[scrollbar-gutter:stable_both-edges]"
          onScroll={trackScrollPosition}
        >
          <div
            className={cn(
              'mx-auto min-h-full w-full max-w-[750px] py-8 pb-12 max-md:py-6',
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
                  <h1 className="m-0 text-[1.75rem] leading-tight font-medium tracking-[-0.03em] text-stone-900 max-sm:text-2xl">
                    {t('app.emptyTitle')}
                  </h1>
                  <p className="mt-2.5 text-[0.9375rem] leading-6 text-stone-500">
                    {t('app.emptyDescription')}
                  </p>
                </div>
                {composer(true)}
              </div>
            ) : (
              <>
                {groupItems(items).map((unit) =>
                  unit.kind === 'steps' ? (
                    <StepGroup key={unit.id} items={unit.items} cwd={activeSession?.workspacePath} />
                  ) : (
                    <ThreadItem
                      key={unit.item.id}
                      item={unit.item}
                      cwd={activeSession?.workspacePath}
                    />
                  ),
                )}
                {autoCompacting ? <AutoCompactionStatus /> : awaitingFirstOutput && <AwaitingResponse />}
              </>
            )}
          </div>
        </main>

        {!loading && !emptySession && composer()}
      </div>
      )}

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

      {removeWorkspaceTarget && (
        <RemoveWorkspaceDialog
          workspace={removeWorkspaceTarget}
          removing={removingWorkspace}
          error={removeWorkspaceError}
          onCancel={() => {
            if (!removingWorkspace) setRemoveWorkspaceTarget(undefined)
          }}
          onConfirm={() => void confirmRemoveWorkspace()}
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
  pinnedSessionIDs,
  onTogglePinnedSession,
  onDeleteSession,
  onRenameSession,
  onRemoveWorkspace,
}: {
  path: string
  name: string
  sessions: SessionSummary[]
  activeSessionID?: string
  onSelectWorkspace: (path: string) => void
  onSelectSession: (id: string) => void
  onCreateSession: (path: string) => void
  pinnedSessionIDs: Set<string>
  onTogglePinnedSession: (id: string) => void
  onDeleteSession: (session: SessionSummary) => void
  onRenameSession: (id: string, customTitle: string) => Promise<void>
  onRemoveWorkspace: (path: string, name: string) => void
}) {
  const { t } = useI18n()
  const [expanded, setExpanded] = useState(true)
  const [menuOpen, setMenuOpen] = useState(false)
  return (
    <section aria-label={name}>
      <div className="group/workspace relative flex h-8 items-center">
        <button
          className="flex h-8 min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-[10px] py-0 pr-[4.125rem] pl-2.5 text-left text-[0.875rem] font-normal text-stone-800 transition-colors hover:bg-[rgb(246,246,246)] hover:text-stone-950"
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
              className="size-[1.0625rem] shrink-0 text-stone-600"
              strokeWidth={1.8}
              aria-hidden="true"
            />
          ) : (
            <Folder
              className="size-[1.0625rem] shrink-0 text-stone-600"
              strokeWidth={1.8}
              aria-hidden="true"
            />
          )}
          <span className="min-w-0 flex-1 truncate">{name}</span>
        </button>
        <div
          className={cn(
            'absolute top-0.5 right-0.5 flex items-center opacity-0 transition-opacity duration-100 group-hover/workspace:opacity-100 group-focus-within/workspace:opacity-100 max-md:opacity-100',
            menuOpen && 'opacity-100',
          )}
        >
          <button
            className="grid size-7 cursor-pointer place-items-center rounded-[9px] text-stone-400 transition-colors hover:text-stone-950 focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-stone-400"
            type="button"
            title={t('workspace.newSession', { name })}
            aria-label={t('workspace.newSession', { name })}
            onClick={() => onCreateSession(path)}
          >
            <SquarePen className="size-3.5" aria-hidden="true" />
          </button>
          <DropdownMenu.Root open={menuOpen} onOpenChange={setMenuOpen}>
            <DropdownMenu.Trigger asChild>
              <button
                className="grid size-7 cursor-pointer place-items-center rounded-[9px] text-stone-400 transition-colors hover:text-stone-950 focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-stone-400 data-[state=open]:text-stone-950"
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
                className="z-[120] min-w-[13.75rem] animate-[fade-in_100ms_ease-out] rounded-[14px] border border-stone-200 bg-white p-1 text-[0.84375rem] text-stone-900 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
              >
                <DropdownMenu.Item className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 outline-none data-[highlighted]:bg-[rgb(241,241,241)]">
                  <Pin className="size-4 text-stone-600" aria-hidden="true" />
                  <span>{t('workspace.pinProject')}</span>
                </DropdownMenu.Item>
                <DropdownMenu.Item className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 outline-none data-[highlighted]:bg-[rgb(241,241,241)]">
                  <FolderOpen className="size-4 text-stone-600" aria-hidden="true" />
                  <span>{t('workspace.revealInFinder')}</span>
                </DropdownMenu.Item>
                <DropdownMenu.Item className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 outline-none data-[highlighted]:bg-[rgb(241,241,241)]">
                  <GitFork className="size-4 text-stone-600" aria-hidden="true" />
                  <span>{t('workspace.createWorktree')}</span>
                </DropdownMenu.Item>
                <DropdownMenu.Item className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 outline-none data-[highlighted]:bg-[rgb(241,241,241)]">
                  <PencilLine className="size-4 text-stone-600" aria-hidden="true" />
                  <span>{t('workspace.renameProject')}</span>
                </DropdownMenu.Item>
                <DropdownMenu.Item
                  disabled
                  className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 text-stone-400 outline-none"
                >
                  <Archive className="size-4" aria-hidden="true" />
                  <span>{t('workspace.archiveChats')}</span>
                </DropdownMenu.Item>
                <DropdownMenu.Separator className="mx-1 my-1 h-px bg-stone-100" />
                <DropdownMenu.Item
                  className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 text-red-700 outline-none data-[highlighted]:bg-red-50"
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
            <div className="flex h-8 items-center pr-2.5 pl-[2.375rem] text-[0.84375rem] text-stone-400">
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
                onTogglePin={() => onTogglePinnedSession(session.id)}
                onDelete={() => onDeleteSession(session)}
                onRename={(title) => onRenameSession(session.id, title)}
                indented
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
  pinned,
  onSelect,
  onTogglePin,
  onDelete,
  onRename,
  indented = false,
}: {
  session: SessionSummary
  active: boolean
  pinned: boolean
  onSelect: () => void
  onTogglePin: () => void
  onDelete: () => void
  onRename: (customTitle: string) => Promise<void>
  indented?: boolean
}) {
  const { t } = useI18n()
  const title = session.title === 'New session' ? t('app.newSession') : session.title
  const [menuOpen, setMenuOpen] = useState(false)
  const [draftTitle, setDraftTitle] = useState<string | undefined>(undefined)
  const editing = draftTitle !== undefined
  // Enter commits and then blurs the input; the guard keeps that from sending a
  // second PATCH for the same edit.
  const committing = useRef(false)
  // Rename swaps this row for the editor, so the menu must not restore focus to
  // its (now unmounted) trigger and pull it back off the input.
  const openingEditor = useRef(false)

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
            'h-8 w-full rounded-[10px] bg-white pr-2.5 text-[0.875rem] font-normal leading-5 text-stone-950 outline-2 outline-stone-400',
            indented ? 'pl-[2.375rem]' : 'pl-2.5',
          )}
          ref={(node) => node?.select()}
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
      <button
        className={cn(
          'flex h-8 w-full cursor-pointer items-center rounded-[10px] pr-[4.125rem] text-left transition-colors',
          indented ? 'pl-[2.375rem]' : 'pl-2.5',
          active
            ? 'bg-[rgb(237,237,237)] text-stone-950'
            : 'text-stone-700 hover:bg-[rgb(246,246,246)] hover:text-stone-950',
        )}
        type="button"
        aria-current={active ? 'page' : undefined}
        onClick={onSelect}
      >
        <span className="min-w-0 flex-1 truncate text-[0.875rem] font-normal leading-5" title={title}>
          {title}
        </span>
      </button>
      {(session.hasApproval || session.running) && (
        <span
          className={cn(
            'pointer-events-none absolute top-1/2 right-3 grid size-4 -translate-y-1/2 place-items-center transition-opacity duration-100 group-hover/session:opacity-0 group-focus-within/session:opacity-0 max-md:opacity-0',
            menuOpen && 'opacity-0',
          )}
          title={session.hasApproval ? t('app.approvalNeeded') : t('app.working')}
        >
          {session.hasApproval ? (
            <CircleAlert className="size-3.5 text-amber-700" aria-hidden="true" />
          ) : (
            <LoaderCircle className="size-3.5 animate-spin text-stone-500" aria-hidden="true" />
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
            'grid size-7 cursor-pointer place-items-center rounded-[9px] text-stone-400 transition-colors hover:text-stone-950 focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-stone-400',
            pinned && 'text-stone-500',
          )}
          type="button"
          title={pinned ? t('app.unpinSession') : t('app.pinSession')}
          aria-label={pinned ? t('app.unpinNamedSession', { title }) : t('app.pinNamedSession', { title })}
          aria-pressed={pinned}
          onClick={onTogglePin}
        >
          <Pin className={cn('size-3.5', pinned && 'fill-current')} aria-hidden="true" />
        </button>
        <DropdownMenu.Root open={menuOpen} onOpenChange={setMenuOpen}>
          <DropdownMenu.Trigger asChild>
            <button
              className="grid size-7 cursor-pointer place-items-center rounded-[9px] text-stone-400 transition-colors hover:text-stone-950 focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-stone-400 data-[state=open]:text-stone-950"
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
              className="z-[120] min-w-[11.75rem] animate-[fade-in_100ms_ease-out] rounded-[14px] border border-stone-200 bg-white p-1 text-[0.84375rem] text-stone-900 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
              onCloseAutoFocus={(event) => {
                if (!openingEditor.current) return
                openingEditor.current = false
                event.preventDefault()
              }}
            >
              <DropdownMenu.Item className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 outline-none data-[highlighted]:bg-[rgb(241,241,241)]">
                <Share2 className="size-4 text-stone-600" aria-hidden="true" />
                <span>{t('app.shareSession')}</span>
              </DropdownMenu.Item>
              <DropdownMenu.Item
                className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 outline-none data-[highlighted]:bg-[rgb(241,241,241)]"
                onSelect={() => {
                  openingEditor.current = true
                  setDraftTitle(title)
                }}
              >
                <PencilLine className="size-4 text-stone-600" aria-hidden="true" />
                <span>{t('app.renameSession')}</span>
              </DropdownMenu.Item>
              <DropdownMenu.Item
                className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 outline-none data-[highlighted]:bg-[rgb(241,241,241)]"
                onSelect={onTogglePin}
              >
                <Pin className="size-4 text-stone-600" aria-hidden="true" />
                <span>{pinned ? t('app.unpinSession') : t('app.pinSession')}</span>
              </DropdownMenu.Item>
              <DropdownMenu.Item className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 outline-none data-[highlighted]:bg-[rgb(241,241,241)]">
                <Archive className="size-4 text-stone-600" aria-hidden="true" />
                <span>{t('app.archiveSession')}</span>
              </DropdownMenu.Item>
              <DropdownMenu.Separator className="mx-1 my-1 h-px bg-stone-100" />
              <DropdownMenu.Item
                className="flex h-8 cursor-default select-none items-center gap-2.5 rounded-[9px] px-2.5 text-red-700 outline-none data-[highlighted]:bg-red-50"
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

function SidebarNavItem({
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
        'group flex h-8 w-full cursor-pointer items-center gap-2.5 rounded-[10px] px-2.5 text-left text-[0.875rem] font-normal text-stone-800 transition-[background-color,color,transform] duration-100 active:scale-[0.985] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-stone-400',
        !collapsed && 'hover:bg-[rgb(246,246,246)] hover:text-stone-950',
      )}
      type="button"
      title={label}
      onClick={onClick}
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
          className="relative size-[1.125rem] text-stone-700"
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
        className="relative w-full max-w-[29.25rem] animate-[fade-in_140ms_ease-out] rounded-[22px] border border-white/80 bg-white p-6 shadow-[0_30px_90px_-32px_rgba(28,25,23,0.62)] max-sm:rounded-[18px] max-sm:p-5"
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
            className="text-[1.1875rem] leading-6 font-semibold tracking-[-0.02em] text-stone-950"
          >
            {t('delete.title')}
          </h2>
          <p
            id="delete-session-description"
            className="mt-1.5 text-[0.875rem] leading-[1.55] text-stone-500"
          >
            {t('delete.description')}
          </p>
        </div>

        <div className="mt-5 border-y border-stone-200/80 py-3.5">
          <div className="text-[0.6875rem] leading-4 font-medium tracking-[0.08em] text-stone-400 uppercase">
            {t('delete.session')}
          </div>
          <div className="mt-1 truncate text-[0.90625rem] leading-5 font-medium text-stone-800">
            {title}
          </div>
        </div>

        {blocked && (
          <div className="mt-4 flex gap-2.5 rounded-xl border border-amber-200/70 bg-amber-50/70 px-3.5 py-3 text-[0.8125rem] leading-5 text-amber-900">
            <ShieldAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
            <span>{t('delete.blocked')}</span>
          </div>
        )}
        {error && (
          <div className="mt-4 flex gap-2.5 rounded-xl border border-red-200/70 bg-red-50/70 px-3.5 py-3 text-[0.8125rem] leading-5 text-red-800">
            <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
            <span>{error}</span>
          </div>
        )}

        <div className="mt-6 flex justify-end gap-2.5">
          <button
            className="h-10 cursor-pointer rounded-xl border border-stone-300 bg-white px-4 text-[0.875rem] font-medium text-stone-700 transition-[border-color,background-color,color] hover:border-stone-400 hover:bg-stone-50 hover:text-stone-950 disabled:cursor-wait disabled:opacity-50"
            type="button"
            disabled={deleting}
            onClick={onCancel}
          >
            {t('delete.cancel')}
          </button>
          <button
            className="flex h-10 min-w-[7.875rem] cursor-pointer items-center justify-center gap-2 rounded-xl bg-[#b42318] px-4 text-[0.875rem] font-medium text-white shadow-[0_5px_14px_-8px_rgba(180,35,24,0.85)] transition-[background-color,transform] hover:bg-[#991b1b] active:translate-y-px disabled:cursor-not-allowed disabled:opacity-35"
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

function RemoveWorkspaceDialog({
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
      className="fixed inset-0 z-[100] grid place-items-center bg-stone-950/25 px-4 py-8 backdrop-blur-[2px]"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget && !removing) onCancel()
      }}
    >
      <section
        className="relative w-full max-w-[28rem] animate-[fade-in_140ms_ease-out] rounded-[20px] border border-white/80 bg-white p-6 shadow-[0_28px_80px_-34px_rgba(28,25,23,0.58)] max-sm:rounded-[18px] max-sm:p-5"
        role="dialog"
        aria-modal="true"
        aria-labelledby="remove-workspace-title"
        aria-describedby="remove-workspace-description"
      >
        <button
          className="absolute top-4 right-4 grid size-8 cursor-pointer place-items-center rounded-full text-stone-400 transition-colors hover:bg-stone-100 hover:text-stone-700 disabled:cursor-wait disabled:opacity-40"
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
            className="text-[1.125rem] leading-6 font-semibold tracking-[-0.02em] text-stone-950"
          >
            {t('workspace.removeTitle')}
          </h2>
          <p
            id="remove-workspace-description"
            className="mt-1.5 text-[0.875rem] leading-[1.55] text-stone-500"
          >
            {t('workspace.removeDescription')}
          </p>
        </div>

        <div className="mt-5 rounded-xl border border-stone-200/80 px-3.5 py-3">
          <div className="truncate text-[0.90625rem] leading-5 font-medium text-stone-800">
            {workspace.name}
          </div>
          <div className="mt-0.5 truncate font-mono text-[0.71875rem] leading-4 text-stone-400" title={workspace.path}>
            {workspace.path}
          </div>
        </div>

        {error && (
          <div className="mt-4 flex gap-2.5 rounded-xl border border-red-200/70 bg-red-50/70 px-3.5 py-3 text-[0.8125rem] leading-5 text-red-800">
            <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
            <span>{error}</span>
          </div>
        )}

        <div className="mt-6 flex justify-end gap-2.5">
          <button
            className="h-9 cursor-pointer rounded-[10px] border border-stone-300 bg-white px-4 text-[0.84375rem] font-medium text-stone-700 transition-colors hover:bg-stone-50 hover:text-stone-950 disabled:cursor-wait disabled:opacity-50"
            type="button"
            disabled={removing}
            onClick={onCancel}
          >
            {t('workspace.cancel')}
          </button>
          <button
            className="flex h-9 min-w-[7.5rem] cursor-pointer items-center justify-center gap-2 rounded-[10px] bg-[#b42318] px-4 text-[0.84375rem] font-medium text-white transition-colors hover:bg-[#991b1b] disabled:cursor-wait disabled:opacity-40"
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

// AwaitingResponse holds the thread's place between a sent prompt and the first
// assistant event. It mirrors the streaming Thinking header so the transition to
// real output is not a visual jump.
function AwaitingResponse() {
  const { t } = useI18n()
  return (
    <div
      className="my-1 flex animate-[fade-in_160ms_ease-out] items-center gap-1.5 py-0.5 text-[0.8125rem] text-stone-400"
      role="status"
    >
      <span className="size-1 animate-pulse rounded-full bg-indigo-500" />
      <span className="streaming-sheen">{t('thinking.working')}</span>
    </div>
  )
}

function AutoCompactionStatus() {
  const { t } = useI18n()
  const [visible, setVisible] = useState(false)

  useEffect(() => {
    const timer = window.setTimeout(() => setVisible(true), 350)
    return () => window.clearTimeout(timer)
  }, [])

  if (!visible) return null
  return (
    <div
      className="my-1 flex animate-[fade-in_160ms_ease-out] items-center gap-1.5 py-0.5 text-[0.8125rem] text-stone-400"
      role="status"
    >
      <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
      <span>{t('compaction.automatic')}</span>
    </div>
  )
}

function ThreadItem({ item, cwd }: { item: Item; cwd?: string }) {
  const { locale, t } = useI18n()
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
                    className="size-[8.5rem] shrink-0 rounded-2xl border border-stone-200 bg-white object-cover shadow-[0_7px_18px_-15px_rgba(28,25,23,0.55)] max-sm:size-28"
                    src={`data:${image.mimeType};base64,${image.data}`}
                    alt={t('app.uploadedImage', { index: index + 1 })}
                  />
                ))}
              </div>
            )}
            {item.text && (
              <div className="rounded-xl bg-stone-100 px-3.5 py-2.5 text-[14px] leading-[22px] whitespace-pre-wrap">
                {item.text}
              </div>
            )}
            {(item.sentAt || item.deliveryStatus === 'failed') && (
              <div className="-mt-1 flex items-center justify-end gap-2 px-1 text-[0.75rem] leading-4 tabular-nums">
                {item.deliveryStatus === 'failed' && (
                  <span className="text-red-600">{t('app.notSent')}</span>
                )}
                {item.sentAt && (
                  <time className="text-stone-400" dateTime={item.sentAt}>
                    {formatMessageTime(item.sentAt, locale)}
                  </time>
                )}
              </div>
            )}
          </div>
        </section>
      )
    case 'assistant':
      return (
        <section className="my-4 animate-[fade-in_160ms_ease-out]">
          <Markdown source={item.markdown} />
          {item.complete && (
            <ResponseActions
              usage={item.usage}
              modelName={item.modelName || item.model}
              responseText={item.markdown}
              completedAt={item.completedAt}
            />
          )}
        </section>
      )
    case 'run':
      return <RunDuration item={item} />
    case 'thinking':
      return <Thinking item={item} />
    case 'tool':
      return <ToolCard item={item} cwd={cwd} />
    case 'error':
      return (
        <div
          className="my-4 flex animate-[fade-in_160ms_ease-out] gap-2.5 border-l-2 border-red-300 py-1 pl-3 text-red-700"
          role="alert"
        >
          <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
          <div className="flex flex-col gap-0.5">
            <strong className="text-[0.8125rem] font-semibold">{t('app.somethingWentWrong')}</strong>
            <span className="text-[0.8125rem]">{item.text}</span>
          </div>
        </div>
      )
  }
}

function RunDuration({ item }: { item: Extract<Item, { kind: 'run' }> }) {
  const { locale, t } = useI18n()
  const [now, setNow] = useState(() => Date.now())
  const running = item.durationMs === undefined

  useEffect(() => {
    if (!running) return
    setNow(Date.now())
    const interval = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(interval)
  }, [item.startedAt, running])

  const startedAt = new Date(item.startedAt).getTime()
  const durationMs =
    item.durationMs ?? (Number.isFinite(startedAt) ? Math.max(0, now - startedAt) : 0)
  const duration = formatRunDuration(durationMs, locale)

  return (
    <div className="mt-7 mb-4 animate-[fade-in_160ms_ease-out]">
      <div className="text-[0.8125rem] leading-5 text-stone-400 tabular-nums">
        {t(running ? 'run.working' : 'run.completed', { duration })}
      </div>
      <div className="mt-2.5 h-px bg-stone-200/80" aria-hidden="true" />
    </div>
  )
}

function formatRunDuration(durationMs: number, locale: 'en' | 'zh-CN'): string {
  const totalSeconds = Math.max(0, Math.floor(durationMs / 1000))
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60

  if (locale === 'zh-CN') {
    if (hours > 0) return `${hours} 小时 ${minutes} 分 ${seconds} 秒`
    if (minutes > 0) return `${minutes} 分 ${seconds} 秒`
    return `${seconds} 秒`
  }
  if (hours > 0) return `${hours}h ${minutes}m ${seconds}s`
  if (minutes > 0) return `${minutes}m ${seconds}s`
  return `${seconds}s`
}
