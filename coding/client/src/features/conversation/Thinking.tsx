import { useState } from 'react'
import { ChevronRight } from 'lucide-react'
import type { ThinkingItem } from '@/types'
import { cn } from '@/lib/utils'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { useI18n } from '@/i18n'
import { Markdown } from '@/shared/ui/Markdown'
import { parseThinkingContent } from './thinkingContent'

export function Thinking({ item }: { item: ThinkingItem }) {
  const { t } = useI18n()
  const [open, setOpen] = useState(false)
  const { title, body } = parseThinkingContent(item.text)
  const label = title ?? (item.streaming ? t('thinking.working') : t('thinking.process'))

  return (
    <Collapsible open={open} onOpenChange={setOpen} className="my-0.5 animate-[fade-in_160ms_ease-out] text-ink-faint">
      <CollapsibleTrigger
        className={cn(
          'group flex max-w-full cursor-pointer items-center gap-1.5 border-0 bg-transparent py-0.5 text-[0.8125rem] font-normal text-inherit outline-none hover:text-ink-soft focus-visible:rounded-sm focus-visible:bg-canvas-sunken focus-visible:text-ink-soft',
          item.streaming && 'streaming-sheen',
        )}
      >
        <span
          className={cn(
            'size-1 shrink-0 rounded-full bg-ink-ghost',
            item.streaming && 'animate-pulse bg-info',
          )}
        />
        <span className="min-w-0 truncate" title={title}>
          {label}
        </span>
        <ChevronRight
          className="size-3 shrink-0 text-ink-ghost transition-transform group-hover:text-ink-muted group-data-[state=open]:rotate-90"
          aria-hidden="true"
        />
      </CollapsibleTrigger>
      <CollapsibleContent>
        {body && (
          <div className="mt-1 max-h-56 overflow-auto border-l border-edge pl-3">
            <Markdown
              source={body}
              className="text-[0.84375rem] leading-[1.5] [--tw-prose-body:var(--ink-muted)] [--tw-prose-bold:var(--ink-soft)] prose-headings:text-[0.9375rem] prose-headings:leading-5 prose-headings:text-ink-soft"
            />
          </div>
        )}
      </CollapsibleContent>
    </Collapsible>
  )
}
