import { ChevronRight } from 'lucide-react'
import hljs from 'highlight.js/lib/common'
import type { Change, Hunk as HunkType } from '@/types'
import { cn } from '@/lib/utils'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'

const extensionLanguages: Record<string, string> = {
  c: 'c',
  cc: 'cpp',
  cpp: 'cpp',
  css: 'css',
  go: 'go',
  html: 'xml',
  java: 'java',
  js: 'javascript',
  json: 'json',
  jsx: 'javascript',
  md: 'markdown',
  py: 'python',
  rs: 'rust',
  sh: 'bash',
  ts: 'typescript',
  tsx: 'typescript',
  yaml: 'yaml',
  yml: 'yaml',
}

function languageFor(path: string): string {
  const extension = path.split('.').pop()?.toLowerCase() ?? ''
  const language = extensionLanguages[extension] ?? 'plaintext'
  return hljs.getLanguage(language) ? language : 'plaintext'
}

export function FileChange({ change }: { change: Change }) {
  if (change.changeType === 'failure') {
    return (
      <div className="mt-2 ml-5 border-l-2 border-red-300 py-1 pl-3 font-mono text-xs leading-5 text-red-700 max-md:ml-0">
        {(change.path ? `${change.path}: ` : '') + (change.detail || 'write failed')}
      </div>
    )
  }

  const hunks = Array.isArray(change.hunks) ? change.hunks : []
  const filename = change.path.split('/').filter(Boolean).pop() || change.path
  const showPath = change.path !== filename
  const language = languageFor(change.path)

  return (
    <Collapsible
      defaultOpen
      className="mt-2 ml-5 overflow-hidden rounded-lg border border-stone-200 bg-white max-md:ml-0"
    >
      <div className={cn('px-3.5', showPath ? 'py-3' : 'py-2.5')}>
        <CollapsibleTrigger
          className="group flex w-full min-w-0 cursor-pointer items-baseline gap-1.5 border-0 bg-transparent p-0 text-left focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-400"
          disabled={hunks.length === 0}
        >
          <span className="shrink-0 text-[14.5px] text-stone-500">
            {change.op === 'create' ? 'Created' : 'Updated'}
          </span>
          <strong className="min-w-0 overflow-hidden text-[14.5px] font-medium text-stone-900 text-ellipsis whitespace-nowrap">
            {filename}
          </strong>
          <span className="flex shrink-0 gap-1 font-mono text-sm font-medium">
            <b className="font-inherit text-emerald-600">+{change.additions || 0}</b>
            <b className="font-inherit text-rose-600">-{change.deletions || 0}</b>
          </span>
          {hunks.length > 0 && (
            <ChevronRight
              className="size-3.5 shrink-0 text-stone-400 transition-transform group-data-[state=open]:rotate-90"
              aria-hidden="true"
            />
          )}
        </CollapsibleTrigger>
        {showPath && (
          <div
            className="mt-1.5 overflow-hidden font-mono text-xs text-stone-400 text-ellipsis whitespace-nowrap"
            title={change.path}
          >
            {change.path}
          </div>
        )}
      </div>

      {hunks.length > 0 && (
        <CollapsibleContent>
          <div className="max-h-[460px] overflow-auto border-t border-stone-200 bg-[#fdfdfc] [scrollbar-color:#a8a8a3_transparent] [scrollbar-width:thin]">
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
    <div className="border-b border-stone-200 last:border-b-0">
      <div className="bg-stone-100/70 px-3 py-1 font-mono text-[11px] leading-4 text-stone-400">
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
        const html = hljs.highlight(code, { language }).value || ' '

        return (
          <div
            key={index}
            className={cn(
              'grid min-h-6 grid-cols-[24px_42px_minmax(max-content,1fr)] font-mono text-[12.75px] leading-6 text-stone-800',
              isAdd && 'bg-emerald-50/80',
              isDelete && 'bg-rose-50/80',
            )}
          >
            <span
              className={cn(
                'pl-2.5 text-stone-300 select-none',
                isAdd && 'text-emerald-600',
                isDelete && 'text-rose-600',
              )}
            >
              {isAdd ? '+' : isDelete ? '−' : ''}
            </span>
            <span className="pr-2.5 text-right text-stone-400 select-none">{number}</span>
            <code
              className="hljs block min-w-full overflow-visible bg-transparent! px-3.5 whitespace-pre"
              dangerouslySetInnerHTML={{ __html: html }}
            />
          </div>
        )
      })}
    </div>
  )
}
