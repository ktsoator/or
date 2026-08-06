import { useEffect, useState } from 'react'
import {
  currentSyntaxHighlighter,
  loadSyntaxHighlighter,
  type SyntaxHighlighter,
} from '@/shared/lib/highlight'

export function useSyntaxHighlighter(enabled: boolean): SyntaxHighlighter | undefined {
  const [highlighter, setHighlighter] = useState<SyntaxHighlighter | undefined>(() =>
    currentSyntaxHighlighter(),
  )

  useEffect(() => {
    if (!enabled || highlighter) return
    let active = true
    void loadSyntaxHighlighter().then((loaded) => {
      if (active) setHighlighter(loaded)
    })
    return () => {
      active = false
    }
  }, [enabled, highlighter])

  return highlighter
}
