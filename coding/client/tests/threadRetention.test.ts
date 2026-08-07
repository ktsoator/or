import { describe, expect, test } from 'bun:test'
import {
  findEvictableThreadIDs,
} from '../src/features/session/threadRetention'
import {
  createThreadState,
  type ThreadsState,
} from '../src/features/session/reducer'

describe('thread retention', () => {
  test('keeps only the primary and secondary pane threads', () => {
    const threads: ThreadsState = {
      primary: createThreadState(),
      secondary: createThreadState(),
      inactive: createThreadState(),
    }

    expect(
      findEvictableThreadIDs(threads, new Set(['primary', 'secondary'])),
    ).toEqual(['inactive'])
  })

  test('keeps an inactive thread until its browser result is acknowledged', () => {
    const threads: ThreadsState = {
      inactive: {
        ...createThreadState(),
        browserResultOutbox: {
          command: {
            commandID: 'command',
            result: { status: 'committed' },
          },
        },
      },
    }

    expect(findEvictableThreadIDs(threads, new Set())).toEqual([])
    threads.inactive = {
      ...threads.inactive,
      browserResultOutbox: {},
    }
    expect(findEvictableThreadIDs(threads, new Set())).toEqual(['inactive'])
  })
})
