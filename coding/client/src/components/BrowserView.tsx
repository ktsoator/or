import { useEffect, useRef, useState, type FormEvent } from 'react'
import {
  ArrowLeft,
  ArrowRight,
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
import type { PreviewState } from '@/types'
import {
  isLocalPreviewURL,
  normalizeBrowserAddress,
  workspaceFileURL,
  workspacePreviewURL,
} from '@/lib/browser'
import { openExternalURL } from '@/lib/desktop'
import { cn } from '@/lib/utils'
import { useI18n } from '@/i18n'
import { BrowserSurface } from './BrowserSurface'

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
}

const PREVIEW_TAB_ID = 'preview'

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
  }
}

export function BrowserView({
  preview,
  sessionID,
  onCloseTab,
  maximized,
  onToggleMaximized,
}: {
  preview?: PreviewState
  sessionID?: string
  onCloseTab: () => void
  maximized: boolean
  onToggleMaximized: () => void
}) {
  const { t } = useI18n()
  const initialTabRef = useRef<BrowserTab | undefined>(undefined)
  if (!initialTabRef.current) {
    initialTabRef.current = createBrowserTab(
      preview ? PREVIEW_TAB_ID : 'tab-1',
      preview,
      sessionID,
    )
  }
  const [tabs, setTabs] = useState<BrowserTab[]>([initialTabRef.current])
  const [activeTabID, setActiveTabID] = useState(initialTabRef.current.id)
  const tabSequenceRef = useRef(preview ? 0 : 1)
  const previewRevisionRef = useRef(preview?.revision)
  const activeTab = (tabs.find((tab) => tab.id === activeTabID) ?? tabs[0])!
  const activeTitle =
    activeTab.title ||
    activeTab.workspacePath?.split('/').at(-1) ||
    addressTitle(activeTab.url) ||
    t('preview.newTab')
  const activeExternalURL = activeTab.workspacePath
    ? workspaceFileURL(activeTab.workspacePath)
    : activeTab.url

  const updateTab = (tabID: string, values: Partial<BrowserTab>) => {
    setTabs((current) =>
      current.map((tab) => (tab.id === tabID ? { ...tab, ...values } : tab)),
    )
  }

  useEffect(() => {
    if (!preview?.url && !preview?.path) return
    if (previewRevisionRef.current === preview.revision) return
    previewRevisionRef.current = preview.revision
    const source = createBrowserTab(PREVIEW_TAB_ID, preview, sessionID)
    setTabs((current) => {
      const existing = current.find((tab) => tab.kind === 'preview')
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
    setActiveTabID(PREVIEW_TAB_ID)
  }, [preview, sessionID])

  const reload = () => {
    if (!activeTab.url) return
    updateTab(activeTab.id, {
      navigation: activeTab.navigation + 1,
    })
  }

  const navigate = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (activeTab.workspacePath && activeTab.address === activeTab.workspacePath) {
      reload()
      return
    }
    const next = normalizeBrowserAddress(activeTab.address)
    if (!next) {
      updateTab(activeTab.id, { invalidAddress: true })
      return
    }
    if (!isLocalPreviewURL(next)) openExternalURL(next)
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
    if (tabs.length === 1) {
      onCloseTab()
      return
    }
    const closingIndex = tabs.findIndex((tab) => tab.id === tabID)
    const remaining = tabs.filter((tab) => tab.id !== tabID)
    setTabs(remaining)
    if (tabID === activeTabID) {
      setActiveTabID(remaining[Math.min(closingIndex, remaining.length - 1)].id)
    }
  }

  return (
    <section
      className="flex h-full min-h-0 flex-col bg-white"
      data-testid="browser-view"
      aria-label={t('view.browser')}
    >
      <div
        className="window-drag-region flex h-[45px] shrink-0 select-none items-center bg-white px-2.5 pr-11"
        data-testid="browser-titlebar"
      >
        <div
          className="flex min-w-0 flex-1 items-center gap-0.5 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
          role="tablist"
          aria-label={t('preview.tabs')}
        >
          {tabs.map((tab) => {
            const title =
              tab.title ||
              tab.workspacePath?.split('/').at(-1) ||
              addressTitle(tab.url) ||
              t('preview.newTab')
            const active = tab.id === activeTab.id
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
        </div>
        <WorkbenchHeaderActions
          maximized={maximized}
          onToggleMaximized={onToggleMaximized}
          onOpenBrowser={newTab}
        />
      </div>

      <div className="flex h-10 shrink-0 items-center gap-1.5 border-b border-stone-200 bg-white px-2.5">
        <button
          className="grid size-7 place-items-center rounded-md text-stone-300"
          type="button"
          title={t('preview.back')}
          aria-label={t('preview.back')}
          disabled
        >
          <ArrowLeft className="size-4" aria-hidden="true" />
        </button>
        <button
          className="grid size-7 place-items-center rounded-md text-stone-300"
          type="button"
          title={t('preview.forward')}
          aria-label={t('preview.forward')}
          disabled
        >
          <ArrowRight className="size-4" aria-hidden="true" />
        </button>
        <button
          className="grid size-7 cursor-pointer place-items-center rounded-md text-stone-500 transition-colors hover:bg-stone-100 hover:text-stone-900 focus-visible:outline-2 focus-visible:outline-stone-400 disabled:cursor-default disabled:text-stone-300 disabled:hover:bg-transparent"
          type="button"
          title={t('preview.refresh')}
          aria-label={t('preview.refresh')}
          disabled={
            !activeTab.url ||
            (!activeTab.workspacePath && !isLocalPreviewURL(activeTab.url))
          }
          onClick={reload}
        >
          <RefreshCw className="size-3.5" aria-hidden="true" />
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
        key={`${activeTab.id}-${activeTab.navigation}-${activeTab.url}`}
        navigation={activeTab.navigation}
        title={activeTitle}
        url={activeTab.url}
        workspaceFile={Boolean(activeTab.workspacePath)}
        onResolveURL={(url) => updateTab(activeTab.id, { address: url, url })}
        onRetry={reload}
      />
    </section>
  )
}

export function WorkbenchHeaderActions({
  maximized,
  onToggleMaximized,
  onOpenBrowser,
}: {
  maximized: boolean
  onToggleMaximized: () => void
  onOpenBrowser: () => void
}) {
  const { t } = useI18n()
  const ExpandIcon = maximized ? Minimize2 : Maximize2

  return (
    <div className="ml-1 flex shrink-0 items-center gap-0.5">
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
            <WorkbenchMenuItem icon={MessageSquare} label={t('workbench.chat')} disabled />
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
    </div>
  )
}

function WorkbenchMenuItem({
  disabled,
  icon: Icon,
  label,
  onSelect,
}: {
  disabled?: boolean
  icon: typeof Globe2
  label: string
  onSelect?: () => void
}) {
  return (
    <DropdownMenu.Item
      className="mb-0.5 flex h-[30px] cursor-default select-none items-center gap-2.5 rounded-[10px] px-2.5 outline-none last:mb-0 data-[disabled]:opacity-40 data-[highlighted]:bg-[rgb(241,241,241)]"
      disabled={disabled}
      onSelect={onSelect}
    >
      <Icon className="size-4 shrink-0 text-stone-600" aria-hidden="true" />
      <span>{label}</span>
    </DropdownMenu.Item>
  )
}
