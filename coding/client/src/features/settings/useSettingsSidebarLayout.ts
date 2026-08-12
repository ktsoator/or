import {
  useCallback,
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

export function useSettingsSidebarLayout() {
  const [collapsed, setCollapsed] = useState(false)
  const [width, setWidth] = useState(DEFAULT_SIDEBAR_WIDTH)

  const collapse = useCallback(() => setCollapsed(true), [])
  const expand = useCallback(() => setCollapsed(false), [])

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
