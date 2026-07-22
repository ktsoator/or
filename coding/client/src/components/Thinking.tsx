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

export function Thinking({ item }: { item: ThinkingItem }) {
  const { t } = useI18n()
  const [open, setOpen] = useState(false)

  return (
    <Collapsible open={open} onOpenChange={setOpen} className="my-0.5 animate-[fade-in_160ms_ease-out] text-stone-400">
      <CollapsibleTrigger
        className={cn(
          'group flex cursor-pointer items-center gap-1.5 border-0 bg-transparent py-0.5 text-[0.8125rem] font-normal text-inherit hover:text-stone-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-400',
          item.streaming && 'streaming-sheen',
        )}
      >
        <span className={cn('size-1 rounded-full bg-stone-300', item.streaming && 'animate-pulse bg-indigo-500')} />
        <span>{item.streaming ? t('thinking.working') : t('thinking.process')}</span>
        <ChevronRight
          className="size-3 text-stone-300 transition-transform group-hover:text-stone-500 group-data-[state=open]:rotate-90"
          aria-hidden="true"
        />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="mt-1 max-h-56 overflow-auto border-l border-stone-200 pl-3 text-[0.84375rem] leading-[1.5] whitespace-pre-wrap text-stone-500">
          {item.text}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}
