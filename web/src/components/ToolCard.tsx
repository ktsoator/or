import type { ReactNode } from 'react'
import {
  ChevronRight,
  CircleX,
  File,
  FileCode2,
  FilePlus2,
  FileSearch,
  Folder,
  FolderSearch,
  LoaderCircle,
  PencilLine,
  Search,
  Terminal,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { ToolItem } from '@/types'
import { highlightCode, languageForPath } from '@/lib/highlight'
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
  const value =
    record.pattern ??
    record.query ??
    record.path ??
    record.file_path ??
    record.file ??
    record.command ??
    record.cmd
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
      <span className="ml-auto flex shrink-0 items-center gap-1 text-[12px] text-stone-500">
        <LoaderCircle className="size-3 animate-spin" aria-hidden="true" />
        Running
      </span>
    )
  }
  if (status === 'error') {
    return (
      <span className="ml-auto flex shrink-0 items-center gap-1 text-[12px] text-red-600">
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
      <div className="border-b border-stone-200 bg-stone-50 px-3 py-1.5 text-[11.5px] font-semibold tracking-wide text-stone-500 uppercase">
        {title}
      </div>
      {children}
    </div>
  )
}

type ReadContent = {
  code: string
  lineNumbers: string
  notice: string
  hasLineNumbers: boolean
}

function parseReadContent(output: string): ReadContent {
  const code: string[] = []
  const lineNumbers: string[] = []
  const notice: string[] = []
  let hasLineNumbers = false

  for (const line of output.replace(/\n$/, '').split('\n')) {
    const match = /^\s*(\d+)\t(.*)$/.exec(line)
    if (match) {
      hasLineNumbers = true
      lineNumbers.push(match[1])
      code.push(match[2])
      continue
    }

    if (hasLineNumbers && (line === '' || line.startsWith('[Showing '))) {
      notice.push(line)
      continue
    }

    lineNumbers.push('')
    code.push(line)
  }

  return {
    code: code.join('\n'),
    lineNumbers: lineNumbers.join('\n'),
    notice: notice.join('\n').trim(),
    hasLineNumbers,
  }
}

function ReadPreview({ output, path, failed }: { output: string; path: string; failed: boolean }) {
  const content = parseReadContent(output)
  const html = content.hasLineNumbers
    ? highlightCode(content.code, languageForPath(path))
    : ''

  return (
    <div
      className={cn(
        'mt-1.5 ml-5 overflow-hidden rounded-lg border border-stone-300/80 bg-[#fdfdfc] max-md:ml-0',
        failed && 'border-red-200 bg-red-50/60',
      )}
    >
      <div
        className="overflow-hidden border-b border-stone-300/70 px-3.5 py-2 font-mono text-[12.5px] text-stone-500 text-ellipsis whitespace-nowrap"
        title={path}
      >
        {path}
      </div>
      {content.hasLineNumbers && !failed ? (
        <>
          <div
            className="code-scroll-area grid max-h-[min(52vh,520px)] grid-cols-[52px_minmax(max-content,1fr)] overflow-auto overscroll-contain bg-[#fdfdfc]"
            role="region"
            aria-label={`Contents of ${path}`}
            tabIndex={0}
          >
            <pre className="sticky left-0 z-10 m-0 border-r border-stone-200 bg-stone-50 px-3 py-3 text-right font-mono text-[14px] leading-6 whitespace-pre text-stone-400 select-none">
              {content.lineNumbers}
            </pre>
            <pre className="m-0 min-w-full bg-transparent px-3.5 py-3 font-mono text-[14px] leading-6 whitespace-pre text-stone-900">
              <code
                className="or-code-theme hljs block min-w-full bg-transparent! p-0!"
                dangerouslySetInnerHTML={{ __html: html }}
              />
            </pre>
          </div>
          {content.notice && (
            <div className="border-t border-stone-200 bg-stone-50 px-3.5 py-2 font-mono text-[12px] text-stone-500">
              {content.notice}
            </div>
          )}
        </>
      ) : (
        <pre
          className={cn(
            'code-scroll-area m-0 max-h-[min(52vh,520px)] overflow-auto overscroll-contain bg-transparent px-3.5 py-3 font-mono text-[14px] leading-6 whitespace-pre-wrap text-stone-800',
            failed && 'text-red-700',
          )}
        >
          {output}
        </pre>
      )}
    </div>
  )
}

