import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from 'react'
import { usePointerResize } from '@/shared/hooks/usePointerResize'
import {
  DEFAULT_SIDEBAR_WIDTH,
  MAX_SIDEBAR_WIDTH,
  MIN_SIDEBAR_WIDTH,
  keyboardSidebarWidth,
  resizedSidebarWidth,
} from '@/shared/lib/sidebarLayout'
import type { SessionSummary, WorkspaceSummary } from '@/types'
import {
  groupSidebarSessions,
  parsePinnedSessionIDs,
} from './sessionSidebarLayout'

const PINNED_SESSIONS_KEY = 'coding.pinned-session-ids'
const SIDEBAR_WIDTH_KEY = 'coding.sidebar-width'

function storedSidebarWidth(): number {
  if (typeof localStorage === 'undefined') return DEFAULT_SIDEBAR_WIDTH
  const storedValue = localStorage.getItem(SIDEBAR_WIDTH_KEY)
  if (storedValue === null) return DEFAULT_SIDEBAR_WIDTH
  const value = Number(storedValue)
  return Number.isFinite(value)
    ? Math.min(MAX_SIDEBAR_WIDTH, Math.max(MIN_SIDEBAR_WIDTH, value))
    : DEFAULT_SIDEBAR_WIDTH
}

export function useSidebarLayout(
  sessions: SessionSummary[],
  workspaces: WorkspaceSummary[],
) {
  const [mobileSessionsOpen, setMobileSessionsOpen] = useState(false)
  const [collapsed, setCollapsed] = useState(false)
  const [width, setWidth] = useState(storedSidebarWidth)
  const [pinnedSessionIDs, setPinnedSessionIDs] = useState(() =>
    parsePinnedSessionIDs(
      typeof localStorage === 'undefined'
        ? null
        : localStorage.getItem(PINNED_SESSIONS_KEY),
    ),
  )
  const pinnedSessionIDSet = useMemo(
    () => new Set(pinnedSessionIDs),
    [pinnedSessionIDs],
  )
  const { chatSessions, workspaceGroups } = useMemo(
    () => groupSidebarSessions(sessions, workspaces, pinnedSessionIDSet),
    [pinnedSessionIDSet, sessions, workspaces],
  )

  useEffect(() => {
    localStorage.setItem(PINNED_SESSIONS_KEY, JSON.stringify(pinnedSessionIDs))
  }, [pinnedSessionIDs])

  useEffect(() => {
    localStorage.setItem(SIDEBAR_WIDTH_KEY, String(Math.round(width)))
  }, [width])

  const toggleSidebar = useCallback(() => {
    if (mobileSessionsOpen) {
      setMobileSessionsOpen(false)
      return
    }
    setCollapsed((current) => !current)
  }, [mobileSessionsOpen])

  const expandSidebar = useCallback(() => {
    setCollapsed(false)
  }, [])

  const openMobileSessions = useCallback(() => {
    setCollapsed(false)
    setMobileSessionsOpen(true)
  }, [])

  const closeMobileSessions = useCallback(() => {
    setMobileSessionsOpen(false)
  }, [])

  const togglePinnedSession = useCallback((id: string) => {
    setPinnedSessionIDs((current) =>
      current.includes(id)
        ? current.filter((sessionID) => sessionID !== id)
        : [...current, id],
    )
  }, [])

  const removePinnedSession = useCallback((id: string) => {
    setPinnedSessionIDs((current) => current.filter((sessionID) => sessionID !== id))
  }, [])

  const beginResize = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    if (collapsed) return
    return {
      startX: event.clientX,
      startWidth: width,
    }
  }, [collapsed, width])

  const updateResize = useCallback((
    currentResize: { startX: number; startWidth: number },
    clientX: number,
  ) => {
    setWidth(resizedSidebarWidth(
      currentResize.startWidth,
      currentResize.startX,
      clientX,
    ))
  }, [])

  const {
    resizing,
    startResize,
    resize,
    stopResize,
  } = usePointerResize({ start: beginResize, move: updateResize })

  const resizeWithKeyboard = useCallback((event: ReactKeyboardEvent<HTMLDivElement>) => {
    const nextWidth = keyboardSidebarWidth(event.key, width)
    if (nextWidth === undefined) return
    event.preventDefault()
    setWidth(nextWidth)
  }, [width])

  return {
    mobileSessionsOpen,
    collapsed,
    width,
    resizing,
    pinnedSessionIDSet,
    chatSessions,
    workspaceGroups,
    minimumWidth: MIN_SIDEBAR_WIDTH,
    maximumWidth: MAX_SIDEBAR_WIDTH,
    toggleSidebar,
    expandSidebar,
    openMobileSessions,
    closeMobileSessions,
    togglePinnedSession,
    removePinnedSession,
    startResize,
    resize,
    stopResize,
    resizeWithKeyboard,
  }
}
