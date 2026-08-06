import { useCallback, useEffect, useRef } from 'react'
import { isAPIError } from '@/api'

const attemptDelays = [0, 250, 1000]

/**
 * Reports one terminal result per session command.
 *
 * A command has a single result, so a report that is retrying must not swallow
 * a later observation: the newest result always replaces an unsent one, and a
 * finished attempt run repeats itself when something newer arrived while it
 * was running. Resolving on the server's not-found code keeps a command that
 * the server already finished from being retried forever.
 *
 * Returns true when the command is settled and false when every attempt
 * failed, so the caller can decide whether to observe and report again.
 */
export function useCommandResultReporter<Result>({
  send,
  resolvedCode,
  onHandled,
}: {
  send: (sessionID: string, commandID: string, result: Result) => Promise<void>
  resolvedCode: string
  onHandled: (sessionID: string, commandID: string) => void
}): (
  sessionID: string | undefined,
  commandID: string,
  result: Result,
) => Promise<boolean> {
  const reportingRef = useRef(new Set<string>())
  const reportedRef = useRef(new Set<string>())
  const latestRef = useRef(new Map<string, Result>())
  const sendRef = useRef(send)
  const onHandledRef = useRef(onHandled)
  const mountedRef = useRef(true)

  sendRef.current = send
  onHandledRef.current = onHandled

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  return useCallback(async (sessionID, commandID, result) => {
    if (!sessionID) return false
    const key = `${sessionID}:${commandID}`
    if (reportedRef.current.has(key)) return true
    latestRef.current.set(key, result)
    if (reportingRef.current.has(key)) return false

    reportingRef.current.add(key)
    try {
      for (;;) {
        const attempted = latestRef.current.get(key)
        if (attempted === undefined) return false
        for (const delay of attemptDelays) {
          if (delay > 0) {
            await new Promise((resolve) => window.setTimeout(resolve, delay))
          }
          if (!mountedRef.current) return false
          try {
            await sendRef.current(sessionID, commandID, attempted)
          } catch (error) {
            if (!isAPIError(error, resolvedCode)) continue
          }
          reportedRef.current.add(key)
          onHandledRef.current(sessionID, commandID)
          return true
        }
        if (latestRef.current.get(key) === attempted) return false
      }
    } finally {
      reportingRef.current.delete(key)
      latestRef.current.delete(key)
    }
  }, [resolvedCode])
}
