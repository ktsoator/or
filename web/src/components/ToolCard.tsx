import type { ReactNode } from 'react'
import {
  ChevronRight,
  CircleX,
  FileCode2,
  FilePlus2,
  FileSearch,
  FolderSearch,
  LoaderCircle,
  PencilLine,
  Search,
  Terminal,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { ToolItem } from '@/types'
import { cn } from '@/lib/utils'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { FileChange } from './Diff'

function prettyArgs(args: unknown): string {
  if (args === undefined || args === null) return ''
  if (typeof args === 'string') return args
  try {
    return JSON.stringify(args, null, 2)
  } catch {
    return String(args)
  }
}

function toolPresentation(name: string): { Icon: LucideIcon; verb: string } {
  const value = name.toLowerCase()
  if (value.includes('read') || value.includes('cat')) return { Icon: FileSearch, verb: 'Read' }
  if (value.includes('write')) return { Icon: FilePlus2, verb: 'Write' }
  if (value.includes('edit')) return { Icon: PencilLine, verb: 'Edit' }
  if (value.includes('patch')) return { Icon: FileCode2, verb: 'Patch' }
  if (value.includes('glob') || value.includes('list') || value.includes('folder')) {
    return { Icon: FolderSearch, verb: 'Inspect' }
  }
  if (value.includes('search') || value.includes('grep') || value === 'rg') {
    return { Icon: Search, verb: 'Search' }
  }
  if (value.includes('file')) return { Icon: FileCode2, verb: 'Inspect' }
  return { Icon: Terminal, verb: 'Run' }
}

function argHint(args: unknown): string {
  if (!args || typeof args !== 'object') return ''
  const record = args as Record<string, unknown>
  const value = record.path ?? record.file_path ?? record.file ?? record.command ?? record.cmd ?? record.query
  return typeof value === 'string' ? value : ''
}

function explicitCommand(args: unknown): string {
  if (!args || typeof args !== 'object') return ''
  const record = args as Record<string, unknown>
  const value = record.command ?? record.cmd
  return typeof value === 'string' ? value : ''
}

function Status({ status }: { status: ToolItem['status'] }) {
  if (status === 'running') {
    return (
      <span className="ml-auto flex shrink-0 items-center gap-1 text-[11px] text-stone-500">
        <LoaderCircle className="size-3 animate-spin" aria-hidden="true" />
        Running
      </span>
    )
  }
  if (status === 'error') {
    return (
      <span className="ml-auto flex shrink-0 items-center gap-1 text-[11px] text-red-600">
        <CircleX className="size-3" aria-hidden="true" />
        Failed
      </span>
    )
  }
  return null
}

function DetailBlock({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="border-b border-stone-200 last:border-b-0">
      <div className="border-b border-stone-200 bg-stone-50 px-3 py-1.5 text-[10.5px] font-semibold tracking-wide text-stone-500 uppercase">
        {title}
      </div>
      {children}
    </div>
  )
}

export function ToolCard({ item }: { item: ToolItem }) {
  const args = prettyArgs(item.args)
  const hint = argHint(item.args)
  const { Icon, verb } = toolPresentation(item.name)
  const command = explicitCommand(item.args) || (hint ? `${item.name} ${hint}` : item.name)
  const target = verb === 'Run' ? command : hint || item.name
  const hasDetails = verb !== 'Read' && Boolean(args || item.change || item.result || item.status === 'error')
  const shellOutput = item.result || (item.status === 'error' ? 'Tool failed without an error message.' : '')

  const summary = (
    <span className="flex min-h-7 min-w-0 flex-1 items-center gap-2 text-[13.5px] text-stone-500">
      <Icon className="size-3.5 shrink-0 stroke-[1.7]" aria-hidden="true" />
      <span>{verb}</span>
      <code
        className="min-w-0 overflow-hidden font-mono text-[13px] leading-5 font-medium text-stone-800 text-ellipsis whitespace-nowrap"
        title={target}
      >
        {target}
      </code>
      <Status status={item.status} />
    </span>
  )

  if (!hasDetails) {
    return <div className="my-1.5 flex animate-[fade-in_160ms_ease-out]">{summary}</div>
  }

  return (
    <Collapsible className="my-1.5 animate-[fade-in_160ms_ease-out]">
      <CollapsibleTrigger className="group flex w-full cursor-pointer items-center border-0 bg-transparent text-left hover:text-stone-900 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-400">
        {summary}
        <ChevronRight
          className="ml-1 size-3.5 shrink-0 text-stone-400 transition-transform group-data-[state=open]:rotate-90"
          aria-hidden="true"
        />
      </CollapsibleTrigger>
      <CollapsibleContent>
        {verb === 'Run' && !item.change ? (
          <div
            className={cn(
              'mt-1.5 ml-5 overflow-hidden rounded-lg border border-stone-300 bg-[#f3f3f2] max-md:ml-0',
              item.status === 'error' && 'border-red-200 bg-red-50/60',
            )}
          >
            <div className="px-4 pt-3 text-[15px] font-medium text-stone-600">Shell</div>
            <pre
              className={cn(
                'm-0 max-h-[380px] overflow-auto bg-transparent px-4 pt-3 pb-4 font-mono text-sm leading-[1.55] whitespace-pre-wrap text-stone-600',
                item.status === 'error' && 'text-red-700',
              )}
            >
              {`$ ${command}${shellOutput ? `\n${shellOutput}` : ''}`}
            </pre>
          </div>
        ) : item.change ? (
          <FileChange change={item.change} />
        ) : (
          <div className="mt-1.5 ml-5 overflow-hidden rounded-lg border border-stone-200 bg-white max-md:ml-0">
            {args && (
              <DetailBlock title="Input">
                <pre className="m-0 max-h-80 overflow-auto bg-white px-3 py-2.5 font-mono text-[12.5px] leading-5 whitespace-pre-wrap text-stone-700">
                  {args}
                </pre>
              </DetailBlock>
            )}
            {(item.result || item.status === 'error') && (
              <DetailBlock title={item.status === 'error' ? 'Error output' : 'Output'}>
                <pre
                  className={cn(
                    'm-0 max-h-80 overflow-auto bg-white px-3 py-2.5 font-mono text-[12.5px] leading-5 whitespace-pre-wrap text-stone-700',
                    item.status === 'error' && 'bg-red-50/50 text-red-700',
                  )}
                >
                  {item.result || 'Tool failed without an error message.'}
                </pre>
              </DetailBlock>
            )}
          </div>
        )}
      </CollapsibleContent>
    </Collapsible>
  )
}
