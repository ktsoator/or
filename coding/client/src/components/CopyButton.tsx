import { useEffect, useRef, useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useI18n } from '@/i18n'

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
        'grid size-6 shrink-0 cursor-pointer place-items-center rounded text-stone-400 transition-colors hover:text-stone-950 focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-stone-400',
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
