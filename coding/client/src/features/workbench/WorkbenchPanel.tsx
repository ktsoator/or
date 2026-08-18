import { useEffect, useState, type ReactNode } from 'react'
import { PanelTopDashed } from 'lucide-react'
import type {
  BrowserCommandState,
  BrowserInspectionCommandState,
  BrowserResult,
  BrowserTabsCommandState,
  ModelOption,
  PreviewState,
  WorkspaceSummary,
} from '@/types'
import {
  useBrowserInspectionRequests,
  useBrowserTabsRequests,
  useBrowserWorkspace,
  type BrowserWorkspaceState,
  type WorkbenchTaskRequest,
} from '@/features/browser'
import { cn } from '@/lib/utils'
import { WorkbenchView, WorkbenchHeaderActions } from './WorkbenchView'
import type { WorkbenchTaskSource } from './BackgroundTasksView'
import type { WorkbenchConversation } from './conversations'
import { useI18n } from '@/i18n'

type WorkbenchMode = 'launcher' | 'views'

export type BrowserInspectionSource = {
  sessionID?: string
  browserCommands: BrowserCommandState[]
  browserTabsRequests: BrowserTabsCommandState[]
  browserInspections: BrowserInspectionCommandState[]
}

export function WorkbenchPanel({
  open,
  preview,
  browserCommands,
  sessionID,
  activatePreview,
  conversations,
  activeConversationID,
  taskRequest,
  taskSources,
  models,
  workspaces,
  maximized,
  creatingConversation,
  onCreateConversation,
  onSelectConversation,
  onCloseConversation,
  onBrowserResult,
  browserInspectionSources,
  onBrowserTabsHandled,
  onBrowserInspectionHandled,
  onConfigureModel,
  onToggleMaximized,
  toggleControl,
}: {
  open: boolean
  preview?: PreviewState
  browserCommands: BrowserCommandState[]
  sessionID?: string
  activatePreview: boolean
  conversations: WorkbenchConversation[]
  activeConversationID?: string
  taskRequest?: WorkbenchTaskRequest
  taskSources: WorkbenchTaskSource[]
  models: ModelOption[]
  workspaces: WorkspaceSummary[]
  maximized: boolean
  creatingConversation: boolean
  onCreateConversation: () => void
  onSelectConversation: (conversationID: string) => void
  onCloseConversation: (conversationID: string) => void
  onBrowserResult: (sessionID: string, commandID: string, result: BrowserResult) => void
  browserInspectionSources: BrowserInspectionSource[]
  onBrowserTabsHandled: (sessionID: string, commandID: string) => void
  onBrowserInspectionHandled: (sessionID: string, commandID: string) => void
  onConfigureModel: () => void
  onToggleMaximized: () => void
  toggleControl?: ReactNode
}) {
  const { t } = useI18n()
  const activeConversation = conversations.find(
    (conversation) => conversation.id === activeConversationID,
  )
  const workspaceSessionID = sessionID ??
    (activeConversation?.kind === 'session'
      ? activeConversation.thread.session.id
      : activeConversation?.id)
  const [mode, setMode] = useState<WorkbenchMode>(
    preview || conversations.length > 0 || taskRequest ? 'views' : 'launcher',
  )
  const browserWorkspace = useBrowserWorkspace({
    activatePreview,
    browserCommands,
    activeConversationID,
    conversationIDs: conversations.map((conversation) => conversation.id),
    onBrowserResult,
    onSelectConversation,
    preview,
    sessionID: workspaceSessionID,
    taskRequest,
  })
  const taskSource = taskSources.find(
    (source) => source.sessionID === browserWorkspace.workspaceID,
  )

  useEffect(() => {
    if (preview || conversations.length > 0 || taskRequest) setMode('views')
  }, [conversations.length, preview, taskRequest])

  return (
    <section
      className={cn(
        'relative h-full min-h-0 bg-canvas transition-opacity duration-[220ms] ease-[cubic-bezier(0.4,0,0.2,1)] motion-reduce:transition-none [contain:layout_paint] md:absolute md:inset-y-0 md:right-0',
        maximized ? 'md:w-full' : 'md:w-[var(--workbench-expanded-width)]',
        open
          ? 'opacity-100 delay-[40ms]'
          : 'opacity-0 delay-0',
      )}
      data-testid="workbench-panel"
      aria-label={t('workbench.title')}
    >
      {browserInspectionSources.map((source, index) => (
        <BrowserInspectionWorker
          key={`${source.sessionID ?? 'no-session'}:${index}`}
          source={source}
          workspace={browserWorkspace.workspaceForSession(source.sessionID)}
          runtimeWorkspaceID={browserWorkspace.runtimeWorkspaceID}
          attachControl={browserWorkspace.attachControl}
          releaseControl={browserWorkspace.releaseControl}
          onBrowserTabsHandled={onBrowserTabsHandled}
          onInspectionHandled={onBrowserInspectionHandled}
        />
      ))}
      {mode === 'views' ? (
        <WorkbenchView
          workspace={browserWorkspace}
          conversations={conversations}
          taskSource={taskSource}
          creatingConversation={creatingConversation}
          models={models}
          workspaces={workspaces}
          onCloseTab={() => setMode('launcher')}
          onCloseConversation={(conversationID) => {
            onCloseConversation(conversationID)
            if (
              conversations.length === 1 &&
              browserWorkspace.tabs.length === 0 &&
              !browserWorkspace.taskTabID
            ) {
              setMode('launcher')
            }
          }}
          onConfigureModel={onConfigureModel}
          onCreateConversation={onCreateConversation}
          maximized={maximized}
          onToggleMaximized={onToggleMaximized}
          toggleControl={toggleControl}
        />
      ) : (
        <WorkbenchLauncher
          maximized={maximized}
          creatingConversation={creatingConversation}
          onCreateConversation={onCreateConversation}
          onToggleMaximized={onToggleMaximized}
          onOpenBrowser={() => {
            if (browserWorkspace.tabs.length === 0) browserWorkspace.newTab()
            setMode('views')
          }}
          onOpenTasks={taskSource
            ? () => {
                browserWorkspace.openTasks()
                setMode('views')
              }
            : undefined}
          toggleControl={toggleControl}
        />
      )}
    </section>
  )
}

