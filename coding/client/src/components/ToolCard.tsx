import type { ReactNode } from 'react'
import {
  BookOpenText,
  ChevronRight,
  CircleStop,
  CircleX,
  Eye,
  File,
  FileCode2,
  FilePlus2,
  FileSearch,
  Folder,
  FolderSearch,
  Globe2,
  LoaderCircle,
  PanelsTopLeft,
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
  | 'browserOpen'
  | 'browserTabs'
  | 'browserInspect'

function toolPresentation(name: string): { Icon: LucideIcon; kind: ToolKind } {
  const value = name.toLowerCase()
  if (value === 'skill') return { Icon: BookOpenText, kind: 'skill' }
  if (value === 'open_preview') return { Icon: Globe2, kind: 'browserOpen' }
  if (value === 'tabs_context') return { Icon: PanelsTopLeft, kind: 'browserTabs' }
  if (value === 'inspect_browser') return { Icon: Eye, kind: 'browserInspect' }
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

function browserResultURL(output: string): string {
  return /^Browser URL:\s*(.+)$/m.exec(output)?.[1]?.trim() ?? ''
}

function browserTargetLabel(value: string): string {
  if (!value) return ''
  try {
    const url = new URL(value)
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return value
    const path = url.pathname === '/' ? '' : url.pathname.replace(/\/$/, '')
    return `${url.host}${path}`
  } catch {
    return value
  }
}

function browserVerb(
  kind: 'browserOpen' | 'browserTabs' | 'browserInspect',
  status: ToolItem['status'],
  t: ReturnType<typeof useI18n>['t'],
): string {
  if (kind === 'browserOpen') {
    if (status === 'error') return t('tool.browserOpenFailed')
    return t(status === 'complete' ? 'tool.browserOpened' : 'tool.browserOpening')
  }
  if (kind === 'browserTabs') {
    if (status === 'error') return t('tool.browserTabsFailed')
    return t(status === 'complete' ? 'tool.browserTabsChecked' : 'tool.browserTabsChecking')
  }
  if (status === 'error') return t('tool.browserReadFailed')
  return t(status === 'complete' ? 'tool.browserRead' : 'tool.browserReading')
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
    record.task_id
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
  compact = false,
}: {
  status: ToolItem['status']
  generatedBytes?: number
  lineCount?: number
  compact?: boolean
}) {
  const { t } = useI18n()
  if (status === 'preparing' || status === 'running') {
    const detail =
      lineCount !== undefined
        ? t('tool.lines', { count: lineCount })
        : status === 'preparing' && generatedBytes
          ? formatBytes(generatedBytes)
          : status === 'running' && !compact
            ? t('tool.running')
            : ''
    return (
      <span className="ml-auto flex shrink-0 items-center gap-1 text-[0.75rem] text-ink-muted">
        <LoaderCircle className="size-3 animate-spin" aria-hidden="true" />
        {detail}
      </span>
    )
  }
  if (compact) return null
  if (status === 'error') {
    return (
      <span className="ml-auto flex shrink-0 items-center gap-1 text-[0.75rem] text-danger-soft">
        <CircleX className="size-3" aria-hidden="true" />
        {t('tool.failed')}
      </span>
    )
  }
  return null
}

function DetailBlock({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="border-b border-edge last:border-b-0">
      <div className="border-b border-edge bg-canvas-raised px-3 py-1 text-[0.71875rem] font-medium tracking-wide text-ink-muted uppercase">
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
        'mt-1 ml-5 overflow-hidden rounded-lg border border-edge-strong/80 bg-canvas max-md:ml-0',
        failed && 'border-danger-edge bg-danger-surface/60',
      )}
    >
      <div
        className="overflow-hidden border-b border-edge-strong/70 bg-canvas px-3 py-1 font-mono text-[0.75rem] text-ink-muted text-ellipsis whitespace-nowrap"
        title={path}
      >
        {path}
      </div>
      {content.hasLineNumbers && !failed ? (
        <>
          <div
            className="code-scroll-area grid max-h-[min(52vh,32.5rem)] grid-cols-[3.25rem_minmax(max-content,1fr)] overflow-auto bg-canvas"
            role="region"
            aria-label={t('tool.contentsOf', { path })}
            tabIndex={0}
          >
            <pre className="sticky left-0 z-10 m-0 border-r border-edge bg-canvas px-2.5 py-1 text-right font-mono text-[0.8125rem] leading-4.5 whitespace-pre text-ink-faint select-none">
              {content.lineNumbers}
            </pre>
            <pre className="m-0 min-w-full bg-transparent px-2.5 py-1 font-mono text-[0.8125rem] leading-4.5 whitespace-pre text-ink">
              <code
                className="or-code-theme hljs block min-w-full bg-transparent! p-0!"
                dangerouslySetInnerHTML={{ __html: html }}
              />
            </pre>
          </div>
          {content.notice && (
            <div className="border-t border-edge bg-canvas px-3 py-1.5 font-mono text-[0.71875rem] text-ink-muted">
              {content.notice}
            </div>
          )}
        </>
      ) : (
        <pre
          className={cn(
            'code-scroll-area m-0 max-h-[min(52vh,32.5rem)] overflow-auto bg-transparent px-2.5 py-1 font-mono text-[0.8125rem] leading-4.5 whitespace-pre-wrap text-ink-soft',
            failed && 'text-danger',
          )}
        >
          {output}
        </pre>
      )}
    </div>
  )
}

