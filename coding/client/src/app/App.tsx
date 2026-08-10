import {
  lazy,
  Suspense,
  useEffect,
  useState,
  type CSSProperties,
} from 'react'
import {
  LoaderCircle,
  PanelRight,
} from 'lucide-react'
import { useSession } from '@/features/session'
import type { SessionSummary } from '@/types'
import { cn } from '@/lib/utils'
import { Composer } from '@/features/composer'
import { chooseNativeDirectory } from '@/lib/desktop'
import type { SettingsSection } from '@/features/settings'
import {
  WorkbenchPanel,
  useWorkbenchLayout,
  type WorkbenchTaskRequest,
  type WorkbenchTaskSource,
} from '@/features/workbench'
import { AppSidebar } from './AppSidebar'
import {
  ConversationPane,
  useConversationScroll,
} from '@/features/conversation'
import {
  DeleteSessionDialog,
  RemoveWorkspaceDialog,
} from './SessionDialogs'
import { useI18n } from '@/i18n'
import { useSidebarLayout } from '@/useSidebarLayout'

const SettingsPage = lazy(() =>
  import('@/features/settings/SettingsPage').then((module) => ({ default: module.SettingsPage })),
)
const SkillsPage = lazy(() =>
  import('@/features/skills/SkillsPage').then((module) => ({ default: module.SkillsPage })),
)
const MCPPage = lazy(() =>
  import('@/features/mcp/MCPPage').then((module) => ({ default: module.MCPPage })),
)

type AppView =
  | { type: 'conversation' }
  | { type: 'settings'; section: SettingsSection }
  | { type: 'skills' }
  | { type: 'mcp' }

