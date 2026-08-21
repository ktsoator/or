import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
  useRef,
  useState,
  type CSSProperties,
} from 'react'
import {
  LoaderCircle,
  PanelRight,
} from 'lucide-react'
import { useSession, type SessionDraft } from '@/features/session'
import type { MessageImage, PromptFile, SessionSummary } from '@/types'
import { cn } from '@/lib/utils'
import { Composer } from '@/features/composer'
import { chooseNativeDirectory, revealNativePath } from '@/lib/desktop'
import type { SettingsSection } from '@/features/settings'
import {
  WorkbenchPanel,
  useWorkbenchLayout,
  type WorkbenchTaskRequest,
  type WorkbenchTaskSource,
  type WorkbenchConversation,
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
import { useSidebarLayout } from './useSidebarLayout'
import type { DiagnosticsSessionState } from '@/features/diagnostics/DiagnosticsPage'

const SettingsPage = lazy(() =>
  import('@/features/settings/SettingsPage').then((module) => ({ default: module.SettingsPage })),
)
const SkillsPage = lazy(() =>
  import('@/features/skills/SkillsPage').then((module) => ({ default: module.SkillsPage })),
)
const MCPPage = lazy(() =>
  import('@/features/mcp/MCPPage').then((module) => ({ default: module.MCPPage })),
)
const DiagnosticsPage = lazy(() =>
  import('@/features/diagnostics/DiagnosticsPage').then((module) => ({ default: module.DiagnosticsPage })),
)

type AppView =
  | { type: 'conversation' }
  | { type: 'settings'; section: SettingsSection }
  | { type: 'skills' }
  | { type: 'mcp' }

type ConversationSurface = 'conversation' | 'diagnostics'

type BranchPointTarget = {
  sessionID: string
  messageID: string
}

type WorkbenchConversationTab =
  | { id: string; kind: 'draft'; draft: SessionDraft }
  | { id: string; kind: 'session' }

export default function App() {
  const { t } = useI18n()
  const [workbenchConversationTabs, setWorkbenchConversationTabs] =
    useState<WorkbenchConversationTab[]>([])
  const [activeWorkbenchConversationID, setActiveWorkbenchConversationID] =
    useState<string>()
  const [creatingWorkbenchDraftIDs, setCreatingWorkbenchDraftIDs] =
    useState<ReadonlySet<string>>(() => new Set())
  const secondarySessionIDs = workbenchConversationTabs.flatMap((conversation) =>
    conversation.kind === 'session' ? [conversation.id] : [],
  )
  const [branchPointTarget, setBranchPointTarget] = useState<BranchPointTarget>()
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
    todos,
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
    forking,
    updatingSettings,
    compacting,
    status,
    models,
    refreshModels,
    registerWorkspace,
    removeWorkspace,
    startDraft,
    createChatDraft,
    createChatSession,
    draftReady,
    updateDraftWorkspace,
    deleteSession,
    renameSession,
    forkMessage,
    editMessage,
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
    secondaryThreads,
  } = useSession(secondarySessionIDs)
  const {
    scrollRef: logRef,
    onScroll: trackScrollPosition,
    onWheelCapture: pauseFollowOnWheel,
    scrollToLatest,
    awayFromLatest,
    hasNewContent,
  } = useConversationScroll(activeSessionID, items)
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
  const [conversationSurfaces, setConversationSurfaces] = useState<
    Record<string, ConversationSurface>
  >({})
  const diagnosticsStatesRef = useRef<Record<string, DiagnosticsSessionState>>({})
  const [workspaceOpenError, setWorkspaceOpenError] = useState('')
  const [selectedWorkspacePath, setSelectedWorkspacePath] = useState<string>()
  const activeSecondaryThread = secondaryThreads.find(
    (thread) => thread.session.id === activeWorkbenchConversationID,
  )
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
    secondarySessionID: activeSecondaryThread?.session.id,
    secondaryPreviewRevision: activeSecondaryThread?.preview?.revision,
    secondaryPreviewOpen: activeSecondaryThread?.previewOpen ?? false,
  })

  const workspacePickerPath =
    selectedWorkspacePath || draft?.workspacePath || activeSession?.workspacePath || workspaceGroups[0]?.path
  const parentSession = activeSession?.forkedFromSessionId
    ? sessions.find((session) => session.id === activeSession.forkedFromSessionId)
    : undefined
  const branchSessions = activeSession
    ? sessions.filter((session) => session.forkedFromSessionId === activeSession.id)
    : []
  const previewSecondaryThread = secondaryThreads.find(
    (thread) => thread.session.id === workbenchPreviewSessionID,
  )
  const workbenchPreview =
    previewSecondaryThread
      ? previewSecondaryThread.preview
      : !workbenchPreviewSessionID || workbenchPreviewSessionID === activeSessionID
        ? preview
        : undefined
  const workbenchPreviewOwnerID = workbenchPreview
    ? workbenchPreviewSessionID ?? activeSessionID
    : undefined
  const workbenchBrowserSessionID =
    workbenchPreviewOwnerID ??
    workbenchTaskRequest?.sessionID ??
    activeSecondaryThread?.session.id ??
    activeSessionID ??
    draft?.id
  const workbenchBrowserCommands =
    secondaryThreads.find(
      (thread) => thread.session.id === workbenchBrowserSessionID,
    )?.browserCommands ??
    (workbenchBrowserSessionID === activeSessionID
      ? browserCommands
      : [])
  const activateWorkbenchPreview = workbenchPreviewOwnerID === activeSessionID
    ? previewOpen
    : previewSecondaryThread
      ? previewSecondaryThread.previewOpen
      : false
  const workbenchTaskSources: WorkbenchTaskSource[] = activeSessionID
    ? [{
        sessionID: activeSessionID,
        tasks,
        onStopTask: stopTask,
        onReadTaskOutput: readTaskOutput,
      }]
    : []
  for (const secondaryThread of secondaryThreads) {
    if (secondaryThread.session.id === activeSessionID) continue
    workbenchTaskSources.push({
      sessionID: secondaryThread.session.id,
      tasks: secondaryThread.tasks,
      onStopTask: secondaryThread.stopTask,
      onReadTaskOutput: secondaryThread.readTaskOutput,
    })
  }
  useEffect(() => {
    if (
      workbenchTaskRequest &&
      !loading &&
      !sessions.some((session) => session.id === workbenchTaskRequest.sessionID)
    ) {
      setWorkbenchTaskRequest(undefined)
    }
  }, [loading, sessions, workbenchTaskRequest])

  useEffect(() => {
    if (loading) return
    const invalidSessionIDs = new Set(
      workbenchConversationTabs.flatMap((conversation) =>
        conversation.kind === 'session' &&
        !sessions.some((session) => session.id === conversation.id)
          ? [conversation.id]
          : [],
      ),
    )
    if (invalidSessionIDs.size === 0) return
    setWorkbenchConversationTabs((current) =>
      current.filter((conversation) => !invalidSessionIDs.has(conversation.id)),
    )
    if (
      activeWorkbenchConversationID &&
      invalidSessionIDs.has(activeWorkbenchConversationID)
    ) {
      setActiveWorkbenchConversationID(undefined)
    }
  }, [
    activeWorkbenchConversationID,
    loading,
    sessions,
    workbenchConversationTabs,
  ])

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

  const chooseSession = (id: string, messageID?: string) => {
    const session = sessions.find((candidate) => candidate.id === id)
    if (session) setSelectedWorkspacePath(session.scope === 'project' ? session.workspacePath : undefined)
    setBranchPointTarget(messageID ? { sessionID: id, messageID } : undefined)
    selectSession(id)
    setView({ type: 'conversation' })
    setWorkbenchTaskRequest(undefined)
    closeMobileSessions()
  }

  const handleBranchPointLocated = useCallback((target: BranchPointTarget) => {
    setBranchPointTarget((current) =>
      current?.sessionID === target.sessionID && current.messageID === target.messageID
        ? undefined
        : current,
    )
  }, [])

  const openSessionInWorkbench = (id: string) => {
    if (!sessions.some((session) => session.id === id)) return
    setWorkbenchTaskRequest(undefined)
    setWorkbenchConversationTabs((current) =>
      current.some((conversation) => conversation.id === id)
        ? current
        : [...current, { id, kind: 'session' }],
    )
    setActiveWorkbenchConversationID(id)
    showSessionInWorkbench(id)
    setView({ type: 'conversation' })
    closeMobileSessions()
  }

  const createSessionInWorkbench = () => {
    setWorkbenchTaskRequest(undefined)
    const nextDraft = createChatDraft()
    setWorkbenchConversationTabs((current) => [
      ...current,
      { id: nextDraft.id, kind: 'draft', draft: nextDraft },
    ])
    setActiveWorkbenchConversationID(nextDraft.id)
    showSessionInWorkbench(nextDraft.id)
  }

  const selectWorkbenchConversation = (conversationID: string) => {
    setActiveWorkbenchConversationID(conversationID)
  }

  const closeWorkbenchConversation = (conversationID: string) => {
    const closingIndex = workbenchConversationTabs.findIndex(
      (conversation) => conversation.id === conversationID,
    )
    if (closingIndex < 0) return
    const next = workbenchConversationTabs.filter(
      (conversation) => conversation.id !== conversationID,
    )
    setWorkbenchConversationTabs(next)
    if (activeWorkbenchConversationID === conversationID) {
      setActiveWorkbenchConversationID(
        next[Math.min(closingIndex, next.length - 1)]?.id,
      )
    }
  }

  const updateWorkbenchDraft = (draft: SessionDraft) => {
    setWorkbenchConversationTabs((current) =>
      current.map((conversation) =>
        conversation.kind === 'draft' && conversation.id === draft.id
          ? { ...conversation, draft }
          : conversation,
      ),
    )
  }

  const sendWorkbenchDraft = async (
    draft: SessionDraft,
    text: string,
    images: MessageImage[],
    files: PromptFile[],
  ): Promise<boolean> => {
    setCreatingWorkbenchDraftIDs((current) => new Set(current).add(draft.id))
    try {
      const created = await createChatSession(draft, { text, images, files })
      setWorkbenchConversationTabs((current) =>
        current.map((conversation) =>
          conversation.kind === 'draft' && conversation.id === draft.id
            ? { id: created.id, kind: 'session' }
            : conversation,
        ),
      )
      setActiveWorkbenchConversationID((current) =>
        current === draft.id ? created.id : current,
      )
      return true
    } finally {
      setCreatingWorkbenchDraftIDs((current) => {
        const next = new Set(current)
        next.delete(draft.id)
        return next
      })
    }
  }

  const workbenchConversations: WorkbenchConversation[] =
    workbenchConversationTabs.flatMap<WorkbenchConversation>((conversation) => {
      if (conversation.kind === 'session') {
        const thread = secondaryThreads.find(
          (candidate) => candidate.session.id === conversation.id,
        )
        return thread
          ? [{ id: conversation.id, kind: 'session' as const, thread }]
          : []
      }
      return [{
        id: conversation.id,
        kind: 'draft' as const,
        draft: conversation.draft,
        connected: draftReady,
        creating: creatingWorkbenchDraftIDs.has(conversation.id),
        onChange: updateWorkbenchDraft,
        onSend: (text: string, images: MessageImage[], files: PromptFile[]) =>
          sendWorkbenchDraft(conversation.draft, text, images, files),
      }]
    })

  const addSession = (workspacePath?: string, projectScoped = false) => {
    setSelectedWorkspacePath(projectScoped ? workspacePath : undefined)
    startDraft(workspacePath, projectScoped)
    setView({ type: 'conversation' })
    setWorkbenchTaskRequest(undefined)
    closeMobileSessions()
  }

  const openTaskInWorkbench = (taskID: string) => {
    if (!activeSessionID) return
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
      delete diagnosticsStatesRef.current[deleteTarget.id]
      setConversationSurfaces((current) => {
        if (!(deleteTarget.id in current)) return current
        const next = { ...current }
        delete next[deleteTarget.id]
        return next
      })
      closeWorkbenchConversation(deleteTarget.id)
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
      todos={todos}
      contextUsage={contextUsage}
      centered={centered}
      projectPickerVisible={Boolean(draft)}
      workspaces={workspaces}
      workspacePath={draft
        ? draft.projectScoped
          ? draft.workspacePath
          : undefined
        : activeSession?.workspacePath}
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

  const diagnosticsOpen = Boolean(
    activeSessionID && conversationSurfaces[activeSessionID] === 'diagnostics',
  )

  const toggleSessionDiagnostics = () => {
    if (!activeSessionID) return
    setConversationSurfaces((current) => ({
      ...current,
      [activeSessionID]: current[activeSessionID] === 'diagnostics'
        ? 'conversation'
        : 'diagnostics',
    }))
  }

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
          revealWorkspace: revealNativePath,
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
          parentSession,
          branchSessions,
          branchPointTarget,
          tasks,
          items,
          approval,
          running,
          autoCompacting,
          loading,
          forking,
        }}
        layout={{
          sidebarCollapsed,
          workbenchMaximized,
          workbenchOwnsToggle,
          workbenchToggleControl,
          awayFromLatest,
          hasNewContent,
          diagnosticsOpen,
          diagnosticsContent: diagnosticsOpen && activeSessionID ? (
            <Suspense fallback={<AppViewFallback />}>
              <DiagnosticsPage
                key={activeSessionID}
                embedded
                sessionID={activeSessionID}
                liveItems={items}
                running={running}
                initialState={diagnosticsStatesRef.current[activeSessionID]}
                onStateChange={(patch) => {
                  diagnosticsStatesRef.current[activeSessionID] = {
                    ...diagnosticsStatesRef.current[activeSessionID],
                    ...patch,
                  }
                }}
              />
            </Suspense>
          ) : null,
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
          selectSession: chooseSession,
          returnToParent: parentSession
            ? () => chooseSession(parentSession.id, activeSession?.forkedFromMessageId)
            : undefined,
          branchPointLocated: handleBranchPointLocated,
          openSessionDiagnostics: toggleSessionDiagnostics,
          forkMessage,
          editMessage,
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
            conversations={workbenchConversations}
            activeConversationID={activeWorkbenchConversationID}
            taskRequest={workbenchTaskRequest}
            taskSources={workbenchTaskSources}
            models={models}
            workspaces={workspaces}
            maximized={workbenchMaximized}
            creatingConversation={creating}
            onCreateConversation={createSessionInWorkbench}
            onSelectConversation={selectWorkbenchConversation}
            onCloseConversation={closeWorkbenchConversation}
            onBrowserResult={queueBrowserResult}
            browserInspectionSources={[
              {
                sessionID: activeSessionID,
                browserCommands,
                browserTabsRequests,
                browserInspections,
              },
              ...secondaryThreads.flatMap((secondaryThread) =>
                secondaryThread.session.id === activeSessionID
                  ? []
                  : [{
                      sessionID: secondaryThread.session.id,
                      browserCommands: secondaryThread.browserCommands,
                      browserTabsRequests: secondaryThread.browserTabsRequests,
                      browserInspections: secondaryThread.browserInspections,
                    }],
              ),
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
          branchCount={sessions.filter(
            (session) => session.forkedFromSessionId === deleteTarget.id,
          ).length}
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
