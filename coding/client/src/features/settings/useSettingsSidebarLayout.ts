import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from 'react'
import {
  DEFAULT_SIDEBAR_WIDTH,
  MAX_SIDEBAR_WIDTH,
  MIN_SIDEBAR_WIDTH,
  keyboardSidebarWidth,
  resizedSidebarWidth,
} from '@/sidebarLayout'

export function useSettingsSidebarLayout() {
  const resizeRef = useRef<
    | {
        pointerID: number
        startX: number
        startWidth: number
      }
    | undefined
  >(undefined)
  const [collapsed, setCollapsed] = useState(false)
  const [width, setWidth] = useState(DEFAULT_SIDEBAR_WIDTH)
  const [resizing, setResizing] = useState(false)

  useEffect(() => {
    if (!resizing) return
    const previousCursor = document.body.style.cursor
    const previousUserSelect = document.body.style.userSelect
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    return () => {
      document.body.style.cursor = previousCursor
      document.body.style.userSelect = previousUserSelect
    }
  }, [resizing])

  const collapse = useCallback(() => setCollapsed(true), [])
  const expand = useCallback(() => setCollapsed(false), [])

  const startResize = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    if (collapsed) return
    event.preventDefault()
    resizeRef.current = {
      pointerID: event.pointerId,
      startX: event.clientX,
      startWidth: width,
    }
    event.currentTarget.setPointerCapture(event.pointerId)
    setResizing(true)
  }, [collapsed, width])

  const resize = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    const currentResize = resizeRef.current
    if (!currentResize || currentResize.pointerID !== event.pointerId) return
    setWidth(resizedSidebarWidth(
      currentResize.startWidth,
      currentResize.startX,
      event.clientX,
    ))
  }, [])

  const stopResize = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    const currentResize = resizeRef.current
    if (!currentResize || currentResize.pointerID !== event.pointerId) return
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
    resizeRef.current = undefined
    setResizing(false)
  }, [])

  const resizeWithKeyboard = useCallback((event: ReactKeyboardEvent<HTMLDivElement>) => {
    const nextWidth = keyboardSidebarWidth(event.key, width)
    if (nextWidth === undefined) return
    event.preventDefault()
    setWidth(nextWidth)
  }, [width])

  return {
    collapsed,
    width,
    resizing,
    minimumWidth: MIN_SIDEBAR_WIDTH,
    maximumWidth: MAX_SIDEBAR_WIDTH,
    collapse,
    expand,
    startResize,
    resize,
    stopResize,
    resizeWithKeyboard,
  }
}
