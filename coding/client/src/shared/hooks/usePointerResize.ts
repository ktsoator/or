import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
} from 'react'

type PointerResizeOptions<Session> = {
  start: (event: ReactPointerEvent<HTMLDivElement>) => Session | undefined
  move: (session: Session, clientX: number) => void
}

type ActivePointerResize<Session> = {
  pointerID: number
  target: HTMLDivElement
  session: Session
  latestClientX: number
  frameID?: number
  pending: boolean
}

export function usePointerResize<Session>({
  start,
  move,
}: PointerResizeOptions<Session>) {
  const activeRef = useRef<ActivePointerResize<Session> | undefined>(undefined)
  const moveRef = useRef(move)
  const [resizing, setResizing] = useState(false)
  moveRef.current = move

  const flushMove = useCallback((active: ActivePointerResize<Session>) => {
    if (!active.pending) return
    active.pending = false
    moveRef.current(active.session, active.latestClientX)
  }, [])

  const finishResize = useCallback((pointerID?: number, clientX?: number) => {
    const active = activeRef.current
    if (!active || (pointerID !== undefined && active.pointerID !== pointerID)) return

    if (clientX !== undefined && clientX !== active.latestClientX) {
      active.latestClientX = clientX
      active.pending = true
    }
    if (active.frameID !== undefined) {
      window.cancelAnimationFrame(active.frameID)
      active.frameID = undefined
    }
    flushMove(active)
    activeRef.current = undefined
    if (active.target.hasPointerCapture(active.pointerID)) {
      active.target.releasePointerCapture(active.pointerID)
    }
    setResizing(false)
  }, [flushMove])

  useEffect(() => {
    if (!resizing) return

    const previousCursor = document.body.style.cursor
    const previousUserSelect = document.body.style.userSelect
    const stopFromPointer = (event: PointerEvent) => finishResize(event.pointerId, event.clientX)
    const stopIfReleased = (event: PointerEvent) => {
      if ((event.buttons & 1) === 0) finishResize(event.pointerId, event.clientX)
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
      latestClientX: event.clientX,
      pending: false,
    }
    setResizing(true)
  }, [start])

  const resize = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    const active = activeRef.current
    if (!active || active.pointerID !== event.pointerId) return
    if ((event.buttons & 1) === 0) {
      finishResize(event.pointerId, event.clientX)
      return
    }
    active.latestClientX = event.clientX
    active.pending = true
    if (active.frameID !== undefined) return
    active.frameID = window.requestAnimationFrame(() => {
      active.frameID = undefined
      if (activeRef.current !== active) return
      flushMove(active)
    })
  }, [finishResize, flushMove])

  const stopResize = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    finishResize(event.pointerId, event.clientX)
  }, [finishResize])

  return {
    resizing,
    startResize,
    resize,
    stopResize,
  }
}
