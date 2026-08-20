import type { FormEvent, ReactNode } from 'react'
import {
  Activity,
  ArrowLeft,
  ArrowRight,
  LoaderCircle,
  ExternalLink,
  FileCode2,
  Globe2,
  MessageSquare,
  Maximize2,
  Minimize2,
  Plus,
  RefreshCw,
  X,
} from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import type {
  ModelOption,
  WorkspaceSummary,
} from '@/types'
import {
  BrowserSurface,
  browserRuntimeTabID,
  browserTabNavigationURL,
  conversationWorkbenchTabID,
  type BrowserWorkspaceController,
} from '@/features/browser'
import { cn } from '@/lib/utils'
import { useI18n } from '@/i18n'
import {
  BackgroundTasksView,
  type WorkbenchTaskSource,
} from './BackgroundTasksView'
import {
  ConversationActionsMenu,
  ConversationView,
  DraftConversationView,
} from '@/features/conversation'
import type { WorkbenchConversation } from './conversations'

function addressTitle(url: string): string {
  try {
    return new URL(url).host
  } catch {
    return ''
  }
}

export function WorkbenchView({
  workspace,
  conversations,
  taskSource,
  creatingConversation,
  models,
  workspaces,
  onCloseTab,
  onCloseConversation,
  onCreateConversation,
  onConfigureModel,
  maximized,
  onToggleMaximized,
  toggleControl,
}: {
  workspace: BrowserWorkspaceController
  conversations: WorkbenchConversation[]
  taskSource?: WorkbenchTaskSource
  creatingConversation: boolean
  models: ModelOption[]
  workspaces: WorkspaceSummary[]
  onCloseTab: () => void
  onCloseConversation: (conversationID: string) => void
  onCreateConversation: () => void
  onConfigureModel: () => void
  maximized: boolean
  onToggleMaximized: () => void
  toggleControl?: ReactNode
}) {
  const { t } = useI18n()
  const runningTaskCount =
    taskSource?.tasks.filter((task) => task.status === 'running').length ?? 0
  const {
    tabs,
    runtimeWorkspaceID,
    activeConversationID,
    conversationActive,
    taskTabID,
    selectedTaskID,
    tasksActive,
    activeTab,
    activeDesired,
    activeObserved,
    activeExternalURL,
    browserRuntime,
    selectItem,
    newTab,
    closeTab,
    reload,
    navigateActiveAddress,
    editAddress,
    resolveNavigation,
    runtimeStateReceived,
    goBack,
    goForward,
    openExternal,
  } = workspace
  const browserItemActive = !conversationActive && !tasksActive
  const activeConversation = conversations.find(
    (conversation) => conversation.id === activeConversationID,
  )

  const navigate = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    navigateActiveAddress()
  }

  return (
    <section
      className="flex h-full min-h-0 flex-col bg-canvas"
      data-testid="browser-view"
      aria-label={t('view.browser')}
    >
      <div
        className="window-titlebar flex h-[45px] shrink-0 select-none items-center bg-canvas px-2"
        data-testid="browser-titlebar"
      >
        <div
          className="flex min-w-0 flex-1 items-center gap-0.5 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
          role="tablist"
          aria-label={t('workbench.tabs')}
        >
          {tabs.map((tab) => {
            const desired = tab.desired
            const title =
              tab.observed.title ||
              desired?.title ||
              desired?.workspacePath?.split('/').at(-1) ||
              addressTitle(tab.observed.committedURL || desired?.requestedURL || '') ||
              t('preview.newTab')
            const active = browserItemActive && tab.id === activeTab?.id
            return (
              <div
                key={`${runtimeWorkspaceID}:${tab.id}`}
                className={cn(
                  'group flex h-8 min-w-[7rem] max-w-[11rem] shrink-0 items-center rounded-md border transition-colors',
                  active
                    ? 'border-edge/80 bg-canvas text-ink-soft shadow-sm'
                    : 'border-transparent text-ink-muted hover:bg-canvas-sunken/80 hover:text-ink-soft',
                )}
                data-testid="browser-tab"
                data-active={active}
              >
                <button
                  className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 self-stretch px-2.5 text-left text-[0.8125rem] outline-none focus-visible:bg-canvas-sunken"
                  type="button"
                  role="tab"
                  aria-selected={active}
                  title={title}
                  onClick={() => selectItem(tab.id)}
                >
                  {desired?.kind === 'workspace-preview' ? (
                    <FileCode2 className="size-3.5 shrink-0 text-ink-faint" aria-hidden="true" />
                  ) : (
                    <Globe2 className="size-3.5 shrink-0 text-ink-faint" aria-hidden="true" />
                  )}
                  <span className="min-w-0 flex-1 truncate">{title}</span>
                </button>
                <button
                  className={cn(
                    'mr-1 grid size-5 shrink-0 cursor-pointer place-items-center rounded text-ink-faint outline-none transition-[opacity,color,background-color] hover:bg-canvas-sunken hover:text-ink-soft focus-visible:bg-canvas-sunken focus-visible:text-ink-soft focus-visible:opacity-100 group-hover:opacity-100 group-focus-within:opacity-100',
                    active ? 'opacity-100' : 'opacity-0',
                  )}
                  type="button"
                  title={t('preview.closeNamedTab', { title })}
                  aria-label={t('preview.closeNamedTab', { title })}
                  onClick={() => {
                    if (closeTab(tab.id)) onCloseTab()
                  }}
                >
                  <X className="size-3.5" aria-hidden="true" />
                </button>
              </div>
            )
          })}
          {conversations.map((conversation) => {
            const conversationTabID = conversationWorkbenchTabID(conversation.id)!
            const active =
              conversationActive && conversation.id === activeConversationID
            const title = conversation.kind === 'draft'
              ? t('app.newSession')
              : conversation.thread.session.title === 'New session'
                ? t('app.newSession')
                : conversation.thread.session.title
            return (
              <div
                key={conversation.id}
                className={cn(
                  'group flex h-8 min-w-[7rem] max-w-[11rem] shrink-0 items-center rounded-md border transition-colors',
                  active
                    ? 'border-edge/80 bg-canvas text-ink-soft shadow-sm'
                    : 'border-transparent text-ink-muted hover:bg-canvas-sunken/80 hover:text-ink-soft',
                )}
                data-testid="conversation-tab"
                data-active={active}
              >
                <button
                  className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 self-stretch px-2.5 text-left text-[0.8125rem] outline-none focus-visible:bg-canvas-sunken"
                  type="button"
                  role="tab"
                  aria-selected={active}
                  title={title}
                  onClick={() => selectItem(conversationTabID)}
                >
                  <MessageSquare className="size-3.5 shrink-0 text-ink-faint" aria-hidden="true" />
                  <span className="min-w-0 flex-1 truncate">{title}</span>
                </button>
                <button
                  className={cn(
                    'mr-1 grid size-5 shrink-0 cursor-pointer place-items-center rounded text-ink-faint outline-none transition-[opacity,color,background-color] hover:bg-canvas-sunken hover:text-ink-soft focus-visible:bg-canvas-sunken focus-visible:text-ink-soft focus-visible:opacity-100 group-hover:opacity-100 group-focus-within:opacity-100',
                    active ? 'opacity-100' : 'opacity-0',
                  )}
                  type="button"
                  title={t('workbench.closeConversation')}
                  aria-label={t('workbench.closeConversation')}
                  onClick={() => onCloseConversation(conversation.id)}
                >
                  <X className="size-3.5" aria-hidden="true" />
                </button>
              </div>
            )
          })}
          {taskTabID && taskSource && (
            <div
              className={cn(
                'group flex h-8 min-w-[7rem] max-w-[11rem] shrink-0 items-center rounded-md border transition-colors',
                tasksActive
                  ? 'border-edge/80 bg-canvas text-ink-soft shadow-sm'
                  : 'border-transparent text-ink-muted hover:bg-canvas-sunken/80 hover:text-ink-soft',
              )}
              data-testid="background-tasks-tab"
              data-active={tasksActive}
            >
              <button
                className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 self-stretch px-2.5 text-left text-[0.8125rem] outline-none focus-visible:bg-canvas-sunken"
                type="button"
                role="tab"
                aria-selected={tasksActive}
                title={t('tasks.title')}
                onClick={() => selectItem(taskTabID)}
              >
                <Activity
                  className={cn(
                    'size-3.5 shrink-0 text-ink-faint',
                    runningTaskCount > 0 && 'text-success',
                  )}
                  aria-hidden="true"
                />
                <span className="min-w-0 flex-1 truncate">{t('tasks.title')}</span>
                {runningTaskCount > 0 && (
                  <span className="text-[0.625rem] tabular-nums text-success">
                    {runningTaskCount}
                  </span>
                )}
              </button>
              <button
                className={cn(
                  'mr-1 grid size-5 shrink-0 cursor-pointer place-items-center rounded text-ink-faint outline-none transition-[opacity,color,background-color] hover:bg-canvas-sunken hover:text-ink-soft focus-visible:bg-canvas-sunken focus-visible:text-ink-soft focus-visible:opacity-100 group-hover:opacity-100 group-focus-within:opacity-100',
                  tasksActive ? 'opacity-100' : 'opacity-0',
                )}
                type="button"
                title={t('tasks.close')}
                aria-label={t('tasks.close')}
                onClick={() => {
                  if (workspace.closeTasks()) onCloseTab()
                }}
              >
                <X className="size-3.5" aria-hidden="true" />
              </button>
            </div>
          )}
        </div>
        <WorkbenchHeaderActions
          maximized={maximized}
          onToggleMaximized={onToggleMaximized}
          onOpenBrowser={newTab}
          creatingConversation={creatingConversation}
          onCreateConversation={onCreateConversation}
          conversationActions={
            conversationActive && activeConversation?.kind === 'session' ? (
              <ConversationActionsMenu
                sessionID={activeConversation.thread.session.id}
                tasks={activeConversation.thread.tasks}
                onSelectTask={workspace.openTasks}
              />
            ) : undefined
          }
          onOpenTasks={taskSource ? workspace.openTasks : undefined}
          toggleControl={toggleControl}
        />
      </div>

      {conversations.map((conversation) => {
        const active = conversationActive && conversation.id === activeConversationID
        return (
          <div
            key={conversation.id}
            className={active ? 'min-h-0 flex-1' : 'hidden'}
            aria-hidden={!active}
          >
            {conversation.kind === 'session' ? (
              <ConversationView
                thread={conversation.thread}
                models={models}
                workspaces={workspaces}
                onConfigureModel={onConfigureModel}
              />
            ) : (
              <DraftConversationView
                draft={conversation.draft}
                connected={conversation.connected}
                creating={conversation.creating}
                models={models}
                workspaces={workspaces}
                onChange={conversation.onChange}
                onSend={conversation.onSend}
                onConfigureModel={onConfigureModel}
              />
            )}
          </div>
        )
      })}
      {tasksActive && taskSource && (
        <BackgroundTasksView
          {...taskSource}
          selectedTaskID={selectedTaskID}
          onSelectTask={workspace.selectTask}
        />
      )}
      {activeTab && (
        <div
          className={cn(
            'min-h-0 flex-1 flex-col',
            browserItemActive ? 'flex' : 'hidden',
          )}
        >
          <div className="flex h-10 shrink-0 items-center gap-1.5 border-b border-edge bg-canvas px-2.5">
            <button
              className="grid size-7 cursor-pointer place-items-center rounded-md text-ink-muted outline-none transition-colors hover:bg-canvas-sunken hover:text-ink focus-visible:bg-canvas-sunken focus-visible:text-ink disabled:cursor-default disabled:text-ink-ghost disabled:hover:bg-transparent"
              type="button"
              title={t('preview.back')}
              aria-label={t('preview.back')}
              disabled={!browserRuntime || !activeObserved?.canGoBack}
              onClick={goBack}
            >
              <ArrowLeft className="size-4" aria-hidden="true" />
            </button>
            <button
              className="grid size-7 cursor-pointer place-items-center rounded-md text-ink-muted outline-none transition-colors hover:bg-canvas-sunken hover:text-ink focus-visible:bg-canvas-sunken focus-visible:text-ink disabled:cursor-default disabled:text-ink-ghost disabled:hover:bg-transparent"
              type="button"
              title={t('preview.forward')}
              aria-label={t('preview.forward')}
              disabled={!browserRuntime || !activeObserved?.canGoForward}
              onClick={goForward}
            >
              <ArrowRight className="size-4" aria-hidden="true" />
            </button>
            <button
              className="grid size-7 cursor-pointer place-items-center rounded-md text-ink-muted outline-none transition-colors hover:bg-canvas-sunken hover:text-ink focus-visible:bg-canvas-sunken focus-visible:text-ink disabled:cursor-default disabled:text-ink-ghost disabled:hover:bg-transparent"
              type="button"
              title={t('preview.refresh')}
              aria-label={t('preview.refresh')}
              disabled={!browserRuntime || !activeDesired?.requestedURL}
              onClick={() => reload()}
            >
              <RefreshCw
                className={cn(
                  'size-3.5',
                  activeObserved?.status === 'navigating' && 'animate-spin',
                )}
                aria-hidden="true"
              />
            </button>
            <form className="mx-1 min-w-0 flex-1" onSubmit={navigate}>
              <input
                className={cn(
                  'h-7 w-full rounded-lg border border-edge bg-canvas-raised px-2.5 font-mono text-[0.75rem] text-ink-soft outline-none placeholder:text-center placeholder:font-sans placeholder:text-ink-faint focus:border-edge-strong focus:bg-canvas focus:placeholder:text-left',
                  activeTab.invalidAddress && 'border-danger-edge bg-danger-surface/50',
                )}
                data-testid="browser-address"
                value={activeTab.addressDraft}
                aria-label={t('preview.address')}
                placeholder={t('preview.enterURL')}
                spellCheck={false}
                onChange={(event) => {
                  editAddress(activeTab.id, event.target.value)
                }}
              />
            </form>
            <button
              className="grid size-7 cursor-pointer place-items-center rounded-md text-ink-muted outline-none transition-colors hover:bg-canvas-sunken hover:text-ink focus-visible:bg-canvas-sunken focus-visible:text-ink disabled:cursor-default disabled:text-ink-ghost disabled:hover:bg-transparent"
              type="button"
              title={t('preview.openExternal')}
              aria-label={t('preview.openExternal')}
              disabled={!activeExternalURL}
              onClick={openExternal}
            >
              <ExternalLink className="size-3.5" aria-hidden="true" />
            </button>
          </div>

          <div className="relative min-h-0 flex-1 bg-canvas">
            {tabs.map((tab) => {
              const desired = tab.desired
              const active = tab.id === activeTab.id
              return (
                <div
                  key={`${runtimeWorkspaceID}:${tab.id}`}
                  className={cn(
                    'absolute inset-0 flex',
                    active ? 'visible' : 'invisible pointer-events-none',
                  )}
                  aria-hidden={!active}
                >
                  <BrowserSurface
                    active={active}
                    runtimeTabID={browserRuntimeTabID(runtimeWorkspaceID, tab.id)}
                    tabID={tab.id}
                    navigation={desired?.revision ?? 0}
                    observed={tab.observed}
                    url={browserTabNavigationURL(tab)}
                    workspaceFile={desired?.kind === 'workspace-preview'}
                    onResolveURL={(url) => {
                      if (!desired) return
                      resolveNavigation(tab.id, desired.revision, url)
                    }}
                    onRetry={() => reload(tab.id)}
                    onState={(state) => runtimeStateReceived(tab.id, state)}
                  />
                </div>
              )
            })}
          </div>
        </div>
      )}
    </section>
  )
}

