import { useLayoutEffect, useRef, useState } from 'react'
import type { LucideIcon } from 'lucide-react'
import {
  ArrowDown,
  CircleAlert,
  Clock3,
  Ellipsis,
  Files,
  FolderOpen,
  LoaderCircle,
  PanelLeft,
  Search,
  ShieldAlert,
  SquarePen,
  Trash2,
  Wrench,
} from 'lucide-react'
import { useSession } from './useSession'
import type { Item, SessionSummary } from './types'
import { cn } from './lib/utils'
import { Markdown } from './components/Markdown'
import { ToolCard } from './components/ToolCard'
import { Composer } from './components/Composer'
import { Thinking } from './components/Thinking'

export default function App() {
  const {
    sessions,
    activeSession,
    activeSessionID,
    items,
    confirmation,
    running,
    loading,
    creating,
    status,
    createSession,
    deleteSession,
    selectSession,
    send,
    stop,
    resolveConfirm,
  } = useSession()
  const logRef = useRef<HTMLDivElement>(null)
  const followLatestRef = useRef(true)
  const previousSessionIDRef = useRef<string | undefined>(undefined)
  const [mobileSessionsOpen, setMobileSessionsOpen] = useState(false)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [activeShortcut, setActiveShortcut] = useState<string>()
  const [atLatest, setAtLatest] = useState(true)
  const [deleteTarget, setDeleteTarget] = useState<SessionSummary>()
  const [deleting, setDeleting] = useState(false)
  const [deleteError, setDeleteError] = useState('')

  useLayoutEffect(() => {
    const el = logRef.current
    if (!el) return

    const sessionChanged = previousSessionIDRef.current !== activeSessionID
    previousSessionIDRef.current = activeSessionID
    if (sessionChanged) followLatestRef.current = true

    if (followLatestRef.current) {
      el.scrollTop = el.scrollHeight
      setAtLatest(true)
    }
  }, [activeSessionID, items])

  const toggleSidebar = () => {
    if (mobileSessionsOpen) {
      setMobileSessionsOpen(false)
      return
    }
    setSidebarCollapsed((collapsed) => !collapsed)
  }

  const scrollToLatest = () => {
    followLatestRef.current = true
    setAtLatest(true)
    logRef.current?.scrollTo({ top: logRef.current.scrollHeight, behavior: 'smooth' })
  }

  const trackScrollPosition = () => {
    const el = logRef.current
    if (!el) return
    const latest = el.scrollHeight - el.scrollTop - el.clientHeight < 72
    followLatestRef.current = latest
    setAtLatest(latest)
  }

  const chooseSession = (id: string) => {
    setActiveShortcut(undefined)
    selectSession(id)
    setMobileSessionsOpen(false)
  }

  const addSession = () => {
    setActiveShortcut(undefined)
    void createSession()
      .then(() => setMobileSessionsOpen(false))
      .catch(() => undefined)
  }

  const requestDelete = (session: SessionSummary) => {
    setDeleteError('')
    setDeleteTarget(session)
  }

  const confirmDelete = async () => {
    if (!deleteTarget || deleteTarget.running || deleteTarget.hasApproval) return
    setDeleting(true)
    setDeleteError('')
    try {
      await deleteSession(deleteTarget.id)
      setDeleteTarget(undefined)
      setMobileSessionsOpen(false)
    } catch (error) {
      setDeleteError(error instanceof Error ? error.message : 'Could not delete the session')
    } finally {
      setDeleting(false)
    }
  }

  const emptySession = !loading && items.length === 0 && !confirmation

  const composer = (centered = false) => (
    <Composer
      connected={status === 'ready'}
      running={running}
      confirmation={confirmation}
      centered={centered}
      onSend={send}
      onStop={stop}
      onResolve={resolveConfirm}
    />
  )

  return (
    <div
      className={cn(
        'grid h-full grid-rows-[minmax(0,1fr)] overflow-hidden bg-[#fcfcfb] transition-[grid-template-columns] duration-200 ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none max-md:grid-cols-1',
        sidebarCollapsed
          ? 'grid-cols-[64px_minmax(0,1fr)]'
          : 'grid-cols-[256px_minmax(0,1fr)]',
      )}
    >
      {mobileSessionsOpen && (
        <button
          className="fixed inset-0 z-40 bg-stone-950/15 backdrop-blur-[1px] md:hidden"
          type="button"
          aria-label="Close sessions"
          onClick={() => setMobileSessionsOpen(false)}
        />
      )}
      <aside
        className={cn(
          'z-50 flex min-h-0 min-w-0 flex-col overflow-hidden border-r border-stone-200/75 bg-[#f7f7f5] text-stone-700 transition-transform duration-200 ease-out',
          'max-md:fixed max-md:inset-y-0 max-md:left-0 max-md:w-[280px] max-md:shadow-2xl',
          mobileSessionsOpen ? 'max-md:translate-x-0' : 'max-md:-translate-x-full',
        )}
        aria-label="Sessions"
      >
        <div className="relative h-16 w-[256px] shrink-0 max-md:w-[280px]">
          <div
            className={cn(
              'absolute inset-y-0 left-4 flex items-center whitespace-nowrap transition-opacity duration-100 ease-out motion-reduce:transition-none',
              sidebarCollapsed ? 'opacity-0' : 'opacity-100',
            )}
          >
            <span className="truncate text-[15.5px] font-[640] tracking-[-0.02em] text-stone-950">
              OR coding
            </span>
          </div>
          <button
            className={cn(
              'absolute top-4 right-14 grid size-8 cursor-pointer place-items-center rounded-lg text-stone-500 transition-[opacity,color,background-color,transform] duration-100 ease-out motion-reduce:transition-none hover:bg-stone-200/75 hover:text-stone-950 active:scale-95 focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-stone-400',
              sidebarCollapsed ? 'pointer-events-none opacity-0' : 'opacity-100',
              activeShortcut === 'Search' && 'bg-stone-200/80 text-stone-950',
            )}
            type="button"
            title="Search sessions"
            aria-label="Search sessions"
            aria-pressed={activeShortcut === 'Search'}
            onClick={() =>
              setActiveShortcut((current) => (current === 'Search' ? undefined : 'Search'))
            }
          >
            <Search className="size-[18px]" aria-hidden="true" />
          </button>
          <button
            className={cn(
              'absolute top-4 left-4 grid size-8 cursor-pointer place-items-center rounded-lg text-stone-500 transition-[transform,color,background-color] duration-200 ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none hover:bg-stone-200/75 hover:text-stone-950',
              sidebarCollapsed ? 'translate-x-0' : 'translate-x-[192px]',
              'max-md:translate-x-[216px]',
            )}
            type="button"
            title={sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
            aria-label={sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
            aria-expanded={!sidebarCollapsed}
            onClick={toggleSidebar}
          >
            <PanelLeft className="size-[18px]" aria-hidden="true" />
          </button>
        </div>

        <div className="w-[256px] shrink-0 px-3 pb-3 max-md:w-[280px]">
          <button
            className={cn(
              'flex h-11 w-full cursor-pointer items-center gap-3 rounded-xl px-3 text-left text-[14.5px] font-[540] text-stone-950 transition-colors duration-100 motion-reduce:transition-none disabled:cursor-wait disabled:opacity-50',
              sidebarCollapsed ? 'bg-transparent' : 'bg-stone-200/75 hover:bg-stone-200',
            )}
            type="button"
            title="New session"
            disabled={creating}
            onClick={addSession}
          >
            {creating ? (
              <LoaderCircle className="size-[18px] shrink-0 animate-spin" aria-hidden="true" />
            ) : (
              <SquarePen className="size-[18px] shrink-0" aria-hidden="true" />
            )}
            <span
              className={cn(
                'whitespace-nowrap transition-opacity duration-100 ease-out motion-reduce:transition-none',
                sidebarCollapsed ? 'opacity-0' : 'opacity-100',
              )}
            >
              New session
            </span>
          </button>

          <div className="mt-1 space-y-0.5" aria-label="Workspace shortcuts">
            <SidebarNavItem
              icon={FolderOpen}
              label="Workspace"
              collapsed={sidebarCollapsed}
              active={activeShortcut === 'Workspace'}
              onClick={() => setActiveShortcut('Workspace')}
            />
            <SidebarNavItem
              icon={Files}
              label="Files"
              collapsed={sidebarCollapsed}
              active={activeShortcut === 'Files'}
              onClick={() => setActiveShortcut('Files')}
            />
            <SidebarNavItem
              icon={Clock3}
              label="Scheduled"
              collapsed={sidebarCollapsed}
              active={activeShortcut === 'Scheduled'}
              onClick={() => setActiveShortcut('Scheduled')}
            />
            <SidebarNavItem
              icon={Wrench}
              label="Tools"
              collapsed={sidebarCollapsed}
              active={activeShortcut === 'Tools'}
              onClick={() => setActiveShortcut('Tools')}
            />
            <SidebarNavItem
              icon={Ellipsis}
              label="More"
              collapsed={sidebarCollapsed}
              active={activeShortcut === 'More'}
              onClick={() => setActiveShortcut('More')}
            />
          </div>
        </div>

        <div
          className={cn(
            'w-[256px] px-5 pt-2 pb-2 text-[13px] font-[620] tracking-[-0.01em] whitespace-nowrap text-stone-900 transition-opacity duration-100 ease-out motion-reduce:transition-none max-md:w-[280px]',
            sidebarCollapsed ? 'pointer-events-none opacity-0' : 'opacity-100',
          )}
        >
          Recents
        </div>
        <nav
          className={cn(
            'min-h-0 w-[256px] flex-1 overflow-y-auto px-3 pb-3 transition-opacity duration-100 ease-out motion-reduce:transition-none max-md:w-[280px]',
            sidebarCollapsed ? 'pointer-events-none opacity-0' : 'opacity-100',
          )}
          aria-hidden={sidebarCollapsed}
          aria-label="Coding sessions"
        >
          <div className="space-y-px">
            {sessions.map((session) => (
              <SessionRow
                key={session.id}
                session={session}
                active={session.id === activeSessionID}
                onSelect={() => chooseSession(session.id)}
                onDelete={() => requestDelete(session)}
              />
            ))}
          </div>
        </nav>

        <div className="w-[256px] shrink-0 border-t border-stone-200/70 p-3 max-md:w-[280px]">
          <div className="flex items-center gap-3 rounded-xl px-1 py-1.5">
            <span
              className="grid size-8 shrink-0 place-items-center rounded-full bg-stone-900 text-[13px] font-[650] text-white shadow-[inset_0_0_0_1px_rgba(255,255,255,0.12)]"
              aria-hidden="true"
            >
              K
            </span>
            <span
              className={cn(
                'truncate whitespace-nowrap text-[14px] font-[560] text-stone-900 transition-opacity duration-100 ease-out motion-reduce:transition-none',
                sidebarCollapsed ? 'opacity-0' : 'opacity-100',
              )}
            >
              ktsoator
            </span>
          </div>
        </div>
      </aside>

      <div className="relative flex h-full min-w-0 flex-col">
        <header className="z-20 flex h-13 shrink-0 items-center border-b border-stone-200/80 bg-[#fcfcfb] px-6 max-md:h-12 max-md:px-4">
          <div className="flex min-w-0 items-center gap-2.5">
            <button
              className="-ml-1 grid size-7 shrink-0 place-items-center rounded-md text-stone-500 transition-colors hover:bg-stone-100 hover:text-stone-900 md:hidden"
              type="button"
              title="Sessions"
              onClick={() => {
                setSidebarCollapsed(false)
                setMobileSessionsOpen(true)
              }}
            >
              <PanelLeft className="size-4" aria-hidden="true" />
              <span className="sr-only">Open sessions</span>
            </button>
            <span
              className="truncate text-[15px] font-[620] tracking-[-0.015em] text-stone-900"
              title={activeSession?.title}
            >
              {activeSession?.title ?? 'OR coding'}
            </span>
          </div>
        </header>

        <main
          ref={logRef}
          className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto"
          onScroll={trackScrollPosition}
        >
          <div
            className={cn(
              'mx-auto min-h-full w-full max-w-[944px] px-6 py-8 pb-12 max-md:px-4 max-md:py-6',
              (loading || emptySession) && 'grid place-items-center',
            )}
          >
            {loading ? (
              <div className="flex items-center gap-2 pb-[8vh] text-xs text-stone-400">
                <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
                Loading session
              </div>
            ) : emptySession ? (
              <div className="flex w-full -translate-y-[3vh] flex-col items-center gap-9">
                <div className="max-w-lg text-center">
                  <h1 className="m-0 text-[28px] leading-tight font-[560] tracking-[-0.03em] text-stone-900 max-sm:text-2xl">
                    What should we work on?
                  </h1>
                  <p className="mt-2.5 text-[15px] leading-6 text-stone-500">
                    Ask for a change, an explanation, or a closer look at the codebase.
                  </p>
                </div>
                {composer(true)}
              </div>
            ) : (
              items.map((item) => <ThreadItem key={item.id} item={item} />)
            )}
          </div>
        </main>

        {items.length > 0 && !atLatest && (
          <button
            className={cn(
              'absolute right-1/2 z-40 grid size-7 translate-x-1/2 place-items-center rounded-md border border-stone-300 bg-white/95 text-stone-500 transition-all hover:border-stone-400 hover:text-stone-900 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-400',
              confirmation ? 'bottom-[232px] max-sm:bottom-[266px]' : 'bottom-[148px]',
            )}
            type="button"
            onClick={scrollToLatest}
            title="Jump to latest"
          >
            <ArrowDown className="size-3.5" aria-hidden="true" />
            <span className="sr-only">Jump to latest</span>
          </button>
        )}

        {!loading && !emptySession && composer()}
      </div>

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
    </div>
  )
}

function SessionRow({
  session,
  active,
  onSelect,
  onDelete,
}: {
  session: SessionSummary
  active: boolean
  onSelect: () => void
  onDelete: () => void
}) {
  return (
    <div className="group relative">
      <button
        className={cn(
          'flex min-h-9 w-full cursor-pointer items-center rounded-lg px-3 py-2 pr-9 text-left transition-colors',
          active
            ? 'bg-stone-200/75 text-stone-950'
            : 'text-stone-700 hover:bg-stone-200/55 hover:text-stone-950',
        )}
        type="button"
        aria-current={active ? 'page' : undefined}
        onClick={onSelect}
      >
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[14px] font-[510] leading-5" title={session.title}>
            {session.title}
          </span>
          {(session.hasApproval || session.running) && (
            <span className="mt-0.5 flex items-center gap-1.5 text-[11.5px] leading-4 text-stone-500">
              {session.hasApproval ? (
              <>
                <ShieldAlert className="size-3 text-amber-700" aria-hidden="true" />
                Approval needed
              </>
              ) : (
              <>
                <LoaderCircle className="size-3 animate-spin text-stone-500" aria-hidden="true" />
                Working
              </>
              )}
            </span>
          )}
        </span>
      </button>
      <button
        className="absolute top-1.5 right-1.5 grid size-7 cursor-pointer place-items-center rounded-md text-stone-400 opacity-0 transition-[opacity,color,background-color] group-hover:opacity-100 hover:bg-stone-300/60 hover:text-red-700 focus-visible:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-stone-400 max-md:opacity-100"
        type="button"
        title={`Delete ${session.title}`}
        aria-label={`Delete ${session.title}`}
        onClick={onDelete}
      >
        <Trash2 className="size-3.5" aria-hidden="true" />
      </button>
    </div>
  )
}

function SidebarNavItem({
  icon: Icon,
  label,
  collapsed = false,
  active = false,
  onClick,
}: {
  icon: LucideIcon
  label: string
  collapsed?: boolean
  active?: boolean
  onClick: () => void
}) {
  return (
    <button
      className={cn(
        'group flex h-10 w-full cursor-pointer items-center gap-3 rounded-lg px-3 text-left text-[14.5px] font-[480] text-stone-800 transition-[background-color,color,transform] duration-100 active:scale-[0.985] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-stone-400',
        !collapsed && (active ? 'bg-stone-200/80 text-stone-950' : 'hover:bg-stone-200/55'),
      )}
      type="button"
      title={label}
      aria-pressed={active}
      onClick={onClick}
    >
      <span className="relative shrink-0">
        <span
          className={cn(
            'pointer-events-none absolute -inset-2 rounded-lg transition-colors duration-100',
            collapsed && (active ? 'bg-stone-200/80' : 'group-hover:bg-stone-200/65'),
          )}
          aria-hidden="true"
        />
        <Icon
          className="relative size-[18px] text-stone-700"
          strokeWidth={1.85}
          aria-hidden="true"
        />
      </span>
      <span
        className={cn(
          'whitespace-nowrap transition-opacity duration-100 ease-out motion-reduce:transition-none',
          collapsed ? 'opacity-0' : 'opacity-100',
        )}
      >
        {label}
      </span>
    </button>
  )
}

function DeleteSessionDialog({
  session,
  deleting,
  error,
  onCancel,
  onConfirm,
}: {
  session: SessionSummary
  deleting: boolean
  error: string
  onCancel: () => void
  onConfirm: () => void
}) {
  const blocked = session.running || session.hasApproval
  return (
    <div
      className="fixed inset-0 z-[100] grid place-items-center bg-stone-950/20 px-4 backdrop-blur-[1px]"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onCancel()
      }}
    >
      <section
        className="w-full max-w-[380px] animate-[fade-in_140ms_ease-out] rounded-xl border border-stone-300 bg-[#fffefa] p-4 shadow-[0_18px_55px_-24px_rgba(28,25,23,0.55)]"
        role="dialog"
        aria-modal="true"
        aria-labelledby="delete-session-title"
      >
        <div className="flex items-start gap-3">
          <span className="mt-0.5 grid size-7 shrink-0 place-items-center rounded-full bg-red-50 text-red-700">
            <Trash2 className="size-3.5" aria-hidden="true" />
          </span>
          <div className="min-w-0">
            <h2 id="delete-session-title" className="text-sm font-semibold text-stone-900">
              Delete session?
            </h2>
            <p className="mt-1 text-xs leading-5 text-stone-500">
              “{session.title}” and its stored tool details will be permanently removed.
            </p>
          </div>
        </div>

        {blocked && (
          <div className="mt-3 rounded-md bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-900">
            Stop this session or resolve its approval request before deleting it.
          </div>
        )}
        {error && <div className="mt-3 text-xs leading-5 text-red-700">{error}</div>}

        <div className="mt-4 flex justify-end gap-2">
          <button
            className="h-8 cursor-pointer rounded-md px-3 text-xs font-semibold text-stone-600 transition-colors hover:bg-stone-100 hover:text-stone-900 disabled:cursor-wait disabled:opacity-50"
            type="button"
            disabled={deleting}
            onClick={onCancel}
          >
            Cancel
          </button>
          <button
            className="h-8 cursor-pointer rounded-md bg-red-700 px-3 text-xs font-semibold text-white transition-colors hover:bg-red-800 disabled:cursor-not-allowed disabled:opacity-35"
            type="button"
            disabled={deleting || blocked}
            onClick={onConfirm}
          >
            {deleting ? 'Deleting…' : 'Delete'}
          </button>
        </div>
      </section>
    </div>
  )
}

function ThreadItem({ item }: { item: Item }) {
  switch (item.kind) {
    case 'user':
      return (
        <section className="my-5 flex animate-[fade-in_160ms_ease-out] justify-end">
          <div className="max-w-[78%] rounded-xl bg-stone-100 px-3.5 py-2.5 text-[16.5px] leading-[1.58] whitespace-pre-wrap max-md:max-w-[88%]">
            {item.text}
          </div>
        </section>
      )
    case 'assistant':
      return (
        <section className="my-4 animate-[fade-in_160ms_ease-out]">
          <Markdown source={item.markdown} />
        </section>
      )
    case 'thinking':
      return <Thinking item={item} />
    case 'tool':
      return <ToolCard item={item} />
    case 'confirm':
      return null
    case 'error':
      return (
        <div
          className="my-4 flex animate-[fade-in_160ms_ease-out] gap-2.5 border-l-2 border-red-300 py-1 pl-3 text-red-700"
          role="alert"
        >
          <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
          <div className="flex flex-col gap-0.5">
            <strong className="text-[13px] font-semibold">Something went wrong</strong>
            <span className="text-[13px]">{item.text}</span>
          </div>
        </div>
      )
  }
}
