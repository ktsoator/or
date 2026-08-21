import type {
  KeyboardEventHandler,
  PointerEventHandler,
} from 'react'
import { cn } from '@/lib/utils'

export function PanelResizeHandle({
  edge,
  label,
  minimum,
  maximum,
  value,
  resizing,
  className,
  testID,
  dividerTestID,
  onPointerDown,
  onPointerMove,
  onPointerUp,
  onPointerCancel,
  onLostPointerCapture,
  onKeyDown,
}: {
  edge: 'left' | 'right'
  label: string
  minimum: number
  maximum: number
  value: number
  resizing: boolean
  className?: string
  testID?: string
  dividerTestID?: string
  onPointerDown: PointerEventHandler<HTMLDivElement>
  onPointerMove: PointerEventHandler<HTMLDivElement>
  onPointerUp: PointerEventHandler<HTMLDivElement>
  onPointerCancel: PointerEventHandler<HTMLDivElement>
  onLostPointerCapture: PointerEventHandler<HTMLDivElement>
  onKeyDown: KeyboardEventHandler<HTMLDivElement>
}) {
  return (
    <div
      className={cn(
        'panel-resize-handle window-titlebar-control group absolute inset-y-0 z-[60] hidden w-[8px] touch-none cursor-col-resize outline-none md:block',
        edge === 'left' ? '-left-[4px]' : '-right-[4px]',
        className,
      )}
      data-testid={testID}
      role="separator"
      aria-label={label}
      aria-orientation="vertical"
      aria-valuemin={minimum}
      aria-valuemax={maximum}
      aria-valuenow={value}
      tabIndex={0}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerCancel}
      onLostPointerCapture={onLostPointerCapture}
      onKeyDown={onKeyDown}
    >
      <span
        className={cn(
          'absolute inset-y-0 left-1/2 w-px -translate-x-1/2 bg-edge/75 transition-colors duration-100',
          'group-hover:bg-ink-muted/60 group-focus-visible:bg-ink-muted/70',
          resizing && 'bg-ink-muted/70',
        )}
        data-testid={dividerTestID}
        aria-hidden="true"
      />
    </div>
  )
}