function InspectPreview({ output, failed }: { output: string; failed: boolean }) {
  if (failed) {
    return (
      <div className="mt-1.5 ml-5 rounded-md border-l-2 border-red-300 bg-red-50/50 px-3 py-2 font-mono text-xs leading-5 text-red-700 max-md:ml-0">
        {output || 'Inspection failed.'}
      </div>
    )
  }

  const rawLines = output.trimEnd().split('\n')
  const noticeStart = rawLines.findIndex((line) => line.startsWith('[truncated '))
  const notice = noticeStart >= 0 ? rawLines.slice(noticeStart).join('\n') : ''
  const paths = (noticeStart >= 0 ? rawLines.slice(0, noticeStart) : rawLines).filter(Boolean)
  const empty = paths.length === 0 || (paths.length === 1 && paths[0] === 'No files found.')

  return (
    <div className="mt-1.5 ml-5 max-w-full overflow-hidden rounded-lg border border-stone-200/90 bg-stone-50/70 max-md:ml-0">
      <div className="flex h-8 items-center px-3 text-[12.5px] text-stone-500">
        {empty ? 'No matching files' : `${paths.length} ${paths.length === 1 ? 'path' : 'paths'}`}
      </div>
      {!empty && (
        <div
          className="code-scroll-area max-h-72 overflow-auto overscroll-contain border-t border-stone-200/80 bg-[#fdfdfc] py-1"
          role="region"
          aria-label="Matching files"
          tabIndex={0}
        >
          {paths.map((path, index) => {
            const directory = path.endsWith('/')
            const PathIcon = directory ? Folder : File
            return (
              <div
                key={`${path}-${index}`}
                className="flex min-h-7 min-w-max items-center gap-2 px-3 text-stone-700 hover:bg-stone-100/80"
              >
                <PathIcon className="size-3.25 shrink-0 text-stone-400" aria-hidden="true" />
                <code className="pr-4 font-mono text-[13.5px] leading-5">{path}</code>
              </div>
            )
          })}
        </div>
      )}
      {notice && (
        <div className="border-t border-stone-200/80 px-3 py-2 text-[12px] leading-4 text-stone-500">
          {notice.slice(1, -1)}
        </div>
      )}
    </div>
  )
}

function ShellPreview({
  command,
  output,
  failed,
}: {
  command: string
  output: string
  failed: boolean
}) {
  return (
    <div
      className={cn(
        'mt-1.5 ml-5 overflow-hidden rounded-lg border border-stone-300/90 bg-[#f3f3f1] max-md:ml-0',
        failed && 'border-red-200 bg-red-50/60',
      )}
    >
      <div className="flex max-h-28 min-h-10 items-start gap-2 overflow-auto overscroll-contain px-3.5 py-2.5 font-mono text-[14px] leading-5.5">
        <span className="shrink-0 text-stone-400 select-none">$</span>
        <code className="whitespace-pre-wrap text-stone-800">{command}</code>
      </div>
      {output && (
        <pre
          className={cn(
            'code-scroll-area m-0 max-h-[min(46vh,420px)] overflow-auto overscroll-contain border-t border-stone-300/80 bg-[#fdfdfc] px-3.5 py-3 font-mono text-[14px] leading-6 whitespace-pre text-stone-700',
            failed && 'border-red-200 bg-red-50/40 text-red-700',
          )}
          role="region"
          aria-label="Shell output"
          tabIndex={0}
        >
          {output}
        </pre>
      )}
    </div>
  )
}

export function ToolCard({ item }: { item: ToolItem }) {
  const args = prettyArgs(item.args)
  const hint = argHint(item.args)
  const { Icon, verb } = toolPresentation(item.name)
  const command = explicitCommand(item.args) || (hint ? `${item.name} ${hint}` : item.name)
  const target = verb === 'Run' ? command : hint || item.name
  const hasDetails =
    verb === 'Read'
      ? item.status !== 'running'
      : Boolean(args || item.change || item.result || item.status === 'error')
  const shellOutput = item.result || (item.status === 'error' ? 'Tool failed without an error message.' : '')
  const readOutput =
    item.result || (item.status === 'error' ? 'File could not be read.' : 'File is empty.')

  const summary = (
    <span className="flex min-h-7 min-w-0 flex-1 items-center gap-2 text-[14.5px] text-stone-500">
      <Icon className="size-3.5 shrink-0" aria-hidden="true" />
      <span>{verb}</span>
      <code
        className="min-w-0 overflow-hidden font-mono text-[14px] leading-5.5 font-medium text-stone-800 text-ellipsis whitespace-nowrap"
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
        {verb === 'Read' ? (
          <ReadPreview output={readOutput} path={target} failed={item.status === 'error'} />
        ) : verb === 'Inspect' && !item.change ? (
          <InspectPreview output={item.result || ''} failed={item.status === 'error'} />
        ) : verb === 'Run' && !item.change ? (
          <ShellPreview
            command={command}
            output={shellOutput}
            failed={item.status === 'error'}
          />
        ) : item.change ? (
          <FileChange change={item.change} />
        ) : (
          <div className="mt-1.5 ml-5 overflow-hidden rounded-lg border border-stone-200 bg-white max-md:ml-0">
            {args && (
              <DetailBlock title="Input">
                <pre className="m-0 max-h-80 overflow-auto bg-white px-3 py-2.5 font-mono text-[13.5px] leading-5.5 whitespace-pre-wrap text-stone-700">
                  {args}
                </pre>
              </DetailBlock>
            )}
            {(item.result || item.status === 'error') && (
              <DetailBlock title={item.status === 'error' ? 'Error output' : 'Output'}>
                <pre
                  className={cn(
                    'm-0 max-h-80 overflow-auto bg-white px-3 py-2.5 font-mono text-[13.5px] leading-5.5 whitespace-pre-wrap text-stone-700',
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
