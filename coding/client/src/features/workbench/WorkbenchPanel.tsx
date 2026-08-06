import { useEffect, useState, type ReactNode } from 'react'
import { CircleAlert, PanelTopDashed, X } from 'lucide-react'
import type {
  BrowserCommandState,
  BrowserInspectionCommandState,
  BrowserResult,
  BrowserTabsCommandState,
  ModelOption,
  PreviewState,
  WorkspaceSummary,
} from '@/types'
import type { SessionThread } from '@/features/session'
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
  conversation,
  taskRequest,
  taskSources,
  models,
  workspaces,
  maximized,
  creatingConversation,
  creationError,
  onCreateConversation,
  onDismissCreationError,
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
  conversation?: SessionThread
  taskRequest?: WorkbenchTaskRequest
  taskSources: WorkbenchTaskSource[]
  models: ModelOption[]
  workspaces: WorkspaceSummary[]
  maximized: boolean
  creatingConversation: boolean
  creationError: string
  onCreateConversation: () => void
  onDismissCreationError: () => void
  onCloseConversation: () => void
  onBrowserResult: (sessionID: string, commandID: string, result: BrowserResult) => void
  browserInspectionSources: BrowserInspectionSource[]
  onBrowserTabsHandled: (sessionID: string, commandID: string) => void
  onBrowserInspectionHandled: (sessionID: string, commandID: string) => void
  onConfigureModel: () => void
  onToggleMaximized: () => void
  toggleControl?: ReactNode
}) {
  const { t } = useI18n()
  const workspaceSessionID = sessionID ?? conversation?.session.id
  const workspaceConversation =
    conversation?.session.id === workspaceSessionID ? conversation : undefined
  const [mode, setMode] = useState<WorkbenchMode>(
    preview || workspaceConversation || taskRequest ? 'views' : 'launcher',
  )
  const browserWorkspace = useBrowserWorkspace({
    activatePreview,
    browserCommands,
    conversationID: workspaceConversation?.session.id,
    onBrowserResult,
    preview,
    sessionID: workspaceSessionID,
    taskRequest,
  })
  const taskSource = taskSources.find(
    (source) => source.sessionID === browserWorkspace.workspaceID,
  )

  useEffect(() => {
    if (preview || workspaceConversation || taskRequest) setMode('views')
  }, [preview, taskRequest, workspaceConversation])

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
          attachControl={browserWorkspace.attachControl}
          releaseControl={browserWorkspace.releaseControl}
          onBrowserTabsHandled={onBrowserTabsHandled}
          onInspectionHandled={onBrowserInspectionHandled}
        />
      ))}
      {mode === 'views' ? (
        <WorkbenchView
          workspace={browserWorkspace}
          conversation={workspaceConversation}
          taskSource={taskSource}
          creatingConversation={creatingConversation}
          models={models}
          workspaces={workspaces}
          onCloseTab={() => setMode('launcher')}
          onCloseConversation={() => {
            onCloseConversation()
            if (browserWorkspace.tabs.length === 0 && !browserWorkspace.taskTabID) {
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
      {creationError && (
        <div
          className="absolute inset-x-3 top-[49px] z-[80] flex min-h-9 items-center gap-2 rounded-lg border border-danger-edge/80 bg-canvas px-2.5 py-2 text-xs leading-4 text-danger shadow-[0_10px_28px_-18px_rgba(127,29,29,0.45)]"
          role="alert"
        >
          <CircleAlert className="size-3.5 shrink-0" aria-hidden="true" />
          <span className="min-w-0 flex-1 truncate" title={creationError}>
            {creationError}
          </span>
          <button
            className="grid size-5 shrink-0 cursor-pointer place-items-center rounded text-danger-soft outline-none hover:bg-danger-surface hover:text-danger focus-visible:bg-danger-surface focus-visible:text-danger"
            type="button"
            title={t('workbench.dismissError')}
            aria-label={t('workbench.dismissError')}
            onClick={onDismissCreationError}
          >
            <X className="size-3.5" aria-hidden="true" />
          </button>
        </div>
      )}
    </section>
  )
}

function BrowserInspectionWorker({
  workspace,
  attachControl,
  releaseControl,
  onBrowserTabsHandled,
  onInspectionHandled,
  source,
}: {
  workspace: BrowserWorkspaceState | undefined
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
