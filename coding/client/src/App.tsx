import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
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
  PanelRight,
  PanelRightOpen,
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
import type { SessionSummary } from './types'
import { cn } from './lib/utils'
import { Composer } from './components/Composer'
import { StepGroup } from './components/StepGroup'
import {
  AutoCompactionStatus,
  AwaitingResponse,
  ThreadItem,
} from './components/ConversationThread'
import { groupItems } from './lib/steps'
import { chooseNativeDirectory } from './lib/desktop'
import { ProfileMenu } from './components/ProfileMenu'
import { SettingsPage, type SettingsSection } from './components/SettingsPage'
import { SkillsPage } from './components/SkillsPage'
import { WorkspacePickerDialog } from './components/WorkspacePickerDialog'
import { WorkbenchPanel } from './components/WorkbenchPanel'
import { SidebarToggleButton } from './components/SidebarToggleButton'
import { useI18n } from './i18n'
import { useSidebarLayout } from './useSidebarLayout'
import { useWorkbenchLayout } from './useWorkbenchLayout'

function wheelDeltaInPixels(event: WheelEvent, pageHeight: number) {
  if (event.deltaMode === WheelEvent.DOM_DELTA_LINE) return event.deltaY * 16
  if (event.deltaMode === WheelEvent.DOM_DELTA_PAGE) return event.deltaY * pageHeight
  return event.deltaY
}