export function WorkbenchHeaderActions({
  maximized,
  onToggleMaximized,
  onOpenBrowser,
  onOpenTasks,
  creatingConversation,
  onCreateConversation,
  conversationActions,
  toggleControl,
}: {
  maximized: boolean
  onToggleMaximized: () => void
  onOpenBrowser: () => void
  onOpenTasks?: () => void
  creatingConversation: boolean
  onCreateConversation: () => void
  conversationActions?: ReactNode
  toggleControl?: ReactNode
}) {
  const { t } = useI18n()
  const ExpandIcon = maximized ? Minimize2 : Maximize2

  return (
    <div className="window-titlebar-controls ml-1 flex h-[44px] shrink-0 items-center gap-0.5 self-start">
      {conversationActions}
      <DropdownMenu.Root>
        <DropdownMenu.Trigger asChild>
          <button
            className="grid size-7 shrink-0 cursor-pointer place-items-center rounded-md text-ink-muted outline-none transition-colors hover:bg-surface-active hover:text-ink focus-visible:bg-surface-active focus-visible:text-ink data-[state=open]:bg-surface-selected data-[state=open]:text-ink"
            data-testid="workbench-add-view"
            type="button"
            title={t('workbench.addView')}
            aria-label={t('workbench.addView')}
          >
            <Plus className="size-4" aria-hidden="true" />
          </button>
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content
            side="bottom"
            align="end"
            sideOffset={7}
            collisionPadding={10}
            className="z-[120] min-w-[15.5rem] animate-[fade-in_110ms_ease-out] rounded-2xl border border-edge bg-canvas p-1 text-[0.875rem] text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
          >
            <WorkbenchMenuItem icon={Globe2} label={t('view.browser')} onSelect={onOpenBrowser} />
            {onOpenTasks && (
              <WorkbenchMenuItem
                icon={Activity}
                label={t('tasks.title')}
                onSelect={onOpenTasks}
              />
            )}
            <WorkbenchMenuItem
              icon={MessageSquare}
              label={t('workbench.chat')}
              loading={creatingConversation}
              disabled={creatingConversation}
              onSelect={onCreateConversation}
            />
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
      <button
        className="grid size-7 shrink-0 cursor-pointer place-items-center rounded-md text-ink-muted outline-none transition-colors hover:bg-surface-active hover:text-ink focus-visible:bg-surface-active focus-visible:text-ink"
        data-testid="workbench-maximize"
        type="button"
        title={maximized ? t('workbench.restore') : t('workbench.maximize')}
        aria-label={maximized ? t('workbench.restore') : t('workbench.maximize')}
        aria-pressed={maximized}
        onClick={onToggleMaximized}
      >
        <ExpandIcon className="size-3.5" aria-hidden="true" />
      </button>
      {toggleControl}
    </div>
  )
}

function WorkbenchMenuItem({
  disabled,
  icon: Icon,
  label,
  loading,
  onSelect,
}: {
  disabled?: boolean
  icon: typeof Globe2
  label: string
  loading?: boolean
  onSelect?: () => void
}) {
  return (
    <DropdownMenu.Item
      className="mb-0.5 flex h-[30px] cursor-default select-none items-center gap-2.5 rounded-[10px] px-2.5 outline-none last:mb-0 data-[disabled]:opacity-40 data-[highlighted]:bg-surface-active"
      disabled={disabled}
      aria-busy={loading}
      onSelect={onSelect}
    >
      {loading ? (
        <LoaderCircle className="size-4 shrink-0 animate-spin text-ink-muted" aria-hidden="true" />
      ) : (
        <Icon className="size-4 shrink-0 text-ink-muted" aria-hidden="true" />
      )}
      <span>{label}</span>
    </DropdownMenu.Item>
  )
}
