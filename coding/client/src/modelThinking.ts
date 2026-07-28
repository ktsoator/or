import type { ModelOption } from '@/types'

export function isFixedHiddenThinking(model?: ModelOption): boolean {
  return Boolean(
    model?.thinkingVisibility === 'hidden' &&
      model.thinkingLevels.length === 1 &&
      model.thinkingLevels[0] !== 'off',
  )
}