export default function App() {
  const { t } = useI18n()
  const [secondarySessionID, setSecondarySessionID] = useState<string>()
  const {
    sessions,
    workspaces,
    draft,
    activeSession,
    activeSessionID,
    items,
    queuedMessages,
    contextUsage,
    preview,
    previewOpen,
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
    createChatSession,
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
    secondaryThread,
  } = useSession(secondarySessionID)
  const logRef = useRef<HTMLDivElement>(null)
  const followLatestRef = useRef(true)
  const previousSessionIDRef = useRef<string | undefined>(undefined)
  const [workbenchCreateError, setWorkbenchCreateError] = useState('')
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
  const {
    mobileSessionsOpen,
    collapsed: sidebarCollapsed,
    width: sidebarWidth,
    resizing: sidebarResizing,
    pinnedSessionIDSet,
    chatSessions,
    workspaceGroups,
    minimumWidth: sidebarMinimumWidth,
    maximumWidth: sidebarMaximumWidth,
    toggleSidebar,
    expandSidebar,
    openMobileSessions,
    closeMobileSessions,
    togglePinnedSession,
    removePinnedSession,
    startResize: startSidebarResize,
    resize: resizeSidebar,
    stopResize: stopSidebarResize,
    resizeWithKeyboard: resizeSidebarWithKeyboard,
  } = useSidebarLayout(sessions, workspaces)
  const {
    layoutRef: workbenchLayoutRef,
    viewportRef: workbenchViewportRef,
    open: workbenchOpen,
    previewSessionID: workbenchPreviewSessionID,
    expandedWidth: workbenchExpandedWidth,
    resizing: workbenchResizing,
    maximized: workbenchMaximized,
    autoLayoutChanging: workbenchAutoLayoutChanging,
    closing: workbenchClosing,
    resizeMinimum: workbenchResizeMinimum,
    resizeMaximum: workbenchResizeMaximum,
    resizeValue: workbenchResizeValue,
    toggle: toggleWorkbench,
    showSession: showSessionInWorkbench,
    toggleMaximized: toggleWorkbenchMaximized,
    startResize: startWorkbenchResize,
    resize: resizeWorkbench,
    stopResize: stopWorkbenchResize,
    resizeWithKeyboard: resizeWorkbenchWithKeyboard,
  } = useWorkbenchLayout({
    enabled: !settingsOpen && !skillsOpen,
    activeSessionID,
    activeDraftID: draft?.id,
    primaryPreviewRevision: preview?.revision,
    primaryPreviewOpen: previewOpen,
    secondarySessionID: secondaryThread?.session.id,
    secondaryPreviewRevision: secondaryThread?.preview?.revision,
    secondaryPreviewOpen: secondaryThread?.previewOpen ?? false,
  })

  const workspacePickerPath =
    selectedWorkspacePath || draft?.workspacePath || activeSession?.workspacePath || workspaceGroups[0]?.path
  const workbenchPreview =
    secondaryThread && workbenchPreviewSessionID === secondaryThread.session.id
      ? secondaryThread.preview
      : !workbenchPreviewSessionID || workbenchPreviewSessionID === activeSessionID
        ? preview
        : undefined
  const workbenchPreviewOwnerID = workbenchPreview
    ? workbenchPreviewSessionID ?? activeSessionID
    : undefined
  const activateWorkbenchPreview = workbenchPreviewOwnerID === activeSessionID
    ? previewOpen
    : secondaryThread && workbenchPreviewOwnerID === secondaryThread.session.id
      ? secondaryThread.previewOpen
      : false
  useEffect(() => {
    if (
      secondarySessionID &&
      !loading &&
      !sessions.some((session) => session.id === secondarySessionID)
    ) {
      setSecondarySessionID(undefined)
    }
  }, [loading, secondarySessionID, sessions])

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

  const trackScrollPosition = () => {
    const el = logRef.current
    if (!el) return
    followLatestRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 72
  }

  const chooseSession = (id: string) => {
    const session = sessions.find((candidate) => candidate.id === id)
    if (session) setSelectedWorkspacePath(session.scope === 'project' ? session.workspacePath : undefined)
    selectSession(id)
    closeMobileSessions()
  }

  const openSessionInWorkbench = (id: string) => {
    if (!sessions.some((session) => session.id === id)) return
    setWorkbenchCreateError('')
    setSecondarySessionID(id)
    showSessionInWorkbench(id)
    closeMobileSessions()
  }

  const createSessionInWorkbench = async () => {
    setWorkbenchCreateError('')
    try {
      const created = await createChatSession()
      setSecondarySessionID(created.id)
      showSessionInWorkbench(created.id)
    } catch (error) {
      setWorkbenchCreateError(
        error instanceof Error ? error.message : t('workbench.createChatFailed'),
      )
    }
  }

  const addSession = (workspacePath?: string, projectScoped = false) => {
    setSelectedWorkspacePath(projectScoped ? workspacePath : undefined)
    startDraft(workspacePath, projectScoped)
    closeMobileSessions()
  }

  const requestDelete = (session: SessionSummary) => {
    setDeleteError('')
    setDeleteTarget(session)
  }

  const requestRemoveWorkspace = (path: string, name: string) => {
    setRemoveWorkspaceError('')
    setRemoveWorkspaceTarget({ path, name })
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
      if (secondarySessionID === deleteTarget.id) setSecondarySessionID(undefined)
      removePinnedSession(deleteTarget.id)
      setDeleteTarget(undefined)
      closeMobileSessions()
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
      closeMobileSessions()
    } catch (error) {
      setRemoveWorkspaceError(
        error instanceof Error ? error.message : t('workspace.removeFailed'),
      )
    } finally {
      setRemovingWorkspace(false)
    }
  }

  const selectWorkspaceFolder = async (path: string) => {
    const workspace = await registerWorkspace(path)
    updateDraftWorkspace(workspace.path, true)
    setSelectedWorkspacePath(workspace.path)
    setWorkspacePickerOpen(false)
    closeMobileSessions()
  }

  const browseWorkspaceFolders = async () => {
    try {
      const path = await chooseNativeDirectory(workspacePickerPath, t('workspace.chooseFolder'))
      if (path === undefined) {
        setWorkspacePickerOpen(true)
        return
      }
      if (!path) return
      await selectWorkspaceFolder(path)
    } catch {
      // The web picker remains a usable fallback when the native bridge fails.
      setWorkspacePickerOpen(true)
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
        void browseWorkspaceFolders()
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

  const workbenchOwnsToggle =
    workbenchOpen || workbenchClosing || workbenchAutoLayoutChanging
  const workbenchToggleControl = (
    <button
      className="window-titlebar-control relative grid size-7 shrink-0 cursor-pointer place-items-center rounded-md text-stone-500 outline-none transition-colors duration-100 hover:bg-stone-200/75 hover:text-stone-950 focus-visible:ring-2 focus-visible:ring-stone-300"
      data-testid="workbench-panel-toggle"
      type="button"
      title={workbenchOpen ? t('workbench.hide') : t('workbench.show')}
      aria-label={workbenchOpen ? t('workbench.hide') : t('workbench.show')}
      aria-expanded={workbenchOpen}
      onClick={toggleWorkbench}
    >
      <PanelRight className="size-4" aria-hidden="true" />
      {preview && !workbenchOpen && (
        <span
          className="absolute top-0.5 right-0.5 size-1.5 rounded-full border border-white bg-blue-500"
          aria-hidden="true"
        />
      )}
    </button>
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
        'relative grid h-full grid-cols-[var(--sidebar-width)_minmax(0,1fr)] grid-rows-[minmax(0,1fr)] overflow-hidden bg-white motion-reduce:transition-none max-md:grid-cols-1',
        !sidebarResizing &&
          'transition-[grid-template-columns] duration-[180ms] ease-[cubic-bezier(0.2,0,0,1)]',
      )}
      style={
        {
          '--sidebar-expanded-width': `${sidebarWidth}px`,
          '--sidebar-width': sidebarCollapsed
            ? '0px'
            : 'var(--sidebar-expanded-width)',
        } as CSSProperties
      }
    >
      {mobileSessionsOpen && (
        <button
          className="fixed inset-0 z-40 bg-stone-950/15 backdrop-blur-[1px] md:hidden"
          type="button"
          aria-label={t('app.closeSessions')}
          onClick={closeMobileSessions}
        />
      )}
      <div
        className="sidebar-viewport relative z-50 min-h-0 min-w-0 overflow-hidden max-md:contents"
        data-testid="sidebar-viewport"
      >
        <aside
          className={cn(
            'app-sidebar relative flex h-full w-[var(--sidebar-expanded-width)] min-h-0 min-w-0 flex-col overflow-hidden border-r border-stone-200/75 bg-white text-stone-700 transition-transform duration-200 ease-out',
            'max-md:fixed max-md:inset-y-0 max-md:left-0 max-md:w-[17.5rem] max-md:shadow-2xl',
            mobileSessionsOpen ? 'max-md:translate-x-0' : 'max-md:-translate-x-full',
          )}
          aria-label={t('app.sessions')}
          aria-hidden={sidebarCollapsed && !mobileSessionsOpen ? true : undefined}
          inert={sidebarCollapsed && !mobileSessionsOpen}
        >
          <div className="app-sidebar-header window-titlebar relative h-16 w-full shrink-0 max-md:w-[17.5rem]">
            <div className="window-titlebar-controls">
              <button
                className={cn(
                  'sidebar-header-action sidebar-search-action absolute top-4 right-14 grid size-8 cursor-pointer place-items-center rounded-lg text-stone-500 transition-[opacity,color,background-color,transform] duration-100 ease-out motion-reduce:transition-none hover:bg-stone-200/75 hover:text-stone-950 active:scale-95 focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-stone-400',
                  sidebarCollapsed ? 'pointer-events-none opacity-0' : 'opacity-100',
                )}
                type="button"
                title={t('app.searchSessions')}
                aria-label={t('app.searchSessions')}
              >
                <Search className="size-4" aria-hidden="true" />
              </button>
              {!sidebarCollapsed && (
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
                    onOpenInWorkbench={() => openSessionInWorkbench(session.id)}
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
                  onOpenSessionInWorkbench={openSessionInWorkbench}
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
            aria-valuemin={sidebarMinimumWidth}
            aria-valuemax={sidebarMaximumWidth}
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
      </div>

      {skillsOpen ? (
        <SkillsPage
          onBack={() => setSkillsOpen(false)}
          sidebarCollapsed={sidebarCollapsed}
          onExpandSidebar={expandSidebar}
          workspacePath={activeSession?.workspacePath}
          workspaceName={activeSession?.workspaceName}
        />
      ) : (
      <div
        ref={workbenchLayoutRef}
        className={cn(
          'relative grid h-full min-h-0 min-w-0 grid-cols-1 grid-rows-[minmax(0,1fr)] overflow-hidden motion-reduce:transition-none [container-type:inline-size] md:grid-cols-[minmax(0,1fr)_minmax(0,var(--workbench-width))]',
          !workbenchResizing &&
            !workbenchAutoLayoutChanging &&
            'transition-[grid-template-columns] duration-[260ms] ease-[cubic-bezier(0.4,0,0.2,1)]',
        )}
        data-testid="workbench-layout"
        style={
          {
            '--workbench-expanded-width': workbenchExpandedWidth,
            '--workbench-width': workbenchOpen
              ? 'var(--workbench-expanded-width)'
              : '0px',
          } as CSSProperties
        }
      >
      <div
        className="relative flex h-full min-h-0 min-w-0 flex-col overflow-hidden"
        data-testid="conversation-pane"
        aria-hidden={workbenchMaximized}
        inert={workbenchMaximized}
      >
        <header
          className={cn(
            'conversation-header window-titlebar z-20 flex h-[45px] shrink-0 items-center gap-3 border-b border-stone-200/80 bg-white py-0 pr-2 pl-6 max-md:h-12 max-md:px-2 max-md:pl-4',
            sidebarCollapsed && 'sidebar-is-collapsed',
          )}
          data-testid="conversation-header"
        >
          {sidebarCollapsed && (
            <SidebarToggleButton
              expanded={false}
              className="desktop-sidebar-toggle hidden md:grid"
              onToggle={expandSidebar}
            />
          )}
          <div
            className="conversation-title-group flex min-w-0 flex-1 select-none items-center gap-2.5"
            data-testid="conversation-title"
          >
            <button
              className="window-titlebar-control -ml-1 grid size-7 shrink-0 place-items-center rounded-md text-stone-500 transition-colors hover:bg-stone-100 hover:text-stone-900 md:hidden"
              type="button"
              title={t('app.sessions')}
              onClick={openMobileSessions}
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
          {!workbenchOwnsToggle && workbenchToggleControl}
        </header>

        <main
          ref={logRef}
          data-testid="conversation-transcript"
          className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto px-3 md:px-6 md:[scrollbar-gutter:stable_both-edges]"
          onScroll={trackScrollPosition}
        >
          <div
            className={cn(
              'mx-auto min-h-full w-full max-w-[750px] pt-5 pb-9 max-md:pt-4 max-md:pb-7',
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
      <aside
        ref={workbenchViewportRef}
        className={cn(
          'relative min-h-0 min-w-0 overflow-visible bg-white transition-[visibility] duration-0 motion-reduce:delay-0',
          workbenchOpen
            ? workbenchMaximized
              ? 'visible absolute inset-0 z-40 delay-0'
              : 'visible absolute inset-0 z-40 delay-0 md:relative md:z-auto'
            : workbenchAutoLayoutChanging
              ? 'invisible hidden delay-0 md:block'
              : 'invisible hidden delay-[260ms] md:block',
        )}
        data-testid="workbench-viewport"
        aria-hidden={!workbenchOpen}
        inert={!workbenchOpen}
      >
        {workbenchOpen && !workbenchMaximized && (
          <div
            className="group absolute inset-y-0 -left-1.5 z-50 hidden w-1.5 touch-none cursor-col-resize outline-none md:block"
            data-testid="workbench-resize-handle"
            role="separator"
            aria-label={t('workbench.resize')}
            aria-orientation="vertical"
            aria-valuemin={workbenchResizeMinimum}
            aria-valuemax={workbenchResizeMaximum}
            aria-valuenow={workbenchResizeValue}
            tabIndex={0}
            onPointerDown={startWorkbenchResize}
            onPointerMove={resizeWorkbench}
            onPointerUp={stopWorkbenchResize}
            onPointerCancel={stopWorkbenchResize}
            onKeyDown={resizeWorkbenchWithKeyboard}
          >
            <span
              className={cn(
                'absolute inset-y-0 right-0 w-px bg-stone-300/80 transition-colors group-hover:bg-stone-500/70 group-focus-visible:bg-stone-500/80',
                workbenchResizing && 'bg-stone-500/80',
              )}
              data-testid="workbench-divider-line"
              aria-hidden="true"
            />
          </div>
        )}
        <div className="relative h-full min-h-0 min-w-0 overflow-hidden">
          <WorkbenchPanel
            open={workbenchOpen}
            preview={workbenchPreview}
            sessionID={workbenchPreviewOwnerID}
            activatePreview={activateWorkbenchPreview}
            conversation={secondaryThread}
            models={models}
            workspaces={workspaces}
            maximized={workbenchMaximized}
            creatingConversation={creating}
            creationError={workbenchCreateError}
            onCreateConversation={() => void createSessionInWorkbench()}
            onDismissCreationError={() => setWorkbenchCreateError('')}
            onCloseConversation={() => setSecondarySessionID(undefined)}
            onConfigureModel={() => {
              setSettingsSection('models')
              setSettingsOpen(true)
            }}
            onToggleMaximized={toggleWorkbenchMaximized}
            toggleControl={workbenchOwnsToggle ? workbenchToggleControl : undefined}
          />
        </div>
      </aside>
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
            await selectWorkspaceFolder(path)
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
  onOpenSessionInWorkbench,
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
  onOpenSessionInWorkbench: (id: string) => void
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
              className="size-4 shrink-0 text-stone-600"
              strokeWidth={1.8}
              aria-hidden="true"
            />
          ) : (
            <Folder
              className="size-4 shrink-0 text-stone-600"
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
                onOpenInWorkbench={() => onOpenSessionInWorkbench(session.id)}
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
  onOpenInWorkbench,
  onTogglePin,
  onDelete,
  onRename,
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
                onSelect={onOpenInWorkbench}
              >
                <PanelRightOpen className="size-4 text-stone-600" aria-hidden="true" />
                <span>{t('app.openInWorkbench')}</span>
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
          className="relative size-4 text-stone-700"
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
