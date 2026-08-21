import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from 'react'
import {
  DEFAULT_WORKBENCH_RATIO,
  MIN_CHAT_WIDTH,
  MIN_WORKBENCH_WIDTH,
  clampWorkbenchWidth,
  isWorkbenchConstrained,
  keyboardWorkbenchWidth,
  resizedWorkbenchWidth,
  workbenchWidthBounds,
} from './layout'
import { usePointerResize } from '@/shared/hooks/usePointerResize'

type UseWorkbenchLayoutOptions = {
  enabled: boolean
  activeSessionID?: string
  activeDraftID?: string
  primaryPreviewRevision?: number
  primaryPreviewOpen: boolean
  secondarySessionID?: string
  secondaryPreviewRevision?: number
  secondaryPreviewOpen: boolean
}

const WORKBENCH_CLOSE_TRANSITION_MS = 180
const WORKBENCH_WIDTH_KEY = 'coding.workbench-width'

function storedWorkbenchWidth(): number | undefined {
  if (typeof localStorage === 'undefined') return undefined
  const value = Number(localStorage.getItem(WORKBENCH_WIDTH_KEY))
  return Number.isFinite(value) && value > 0 ? value : undefined
}

export function useWorkbenchLayout({
  enabled,
  activeSessionID,
  activeDraftID,
  primaryPreviewRevision,
  primaryPreviewOpen,
  secondarySessionID,
  secondaryPreviewRevision,
  secondaryPreviewOpen,
}: UseWorkbenchLayoutOptions) {
  const layoutRef = useRef<HTMLDivElement>(null)
  const viewportRef = useRef<HTMLElement>(null)
  const widthRef = useRef<number | undefined>(undefined)
  const preferredWidthRef = useRef<number | undefined>(storedWorkbenchWidth())
  const constrainedRef = useRef(false)
  const openRef = useRef(false)
  const maximizedRef = useRef(false)
  const resizingRef = useRef(false)
  const previousPreviewKeysRef = useRef<{
    primary?: string
    secondary?: string
  }>({})
  const autoCollapsedRef = useRef(false)
  const autoLayoutFrameRef = useRef<number | undefined>(undefined)
  const closeTransitionTimerRef = useRef<number | undefined>(undefined)

  const [open, setOpenState] = useState(false)
  const [previewSessionID, setPreviewSessionID] = useState<string>()
  const [width, setWidth] = useState<number>()
  const [maximized, setMaximized] = useState(false)
  const [autoLayoutChanging, setAutoLayoutChanging] = useState(false)
  const [closing, setClosing] = useState(false)

  const setOpen = useCallback((nextOpen: boolean) => {
    if (nextOpen) {
      if (closeTransitionTimerRef.current !== undefined) {
        window.clearTimeout(closeTransitionTimerRef.current)
        closeTransitionTimerRef.current = undefined
      }
      setClosing(false)
    } else {
      setMaximized(false)
    }
    setOpenState(nextOpen)
  }, [])

  widthRef.current = width
  openRef.current = open
  maximizedRef.current = maximized

  const toggle = useCallback(() => {
    autoCollapsedRef.current = false
    if (open) {
      setClosing(true)
      if (closeTransitionTimerRef.current !== undefined) {
        window.clearTimeout(closeTransitionTimerRef.current)
      }
      closeTransitionTimerRef.current = window.setTimeout(() => {
        closeTransitionTimerRef.current = undefined
        setClosing(false)
      }, WORKBENCH_CLOSE_TRANSITION_MS)
      setOpen(false)
      return
    }
    if (constrainedRef.current) setMaximized(true)
    setOpen(true)
  }, [open, setOpen])

  const showSession = useCallback((sessionID: string) => {
    setPreviewSessionID(sessionID)
    autoCollapsedRef.current = false
    if (constrainedRef.current) setMaximized(true)
    setOpen(true)
  }, [setOpen])

  const toggleMaximized = useCallback(() => {
    setMaximized((current) => !current)
  }, [])

  useEffect(() => {
    return () => {
      if (autoLayoutFrameRef.current !== undefined) {
        cancelAnimationFrame(autoLayoutFrameRef.current)
      }
      if (closeTransitionTimerRef.current !== undefined) {
        window.clearTimeout(closeTransitionTimerRef.current)
      }
    }
  }, [])

  useEffect(() => {
    autoCollapsedRef.current = false
  }, [activeDraftID, activeSessionID])

  useLayoutEffect(() => {
    if (!enabled) return
    const layout = layoutRef.current
    if (!layout) return

    const setOpenForLayout = (nextOpen: boolean) => {
      setAutoLayoutChanging(true)
      setOpen(nextOpen)
      if (autoLayoutFrameRef.current !== undefined) {
        cancelAnimationFrame(autoLayoutFrameRef.current)
      }
      autoLayoutFrameRef.current = requestAnimationFrame(() => {
        autoLayoutFrameRef.current = requestAnimationFrame(() => {
          autoLayoutFrameRef.current = undefined
          setAutoLayoutChanging(false)
        })
      })
    }

    const keepWidthInBounds = () => {
      const layoutWidth = layout.getBoundingClientRect().width
      if (layoutWidth <= 0) return
      const currentWidth = widthRef.current
      const preferredWidth =
        preferredWidthRef.current ?? layoutWidth * DEFAULT_WORKBENCH_RATIO
      preferredWidthRef.current = preferredWidth
      const nextWidth = clampWorkbenchWidth(preferredWidth, layoutWidth)
      if (currentWidth === undefined || Math.abs(currentWidth - nextWidth) >= 0.5) {
        widthRef.current = nextWidth
        setWidth(nextWidth)
      }

      const wasConstrained = constrainedRef.current
      const nextConstrained = isWorkbenchConstrained(
        layoutWidth,
        nextWidth,
        wasConstrained,
      )
      if (nextConstrained === wasConstrained) return

      constrainedRef.current = nextConstrained
      if (
        nextConstrained &&
        openRef.current &&
        !maximizedRef.current &&
        !resizingRef.current
      ) {
        // Snap the grid closed while the native window is animating so Chat
        // cannot be squeezed to zero by the previous workbench width.
        autoCollapsedRef.current = true
        setOpenForLayout(false)
      } else if (!nextConstrained && autoCollapsedRef.current) {
        autoCollapsedRef.current = false
        setOpenForLayout(true)
      }
    }

    keepWidthInBounds()
    const observer = new ResizeObserver(keepWidthInBounds)
    observer.observe(layout)
    return () => observer.disconnect()
  }, [enabled, setOpen])

  useLayoutEffect(() => {
    const primaryKey = primaryPreviewOpen && primaryPreviewRevision !== undefined
      ? `${activeSessionID ?? 'draft'}:${primaryPreviewRevision}`
      : undefined
    const secondaryKey = secondaryPreviewOpen && secondaryPreviewRevision !== undefined
      ? `${secondarySessionID}:${secondaryPreviewRevision}`
      : undefined
    let changedSessionID: string | undefined
    if (primaryKey && primaryKey !== previousPreviewKeysRef.current.primary) {
      changedSessionID = activeSessionID
    }
    if (secondaryKey && secondaryKey !== previousPreviewKeysRef.current.secondary) {
      changedSessionID = secondarySessionID
    }
    previousPreviewKeysRef.current = {
      primary: primaryKey,
      secondary: secondaryKey,
    }
    if (!changedSessionID) return
    showSession(changedSessionID)
  }, [
    activeSessionID,
    primaryPreviewOpen,
    primaryPreviewRevision,
    secondaryPreviewOpen,
    secondaryPreviewRevision,
    secondarySessionID,
    showSession,
  ])

  const getLayoutWidth = useCallback(() =>
    layoutRef.current?.getBoundingClientRect().width ??
    MIN_WORKBENCH_WIDTH + MIN_CHAT_WIDTH, [])

  const setUserWidth = useCallback((nextWidth: number) => {
    preferredWidthRef.current = nextWidth
    widthRef.current = nextWidth
    setWidth(nextWidth)
    localStorage.setItem(WORKBENCH_WIDTH_KEY, String(Math.round(nextWidth)))
  }, [])

  const beginResize = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    if (!open) return
    const layoutWidth = getLayoutWidth()
    const startWidth = clampWorkbenchWidth(
      viewportRef.current?.getBoundingClientRect().width ??
        width ??
        layoutWidth * DEFAULT_WORKBENCH_RATIO,
      layoutWidth,
    )
    const resize = {
      startX: event.clientX,
      startWidth,
    }
    setUserWidth(startWidth)
    return resize
  }, [getLayoutWidth, open, setUserWidth, width])

  const updateResize = useCallback((
    currentResize: { startX: number; startWidth: number },
    clientX: number,
  ) => {
    setUserWidth(resizedWorkbenchWidth(
      currentResize.startWidth,
      currentResize.startX,
      clientX,
      getLayoutWidth(),
    ))
  }, [getLayoutWidth, setUserWidth])

  const {
    resizing,
    startResize,
    resize,
    stopResize,
  } = usePointerResize({ start: beginResize, move: updateResize })
  resizingRef.current = resizing

  const resizeWithKeyboard = useCallback((event: ReactKeyboardEvent<HTMLDivElement>) => {
    const layoutWidth = getLayoutWidth()
    const currentWidth =
      viewportRef.current?.getBoundingClientRect().width ??
      width ??
      layoutWidth * DEFAULT_WORKBENCH_RATIO
    const nextWidth = keyboardWorkbenchWidth(event.key, currentWidth, layoutWidth)
    if (nextWidth === undefined) return
    event.preventDefault()
    setUserWidth(nextWidth)
  }, [getLayoutWidth, setUserWidth, width])

  const layoutWidth = getLayoutWidth()
  const resizeBounds = workbenchWidthBounds(layoutWidth)

  return {
    layoutRef,
    viewportRef,
    open,
    previewSessionID,
    expandedWidth: width === undefined
      ? `${DEFAULT_WORKBENCH_RATIO * 100}cqw`
      : `${width}px`,
    resizing,
    maximized,
    autoLayoutChanging,
    closing,
    resizeMinimum: Math.round(resizeBounds.minimum),
    resizeMaximum: Math.round(resizeBounds.maximum),
    resizeValue: Math.round(width ?? layoutWidth * DEFAULT_WORKBENCH_RATIO),
    toggle,
    showSession,
    toggleMaximized,
    startResize,
    resize,
    stopResize,
    resizeWithKeyboard,
  }
}
