import type { ThreadsState } from './threadState'

// findEvictableThreadIDs returns loaded client threads that are no longer
// rendered by either conversation pane. Browser-result deliveries are the one
// client-owned state that cannot be rebuilt from the server, so they pin a
// thread until the outbox has been acknowledged.
export function findEvictableThreadIDs(
  threads: ThreadsState,
  retainedSessionIDs: ReadonlySet<string>,
): string[] {
  return Object.entries(threads)
    .filter(
      ([sessionID, thread]) =>
        !retainedSessionIDs.has(sessionID) &&
        Object.keys(thread.browserResultOutbox).length === 0,
    )
    .map(([sessionID]) => sessionID)
}