export default function App() {
  const { t } = useI18n()
  const [secondarySessionID, setSecondarySessionID] = useState<string>()
  const [workbenchTaskRequest, setWorkbenchTaskRequest] =
    useState<WorkbenchTaskRequest>()
  const {
    sessions,
    workspaces,
    draft,
    activeSession,
    activeSessionID,
    items,
    tasks,
    queuedMessages,
    contextUsage,
    preview,
    browserCommands,
    browserTabsRequests,
    browserInspections,
    previewOpen,
    approval,
    question,
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
    stopTask,
    readTaskOutput,
    resolveApproval,
    resolveQuestion,
    queueBrowserResult,
    handleBrowserTabs,
    handleBrowserInspection,
    secondaryThread,
  } = useSession(secondarySessionID)
  const {
    scrollRef: logRef,
    onScroll: trackScrollPosition,
    onWheelCapture: pauseFollowOnWheel,
    scrollToLatest,
    awayFromLatest,
    hasNewContent,
  } = useConversationScroll(activeSessionID, items)
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
  const [view, setView] = useState<AppView>({ type: 'conversation' })
  const [workspaceOpenError, setWorkspaceOpenError] = useState('')
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
    enabled: view.type === 'conversation',
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
  const workbenchBrowserSessionID =
    workbenchPreviewOwnerID ??
    workbenchTaskRequest?.sessionID ??
    secondaryThread?.session.id ??
    activeSessionID ??
    draft?.id
  const workbenchBrowserCommands =
    secondaryThread && workbenchBrowserSessionID === secondaryThread.session.id
      ? secondaryThread.browserCommands
      : workbenchBrowserSessionID === activeSessionID
        ? browserCommands
        : []
  const activateWorkbenchPreview = workbenchPreviewOwnerID === activeSessionID
    ? previewOpen
    : secondaryThread && workbenchPreviewOwnerID === secondaryThread.session.id
      ? secondaryThread.previewOpen
      : false
  const workbenchTaskSources: WorkbenchTaskSource[] = activeSessionID
    ? [{
        sessionID: activeSessionID,
        tasks,
        onStopTask: stopTask,
        onReadTaskOutput: readTaskOutput,
      }]
    : []
  if (secondaryThread && secondaryThread.session.id !== activeSessionID) {
    workbenchTaskSources.push({
      sessionID: secondaryThread.session.id,
      tasks: secondaryThread.tasks,
      onStopTask: secondaryThread.stopTask,
      onReadTaskOutput: secondaryThread.readTaskOutput,
    })
  }
  useEffect(() => {
    if (
      secondarySessionID &&
      !loading &&
      !sessions.some((session) => session.id === secondarySessionID)
    ) {
      setSecondarySessionID(undefined)
    }
    if (
      workbenchTaskRequest &&
      !loading &&
      !sessions.some((session) => session.id === workbenchTaskRequest.sessionID)
    ) {
      setWorkbenchTaskRequest(undefined)
    }
  }, [loading, secondarySessionID, sessions, workbenchTaskRequest])

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
        setView({ type: 'settings', section: 'general' })
      } else if (event.key === 'Escape' && view.type === 'settings') {
        setView({ type: 'conversation' })
      }
    }
    window.addEventListener('keydown', handleSettingsShortcut)
    return () => window.removeEventListener('keydown', handleSettingsShortcut)
  }, [view.type])

  const chooseSession = (id: string) => {
    const session = sessions.find((candidate) => candidate.id === id)
    if (session) setSelectedWorkspacePath(session.scope === 'project' ? session.workspacePath : undefined)
    selectSession(id)
    setView({ type: 'conversation' })
    setWorkbenchTaskRequest(undefined)
    closeMobileSessions()
  }

  const openSessionInWorkbench = (id: string) => {
    if (!sessions.some((session) => session.id === id)) return
    setWorkbenchCreateError('')
    setWorkbenchTaskRequest(undefined)
    setSecondarySessionID(id)
    showSessionInWorkbench(id)
    setView({ type: 'conversation' })
    closeMobileSessions()
  }

  const createSessionInWorkbench = async () => {
    setWorkbenchCreateError('')
    setWorkbenchTaskRequest(undefined)
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
    setView({ type: 'conversation' })
    setWorkbenchTaskRequest(undefined)
    closeMobileSessions()
  }

  const openTaskInWorkbench = (taskID: string) => {
    if (!activeSessionID) return
    setWorkbenchCreateError('')
    setSecondarySessionID(undefined)
    setWorkbenchTaskRequest((current) => ({
      sessionID: activeSessionID,
      taskID,
      revision: (current?.revision ?? 0) + 1,
    }))
    showSessionInWorkbench(activeSessionID)
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
      if (workbenchTaskRequest?.sessionID === deleteTarget.id) {
        setWorkbenchTaskRequest(undefined)
      }
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
    closeMobileSessions()
  }

  const browseWorkspaceFolders = async () => {
    setWorkspaceOpenError('')
    try {
      const path = await chooseNativeDirectory(workspacePickerPath, t('workspace.chooseFolder'))
      if (path === undefined) {
        setWorkspaceOpenError(t('workspace.openFailed'))
        return
      }
      if (!path) return
      await selectWorkspaceFolder(path)
    } catch (error) {
      setWorkspaceOpenError(
        error instanceof Error ? error.message : t('workspace.openFailed'),
      )
    }
  }

  const composer = (centered = false) => (
    <Composer
      key={draft?.id ?? activeSessionID ?? 'empty-session'}
      connected={status === 'ready' && !creating}
      running={running}
      approval={approval}
      question={question}
      queuedMessages={queuedMessages}
      contextUsage={contextUsage}
      centered={centered}
      projectPickerVisible={Boolean(draft)}
      workspaces={workspaces}
      workspacePath={draft?.projectScoped ? draft.workspacePath : undefined}
      workspaceError={workspaceOpenError}
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
      onResolveQuestion={resolveQuestion}
      onSelectProject={(path) => {
        setWorkspaceOpenError('')
        updateDraftWorkspace(path, Boolean(path))
        setSelectedWorkspacePath(path)
      }}
      onBrowseProjects={() => {
        void browseWorkspaceFolders()
      }}
      onConfigureModel={() => {
        setView({ type: 'settings', section: 'models' })
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
      className="window-titlebar-control relative grid size-7 shrink-0 cursor-pointer place-items-center rounded-md text-ink-muted outline-none transition-colors duration-100 hover:bg-canvas-strong/75 hover:text-ink focus-visible:bg-canvas-strong/75 focus-visible:text-ink"
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
          className="absolute top-0.5 right-0.5 size-1.5 rounded-full border border-canvas bg-info"
          aria-hidden="true"
        />
      )}
    </button>
  )

  if (view.type === 'settings') {
    return (
      <Suspense fallback={<AppViewFallback />}>
        <SettingsPage
          initialSection={view.section}
          onBack={() => setView({ type: 'conversation' })}
          onProvidersChanged={refreshModels}
        />
      </Suspense>
    )
  }

  return (
    <div
      className={cn(
        'relative grid h-full grid-cols-[var(--sidebar-width)_minmax(0,1fr)] grid-rows-[minmax(0,1fr)] overflow-hidden bg-canvas motion-reduce:transition-none max-md:grid-cols-1',
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
      <AppSidebar
        layout={{
          mobileSessionsOpen,
          sidebarCollapsed,
          sidebarWidth,
          sidebarResizing,
          sidebarMinimumWidth,
          sidebarMaximumWidth,
          pinnedSessionIDSet,
        }}
        content={{
          creating,
          activeSessionID,
          chatSessions,
          workspaceGroups,
        }}
        actions={{
          closeMobileSessions,
          toggleSidebar,
          addSession,
          onOpenSkills: () => {
            closeMobileSessions()
            setView({ type: 'skills' })
          },
          onOpenMCP: () => {
            closeMobileSessions()
            setView({ type: 'mcp' })
          },
          chooseSession,
          openSessionInWorkbench,
          togglePinnedSession,
          requestDelete,
          handleRename,
          onSelectWorkspace: setSelectedWorkspacePath,
          requestRemoveWorkspace,
          onOpenSettings: () => setView({ type: 'settings', section: 'general' }),
          startSidebarResize,
          resizeSidebar,
          stopSidebarResize,
          resizeSidebarWithKeyboard,
        }}
      />

      {view.type === 'skills' ? (
        <Suspense fallback={<AppViewFallback />}>
          <SkillsPage
            onBack={() => setView({ type: 'conversation' })}
            sidebarCollapsed={sidebarCollapsed}
            onExpandSidebar={expandSidebar}
            workspacePath={activeSession?.workspacePath}
            workspaceName={activeSession?.workspaceName}
          />
        </Suspense>
      ) : view.type === 'mcp' ? (
        <Suspense fallback={<AppViewFallback />}>
          <MCPPage
            onBack={() => setView({ type: 'conversation' })}
            sidebarCollapsed={sidebarCollapsed}
            onExpandSidebar={expandSidebar}
            workspacePath={activeSession?.workspacePath}
            workspaceName={activeSession?.workspaceName}
          />
        </Suspense>
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
      <ConversationPane
        thread={{
          draft,
          activeSession,
          tasks,
          items,
          approval,
          running,
          autoCompacting,
          loading,
        }}
        layout={{
          sidebarCollapsed,
          workbenchMaximized,
          workbenchOwnsToggle,
          workbenchToggleControl,
          awayFromLatest,
          hasNewContent,
        }}
        scroll={{
          logRef,
          trackScrollPosition,
          pauseFollowOnWheel,
          scrollToLatest,
        }}
        actions={{
          expandSidebar,
          openMobileSessions,
          openTaskInWorkbench,
          renderComposer: composer,
        }}
      />
      <aside
        ref={workbenchViewportRef}
        className={cn(
          'relative min-h-0 min-w-0 overflow-visible bg-canvas transition-[visibility] duration-0 motion-reduce:delay-0',
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
            onLostPointerCapture={stopWorkbenchResize}
            onKeyDown={resizeWorkbenchWithKeyboard}
          >
            <span
              className={cn(
                'absolute inset-y-0 right-0 w-px bg-ink-ghost/80 transition-colors group-hover:bg-ink-muted/70 group-focus-visible:bg-ink-muted/80',
                workbenchResizing && 'bg-ink-muted/80',
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
            browserCommands={workbenchBrowserCommands}
            sessionID={workbenchBrowserSessionID}
            activatePreview={activateWorkbenchPreview}
            conversation={secondaryThread}
            taskRequest={workbenchTaskRequest}
            taskSources={workbenchTaskSources}
            models={models}
            workspaces={workspaces}
            maximized={workbenchMaximized}
            creatingConversation={creating}
            creationError={workbenchCreateError}
            onCreateConversation={() => void createSessionInWorkbench()}
            onDismissCreationError={() => setWorkbenchCreateError('')}
            onCloseConversation={() => setSecondarySessionID(undefined)}
            onBrowserResult={queueBrowserResult}
            browserInspectionSources={[
              {
                sessionID: activeSessionID,
                browserCommands,
                browserTabsRequests,
                browserInspections,
              },
              {
                sessionID: secondaryThread?.session.id,
                browserCommands: secondaryThread?.browserCommands ?? [],
                browserTabsRequests: secondaryThread?.browserTabsRequests ?? [],
                browserInspections: secondaryThread?.browserInspections ?? [],
              },
            ]}
            onBrowserTabsHandled={handleBrowserTabs}
            onBrowserInspectionHandled={handleBrowserInspection}
            onConfigureModel={() => {
              setView({ type: 'settings', section: 'models' })
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
    </div>
  )
}

function AppViewFallback() {
  return (
    <main className="grid h-full min-h-0 min-w-0 place-items-center bg-canvas text-ink-faint">
      <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
    </main>
  )
}