type BrowserInspectionContent = {
  url: string
  title: string
  status: string
  visibleText: string
  truncated: boolean
}

function parseBrowserInspection(output: string): BrowserInspectionContent | undefined {
  const lines = output.trim().split('\n')
  const visibleIndex = lines.findIndex((line) => line.startsWith('Visible text:'))
  const url = lines.find((line) => line.startsWith('Browser URL:'))?.slice(12).trim() ?? ''
  if (!url || visibleIndex < 0) return undefined
  const title = lines.find((line) => line.startsWith('Title:'))?.slice(6).trim() ?? ''
  const status = lines.find((line) => line.startsWith('Page status:'))?.slice(12).trim() ?? ''
  const inlineText = lines[visibleIndex]?.slice('Visible text:'.length).trim() ?? ''
  const visibleLines = inlineText ? [inlineText] : lines.slice(visibleIndex + 1)
  const truncated = visibleLines.at(-1) === '[Visible text truncated]'
  if (truncated) visibleLines.pop()
  const visibleText = visibleLines.join('\n').trim()
  return {
    url,
    title,
    status,
    visibleText: visibleText === '(none)' ? '' : visibleText,
    truncated,
  }
}

function BrowserInspectionPreview({
  output,
  failed,
}: {
  output: string
  failed: boolean
}) {
  const { t } = useI18n()
  if (failed) {
    return (
      <div className="mt-1 ml-5 rounded-md border-l-2 border-danger-edge bg-danger-surface/50 px-3 py-1 text-[0.8125rem] leading-5 text-danger max-md:ml-0">
        {output || t('tool.browserInspectionFailed')}
      </div>
    )
  }

  const content = parseBrowserInspection(output)
  if (!content) {
    return (
      <pre className="code-scroll-area mt-1 ml-5 max-h-72 overflow-auto rounded-md border border-edge bg-canvas px-3 py-2 font-mono text-[0.8125rem] leading-5 whitespace-pre-wrap text-ink-soft max-md:ml-0">
        {output}
      </pre>
    )
  }

  return (
    <div className="mt-1 ml-5 max-w-full overflow-hidden rounded-lg border border-edge/90 bg-canvas max-md:ml-0">
      <div className="border-b border-edge/80 px-3 py-2">
        <div className="flex min-w-0 items-center gap-2">
          <span className="min-w-0 flex-1 truncate text-[0.8125rem] font-medium text-ink-soft">
            {content.title || browserTargetLabel(content.url)}
          </span>
          {content.status && (
            <span className="shrink-0 text-[0.6875rem] text-ink-faint">
              {content.status === 'ready' ? t('tool.browserReady') : content.status}
            </span>
          )}
        </div>
        <div className="truncate font-mono text-[0.71875rem] leading-4 text-ink-muted" title={content.url}>
          {content.url}
        </div>
      </div>
      <div
        className="code-scroll-area max-h-72 overflow-auto px-3 py-2 text-[0.8125rem] leading-5 whitespace-pre-wrap text-ink-soft"
        role="region"
        aria-label={t('tool.browserVisibleText')}
        tabIndex={0}
      >
        {content.visibleText || (
          <span className="text-ink-faint">{t('tool.browserNoVisibleText')}</span>
        )}
      </div>
      {content.truncated && (
        <div className="border-t border-edge/80 px-3 py-1 text-[0.6875rem] text-ink-faint">
          {t('tool.browserTextTruncated')}
        </div>
      )}
    </div>
  )
}

