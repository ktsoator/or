import type { ReactNode } from 'react'
import {
  BookOpenText,
  ChevronRight,
  CircleStop,
  CircleX,
  File,
  FileCode2,
  FilePlus2,
  FileSearch,
  Folder,
  FolderSearch,
  Globe2,
  LoaderCircle,
  PencilLine,
  ScrollText,
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
import { CopyButton } from './CopyButton'
import { useI18n } from '@/i18n'

function prettyArgs(args: unknown): string {
  if (args === undefined || args === null) return ''
  if (typeof args === 'string') return args
  try {
    return JSON.stringify(args, null, 2)
  } catch {
    return String(args)
  }
}

// relativize trims the workspace root off absolute paths so tool rows show
// "work/main.go" instead of a long ".../workspaces/<date>/<id>/work/main.go",
// falling back to "~" for anything still under the user's home directory.
function relativize(text: string, cwd?: string): string {
  if (!text) return text
  let out = text
  if (cwd) {
    out = out.split(`${cwd}/`).join('')
    out = out.split(cwd).join('.')
  }
  const home = cwd?.match(/^(\/(?:Users|home)\/[^/]+)/)?.[1]
  if (home) out = out.split(`${home}/`).join('~/').split(home).join('~')
  return out
}

// stripLeadingCd drops an infrastructural "cd <dir> &&" prefix from a command so
// the collapsed row leads with the actual work; the full command still shows
// when the card is expanded.
function stripLeadingCd(command: string): string {
  const match = /^cd\s+\S+\s+&&\s+([\s\S]*)$/.exec(command)
  return match ? match[1] : command
}

type ToolKind =
  | 'read'
  | 'write'
  | 'edit'
  | 'patch'
  | 'inspect'
  | 'search'
  | 'run'
  | 'logs'
  | 'kill'
  | 'skill'
  | 'preview'

function toolPresentation(name: string): { Icon: LucideIcon; kind: ToolKind } {
  const value = name.toLowerCase()
  if (value === 'skill') return { Icon: BookOpenText, kind: 'skill' }
  if (value === 'open_preview') return { Icon: Globe2, kind: 'preview' }
  if (value.includes('read') || value.includes('cat')) return { Icon: FileSearch, kind: 'read' }
  if (value.includes('write')) return { Icon: FilePlus2, kind: 'write' }
  if (value.includes('edit')) return { Icon: PencilLine, kind: 'edit' }
  if (value.includes('patch')) return { Icon: FileCode2, kind: 'patch' }
  if (value.includes('glob') || value.includes('list') || value.includes('folder')) {
    return { Icon: FolderSearch, kind: 'inspect' }
  }
  if (value.includes('search') || value.includes('grep') || value === 'rg') {
    return { Icon: Search, kind: 'search' }
  }
  if (value.includes('kill')) return { Icon: CircleStop, kind: 'kill' }
  if (value.includes('output') || value.includes('log')) return { Icon: ScrollText, kind: 'logs' }
  if (value.includes('file')) return { Icon: FileCode2, kind: 'inspect' }
  return { Icon: Terminal, kind: 'run' }
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
    record.cmd ??
    record.url ??
    record.shell_id
  return typeof value === 'string' ? value : ''
}

// skillField reads a string field from the skill tool's arguments ({ name,
// arguments }); the loaded instructions live in the tool result and are
// intentionally not surfaced in the card.
function skillField(args: unknown, key: 'name' | 'arguments'): string {
  if (!args || typeof args !== 'object') return ''
  const value = (args as Record<string, unknown>)[key]
  return typeof value === 'string' ? value.trim() : ''
}

function explicitCommand(args: unknown): string {
  if (!args || typeof args !== 'object') return ''
  const record = args as Record<string, unknown>
  const value = record.command ?? record.cmd
  return typeof value === 'string' ? value : ''
}

// commandDescription returns the model-written summary of a bash command, shown
// in the row in place of the raw command (the command stays in the expanded
// detail). Empty when the model omitted it, so the row falls back to the command.
function commandDescription(args: unknown): string {
  if (!args || typeof args !== 'object') return ''
  const value = (args as Record<string, unknown>).description
  return typeof value === 'string' ? value.trim() : ''
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(bytes < 10 * 1024 ? 1 : 0)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function generatedLineCount(kind: ToolKind, args: unknown): number | undefined {
  if (!args || typeof args !== 'object') return undefined
  const record = args as Record<string, unknown>
  const content = kind === 'write' ? record.content : kind === 'edit' ? record.new_string : undefined
  if (typeof content !== 'string') return undefined
  return content === '' ? 0 : content.split(/\r?\n/).length
}

function Status({
  status,
  generatedBytes,
  lineCount,
}: {
  status: ToolItem['status']
  generatedBytes?: number
  lineCount?: number
}) {
  const { t } = useI18n()
  if (status === 'preparing' || status === 'running') {
    const detail =
      lineCount !== undefined
        ? t('tool.lines', { count: lineCount })
        : status === 'preparing' && generatedBytes
          ? formatBytes(generatedBytes)
          : status === 'running'
            ? t('tool.running')
            : ''
    return (
      <span className="ml-auto flex shrink-0 items-center gap-1 text-[0.75rem] text-stone-500">
        <LoaderCircle className="size-3 animate-spin" aria-hidden="true" />
        {detail}
      </span>
    )
  }
  if (status === 'error') {
    return (
      <span className="ml-auto flex shrink-0 items-center gap-1 text-[0.75rem] text-red-600">
        <CircleX className="size-3" aria-hidden="true" />
        {t('tool.failed')}
      </span>
    )
  }
  return null
}

function DetailBlock({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="border-b border-stone-200 last:border-b-0">
      <div className="border-b border-stone-200 bg-stone-50 px-3 py-1 text-[0.71875rem] font-medium tracking-wide text-stone-500 uppercase">
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
  const { t } = useI18n()
  const content = parseReadContent(output)
  const html = content.hasLineNumbers
    ? highlightCode(content.code, languageForPath(path))
    : ''

  return (
    <div
      className={cn(
        'mt-1 ml-5 overflow-hidden rounded-lg border border-stone-300/80 bg-white max-md:ml-0',
        failed && 'border-red-200 bg-red-50/60',
      )}
    >
      <div
        className="overflow-hidden border-b border-stone-300/70 bg-white px-3 py-1 font-mono text-[0.75rem] text-stone-500 text-ellipsis whitespace-nowrap"
        title={path}
      >
        {path}
      </div>
      {content.hasLineNumbers && !failed ? (
        <>
          <div
            className="code-scroll-area grid max-h-[min(52vh,32.5rem)] grid-cols-[3.25rem_minmax(max-content,1fr)] overflow-auto bg-white"
            role="region"
            aria-label={t('tool.contentsOf', { path })}
            tabIndex={0}
          >
            <pre className="sticky left-0 z-10 m-0 border-r border-stone-200 bg-white px-2.5 py-1 text-right font-mono text-[var(--tool-detail-font-size)] leading-4.5 whitespace-pre text-stone-400 select-none">
              {content.lineNumbers}
            </pre>
            <pre className="m-0 min-w-full bg-transparent px-2.5 py-1 font-mono text-[var(--tool-detail-font-size)] leading-4.5 whitespace-pre text-stone-900">
              <code
                className="or-code-theme hljs block min-w-full bg-transparent! p-0!"
                dangerouslySetInnerHTML={{ __html: html }}
              />
            </pre>
          </div>
          {content.notice && (
            <div className="border-t border-stone-200 bg-white px-3 py-1.5 font-mono text-[0.71875rem] text-stone-500">
              {content.notice}
            </div>
          )}
        </>
      ) : (
        <pre
          className={cn(
            'code-scroll-area m-0 max-h-[min(52vh,32.5rem)] overflow-auto bg-transparent px-2.5 py-1 font-mono text-[var(--tool-detail-font-size)] leading-4.5 whitespace-pre-wrap text-stone-800',
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
  const { t } = useI18n()
  if (failed) {
    return (
      <div className="mt-1 ml-5 rounded-md border-l-2 border-red-300 bg-red-50/50 px-3 py-1 font-mono text-[var(--tool-detail-font-size)] leading-5 text-red-700 max-md:ml-0">
        {output || t('tool.inspectionFailed')}
      </div>
    )
  }

  const rawLines = output.trimEnd().split('\n')
  const noticeStart = rawLines.findIndex((line) => line.startsWith('[truncated '))
  const notice = noticeStart >= 0 ? rawLines.slice(noticeStart).join('\n') : ''
  const paths = (noticeStart >= 0 ? rawLines.slice(0, noticeStart) : rawLines).filter(Boolean)
  const empty = paths.length === 0 || (paths.length === 1 && paths[0] === 'No files found.')

  return (
    <div className="mt-1 ml-5 max-w-full overflow-hidden rounded-lg border border-stone-200/90 bg-stone-50/70 max-md:ml-0">
      <div className="flex h-7 items-center px-3 text-[0.75rem] text-stone-500">
        {empty
          ? t('tool.noMatchingFiles')
          : `${paths.length} ${paths.length === 1 ? t('tool.path') : t('tool.paths')}`}
      </div>
      {!empty && (
        <div
          className="code-scroll-area max-h-72 overflow-auto border-t border-stone-200/80 bg-[#fdfdfc] py-1"
          role="region"
          aria-label={t('tool.matchingFiles')}
          tabIndex={0}
        >
          {paths.map((path, index) => {
            const directory = path.endsWith('/')
            const PathIcon = directory ? Folder : File
            return (
              <div
                key={`${path}-${index}`}
                className="group flex min-h-5 min-w-max items-center gap-2 px-2.5 text-stone-700 transition-colors duration-100 hover:bg-[rgb(241,241,241)] hover:text-stone-900"
              >
                <PathIcon
                  className="size-3.25 shrink-0 text-stone-400 transition-colors group-hover:text-stone-500"
                  aria-hidden="true"
                />
                <code className="pr-4 font-mono text-[var(--tool-detail-font-size)] leading-4.5">{path}</code>
              </div>
            )
          })}
        </div>
      )}
      {notice && (
        <div className="border-t border-stone-200/80 px-3 py-1.5 text-[0.71875rem] leading-4 text-stone-500">
          {notice.slice(1, -1)}
        </div>
      )}
    </div>
  )
}

type ShellStatus = { running: boolean; exitCode: number; command: string }

// parseBackgroundStatus recognizes the header line bash_output emits, e.g.
// "[bg_1: running] go run ." or "[bg_1: exited with code 0] ...", so the status
// can be shown as a badge instead of raw text and the body kept clean.
function parseBackgroundStatus(output: string): { status: ShellStatus | null; body: string } {
  const newline = output.indexOf('\n')
  const firstLine = newline >= 0 ? output.slice(0, newline) : output
  const match = /^\[\S+: (running|exited with code (-?\d+))\] ?(.*)$/.exec(firstLine)
  if (!match) return { status: null, body: output }
  const running = match[1] === 'running'
  return {
    status: { running, exitCode: running ? 0 : Number(match[2] ?? 0), command: match[3] ?? '' },
    body: newline >= 0 ? output.slice(newline + 1) : '',
  }
}

function ShellStatusBadge({ status }: { status: ShellStatus }) {
  const { t } = useI18n()
  const ok = !status.running && status.exitCode === 0
  return (
    <span className="flex min-w-0 items-center gap-2">
      <span
        className={cn(
          'size-1.5 shrink-0 rounded-full',
          status.running ? 'animate-pulse bg-emerald-500' : ok ? 'bg-stone-400' : 'bg-rose-500',
        )}
        aria-hidden="true"
      />
      <span
        className={cn(
          'shrink-0 text-[0.6875rem] font-medium tracking-wide uppercase',
          status.running ? 'text-emerald-700' : ok ? 'text-stone-500' : 'text-rose-600',
        )}
      >
        {status.running
          ? t('tool.bgRunning')
          : ok
            ? t('tool.bgExited')
            : t('tool.bgExitedCode', { code: status.exitCode })}
      </span>
      {status.command && (
        <code
          className="min-w-0 overflow-hidden font-mono text-[var(--tool-detail-font-size)] text-stone-400 text-ellipsis whitespace-nowrap"
          title={status.command}
        >
          {status.command}
        </code>
      )}
    </span>
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
  const { t } = useI18n()
  const { status, body } = parseBackgroundStatus(output)
  const log = status ? body : output
  return (
    <div
      className={cn(
        'mt-1 ml-5 overflow-hidden rounded-lg border border-stone-200 bg-white antialiased max-md:ml-0',
        failed && 'border-red-200 bg-red-50/60',
      )}
    >
      <div className="flex min-h-7 items-start gap-2 px-2.5 py-1.5 font-mono text-[var(--tool-detail-font-size)] leading-4.5 font-normal">
        {status ? (
          <ShellStatusBadge status={status} />
        ) : (
          <>
            <span className="shrink-0 text-stone-400 select-none">$</span>
            <code className="min-w-0 flex-1 overflow-auto whitespace-pre-wrap text-stone-700">
              {command}
            </code>
          </>
        )}
        {log && <CopyButton value={log} className="ml-auto -mr-0.5" />}
      </div>
      {log && (
        <pre
          className={cn(
            'code-scroll-area m-0 max-h-[min(46vh,26.25rem)] overflow-auto border-t border-stone-200 bg-white px-2.5 py-1.5 font-mono text-[var(--tool-detail-font-size)] leading-4.5 font-normal tracking-[0.005em] whitespace-pre text-stone-600',
            failed && 'border-red-200 bg-red-50/40 text-red-700',
          )}
          role="region"
          aria-label={t('tool.shellOutput')}
          tabIndex={0}
        >
          {log}
        </pre>
      )}
    </div>
  )
}

export function ToolCard({ item, cwd }: { item: ToolItem; cwd?: string }) {
  const { t } = useI18n()
  const args = prettyArgs(item.args)
  const rawHint = argHint(item.args)
  const rawCommand = explicitCommand(item.args) || (rawHint ? `${item.name} ${rawHint}` : item.name)
  const { Icon, kind } = toolPresentation(item.name)
  const verb = t(`tool.${kind}`)
  const hint = relativize(rawHint, cwd)
  const command = relativize(rawCommand, cwd)
  const description = kind === 'run' ? commandDescription(item.args) : ''
  const skillTitle = kind === 'skill' ? skillField(item.args, 'name') : ''
  const skillArgs = kind === 'skill' ? skillField(item.args, 'arguments') : ''
  const lineCount = generatedLineCount(kind, item.args)
  const preparingLabel =
    item.status === 'preparing' && item.args === undefined
      ? kind === 'write'
        ? t('tool.preparingWrite')
        : kind === 'edit'
          ? t('tool.preparingEdit')
          : t('tool.preparing')
      : ''
  const target =
    kind === 'run'
      ? stripLeadingCd(command)
      : kind === 'skill'
        ? skillTitle || item.name
        : hint || item.name
  const targetTitle = kind === 'run' ? rawCommand : kind === 'skill' ? skillTitle : rawHint || item.name
  const fileChange = item.change?.changeType === 'file' ? item.change : undefined
  const changedFilename = fileChange?.path.split('/').filter(Boolean).pop() || fileChange?.path
  const hasDetails =
    kind === 'preview'
      ? false
      : kind === 'read'
        ? item.status === 'complete' || item.status === 'error'
      : kind === 'skill'
        ? Boolean(skillArgs || item.status === 'error')
        : Boolean(args || item.change || item.result || item.status === 'error')
  const shellOutput =
    item.result || (item.status === 'error' ? t('tool.failedNoMessage') : '')
  const readOutput =
    item.result || (item.status === 'error' ? t('tool.fileCouldNotRead') : t('tool.fileEmpty'))

  const summary = (
    <span className="flex min-h-6 min-w-0 flex-1 items-center gap-2 text-[var(--chat-font-size)] leading-6 text-stone-500 transition-colors group-hover:text-stone-900">
      <Icon
        className={cn(
          'size-4 shrink-0 transition-colors',
          kind === 'kill' && 'text-rose-400 group-hover:text-rose-600',
        )}
        aria-hidden="true"
      />
      {preparingLabel ? (
        <span>{preparingLabel}</span>
      ) : !description && (
        <span>
          {fileChange
            ? fileChange.op === 'create'
              ? t('diff.created')
              : t('diff.edited')
            : verb}
        </span>
      )}
      {preparingLabel ? null : fileChange ? (
        <>
          <span
            className="min-w-0 overflow-hidden font-normal text-stone-500 underline decoration-stone-400/70 underline-offset-2 text-ellipsis whitespace-nowrap transition-colors group-hover:text-stone-950"
            title={fileChange.path}
          >
            {changedFilename}
          </span>
          <span className="flex shrink-0 gap-1 font-mono text-[0.75rem] font-normal">
            <span className="text-emerald-700">+{fileChange.additions || 0}</span>
            <span className="text-rose-700">-{fileChange.deletions || 0}</span>
          </span>
        </>
      ) : description ? (
        <span
          className="min-w-0 overflow-hidden font-normal text-ellipsis whitespace-nowrap transition-colors"
          title={targetTitle}
        >
          {description}
        </span>
      ) : (
        <code
          className="min-w-0 overflow-hidden font-mono text-[var(--chat-font-size)] leading-6 font-normal text-stone-500 text-ellipsis whitespace-nowrap transition-colors group-hover:text-stone-950"
          title={targetTitle}
        >
          {target}
        </code>
      )}
      <Status status={item.status} generatedBytes={item.generatedBytes} lineCount={lineCount} />
    </span>
  )

  if (!hasDetails) {
    return <div className="group my-1 flex w-fit max-w-full animate-[fade-in_160ms_ease-out]">{summary}</div>
  }

  return (
    <Collapsible className="my-1 animate-[fade-in_160ms_ease-out]">
      <CollapsibleTrigger className="group inline-flex max-w-full cursor-pointer items-center border-0 bg-transparent p-0 text-left focus-visible:rounded-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-stone-400">
        {summary}
        <ChevronRight
          className="ml-1 size-3.5 shrink-0 text-stone-400 transition-[transform,color] group-hover:text-stone-950 group-data-[state=open]:rotate-90"
          aria-hidden="true"
        />
      </CollapsibleTrigger>
      <CollapsibleContent>
        {kind === 'skill' ? (
          <div className="mt-1 ml-5 overflow-hidden rounded-lg border border-stone-200 bg-white max-md:ml-0">
            {skillArgs && (
              <DetailBlock title={t('tool.skillArguments')}>
                <pre className="m-0 max-h-80 overflow-auto bg-white px-2.5 py-1.5 font-mono text-[var(--tool-detail-font-size)] leading-4.5 whitespace-pre-wrap text-stone-700">
                  {skillArgs}
                </pre>
              </DetailBlock>
            )}
            {item.status === 'error' && (
              <DetailBlock title={t('tool.errorOutput')}>
                <pre className="m-0 max-h-80 overflow-auto bg-red-50/50 px-2.5 py-1.5 font-mono text-[var(--tool-detail-font-size)] leading-4.5 whitespace-pre-wrap text-red-700">
                  {item.result || t('tool.failedNoMessage')}
                </pre>
              </DetailBlock>
            )}
          </div>
        ) : kind === 'read' ? (
          <ReadPreview output={readOutput} path={target} failed={item.status === 'error'} />
        ) : kind === 'inspect' && !item.change ? (
          <InspectPreview output={item.result || ''} failed={item.status === 'error'} />
        ) : (kind === 'run' || kind === 'logs' || kind === 'kill') && !item.change ? (
          <ShellPreview
            command={command}
            output={shellOutput}
            failed={item.status === 'error'}
          />
        ) : item.change ? (
          <FileChange change={item.change} />
        ) : (
          <div className="mt-1 ml-5 overflow-hidden rounded-lg border border-stone-200 bg-white max-md:ml-0">
            {args && (
              <DetailBlock title={t('tool.input')}>
                <pre className="m-0 max-h-80 overflow-auto bg-white px-2.5 py-1.5 font-mono text-[var(--tool-detail-font-size)] leading-4.5 whitespace-pre-wrap text-stone-700">
                  {args}
                </pre>
              </DetailBlock>
            )}
            {(item.result || item.status === 'error') && (
              <DetailBlock
                title={item.status === 'error' ? t('tool.errorOutput') : t('tool.output')}
              >
                <pre
                  className={cn(
                    'm-0 max-h-80 overflow-auto bg-white px-2.5 py-1.5 font-mono text-[var(--tool-detail-font-size)] leading-4.5 whitespace-pre-wrap text-stone-700',
                    item.status === 'error' && 'bg-red-50/50 text-red-700',
                  )}
                >
                  {item.result || t('tool.failedNoMessage')}
                </pre>
              </DetailBlock>
            )}
          </div>
        )}
      </CollapsibleContent>
    </Collapsible>
  )
}
