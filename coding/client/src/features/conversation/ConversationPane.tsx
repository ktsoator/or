import type {
  RefObject,
  ReactNode,
  UIEventHandler,
  WheelEventHandler,
} from 'react'
import { LoaderCircle, PanelLeft } from 'lucide-react'
import type {
  ApprovalItem,
  BackgroundTask,
  Item,
  SessionSummary,
} from '@/types'
import type { SessionDraft } from '@/features/session'
import { useI18n } from '@/i18n'
import { cn } from '@/lib/utils'
import { groupItems } from './groupItems'
import { ConversationActionsMenu } from './ConversationActionsMenu'
import { ScrollToLatestButton } from './ConversationScrollControl'
import {
  AutoCompactionStatus,
  AwaitingResponse,
  ThreadItem,
} from './ConversationThread'
import { SidebarToggleButton } from '@/components/SidebarToggleButton'
import { StepGroup } from './StepGroup'
import { ConversationBranchNavigation } from './ConversationBranchNavigation'

type ConversationPaneProps = {
  thread: {
    draft?: SessionDraft
    activeSession?: SessionSummary
    parentSession?: SessionSummary
    branchSessions: SessionSummary[]
    tasks: BackgroundTask[]
    items: Item[]
    approval?: ApprovalItem
    running: boolean
    autoCompacting: boolean
    loading: boolean
    forking: boolean
  }
  layout: {
    sidebarCollapsed: boolean
    workbenchMaximized: boolean
    workbenchOwnsToggle: boolean
    workbenchToggleControl: ReactNode
    awayFromLatest: boolean
    hasNewContent: boolean
  }
  scroll: {
    logRef: RefObject<HTMLDivElement | null>
    trackScrollPosition: UIEventHandler<HTMLDivElement>
    pauseFollowOnWheel: WheelEventHandler<HTMLDivElement>
    scrollToLatest: () => void
  }
  actions: {
    expandSidebar: () => void
    openMobileSessions: () => void
    openTaskInWorkbench: (taskID: string) => void
    selectSession: (sessionID: string) => void
    forkMessage: (
      messageID: string,
      mode: 'before_user' | 'after_assistant',
      text?: string,
    ) => Promise<SessionSummary>
    renderComposer: (centered?: boolean) => ReactNode
  }
}

export function ConversationPane({
  thread,
  layout,
  scroll,
  actions,
}: ConversationPaneProps) {
  const { t } = useI18n()
  const {
    draft,
    activeSession,
    parentSession,
    branchSessions,
    tasks,
    items,
    approval,
    running,
    autoCompacting,
    loading,
    forking,
  } = thread
  const {
    sidebarCollapsed,
    workbenchMaximized,
    workbenchOwnsToggle,
    workbenchToggleControl,
    awayFromLatest,
    hasNewContent,
  } = layout
  const {
    logRef,
    trackScrollPosition,
    pauseFollowOnWheel,
    scrollToLatest,
  } = scroll
  const {
    expandSidebar,
    openMobileSessions,
    openTaskInWorkbench,
    selectSession,
    forkMessage,
    renderComposer,
  } = actions
  const emptySession = !loading && items.length === 0 && !approval
  const awaitingFirstOutput = running && items.at(-1)?.kind === 'user'

  return (
      <div
        className="relative flex h-full min-h-0 min-w-0 flex-col overflow-hidden"
        data-testid="conversation-pane"
        aria-hidden={workbenchMaximized}
        inert={workbenchMaximized}
      >
        <header
          className={cn(
            'conversation-header window-titlebar z-20 flex h-[45px] shrink-0 items-center gap-3 border-b border-edge/80 bg-canvas py-0 pr-2 pl-6 max-md:h-12 max-md:px-2 max-md:pl-4',
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
              className="window-titlebar-control -ml-1 grid size-7 shrink-0 place-items-center rounded-md text-ink-muted transition-colors hover:bg-canvas-sunken hover:text-ink md:hidden"
              type="button"
              title={t('app.sessions')}
              onClick={openMobileSessions}
            >
              <PanelLeft className="size-4" aria-hidden="true" />
              <span className="sr-only">{t('app.openSessions')}</span>
            </button>
            <div className="flex min-w-0 flex-1 items-center gap-1.5">
              {!draft && activeSession?.scope === 'project' && activeSession.workspaceName && (
                <>
                  <span
                    className="shrink-0 text-[0.9375rem] text-ink-faint max-sm:hidden"
                    title={activeSession.workspacePath}
                  >
                    {activeSession.workspaceName}
                  </span>
                  <span className="shrink-0 text-ink-ghost max-sm:hidden" aria-hidden="true">
                    /
                  </span>
                </>
              )}
              <span
                className="truncate text-[0.9375rem] font-medium tracking-[-0.015em] text-ink"
                title={activeSession?.title}
              >
                {draft || activeSession?.title === 'New session'
                  ? t('app.newSession')
                  : (activeSession?.title ?? 'Or')}
              </span>
            </div>
            {!draft && activeSession && (parentSession || branchSessions.length > 0) && (
              <ConversationBranchNavigation
                key={activeSession.id}
                parentSession={parentSession}
                branches={branchSessions}
                onSelectSession={selectSession}
              />
            )}
          </div>
          {!draft && activeSession && (
            <ConversationActionsMenu
              sessionID={activeSession.id}
              tasks={tasks}
              onSelectTask={openTaskInWorkbench}
            />
          )}
          {!workbenchOwnsToggle && workbenchToggleControl}
        </header>

        <div className="relative min-h-0 flex-1">
          <main
            ref={logRef}
            data-testid="conversation-transcript"
            className="h-full overflow-x-hidden overflow-y-auto px-3 md:px-6 md:[scrollbar-gutter:stable_both-edges]"
            onScroll={trackScrollPosition}
            onWheelCapture={pauseFollowOnWheel}
          >
            <div
              className={cn(
                'mx-auto min-h-full w-full max-w-[750px] pt-5 pb-9 max-md:pt-4 max-md:pb-7',
                (loading || emptySession) && 'grid place-items-center',
              )}
            >
              {loading ? (
                <div className="flex items-center gap-2 pb-[8vh] text-xs text-ink-faint">
                  <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
                  {t('app.loadingSession')}
                </div>
              ) : emptySession ? (
                <div className="flex w-full -translate-y-[3vh] flex-col items-center gap-9">
                  <div className="max-w-lg text-center">
                    <h1 className="m-0 text-[1.75rem] leading-tight font-medium tracking-[-0.03em] text-ink max-sm:text-2xl">
                      {t('app.emptyTitle')}
                    </h1>
                    <p className="mt-2.5 text-[0.9375rem] leading-6 text-ink-muted">
                      {t('app.emptyDescription')}
                    </p>
                  </div>
                  {renderComposer(true)}
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
                        branchingDisabled={running || forking}
                        onForkMessage={forkMessage}
                      />
                    ),
                  )}
                  {autoCompacting ? <AutoCompactionStatus /> : awaitingFirstOutput && <AwaitingResponse />}
                </>
              )}
            </div>
          </main>
          {awayFromLatest && (
            <ScrollToLatestButton
              hasNewContent={hasNewContent}
              onClick={scrollToLatest}
            />
          )}
        </div>

        {!loading && !emptySession && renderComposer()}
      </div>
  )
}
