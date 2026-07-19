import { useEffect, useState } from 'react'
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
  const [open, setOpen] = useState(item.streaming)

  useEffect(() => setOpen(item.streaming), [item.streaming])

  return (
    <Collapsible open={open} onOpenChange={setOpen} className="my-2 animate-[fade-in_160ms_ease-out] text-stone-500">
      <CollapsibleTrigger className="group flex cursor-pointer items-center gap-2 border-0 bg-transparent py-0.5 text-[0.90625rem] font-medium text-inherit hover:text-stone-900 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-400">
        <span className={cn('size-1.5 rounded-full bg-stone-400', item.streaming && 'animate-pulse bg-indigo-500')} />
        <span>{item.streaming ? t('thinking.working') : t('thinking.process')}</span>
        <ChevronRight
          className="size-3.5 transition-transform group-data-[state=open]:rotate-90"
          aria-hidden="true"
        />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="mt-2 max-h-56 overflow-auto border-l border-stone-200 pl-3.5 text-[0.90625rem] leading-[1.62] whitespace-pre-wrap text-stone-500">
          {item.text}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}
