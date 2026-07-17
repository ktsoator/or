import { useEffect, useRef, useState } from 'react'
import {
  ArrowDown,
  Braces,
  CircleAlert,
  LoaderCircle,
  PanelLeft,
  ShieldAlert,
  SquarePen,
  TerminalSquare,
} from 'lucide-react'
import { useSession } from './useSession'
import type { ConnectionStatus, Item, SessionSummary } from './types'
import { cn } from './lib/utils'
import { Markdown } from './components/Markdown'
import { ToolCard } from './components/ToolCard'
import { Composer } from './components/Composer'
import { Thinking } from './components/Thinking'

const statusText: Record<ConnectionStatus, string> = {
  connecting: 'Connecting',
  ready: 'Connected',
  disconnected: 'Offline',
}

function ConnectionState({ status }: { status: ConnectionStatus }) {
  return (
    <div
      className={cn(
        'flex items-center gap-1.5 text-[11.5px] font-medium text-stone-500',
        status === 'disconnected' && 'text-red-600',
      )}
      title={`coding API: ${statusText[status]}`}
    >
      <span
        className={cn(
          'size-1.5 rounded-full bg-stone-400',
          status === 'ready' && 'bg-emerald-600',
          status === 'disconnected' && 'bg-red-600',
          status === 'connecting' && 'animate-pulse',
        )}
      />
      <span className="max-md:hidden">{statusText[status]}</span>
    </div>
  )
}

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
    selectSession,
    send,
    stop,
    resolveConfirm,
  } = useSession()
  const logRef = useRef<HTMLDivElement>(null)
  const [mobileSessionsOpen, setMobileSessionsOpen] = useState(false)

  useEffect(() => {
    const el = logRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [activeSessionID, items])

  const scrollToLatest = () => {
    logRef.current?.scrollTo({ top: logRef.current.scrollHeight, behavior: 'smooth' })
  }

  const chooseSession = (id: string) => {
    selectSession(id)
    setMobileSessionsOpen(false)
  }

  const addSession = () => {
    void createSession()
      .then(() => setMobileSessionsOpen(false))
      .catch(() => undefined)
  }

  return (
    <div className="grid h-full grid-cols-[232px_minmax(0,1fr)] grid-rows-[minmax(0,1fr)] overflow-hidden bg-[#fcfcfb] max-md:grid-cols-1">
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
          'z-50 flex min-h-0 flex-col border-r border-stone-200/70 bg-[#f5f5f3] text-stone-600 transition-transform duration-200 ease-out',
          'max-md:fixed max-md:inset-y-0 max-md:left-0 max-md:w-[268px] max-md:shadow-2xl',
          mobileSessionsOpen ? 'max-md:translate-x-0' : 'max-md:-translate-x-full',
        )}
        aria-label="Sessions"
      >
        <div className="flex h-13 shrink-0 items-center justify-between px-3.5">
          <div className="flex min-w-0 items-center gap-2">
            <Braces className="size-4 shrink-0 stroke-[1.65] text-stone-700" aria-hidden="true" />
            <span className="truncate text-[13px] font-[620] tracking-[-0.015em] text-stone-900">
              OR coding
            </span>
          </div>
          <button
            className="grid size-7 shrink-0 cursor-pointer place-items-center rounded-md text-stone-500 transition-colors hover:bg-stone-200/80 hover:text-stone-950 disabled:cursor-wait disabled:opacity-40"
            type="button"
            title="New session"
            disabled={creating}
            onClick={addSession}
          >
            {creating ? (
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
            ) : (
              <SquarePen className="size-3.5 stroke-[1.7]" aria-hidden="true" />
            )}
            <span className="sr-only">New session</span>
          </button>
        </div>

        <div className="px-4 pt-2 pb-1.5 text-[10px] font-medium tracking-[0.025em] text-stone-400">
          Recent
        </div>
        <nav className="min-h-0 flex-1 overflow-y-auto px-2.5 pb-3" aria-label="Coding sessions">
          <div className="space-y-px">
            {sessions.map((session) => (
              <SessionRow
                key={session.id}
                session={session}
                active={session.id === activeSessionID}
                onSelect={() => chooseSession(session.id)}
              />
            ))}
          </div>
        </nav>

        <div className="shrink-0 px-4 py-3.5">
          <div className="text-[10.5px] text-stone-400">
            Local workspace
          </div>
        </div>
      </aside>

      <div className="relative flex h-full min-w-0 flex-col">
        <header className="z-20 flex h-13 shrink-0 items-center justify-between border-b border-stone-200/80 bg-[#fcfcfb] px-6 max-md:h-12 max-md:px-4">
          <div className="flex min-w-0 items-center gap-2.5">
            <button
              className="-ml-1 grid size-7 shrink-0 place-items-center rounded-md text-stone-500 transition-colors hover:bg-stone-100 hover:text-stone-900 md:hidden"
              type="button"
              title="Sessions"
              onClick={() => setMobileSessionsOpen(true)}
            >
              <PanelLeft className="size-4" aria-hidden="true" />
              <span className="sr-only">Open sessions</span>
            </button>
            <span
              className="truncate text-sm font-[620] tracking-[-0.015em] text-stone-900"
              title={activeSession?.title}
            >
              {activeSession?.title ?? 'OR coding'}
            </span>
          </div>
          <div className="flex items-center gap-3.5 text-stone-500">
            <TerminalSquare className="size-4 stroke-[1.7]" aria-hidden="true" />
            <ConnectionState status={status} />
          </div>
        </header>

        <main ref={logRef} className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto scroll-smooth">
          <div
            className={cn(
              'mx-auto min-h-full w-full max-w-[944px] px-6 py-8 pb-12 max-md:px-4 max-md:py-6',
              (loading || items.length === 0) && 'grid place-items-center',
            )}
          >
            {loading ? (
              <div className="flex items-center gap-2 pb-[8vh] text-xs text-stone-400">
                <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
                Loading session
              </div>
            ) : items.length === 0 ? (
              <div className="max-w-md pb-[8vh] text-center">
                <Braces className="mx-auto mb-5 size-5 text-stone-500" aria-hidden="true" />
                <h1 className="m-0 text-xl font-[590] tracking-[-0.025em] text-stone-900">
                  What should we work on?
                </h1>
                <p className="mt-2 text-sm leading-6 text-stone-500">
                  Ask for a change, an explanation, or a closer look at the codebase.
                </p>
              </div>
            ) : (
              items.map((item) => <ThreadItem key={item.id} item={item} />)
            )}
          </div>
        </main>

        {items.length > 0 && (
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

        <Composer
          connected={status === 'ready'}
          running={running}
          confirmation={confirmation}
          onSend={send}
          onStop={stop}
          onResolve={resolveConfirm}
        />
      </div>
    </div>
  )
}

