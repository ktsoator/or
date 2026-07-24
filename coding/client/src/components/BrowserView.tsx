import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react'
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
import type { ModelOption, PreviewState, WorkspaceSummary } from '@/types'
import type { SessionThread } from '@/useSession'
import {
  normalizeBrowserAddress,
  workspaceFileURL,
  workspacePreviewURL,
} from '@/lib/browser'
import {
  closeNativeBrowser,
  goBackNativeBrowser,
  goForwardNativeBrowser,
  hasNativeBrowser,
  openExternalURL,
  type NativeBrowserState,
} from '@/lib/desktop'
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

type BrowserTab = {
  id: string
  kind: 'preview' | 'manual'
  title?: string
  url: string
  address: string
  navigation: number
  invalidAddress: boolean
  workspacePath?: string
  loading: boolean
  canGoBack: boolean
  canGoForward: boolean
  error?: string
}

function previewTabID(sessionID?: string): string {
  return `preview:${sessionID ?? 'unknown'}`
}

function createBrowserTab(
  id: string,
  preview?: PreviewState,
  sessionID?: string,
): BrowserTab {
  const workspacePath = preview?.path
  const workspaceRelativePath = preview?.relativePath ?? workspacePath
  const url = workspaceRelativePath && sessionID
    ? workspacePreviewURL(sessionID, workspaceRelativePath)
    : preview?.url ?? ''
  return {
    id,
    kind: preview ? 'preview' : 'manual',
    title: preview?.title,
    url,
    address: workspacePath ?? preview?.url ?? '',
    navigation: 0,
    invalidAddress: false,
    workspacePath,
    loading: false,
    canGoBack: false,
    canGoForward: false,
  }
}

