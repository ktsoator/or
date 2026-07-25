import type { FormEvent, ReactNode } from 'react'
import {
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
import type { SessionThread } from '@/useSession'
import type { BrowserWorkspaceController } from '@/useBrowserWorkspace'
import { browserTabNavigationURL } from '@/browserTabs'
import { cn } from '@/lib/utils'
import { useI18n } from '@/i18n'
import { BrowserSurface } from './BrowserSurface'
import { ConversationView } from './ConversationView'

function addressTitle(url: string): string {
  try {
    return new URL(url).host
  } catch {
    return ''
  }
}

export function BrowserView({
  workspace,
  conversation,
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
  conversation?: SessionThread
  creatingConversation: boolean
  models: ModelOption[]
  workspaces: WorkspaceSummary[]
  onCloseTab: () => void
  onCloseConversation: () => void
  onCreateConversation: () => void
  onConfigureModel: () => void
  maximized: boolean
  onToggleMaximized: () => void
  toggleControl?: ReactNode
}) {
  const { t } = useI18n()
  const {
    tabs,
    workspaceID,
    conversationTabID,
    conversationActive,
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

  const navigate = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    navigateActiveAddress()
  }

  return (
    <section
      className="flex h-full min-h-0 flex-col bg-white"
      data-testid="browser-view"
      aria-label={t('view.browser')}
    >
      <div
        className="window-titlebar flex h-[45px] shrink-0 select-none items-center bg-white px-2"
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
            const active = !conversationActive && tab.id === activeTab?.id
            return (
              <div
                key={`${workspaceID}:${tab.id}`}
                className={cn(
                  'group flex h-8 min-w-[7rem] max-w-[11rem] shrink-0 items-center rounded-md border transition-colors',
                  active
                    ? 'border-stone-200/80 bg-white text-stone-800 shadow-sm'
                    : 'border-transparent text-stone-500 hover:bg-stone-100/80 hover:text-stone-800',
                )}
                data-testid="browser-tab"
                data-active={active}
              >
                <button
                  className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 self-stretch px-2.5 text-left text-[0.8125rem] focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-stone-400"
                  type="button"
                  role="tab"
                  aria-selected={active}
                  title={title}
                  onClick={() => selectItem(tab.id)}
                >
                  {desired?.kind === 'workspace-preview' ? (
                    <FileCode2 className="size-3.5 shrink-0 text-stone-400" aria-hidden="true" />
                  ) : (
                    <Globe2 className="size-3.5 shrink-0 text-stone-400" aria-hidden="true" />
                  )}
                  <span className="min-w-0 flex-1 truncate">{title}</span>
                </button>
                <button
                  className={cn(
                    'mr-1 grid size-5 shrink-0 cursor-pointer place-items-center rounded text-stone-400 transition-[opacity,color,background-color] hover:bg-stone-100 hover:text-stone-800 focus-visible:opacity-100 focus-visible:outline-2 focus-visible:outline-stone-400 group-hover:opacity-100 group-focus-within:opacity-100',
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
          {conversation && conversationTabID && (
            <div
              className={cn(
                'group flex h-8 min-w-[7rem] max-w-[11rem] shrink-0 items-center rounded-md border transition-colors',
                conversationActive
                  ? 'border-stone-200/80 bg-white text-stone-800 shadow-sm'
                  : 'border-transparent text-stone-500 hover:bg-stone-100/80 hover:text-stone-800',
              )}
              data-testid="conversation-tab"
              data-active={conversationActive}
            >
              <button
                className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 self-stretch px-2.5 text-left text-[0.8125rem] focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-stone-400"
                type="button"
                role="tab"
                aria-selected={conversationActive}
                title={conversation.session.title}
                onClick={() => selectItem(conversationTabID)}
              >
                <MessageSquare className="size-3.5 shrink-0 text-stone-400" aria-hidden="true" />
                <span className="min-w-0 flex-1 truncate">
                  {conversation.session.title === 'New session'
                    ? t('app.newSession')
                    : conversation.session.title}
                </span>
              </button>
              <button
                className={cn(
                  'mr-1 grid size-5 shrink-0 cursor-pointer place-items-center rounded text-stone-400 transition-[opacity,color,background-color] hover:bg-stone-100 hover:text-stone-800 focus-visible:opacity-100 focus-visible:outline-2 focus-visible:outline-stone-400 group-hover:opacity-100 group-focus-within:opacity-100',
                  conversationActive ? 'opacity-100' : 'opacity-0',
                )}
                type="button"
                title={t('workbench.closeConversation')}
                aria-label={t('workbench.closeConversation')}
                onClick={onCloseConversation}
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
          toggleControl={toggleControl}
        />
      </div>

      {conversationActive && conversation && (
        <ConversationView
          thread={conversation}
          models={models}
          workspaces={workspaces}
          onConfigureModel={onConfigureModel}
        />
      )}
      {activeTab && (
        <div
          className={cn(
            'min-h-0 flex-1 flex-col',
            conversationActive ? 'hidden' : 'flex',
          )}
        >
          <div className="flex h-10 shrink-0 items-center gap-1.5 border-b border-stone-200 bg-white px-2.5">
            <button
              className="grid size-7 cursor-pointer place-items-center rounded-md text-stone-500 transition-colors hover:bg-stone-100 hover:text-stone-900 focus-visible:outline-2 focus-visible:outline-stone-400 disabled:cursor-default disabled:text-stone-300 disabled:hover:bg-transparent"
              type="button"
              title={t('preview.back')}
              aria-label={t('preview.back')}
              disabled={!browserRuntime || !activeObserved?.canGoBack}
              onClick={goBack}
            >
              <ArrowLeft className="size-4" aria-hidden="true" />
            </button>
            <button
              className="grid size-7 cursor-pointer place-items-center rounded-md text-stone-500 transition-colors hover:bg-stone-100 hover:text-stone-900 focus-visible:outline-2 focus-visible:outline-stone-400 disabled:cursor-default disabled:text-stone-300 disabled:hover:bg-transparent"
              type="button"
              title={t('preview.forward')}
              aria-label={t('preview.forward')}
              disabled={!browserRuntime || !activeObserved?.canGoForward}
              onClick={goForward}
            >
              <ArrowRight className="size-4" aria-hidden="true" />
            </button>
            <button
              className="grid size-7 cursor-pointer place-items-center rounded-md text-stone-500 transition-colors hover:bg-stone-100 hover:text-stone-900 focus-visible:outline-2 focus-visible:outline-stone-400 disabled:cursor-default disabled:text-stone-300 disabled:hover:bg-transparent"
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
                  'h-7 w-full rounded-lg border border-stone-200 bg-stone-50 px-2.5 font-mono text-[0.75rem] text-stone-700 outline-none placeholder:text-center placeholder:font-sans placeholder:text-stone-400 focus:border-stone-300 focus:bg-white focus:placeholder:text-left',
                  activeTab.invalidAddress && 'border-red-300 bg-red-50/50',
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
              className="grid size-7 cursor-pointer place-items-center rounded-md text-stone-500 transition-colors hover:bg-stone-100 hover:text-stone-900 focus-visible:outline-2 focus-visible:outline-stone-400 disabled:cursor-default disabled:text-stone-300 disabled:hover:bg-transparent"
              type="button"
              title={t('preview.openExternal')}
              aria-label={t('preview.openExternal')}
              disabled={!activeExternalURL}
              onClick={openExternal}
            >
              <ExternalLink className="size-3.5" aria-hidden="true" />
            </button>
          </div>

          <div className="relative min-h-0 flex-1 bg-white">
            {tabs.map((tab) => {
              const desired = tab.desired
              const active = tab.id === activeTab.id
              return (
                <div
                  key={`${workspaceID}:${tab.id}`}
                  className={cn(
                    'absolute inset-0 flex',
                    active ? 'visible' : 'invisible pointer-events-none',
                  )}
                  aria-hidden={!active}
                >
                  <BrowserSurface
                    active={active}
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
  creatingConversation,
  onCreateConversation,
  toggleControl,
}: {
  maximized: boolean
  onToggleMaximized: () => void
  onOpenBrowser: () => void
  creatingConversation: boolean
  onCreateConversation: () => void
  toggleControl?: ReactNode
}) {
  const { t } = useI18n()
  const ExpandIcon = maximized ? Minimize2 : Maximize2

  return (
    <div className="window-titlebar-controls ml-1 flex h-[44px] shrink-0 items-center gap-0.5 self-start">
      <DropdownMenu.Root>
        <DropdownMenu.Trigger asChild>
          <button
            className="grid size-7 shrink-0 cursor-pointer place-items-center rounded-md text-stone-500 outline-none transition-colors hover:bg-[rgb(241,241,241)] hover:text-stone-900 focus-visible:ring-2 focus-visible:ring-stone-300 data-[state=open]:bg-[rgb(237,237,237)] data-[state=open]:text-stone-900"
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
            className="z-[120] min-w-[15.5rem] animate-[fade-in_110ms_ease-out] rounded-2xl border border-stone-200 bg-white p-1 text-[0.875rem] text-stone-900 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
          >
            <WorkbenchMenuItem icon={Globe2} label={t('view.browser')} onSelect={onOpenBrowser} />
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
        className="grid size-7 shrink-0 cursor-pointer place-items-center rounded-md text-stone-500 outline-none transition-colors hover:bg-[rgb(241,241,241)] hover:text-stone-900 focus-visible:ring-2 focus-visible:ring-stone-300"
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
      className="mb-0.5 flex h-[30px] cursor-default select-none items-center gap-2.5 rounded-[10px] px-2.5 outline-none last:mb-0 data-[disabled]:opacity-40 data-[highlighted]:bg-[rgb(241,241,241)]"
      disabled={disabled}
      aria-busy={loading}
      onSelect={onSelect}
    >
      {loading ? (
        <LoaderCircle className="size-4 shrink-0 animate-spin text-stone-500" aria-hidden="true" />
      ) : (
        <Icon className="size-4 shrink-0 text-stone-600" aria-hidden="true" />
      )}
      <span>{label}</span>
    </DropdownMenu.Item>
  )
}
