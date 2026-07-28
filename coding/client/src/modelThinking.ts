import type { ModelOption } from '@/types'

export function isFixedThinking(model?: ModelOption): boolean {
  return Boolean(
    model && model.thinkingLevels.length === 1 && model.thinkingLevels[0] !== 'off',
  )
}