export function BrowserView({
  preview,
  sessionID,
  activatePreview,
  conversation,
  creatingConversation,
  models,
  workspaces,
  onCloseTab,
  onCloseConversation,
  onCreateConversation,
  onConfigureModel,
  maximized,
  open,
  onToggleMaximized,
  toggleControl,
}: {
  preview?: PreviewState
  sessionID?: string
  activatePreview: boolean
  conversation?: SessionThread
  creatingConversation: boolean
  models: ModelOption[]
  workspaces: WorkspaceSummary[]
  onCloseTab: () => void
  onCloseConversation: () => void
  onCreateConversation: () => void
  onConfigureModel: () => void
  maximized: boolean
  open: boolean
  onToggleMaximized: () => void
  toggleControl?: ReactNode
}) {
  const { t } = useI18n()
  const initialTabRef = useRef<BrowserTab | undefined>(undefined)
  if (!initialTabRef.current && (preview || !conversation)) {
    initialTabRef.current = createBrowserTab(
      preview ? previewTabID(sessionID) : 'tab-1',
      preview,
      sessionID,
    )
  }
  const [tabs, setTabs] = useState<BrowserTab[]>(
    initialTabRef.current ? [initialTabRef.current] : [],
  )
  const [activeTabID, setActiveTabID] = useState(
    activatePreview && initialTabRef.current
      ? initialTabRef.current.id
      : conversation
        ? `conversation:${conversation.session.id}`
        : initialTabRef.current?.id ?? '',
  )
  const tabSequenceRef = useRef(initialTabRef.current && !preview ? 1 : 0)
  const previewKey = preview ? `${sessionID ?? 'unknown'}:${preview.revision}` : undefined
  const previewKeyRef = useRef(previewKey)
  const previousConversationTabIDRef = useRef<string | undefined>(undefined)
  const conversationTabID = conversation ? `conversation:${conversation.session.id}` : undefined
  const conversationActive = activeTabID === conversationTabID
  const activeTab = tabs.find((tab) => tab.id === activeTabID) ?? tabs[0]
  const activeExternalURL = activeTab
    ? activeTab.workspacePath
      ? workspaceFileURL(activeTab.workspacePath)
      : activeTab.url
    : ''
  const nativeBrowser = hasNativeBrowser()

  const updateTab = (tabID: string, values: Partial<BrowserTab>) => {
    setTabs((current) =>
      current.map((tab) => (tab.id === tabID ? { ...tab, ...values } : tab)),
    )
  }

  useEffect(() => {
    const previous = previousConversationTabIDRef.current
    previousConversationTabIDRef.current = conversationTabID
    if (conversationTabID && conversationTabID !== previous) {
      setActiveTabID(conversationTabID)
      return
    }
    if (conversationTabID || !previous || activeTabID !== previous) return
    if (tabs[0]) {
      setActiveTabID(tabs[0].id)
    } else {
      onCloseTab()
    }
  }, [activeTabID, conversationTabID, onCloseTab, tabs])

  useEffect(() => {
    if (!preview?.url && !preview?.path) return
    if (previewKeyRef.current === previewKey) return
    previewKeyRef.current = previewKey
    const source = createBrowserTab(previewTabID(sessionID), preview, sessionID)
    setTabs((current) => {
      const existing = current.find((tab) => tab.id === source.id)
      if (!existing) return [...current, source]
      return current.map((tab) =>
        tab.id === existing.id
          ? {
              ...tab,
              title: preview.title,
              url: source.url,
              address: source.address,
              workspacePath: source.workspacePath,
              navigation: tab.navigation + 1,
              invalidAddress: false,
            }
          : tab,
      )
    })
    if (activatePreview) setActiveTabID(source.id)
  }, [activatePreview, preview, previewKey, sessionID])

  const reload = () => {
    if (!activeTab?.url) return
    updateTab(activeTab.id, {
      navigation: activeTab.navigation + 1,
    })
  }

  const navigate = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!activeTab) return
    if (activeTab.workspacePath && activeTab.address === activeTab.workspacePath) {
      reload()
      return
    }
    const next = normalizeBrowserAddress(activeTab.address)
    if (!next) {
      updateTab(activeTab.id, { invalidAddress: true })
      return
    }
    updateTab(activeTab.id, {
      title: undefined,
      address: next,
      url: next,
      workspacePath: undefined,
      invalidAddress: false,
      navigation: activeTab.navigation + 1,
    })
  }

  const newTab = () => {
    tabSequenceRef.current += 1
    const tab = createBrowserTab(`tab-${tabSequenceRef.current}`)
    setTabs((current) => [...current, tab])
    setActiveTabID(tab.id)
  }

  const closeTab = (tabID: string) => {
    void closeNativeBrowser(tabID)
    if (tabs.length === 1 && !conversation) {
      onCloseTab()
      return
    }
    const closingIndex = tabs.findIndex((tab) => tab.id === tabID)
    const remaining = tabs.filter((tab) => tab.id !== tabID)
    setTabs(remaining)
    if (tabID === activeTabID) {
      const next = remaining[Math.min(closingIndex, remaining.length - 1)]
      setActiveTabID(next?.id ?? conversationTabID ?? '')
    }
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
            const title =
              tab.title ||
              tab.workspacePath?.split('/').at(-1) ||
              addressTitle(tab.url) ||
              t('preview.newTab')
            const active = !conversationActive && tab.id === activeTab?.id
            return (
              <div
                key={tab.id}
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
                  onClick={() => setActiveTabID(tab.id)}
                >
                  {tab.workspacePath ? (
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
                  onClick={() => closeTab(tab.id)}
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
                onClick={() => setActiveTabID(conversationTabID)}
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

      {conversationActive && conversation ? (
        <ConversationView
          thread={conversation}
          models={models}
          workspaces={workspaces}
          onConfigureModel={onConfigureModel}
        />
      ) : activeTab ? (
        <>
          <div className="flex h-10 shrink-0 items-center gap-1.5 border-b border-stone-200 bg-white px-2.5">
            <button
              className="grid size-7 cursor-pointer place-items-center rounded-md text-stone-500 transition-colors hover:bg-stone-100 hover:text-stone-900 focus-visible:outline-2 focus-visible:outline-stone-400 disabled:cursor-default disabled:text-stone-300 disabled:hover:bg-transparent"
              type="button"
              title={t('preview.back')}
              aria-label={t('preview.back')}
              disabled={!nativeBrowser || !activeTab.canGoBack}
              onClick={() => void goBackNativeBrowser(activeTab.id)}
            >
              <ArrowLeft className="size-4" aria-hidden="true" />
            </button>
            <button
              className="grid size-7 cursor-pointer place-items-center rounded-md text-stone-500 transition-colors hover:bg-stone-100 hover:text-stone-900 focus-visible:outline-2 focus-visible:outline-stone-400 disabled:cursor-default disabled:text-stone-300 disabled:hover:bg-transparent"
              type="button"
              title={t('preview.forward')}
              aria-label={t('preview.forward')}
              disabled={!nativeBrowser || !activeTab.canGoForward}
              onClick={() => void goForwardNativeBrowser(activeTab.id)}
            >
              <ArrowRight className="size-4" aria-hidden="true" />
            </button>
            <button
              className="grid size-7 cursor-pointer place-items-center rounded-md text-stone-500 transition-colors hover:bg-stone-100 hover:text-stone-900 focus-visible:outline-2 focus-visible:outline-stone-400 disabled:cursor-default disabled:text-stone-300 disabled:hover:bg-transparent"
              type="button"
              title={t('preview.refresh')}
              aria-label={t('preview.refresh')}
              disabled={
                !nativeBrowser || !activeTab.url
              }
              onClick={reload}
            >
              <RefreshCw
                className={cn('size-3.5', activeTab.loading && 'animate-spin')}
                aria-hidden="true"
              />
            </button>
            <form className="mx-1 min-w-0 flex-1" onSubmit={navigate}>
              <input
                className={cn(
                  'h-7 w-full rounded-lg border border-stone-200 bg-stone-50 px-2.5 font-mono text-[0.75rem] text-stone-700 outline-none placeholder:text-center placeholder:font-sans placeholder:text-stone-400 focus:border-stone-300 focus:bg-white focus:placeholder:text-left',
                  activeTab.invalidAddress && 'border-red-300 bg-red-50/50',
                )}
                value={activeTab.address}
                aria-label={t('preview.address')}
                placeholder={t('preview.enterURL')}
                spellCheck={false}
                onChange={(event) => {
                  updateTab(activeTab.id, {
                    address: event.target.value,
                    invalidAddress: false,
                  })
                }}
              />
            </form>
            <button
              className="grid size-7 cursor-pointer place-items-center rounded-md text-stone-500 transition-colors hover:bg-stone-100 hover:text-stone-900 focus-visible:outline-2 focus-visible:outline-stone-400 disabled:cursor-default disabled:text-stone-300 disabled:hover:bg-transparent"
              type="button"
              title={t('preview.openExternal')}
              aria-label={t('preview.openExternal')}
              disabled={!activeExternalURL}
              onClick={() => {
                if (activeExternalURL) openExternalURL(activeExternalURL)
              }}
            >
              <ExternalLink className="size-3.5" aria-hidden="true" />
            </button>
          </div>

          <BrowserSurface
            key={activeTab.id}
            tabID={activeTab.id}
            navigation={activeTab.navigation}
            url={activeTab.url}
            visible={open}
            workspaceFile={Boolean(activeTab.workspacePath)}
            onResolveURL={(url) => updateTab(activeTab.id, { address: url, url })}
            onRetry={reload}
            onState={(state: NativeBrowserState) => {
              updateTab(activeTab.id, {
                title: state.title || activeTab.title,
                loading: state.loading,
                canGoBack: state.canGoBack,
                canGoForward: state.canGoForward,
                error: state.error,
                ...(activeTab.workspacePath || !state.url
                  ? {}
                  : { address: state.url, url: state.url }),
              })
            }}
          />
        </>
      ) : null}
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
