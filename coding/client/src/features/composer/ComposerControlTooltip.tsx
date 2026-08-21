import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

export function ComposerControlTooltip({
  children,
  align = 'center',
}: {
  children: ReactNode
  align?: 'start' | 'center' | 'end'
}) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        'pointer-events-none absolute bottom-[calc(100%+2px)] z-[210] translate-y-1 whitespace-nowrap rounded-[10px] bg-canvas-inverse px-3 py-1.5 text-[0.875rem] leading-5 font-normal text-ink-inverse opacity-0 shadow-lg transition-[opacity,transform] duration-150',
        'group-hover:translate-y-0 group-hover:opacity-100 group-focus-visible:translate-y-0 group-focus-visible:opacity-100 group-data-[state=open]:hidden',
        align === 'start' && 'left-0',
        align === 'center' && 'left-1/2 -translate-x-1/2',
        align === 'end' && 'right-0',
      )}
    >
      {children}
    </span>
  )
}
