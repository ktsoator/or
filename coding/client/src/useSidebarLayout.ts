import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from 'react'
import { usePointerResize } from './shared/hooks/usePointerResize'
import type { SessionSummary, WorkspaceSummary } from './types'
import {
  DEFAULT_SIDEBAR_WIDTH,
  MAX_SIDEBAR_WIDTH,
  MIN_SIDEBAR_WIDTH,
  groupSidebarSessions,
  keyboardSidebarWidth,
  parsePinnedSessionIDs,
  resizedSidebarWidth,
} from './sidebarLayout'

const PINNED_SESSIONS_KEY = 'coding.pinned-session-ids'

export function useSidebarLayout(
  sessions: SessionSummary[],
  workspaces: WorkspaceSummary[],
) {
  const [mobileSessionsOpen, setMobileSessionsOpen] = useState(false)
  const [collapsed, setCollapsed] = useState(false)
  const [width, setWidth] = useState(DEFAULT_SIDEBAR_WIDTH)
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
    event: ReactPointerEvent<HTMLDivElement>,
  ) => {
    setWidth(resizedSidebarWidth(
      currentResize.startWidth,
      currentResize.startX,
      event.clientX,
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
