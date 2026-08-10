import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
} from 'react'

type PointerResizeOptions<Session> = {
  start: (event: ReactPointerEvent<HTMLDivElement>) => Session | undefined
  move: (
    session: Session,
    event: ReactPointerEvent<HTMLDivElement>,
  ) => void
}

type ActivePointerResize<Session> = {
  pointerID: number
  target: HTMLDivElement
  session: Session
}

export function usePointerResize<Session>({
  start,
  move,
}: PointerResizeOptions<Session>) {
  const activeRef = useRef<ActivePointerResize<Session> | undefined>(undefined)
  const [resizing, setResizing] = useState(false)

  const finishResize = useCallback((pointerID?: number) => {
    const active = activeRef.current
    if (!active || (pointerID !== undefined && active.pointerID !== pointerID)) return

    activeRef.current = undefined
    if (active.target.hasPointerCapture(active.pointerID)) {
      active.target.releasePointerCapture(active.pointerID)
    }
    setResizing(false)
  }, [])

  useEffect(() => {
    if (!resizing) return

    const previousCursor = document.body.style.cursor
    const previousUserSelect = document.body.style.userSelect
    const stopFromPointer = (event: PointerEvent) => finishResize(event.pointerId)
    const stopIfReleased = (event: PointerEvent) => {
      if ((event.buttons & 1) === 0) finishResize(event.pointerId)
    }
    const stopFromWindow = () => finishResize()
    const stopWhenHidden = () => {
      if (document.hidden) finishResize()
    }

    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    window.addEventListener('pointermove', stopIfReleased, true)
    window.addEventListener('pointerup', stopFromPointer, true)
    window.addEventListener('pointercancel', stopFromPointer, true)
    window.addEventListener('blur', stopFromWindow)
    document.addEventListener('visibilitychange', stopWhenHidden)

    return () => {
      window.removeEventListener('pointermove', stopIfReleased, true)
      window.removeEventListener('pointerup', stopFromPointer, true)
      window.removeEventListener('pointercancel', stopFromPointer, true)
      window.removeEventListener('blur', stopFromWindow)
      document.removeEventListener('visibilitychange', stopWhenHidden)
      document.body.style.cursor = previousCursor
      document.body.style.userSelect = previousUserSelect
    }
  }, [finishResize, resizing])

  const startResize = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return
    const session = start(event)
    if (session === undefined) return

    event.preventDefault()
    event.currentTarget.setPointerCapture(event.pointerId)
    activeRef.current = {
      pointerID: event.pointerId,
      target: event.currentTarget,
      session,
    }
    setResizing(true)
  }, [start])

  const resize = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    const active = activeRef.current
    if (!active || active.pointerID !== event.pointerId) return
    if ((event.buttons & 1) === 0) {
      finishResize(event.pointerId)
      return
    }
    move(active.session, event)
  }, [finishResize, move])

  const stopResize = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    finishResize(event.pointerId)
  }, [finishResize])

  return {
    resizing,
    startResize,
    resize,
    stopResize,
  }
}
