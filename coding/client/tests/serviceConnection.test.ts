import { describe, expect, test } from 'bun:test'
import {
  startServiceConnection,
  type ServiceConnectionDependencies,
} from '../src/serviceConnection'
import type { ConnectionStatus } from '../src/types'

type Scheduled = {
  id: number
  callback: () => void
  delayMs: number
  cancelled: boolean
}

function harness(healthResults: boolean[]) {
  const statuses: ConnectionStatus[] = []
  const scheduled: Scheduled[] = []
  let nextID = 1
  let syncs = 0
  let initialized = 0

  const dependencies: ServiceConnectionDependencies = {
    health: async () => {
      if (healthResults.shift() === false) throw new Error('offline')
    },
    sync: async () => {
      syncs++
    },
    schedule: (callback, delayMs) => {
      const entry = { id: nextID++, callback, delayMs, cancelled: false }
      scheduled.push(entry)
      return entry.id
    },
    cancelSchedule: (handle) => {
      const entry = scheduled.find((candidate) => candidate.id === handle)
      if (entry) entry.cancelled = true
    },
  }

  const connection = startServiceConnection(
    {
      onStatus: (status) => statuses.push(status),
      onInitialized: () => initialized++,
    },
    dependencies,
  )

  const nextScheduled = () => scheduled.find((entry) => !entry.cancelled)
  const runNext = async () => {
    const entry = nextScheduled()
    if (!entry) throw new Error('no scheduled connection attempt')
    entry.cancelled = true
    entry.callback()
    await settle()
  }

  return {
    connection,
    statuses,
    scheduled,
    nextScheduled,
    runNext,
    syncs: () => syncs,
    initialized: () => initialized,
  }
}

async function settle() {
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
}

describe('service connection', () => {
  test('retries startup automatically after a health failure', async () => {
    const test = harness([false, true])
    await settle()

    expect(test.statuses).toEqual(['connecting', 'disconnected'])
    expect(test.initialized()).toBe(1)
    expect(test.nextScheduled()?.delayMs).toBe(250)

    await test.runNext()

    expect(test.statuses.at(-1)).toBe('ready')
    expect(test.syncs()).toBe(1)
    expect(test.nextScheduled()?.delayMs).toBe(10_000)
    test.connection.stop()
  })

  test('keeps a ready service usable through two transient failures', async () => {
    const test = harness([true, false, false, false, true])
    await settle()

    expect(test.statuses).toEqual(['connecting', 'ready'])
    expect(test.syncs()).toBe(1)
    expect(test.nextScheduled()?.delayMs).toBe(10_000)

    await test.runNext()
    expect(test.statuses.at(-1)).toBe('ready')
    expect(test.nextScheduled()?.delayMs).toBe(250)

    await test.runNext()
    expect(test.statuses.at(-1)).toBe('ready')
    expect(test.nextScheduled()?.delayMs).toBe(500)

    await test.runNext()
    expect(test.statuses.at(-1)).toBe('disconnected')
    expect(test.nextScheduled()?.delayMs).toBe(1_000)

    await test.runNext()
    expect(test.statuses.at(-1)).toBe('ready')
    expect(test.syncs()).toBe(2)
    expect(test.nextScheduled()?.delayMs).toBe(10_000)
    test.connection.stop()
  })

  test('focus retry cancels the heartbeat and refreshes immediately', async () => {
    const test = harness([true, true])
    await settle()
    const heartbeat = test.nextScheduled()
    expect(heartbeat?.delayMs).toBe(10_000)

    test.connection.retryNow()
    await settle()

    expect(heartbeat?.cancelled).toBe(true)
    expect(test.syncs()).toBe(2)
    expect(test.statuses.at(-1)).toBe('ready')
    test.connection.stop()
  })
})
