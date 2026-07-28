import { describe, expect, test } from 'bun:test'
import { isFixedHiddenThinking } from '../src/modelThinking'
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

describe('isFixedHiddenThinking', () => {
  test('recognizes a single hidden reasoning level', () => {
    expect(isFixedHiddenThinking(model({ thinkingVisibility: 'hidden' }))).toBe(true)
  })

  test('does not hide configurable or visible controls', () => {
    expect(
      isFixedHiddenThinking(
        model({ thinkingVisibility: 'hidden', thinkingLevels: ['off', 'high'] }),
      ),
    ).toBe(false)
    expect(isFixedHiddenThinking(model({ thinkingVisibility: 'visible' }))).toBe(false)
    expect(
      isFixedHiddenThinking(
        model({ thinkingVisibility: 'hidden', thinkingLevels: ['off'] }),
      ),
    ).toBe(false)
  })
})