function BrowserInspectionWorker({
  workspace,
  runtimeWorkspaceID,
  attachControl,
  releaseControl,
  onBrowserTabsHandled,
  onInspectionHandled,
  source,
}: {
  workspace: BrowserWorkspaceState | undefined
  runtimeWorkspaceID: string
  attachControl: ReturnType<typeof useBrowserWorkspace>['attachControl']
  releaseControl: ReturnType<typeof useBrowserWorkspace>['releaseControl']
  onBrowserTabsHandled: (sessionID: string, commandID: string) => void
  onInspectionHandled: (sessionID: string, commandID: string) => void
  source: BrowserInspectionSource
}) {
  useBrowserTabsRequests({
    workspace,
    sessionID: source.sessionID,
    requests: source.browserTabsRequests,
    onHandled: onBrowserTabsHandled,
  })
  useBrowserInspectionRequests({
    workspace,
    runtimeWorkspaceID,
    sessionID: source.sessionID,
    browserCommands: source.browserCommands,
    browserInspections: source.browserInspections,
    attachControl,
    releaseControl,
    onHandled: onInspectionHandled,
  })
  return null
}

function WorkbenchLauncher({
  maximized,
  creatingConversation,
  onCreateConversation,
  onToggleMaximized,
  onOpenBrowser,
  onOpenTasks,
  toggleControl,
}: {
  maximized: boolean
  creatingConversation: boolean
  onCreateConversation: () => void
  onToggleMaximized: () => void
  onOpenBrowser: () => void
  onOpenTasks?: () => void
  toggleControl?: ReactNode
}) {
  const { t } = useI18n()

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div
        className="window-titlebar flex h-[45px] shrink-0 items-center justify-end bg-canvas px-2"
        data-testid="workbench-titlebar"
      >
        <WorkbenchHeaderActions
          maximized={maximized}
          creatingConversation={creatingConversation}
          onCreateConversation={onCreateConversation}
          onToggleMaximized={onToggleMaximized}
          onOpenBrowser={onOpenBrowser}
          onOpenTasks={onOpenTasks}
          toggleControl={toggleControl}
        />
      </div>
      <div className="grid min-h-0 flex-1 place-items-center px-8 pb-[5vh]">
        <div
          className="flex flex-col items-center gap-2 text-ink-faint"
          data-testid="workbench-empty"
        >
          <PanelTopDashed className="size-5 text-ink-ghost" aria-hidden="true" />
          <span className="text-[0.8125rem] leading-5">{t('workbench.empty')}</span>
        </div>
      </div>
    </div>
  )
}
