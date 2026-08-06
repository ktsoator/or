import { useEffect, useRef, useState } from 'react'
import { isAPIError } from '@/api'
import { useI18n } from '@/i18n'

export type CompactFeedback = {
  kind: 'notice' | 'error'
  message: string
}

export function useComposerCompaction(onCompact?: () => Promise<unknown>) {
  const { t } = useI18n()
  const timerRef = useRef<number | undefined>(undefined)
  const [feedback, setFeedback] = useState<CompactFeedback>()

  useEffect(
    () => () => {
      if (timerRef.current !== undefined) window.clearTimeout(timerRef.current)
    },
    [],
  )

  const dismiss = () => {
    if (timerRef.current !== undefined) {
      window.clearTimeout(timerRef.current)
      timerRef.current = undefined
    }
    setFeedback(undefined)
  }

  const show = (next: CompactFeedback) => {
    dismiss()
    setFeedback(next)
    timerRef.current = window.setTimeout(() => {
      timerRef.current = undefined
      setFeedback(undefined)
    }, 4000)
  }

  const compact = async () => {
    if (!onCompact) {
      show({ kind: 'notice', message: t('model.nothingToCompact') })
      return
    }
    dismiss()
    try {
      await onCompact()
    } catch (error) {
      const nothingToCompact = isAPIError(error, 'nothing_to_compact')
      show({
        kind: nothingToCompact ? 'notice' : 'error',
        message: nothingToCompact
          ? t('model.nothingToCompact')
          : t('model.compactFailed'),
      })
    }
  }

  return { feedback, dismiss, compact }
}
