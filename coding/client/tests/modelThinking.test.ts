import { describe, expect, test } from 'bun:test'
import {
  isFixedThinking,
  isToggleThinking,
  thinkingLevelLabelKey,
  toggleThinkingLevel,
} from '../src/modelThinking'
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

describe('toggle thinking presentation', () => {
  test('recognizes only the normalized off/high capability pair', () => {
    expect(isToggleThinking(model({ thinkingLevels: ['off', 'high'] }))).toBe(true)
    expect(isToggleThinking(model({ thinkingLevels: ['high', 'off'] }))).toBe(true)
    expect(isToggleThinking(model({ thinkingLevels: ['high'] }))).toBe(false)
    expect(isToggleThinking(model({ thinkingLevels: ['off'] }))).toBe(false)
    expect(isToggleThinking(model({ thinkingLevels: ['off', 'medium'] }))).toBe(false)
    expect(isToggleThinking(model({ thinkingLevels: ['off', 'high', 'max'] }))).toBe(false)
  })

  test('presents binary values as off/on while preserving wire levels', () => {
    const toggle = model({ thinkingLevels: ['off', 'high'] })
    const effort = model({ thinkingLevels: ['off', 'low', 'high'] })

    expect(thinkingLevelLabelKey(toggle, 'off')).toBe('model.thinkingOff')
    expect(thinkingLevelLabelKey(toggle, 'high')).toBe('model.thinkingOn')
    expect(thinkingLevelLabelKey(effort, 'high')).toBe('effort.high')
    expect(toggleThinkingLevel(false)).toBe('off')
    expect(toggleThinkingLevel(true)).toBe('high')
  })
})
