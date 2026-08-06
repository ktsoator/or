import { useEffect, useRef, useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { useI18n } from '@/i18n'
import { cn } from '@/lib/utils'

// CopyButton copies value to the clipboard and shows a brief confirmation. It
// matches the copy control used by the diff view, so copy affordances read the
// same everywhere.
export function CopyButton({ value, className }: { value: string; className?: string }) {
  const { t } = useI18n()
  const [copied, setCopied] = useState(false)
  const resetRef = useRef<number>(undefined)

  useEffect(
    () => () => {
      if (resetRef.current) window.clearTimeout(resetRef.current)
    },
    [],
  )

  const copy = async () => {
    if (!value) return
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
      if (resetRef.current) window.clearTimeout(resetRef.current)
      resetRef.current = window.setTimeout(() => setCopied(false), 1600)
    } catch {
      // Clipboard access can be unavailable in non-secure browser contexts.
    }
  }

  const label = copied ? t('code.copied') : t('code.copy')
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      onClick={() => void copy()}
      className={cn(
        'grid size-6 shrink-0 cursor-pointer place-items-center rounded text-ink-faint outline-none transition-colors hover:bg-canvas-sunken hover:text-ink focus-visible:bg-canvas-sunken focus-visible:text-ink',
        className,
      )}
    >
      {copied ? (
        <Check className="size-3.5" aria-hidden="true" />
      ) : (
        <Copy className="size-3.5" aria-hidden="true" />
      )}
    </button>
  )
}