function InspectPreview({ output, failed }: { output: string; failed: boolean }) {
  const { t } = useI18n()
  if (failed) {
    return (
      <div className="mt-1 ml-5 rounded-md border-l-2 border-danger-edge bg-danger-surface/50 px-3 py-1 font-mono text-[0.8125rem] leading-5 text-danger max-md:ml-0">
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
    <div className="mt-1 ml-5 max-w-full overflow-hidden rounded-lg border border-edge/90 bg-canvas-raised/70 max-md:ml-0">
      <div className="flex h-7 items-center px-3 text-[0.75rem] text-ink-muted">
        {empty
          ? t('tool.noMatchingFiles')
          : `${paths.length} ${paths.length === 1 ? t('tool.path') : t('tool.paths')}`}
      </div>
      {!empty && (
        <div
          className="code-scroll-area max-h-72 overflow-auto border-t border-edge/80 bg-canvas-raised py-1"
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
                className="group flex min-h-5 min-w-max items-center gap-2 px-2.5 text-ink-soft transition-colors duration-100 hover:bg-surface-active hover:text-ink"
              >
                <PathIcon
                  className="size-3.25 shrink-0 text-ink-faint transition-colors group-hover:text-ink-muted"
                  aria-hidden="true"
                />
                <code className="pr-4 font-mono text-[0.8125rem] leading-4.5">{path}</code>
              </div>
            )
          })}
        </div>
      )}
      {notice && (
        <div className="border-t border-edge/80 px-3 py-1.5 text-[0.71875rem] leading-4 text-ink-muted">
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
  const { t } = useI18n()
  const log = output
  return (
    <div
      className={cn(
        'mt-1 ml-5 overflow-hidden rounded-lg border border-edge bg-canvas antialiased max-md:ml-0',
        failed && 'border-danger-edge bg-danger-surface/60',
      )}
    >
      <div className="flex min-h-7 items-start gap-2 px-2.5 py-1.5 font-mono text-[0.8125rem] leading-4.5 font-normal">
        <span className="shrink-0 text-ink-faint select-none">$</span>
        <code className="min-w-0 flex-1 overflow-auto whitespace-pre-wrap text-ink-soft">
          {command}
        </code>
        {log && <CopyButton value={log} className="ml-auto -mr-0.5" />}
      </div>
      {log && (
        <pre
          className={cn(
            'code-scroll-area m-0 max-h-[min(46vh,26.25rem)] overflow-auto border-t border-edge bg-canvas px-2.5 py-1.5 font-mono text-[0.8125rem] leading-4.5 font-normal tracking-[0.005em] whitespace-pre text-ink-muted',
            failed && 'border-danger-edge bg-danger-surface/40 text-danger',
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
  const hint = relativize(rawHint, cwd)
  const command = relativize(rawCommand, cwd)
  const browserKind =
    kind === 'browserOpen' || kind === 'browserTabs' || kind === 'browserInspect'
  const verb = browserKind
    ? browserVerb(kind, item.status, t)
    : t(`tool.${kind}`)
  const description = kind === 'run' ? commandDescription(item.args) : ''
  const skillTitle = kind === 'skill' ? skillField(item.args, 'name') : ''
  const skillArgs = kind === 'skill' ? skillField(item.args, 'arguments') : ''
  const lineCount = generatedLineCount(kind, item.args)
  const preparingLabel =
    item.status === 'preparing' && item.args === undefined
      ? kind === 'browserOpen'
        ? t('tool.browserOpeningPage')
        : kind === 'browserTabs'
          ? t('tool.browserTabsChecking')
        : kind === 'browserInspect'
          ? t('tool.browserReadingPage')
          : kind === 'write'
            ? t('tool.preparingWrite')
            : kind === 'edit'
              ? t('tool.preparingEdit')
              : t('tool.preparing')
      : ''
  const target =
    kind === 'browserOpen'
      ? browserTargetLabel(hint) || t('tool.browserPage')
      : kind === 'browserTabs'
        ? t('tool.browserTabs')
      : kind === 'browserInspect'
        ? browserTargetLabel(browserResultURL(item.result || '')) || t('tool.browserCurrentPage')
        : kind === 'run'
          ? stripLeadingCd(command)
          : kind === 'skill'
            ? skillTitle || item.name
            : hint || item.name
  const targetTitle =
    kind === 'browserTabs'
      ? t('tool.browserTabs')
      : kind === 'browserInspect'
      ? browserResultURL(item.result || '') || t('tool.browserCurrentPage')
      : kind === 'run'
        ? rawCommand
        : kind === 'skill'
          ? skillTitle
          : rawHint || item.name
  const fileChange = item.change?.changeType === 'file' ? item.change : undefined
  const changedFilename = fileChange?.path.split('/').filter(Boolean).pop() || fileChange?.path
  const hasDetails =
    kind === 'browserOpen'
      ? false
      : kind === 'browserTabs'
        ? Boolean(item.result || item.status === 'error')
      : kind === 'browserInspect'
        ? Boolean(item.result || item.status === 'error')
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
    <span className="flex min-h-6 min-w-0 flex-1 items-center gap-2 text-[1.03125rem] leading-6 text-ink-muted transition-colors group-hover:text-ink">
      <Icon
        className={cn(
          'size-4 shrink-0 transition-colors',
          kind === 'kill' && 'text-danger-soft group-hover:text-danger-soft',
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
            className="min-w-0 overflow-hidden font-normal text-ink-muted underline decoration-ink-faint/70 underline-offset-2 text-ellipsis whitespace-nowrap transition-colors group-hover:text-ink"
            title={fileChange.path}
          >
            {changedFilename}
          </span>
          <span className="flex shrink-0 gap-1 font-mono text-[0.75rem] font-normal">
            <span className="text-success">+{fileChange.additions || 0}</span>
            <span className="text-danger">-{fileChange.deletions || 0}</span>
          </span>
        </>
      ) : description ? (
        <span
          className="min-w-0 overflow-hidden font-normal text-ellipsis whitespace-nowrap transition-colors"
          title={targetTitle}
        >
          {description}
        </span>
      ) : browserKind ? (
        <span
          className="min-w-0 overflow-hidden font-normal text-ink-muted text-ellipsis whitespace-nowrap transition-colors group-hover:text-ink"
          title={targetTitle}
        >
          {target}
        </span>
      ) : (
        <code
          className="min-w-0 overflow-hidden font-mono text-[1.03125rem] leading-6 font-normal text-ink-muted text-ellipsis whitespace-nowrap transition-colors group-hover:text-ink"
          title={targetTitle}
        >
          {target}
        </code>
      )}
      <Status
        status={item.status}
        generatedBytes={item.generatedBytes}
        lineCount={lineCount}
        compact={browserKind}
      />
    </span>
  )

  if (!hasDetails) {
    return <div className="group my-1 flex w-fit max-w-full animate-[fade-in_160ms_ease-out]">{summary}</div>
  }

  return (
    <Collapsible className="my-1 animate-[fade-in_160ms_ease-out]">
      <CollapsibleTrigger className="group inline-flex max-w-full cursor-pointer items-center border-0 bg-transparent p-0 text-left outline-none focus-visible:rounded-sm focus-visible:bg-canvas-sunken focus-visible:text-ink">
        {summary}
        <ChevronRight
          className="ml-1 size-3.5 shrink-0 text-ink-faint transition-[transform,color] group-hover:text-ink group-data-[state=open]:rotate-90"
          aria-hidden="true"
        />
      </CollapsibleTrigger>
      <CollapsibleContent>
        {kind === 'browserInspect' ? (
          <BrowserInspectionPreview output={item.result || ''} failed={item.status === 'error'} />
        ) : kind === 'skill' ? (
          <div className="mt-1 ml-5 overflow-hidden rounded-lg border border-edge bg-canvas max-md:ml-0">
            {skillArgs && (
              <DetailBlock title={t('tool.skillArguments')}>
                <pre className="m-0 max-h-80 overflow-auto bg-canvas px-2.5 py-1.5 font-mono text-[0.8125rem] leading-4.5 whitespace-pre-wrap text-ink-soft">
                  {skillArgs}
                </pre>
              </DetailBlock>
            )}
            {item.status === 'error' && (
              <DetailBlock title={t('tool.errorOutput')}>
                <pre className="m-0 max-h-80 overflow-auto bg-danger-surface/50 px-2.5 py-1.5 font-mono text-[0.8125rem] leading-4.5 whitespace-pre-wrap text-danger">
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
          <div className="mt-1 ml-5 overflow-hidden rounded-lg border border-edge bg-canvas max-md:ml-0">
            {args && (
              <DetailBlock title={t('tool.input')}>
                <pre className="m-0 max-h-80 overflow-auto bg-canvas px-2.5 py-1.5 font-mono text-[0.8125rem] leading-4.5 whitespace-pre-wrap text-ink-soft">
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
                    'm-0 max-h-80 overflow-auto bg-canvas px-2.5 py-1.5 font-mono text-[0.8125rem] leading-4.5 whitespace-pre-wrap text-ink-soft',
                    item.status === 'error' && 'bg-danger-surface/50 text-danger',
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
