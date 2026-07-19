import { ChevronRight } from 'lucide-react'
import type { Change, Hunk as HunkType } from '@/types'
import { highlightCode, languageForPath } from '@/lib/highlight'
import { cn } from '@/lib/utils'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { useI18n } from '@/i18n'

export function FileChange({ change }: { change: Change }) {
  const { t } = useI18n()
  if (change.changeType === 'failure') {
    return (
      <div className="mt-2 ml-5 border-l-2 border-red-300 py-1 pl-3 font-mono text-[0.8125rem] leading-5.5 text-red-700 max-md:ml-0">
        {(change.path ? `${change.path}: ` : '') + (change.detail || t('diff.writeFailed'))}
      </div>
    )
  }

  const hunks = Array.isArray(change.hunks) ? change.hunks : []
  const filename = change.path.split('/').filter(Boolean).pop() || change.path
  const showPath = change.path !== filename
  const language = languageForPath(change.path)

  return (
    <Collapsible
      defaultOpen
      className="mt-1.5 ml-5 overflow-hidden rounded-lg border border-stone-300/80 bg-white max-md:ml-0"
    >
      <div className={cn('px-2.5', showPath ? 'py-1.5' : 'py-1')}>
        <CollapsibleTrigger
          className="group flex w-full min-w-0 cursor-pointer items-baseline gap-1.5 border-0 bg-transparent p-0 text-left focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-400"
          disabled={hunks.length === 0}
        >
          <span className="shrink-0 text-[0.8125rem] text-stone-500">
            {change.op === 'create' ? t('diff.created') : t('diff.updated')}
          </span>
          <strong className="min-w-0 overflow-hidden text-[0.8125rem] font-medium text-stone-900 text-ellipsis whitespace-nowrap">
            {filename}
          </strong>
          <span className="flex shrink-0 gap-1 font-mono text-[0.75rem] font-medium">
            <b className="font-inherit text-emerald-700">+{change.additions || 0}</b>
            <b className="font-inherit text-rose-700">-{change.deletions || 0}</b>
          </span>
          {hunks.length > 0 && (
            <ChevronRight
              className="size-3.5 shrink-0 text-stone-500 transition-transform group-data-[state=open]:rotate-90"
              aria-hidden="true"
            />
          )}
        </CollapsibleTrigger>
        {showPath && (
          <div
            className="mt-1 overflow-hidden font-mono text-[0.75rem] text-stone-500 text-ellipsis whitespace-nowrap"
            title={change.path}
          >
            {change.path}
          </div>
        )}
      </div>

      {hunks.length > 0 && (
        <CollapsibleContent>
          <div className="max-h-[28.75rem] overflow-auto border-t border-stone-300/80 bg-[#fdfdfc] [scrollbar-color:#8f8f89_transparent] [scrollbar-width:thin]">
            {hunks.map((hunk, index) => (
              <Hunk key={index} hunk={hunk} language={language} />
            ))}
          </div>
        </CollapsibleContent>
      )}
    </Collapsible>
  )
}

function Hunk({ hunk, language }: { hunk: HunkType; language: string }) {
  let oldLine = hunk.oldStart
  let newLine = hunk.newStart

  return (
    <div className="border-b border-stone-300/70 last:border-b-0">
      <div className="bg-stone-100 px-2.5 py-0.5 font-mono text-[0.6875rem] leading-4 font-medium text-stone-500">
        {`@@ -${hunk.oldStart},${hunk.oldLines} +${hunk.newStart},${hunk.newLines} @@`}
      </div>
      {(hunk.lines ?? []).map((line, index) => {
        const mark = line.charAt(0)
        const isAdd = mark === '+'
        const isDelete = mark === '-'
        const number = isDelete ? oldLine : newLine
        if (!isAdd) oldLine += 1
        if (!isDelete) newLine += 1
        const code = isAdd || isDelete || mark === ' ' ? line.slice(1) : line
        const html = highlightCode(code, language) || ' '

        return (
          <div
            key={index}
            className={cn(
              'grid min-h-[1.125rem] grid-cols-[1.375rem_2.25rem_minmax(max-content,1fr)] font-mono text-[var(--tool-font-size)] leading-4.5 text-stone-900',
              isAdd && 'bg-[#dcefe2]',
              isDelete && 'bg-[#f5dddd]',
            )}
          >
            <span
              className={cn(
                'pl-2 text-stone-400 select-none',
                isAdd && 'text-emerald-700',
                isDelete && 'text-rose-700',
              )}
            >
              {isAdd ? '+' : isDelete ? '−' : ''}
            </span>
            <span className="pr-2 text-right text-stone-500 select-none">{number}</span>
            <code
              className="or-code-theme hljs block min-w-full overflow-visible bg-transparent! px-2.5 whitespace-pre"
              dangerouslySetInnerHTML={{ __html: html }}
            />
          </div>
        )
      })}
    </div>
  )
}