function SessionRow({
  session,
  active,
  onSelect,
}: {
  session: SessionSummary
  active: boolean
  onSelect: () => void
}) {
  return (
    <button
      className={cn(
        'group flex w-full cursor-pointer items-start rounded-[7px] px-2.5 py-2 text-left transition-colors',
        active
          ? 'bg-stone-200/75 text-stone-950'
          : 'text-stone-600 hover:bg-stone-200/45 hover:text-stone-900',
      )}
      type="button"
      aria-current={active ? 'page' : undefined}
      onClick={onSelect}
    >
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[12.5px] font-[520] leading-4.5" title={session.title}>
          {session.title}
        </span>
        <span className="mt-0.5 flex items-center gap-1.5 text-[10px] leading-4 text-stone-400">
          {session.hasApproval ? (
            <>
              <ShieldAlert className="size-3 text-amber-700" aria-hidden="true" />
              Approval needed
            </>
          ) : session.running ? (
            <>
              <LoaderCircle className="size-3 animate-spin text-stone-500" aria-hidden="true" />
              Working
            </>
          ) : (
            formatSessionTime(session.updatedAt)
          )}
        </span>
      </span>
    </button>
  )
}

function formatSessionTime(value: string): string {
  const timestamp = new Date(value).getTime()
  if (!Number.isFinite(timestamp)) return ''
  const elapsed = Math.max(0, Date.now() - timestamp)
  const minutes = Math.floor(elapsed / 60_000)
  if (minutes < 1) return 'Just now'
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 7) return `${days}d ago`
  return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric' }).format(timestamp)
}

function ThreadItem({ item }: { item: Item }) {
  switch (item.kind) {
    case 'user':
      return (
        <section className="my-5 flex animate-[fade-in_160ms_ease-out] justify-end">
          <div className="max-w-[78%] rounded-xl bg-stone-100 px-3.5 py-2.5 text-[15.5px] leading-[1.58] whitespace-pre-wrap max-md:max-w-[88%]">
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
            <strong className="text-xs font-semibold">Something went wrong</strong>
            <span className="text-xs">{item.text}</span>
          </div>
        </div>
      )
  }
}
