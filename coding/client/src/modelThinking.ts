import type { ModelOption, ThinkingLevel } from '@/types'

export function isFixedThinking(model?: ModelOption): boolean {
  return Boolean(
    model && model.thinkingLevels.length === 1 && model.thinkingLevels[0] !== 'off',
  )
}

export function isToggleThinking(model?: ModelOption): boolean {
  if (!model || model.thinkingLevels.length !== 2) return false
  return model.thinkingLevels.includes('off') && model.thinkingLevels.includes('high')
}

export function toggleThinkingLevel(enabled: boolean): ThinkingLevel {
  return enabled ? 'high' : 'off'
}

export function thinkingLevelLabelKey(
  model: ModelOption | undefined,
  level: ThinkingLevel,
): 'model.thinkingOff' | 'model.thinkingOn' | `effort.${ThinkingLevel}` {
  if (isToggleThinking(model)) {
    return level === 'off' ? 'model.thinkingOff' : 'model.thinkingOn'
  }
  return `effort.${level}`
}
