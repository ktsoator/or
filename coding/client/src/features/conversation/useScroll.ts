import {
  useCallback,
  useLayoutEffect,
  useRef,
  useState,
  type UIEvent,
  type WheelEvent as ReactWheelEvent,
} from 'react'

const LATEST_THRESHOLD = 2

export function useConversationScroll(
  resetKey: string | undefined,
  content: readonly unknown[],
) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const followLatestRef = useRef(true)
  const previousResetKeyRef = useRef(resetKey)
  const [awayFromLatest, setAwayFromLatest] = useState(false)
  const [hasNewContent, setHasNewContent] = useState(false)
  // useSession filters its visible items on every render. The tail identity and
  // length change for actual transcript updates without treating that new array
  // wrapper as new content.
  const contentLength = content.length
  const latestContent = content.at(-1)

  useLayoutEffect(() => {
    const element = scrollRef.current
    if (!element) return

    const reset = previousResetKeyRef.current !== resetKey
    previousResetKeyRef.current = resetKey
    if (reset) {
      followLatestRef.current = true
      setAwayFromLatest(false)
      setHasNewContent(false)
    }

    if (followLatestRef.current) {
      element.scrollTop = element.scrollHeight
    } else {
      setHasNewContent(true)
    }
  }, [contentLength, latestContent, resetKey])

  const onScroll = useCallback((event: UIEvent<HTMLDivElement>) => {
    const element = event.currentTarget
    const atLatest =
      element.scrollHeight - element.scrollTop - element.clientHeight < LATEST_THRESHOLD

    followLatestRef.current = atLatest
    setAwayFromLatest(!atLatest)
    if (atLatest) setHasNewContent(false)
  }, [])

  const onWheelCapture = useCallback((event: ReactWheelEvent<HTMLDivElement>) => {
    if (event.deltaY >= 0) return

    const target = event.target
    const nestedScroller =
      target instanceof Element
        ? target.closest<HTMLElement>('.code-scroll-area')
        : null
    if (nestedScroller && nestedScroller !== event.currentTarget && nestedScroller.scrollTop > 0) {
      return
    }

    // Pause before the browser applies the wheel delta. A streaming layout
    // update can otherwise run first and snap a near-bottom transcript back.
    followLatestRef.current = false
  }, [])

  const scrollToLatest = useCallback(() => {
    const element = scrollRef.current
    if (!element) return

    followLatestRef.current = true
    element.scrollTop = element.scrollHeight
    setAwayFromLatest(false)
    setHasNewContent(false)
  }, [])

  return {
    scrollRef,
    onScroll,
    onWheelCapture,
    scrollToLatest,
    awayFromLatest,
    hasNewContent,
  }
}
