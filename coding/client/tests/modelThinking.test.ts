import { describe, expect, test } from 'bun:test'
import { isFixedThinking } from '../src/modelThinking'
import type { ModelOption } from '../src/types'

function model(overrides: Partial<ModelOption> = {}): ModelOption {
  return {
    provider: 'demo',
    id: 'demo-model',
    name: 'Demo model',
    contextWindow: 128_000,
    thinkingLevels: ['high'],
    supportsImages: false,
    ...overrides,
  }
}

describe('isFixedThinking', () => {
  test('recognizes a single reasoning level regardless of visibility', () => {
    expect(isFixedThinking(model({ thinkingVisibility: 'hidden' }))).toBe(true)
    expect(isFixedThinking(model({ thinkingVisibility: 'visible' }))).toBe(true)
  })

  test('keeps configurable and non-reasoning controls available', () => {
    expect(
      isFixedThinking(
        model({ thinkingVisibility: 'hidden', thinkingLevels: ['off', 'high'] }),
      ),
    ).toBe(false)
    expect(
      isFixedThinking(
        model({ thinkingVisibility: 'hidden', thinkingLevels: ['off'] }),
      ),
    ).toBe(false)
  })
})
