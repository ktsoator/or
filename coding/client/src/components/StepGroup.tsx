import { useState } from 'react'
import { ChevronRight, Layers } from 'lucide-react'
import type { Item } from '@/types'
import { cn } from '@/lib/utils'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { ToolCard } from './ToolCard'
import { Thinking } from './Thinking'
import { useI18n } from '@/i18n'

// stepCategory maps a tool name to a summary bucket, mirroring the icon buckets
// in ToolCard's toolPresentation so the summary reads consistently.
function stepCategory(name: string): string {
  const v = name.toLowerCase()
  if (v === 'open_preview') return 'browserOpen'
  if (v === 'inspect_browser') return 'browserInspect'
  if (v.includes('read') || v.includes('cat')) return 'read'
  if (v.includes('write')) return 'write'
  if (v.includes('edit') || v.includes('patch')) return 'edit'
  if (v.includes('glob') || v.includes('list') || v.includes('folder')) return 'inspect'
  if (v.includes('search') || v.includes('grep') || v === 'rg') return 'search'
  if (v.includes('file')) return 'inspect'
  return 'run'
}

function summarizeSteps(items: Item[], t: ReturnType<typeof useI18n>['t']): string {
  const order: string[] = []
  const counts = new Map<string, number>()
  for (const it of items) {
    if (it.kind !== 'tool') continue
    const category = stepCategory(it.name)
    if (!counts.has(category)) order.push(category)
    counts.set(category, (counts.get(category) ?? 0) + 1)
  }
  const parts = order.map((category) => {
    const n = counts.get(category) ?? 0
    const key = `steps.${category}_${n === 1 ? 'one' : 'other'}` as Parameters<typeof t>[0]
    return t(key, { n })
  })
  const joined = parts.join(t('steps.separator'))
  return joined.charAt(0).toUpperCase() + joined.slice(1)
}

// StepGroup stays compact by default. Running-state updates must not override a
// user's expand/collapse choice or make the conversation jump while streaming.
export function StepGroup({ items, cwd }: { items: Item[]; cwd?: string }) {
  const { t } = useI18n()
  const active = items.some(
    (it) =>
      (it.kind === 'tool' && (it.status === 'preparing' || it.status === 'running')) ||
      (it.kind === 'thinking' && it.streaming),
  )
  const [open, setOpen] = useState(false)

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="my-1 animate-[fade-in_160ms_ease-out]"
    >
      <CollapsibleTrigger
        className={cn(
          'group inline-flex max-w-full cursor-pointer items-center gap-2 border-0 bg-transparent p-0 text-left text-[var(--chat-font-size)] leading-6 text-stone-500 transition-colors hover:text-stone-900 focus-visible:rounded-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-stone-400',
          active && 'streaming-sheen',
        )}
      >
        <Layers className="size-4 shrink-0" aria-hidden="true" />
        <span className="min-w-0 overflow-hidden text-ellipsis whitespace-nowrap">
          {summarizeSteps(items, t)}
        </span>
        <ChevronRight
          className="size-3.5 shrink-0 text-stone-400 transition-transform group-data-[state=open]:rotate-90"
          aria-hidden="true"
        />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="mt-0.5 ml-2 border-l border-stone-200 pl-3 max-md:ml-0">
          {items.map((it) =>
            it.kind === 'tool' ? (
              <ToolCard key={it.id} item={it} cwd={cwd} />
            ) : it.kind === 'thinking' ? (
              <Thinking key={it.id} item={it} />
            ) : null,
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}
