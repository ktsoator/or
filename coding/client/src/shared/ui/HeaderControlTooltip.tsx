import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

export function HeaderControlTooltip({
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
        'header-control-tooltip pointer-events-none absolute top-[calc(100%+2px)] z-[210] whitespace-nowrap opacity-0 transition-opacity duration-150',
        align === 'start' && 'left-0',
        align === 'center' && 'left-1/2 -translate-x-1/2',
        align === 'end' && 'right-0',
      )}
    >
      <span className="block -translate-y-1 rounded-[10px] bg-canvas-inverse px-3 py-1.5 text-[0.875rem] leading-5 font-normal text-ink-inverse shadow-lg transition-transform duration-150">
        {children}
      </span>
    </span>
  )
}
