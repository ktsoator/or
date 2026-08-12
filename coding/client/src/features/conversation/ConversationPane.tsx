import type {
  RefObject,
  ReactNode,
  UIEventHandler,
  WheelEventHandler,
} from 'react'
import { useEffect, useRef, useState } from 'react'
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
import { groupAssistantTurns, type RenderUnit } from './groupItems'
import { ConversationActionsMenu } from './ConversationActionsMenu'
import { ScrollToLatestButton } from './ConversationScrollControl'
import {
  AutoCompactionStatus,
  AwaitingResponse,
  ThreadItem,
} from './ConversationThread'
import { SidebarToggleButton } from '@/shared/ui/SidebarToggleButton'
import { StepGroup } from './StepGroup'
import { ConversationBranchNavigation } from './ConversationBranchNavigation'

type ConversationPaneProps = {
  thread: {
    draft?: SessionDraft
    activeSession?: SessionSummary
    parentSession?: SessionSummary
    branchSessions: SessionSummary[]
    branchPointTarget?: {
      sessionID: string
      messageID: string
    }
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
    returnToParent?: () => void
    branchPointLocated: (target: { sessionID: string; messageID: string }) => void
    forkMessage: (
      messageID: string,
      mode: 'before_user' | 'after_assistant',
      text?: string,
    ) => Promise<SessionSummary>
    editMessage: (messageID: string, text: string) => Promise<SessionSummary>
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
    branchPointTarget,
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
    returnToParent,
    branchPointLocated,
    forkMessage,
    editMessage,
    renderComposer,
  } = actions
  const [highlightedBranchPoint, setHighlightedBranchPoint] = useState<{
    sessionID: string
    messageID: string
  }>()
  const highlightTimerRef = useRef<number>(undefined)
  const branchPointItemID = branchPointTarget
    ? items.find(
        (item) =>
          (item.kind === 'user' || item.kind === 'assistant') &&
          item.messageID === branchPointTarget.messageID,
      )?.id
    : undefined
  const emptySession = !loading && items.length === 0 && !approval
  const awaitingFirstOutput = running && items.at(-1)?.kind === 'user'

  useEffect(() => () => {
    if (highlightTimerRef.current) window.clearTimeout(highlightTimerRef.current)
  }, [])

  useEffect(() => {
    if (
      loading ||
      !branchPointTarget ||
      branchPointTarget.sessionID !== activeSession?.id
    ) {
      return
    }
    if (!branchPointItemID) {
      branchPointLocated(branchPointTarget)
      return
    }

    const transcript = logRef.current
    const message = Array.from(
      transcript?.querySelectorAll<HTMLElement>('[data-branch-point-message-id]') ?? [],
    ).find(
      (element) =>
        element.dataset.branchPointMessageId === branchPointTarget.messageID,
    )
    if (!transcript || !message) return

    setHighlightedBranchPoint(branchPointTarget)
    if (highlightTimerRef.current) window.clearTimeout(highlightTimerRef.current)

    const frame = window.requestAnimationFrame(() => {
      const transcriptBox = transcript.getBoundingClientRect()
      const messageBox = message.getBoundingClientRect()
      const top =
        transcript.scrollTop +
        messageBox.top -
        transcriptBox.top -
        (transcript.clientHeight - messageBox.height) / 2
      const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
      transcript.scrollTo({ top: Math.max(0, top), behavior: reducedMotion ? 'auto' : 'smooth' })
      branchPointLocated(branchPointTarget)
      highlightTimerRef.current = window.setTimeout(() => {
        setHighlightedBranchPoint((current) =>
          current?.sessionID === branchPointTarget.sessionID &&
          current.messageID === branchPointTarget.messageID
            ? undefined
            : current,
        )
      }, 1600)
    })

    return () => window.cancelAnimationFrame(frame)
  }, [
    activeSession?.id,
    branchPointItemID,
    branchPointLocated,
    branchPointTarget,
    loading,
    logRef,
  ])

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
                parentBranchPointAvailable={Boolean(activeSession.forkedFromMessageId)}
                branches={branchSessions}
                onReturnToParent={returnToParent}
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
                  {groupAssistantTurns(items).map((unit) => {
                    if (unit.kind === 'assistant-turn') {
                      const highlighted =
                        activeSession?.id === highlightedBranchPoint?.sessionID &&
                        unit.messageID === highlightedBranchPoint?.messageID
                      return (
                        <div
                          key={unit.id}
                          className={cn(highlighted && 'bg-surface-hover')}
                          data-testid="assistant-turn"
                          data-branch-point-message-id={unit.messageID}
                          data-branch-point-highlighted={highlighted || undefined}
                        >
                          {unit.units.map((turnUnit) => renderUnit(turnUnit))}
                        </div>
                      )
                    }

                    const highlighted =
                      unit.kind === 'item' &&
                      unit.item.kind === 'user' &&
                      activeSession?.id === highlightedBranchPoint?.sessionID &&
                      unit.item.messageID === highlightedBranchPoint?.messageID
                    return renderUnit(unit, highlighted)
                  })}
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

  function renderUnit(unit: RenderUnit, highlighted = false) {
    return unit.kind === 'steps' ? (
      <StepGroup key={unit.id} items={unit.items} cwd={activeSession?.workspacePath} />
    ) : (
      <ThreadItem
        key={unit.item.id}
        item={unit.item}
        cwd={activeSession?.workspacePath}
        highlighted={highlighted}
        branchingDisabled={running || forking}
        onForkMessage={forkMessage}
        onEditMessage={editMessage}
        editRequiresConfirmation={hasLaterConversationContent(unit.item)}
      />
    )
  }

  function hasLaterConversationContent(item: Item) {
    const index = items.findIndex((candidate) => candidate.id === item.id)
    return index >= 0 && index < items.length - 1
  }
}
