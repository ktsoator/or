import { useState } from 'react'
import { Check, Copy } from 'lucide-react'
import type { Change, Hunk as HunkType } from '@/types'
import { highlightCode, languageForPath } from '@/lib/highlight'
import { cn } from '@/lib/utils'
import { useI18n } from '@/i18n'

export function FileChange({ change }: { change: Change }) {
  const { t } = useI18n()
  const [copied, setCopied] = useState(false)
  if (change.changeType === 'failure') {
    return (
      <div className="mt-1 ml-5 border-l-2 border-danger-edge py-1 pl-3 font-mono text-[0.8125rem] leading-5.5 text-danger max-md:ml-0">
        {(change.path ? `${change.path}: ` : '') + (change.detail || t('diff.writeFailed'))}
      </div>
    )
  }

  const hunks = Array.isArray(change.hunks) ? change.hunks : []
  const filename = change.path.split('/').filter(Boolean).pop() || change.path
  const language = languageForPath(change.path)

  const copyDiff = async () => {
    const diff = hunks
      .map((hunk) => [
        `@@ -${hunk.oldStart},${hunk.oldLines} +${hunk.newStart},${hunk.newLines} @@`,
        ...(hunk.lines ?? []),
      ].join('\n'))
      .join('\n')
    try {
      await navigator.clipboard.writeText(diff)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1600)
    } catch {
      // Clipboard access can be unavailable in non-secure browser contexts.
    }
  }

  return (
    <div className="mt-1 ml-5 overflow-hidden rounded-lg border border-edge-strong/80 bg-canvas max-md:ml-0">
      <div className="flex h-7 min-w-0 items-center gap-1.5 border-b border-edge-strong/70 bg-canvas-raised/60 px-2.5">
        <span
          className="min-w-0 overflow-hidden text-[0.8125rem] font-normal text-ink-soft underline decoration-ink-faint/70 underline-offset-2 text-ellipsis whitespace-nowrap"
          title={change.path}
        >
          {filename}
        </span>
        <span className="flex shrink-0 gap-1 font-mono text-[0.75rem] font-normal">
          <span className="text-success">+{change.additions || 0}</span>
          <span className="text-danger">-{change.deletions || 0}</span>
        </span>
        <button
          className="ml-auto grid size-6 shrink-0 cursor-pointer place-items-center rounded text-ink-faint outline-none transition-colors hover:bg-canvas-sunken hover:text-ink focus-visible:bg-canvas-sunken focus-visible:text-ink"
          type="button"
          title={copied ? t('diff.copied') : t('diff.copy')}
          aria-label={copied ? t('diff.copied') : t('diff.copy')}
          onClick={copyDiff}
        >
          {copied ? (
            <Check className="size-3.5" aria-hidden="true" />
          ) : (
            <Copy className="size-3.5" aria-hidden="true" />
          )}
        </button>
      </div>

      {hunks.length > 0 && (
        <div className="max-h-[28.75rem] overflow-auto bg-canvas-raised [scrollbar-color:var(--scrollbar-code-thumb)_transparent] [scrollbar-width:thin]">
          {hunks.map((hunk, index) => (
            <Hunk key={index} hunk={hunk} language={language} />
          ))}
        </div>
      )}
    </div>
  )
}

function Hunk({ hunk, language }: { hunk: HunkType; language: string }) {
  let oldLine = hunk.oldStart
  let newLine = hunk.newStart

  return (
    <div className="border-b border-edge-strong/70 last:border-b-0">
      <div className="bg-canvas-sunken px-2.5 py-0.5 font-mono text-[0.6875rem] leading-4 font-medium text-ink-muted">
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
              'grid min-h-[1.125rem] grid-cols-[1.375rem_2.25rem_minmax(max-content,1fr)] font-mono text-[0.8125rem] leading-4.5 text-ink',
              isAdd && 'bg-diff-add',
              isDelete && 'bg-diff-delete',
            )}
          >
            <span
              className={cn(
                'pl-2 text-ink-faint select-none',
                isAdd && 'text-success',
                isDelete && 'text-danger',
              )}
            >
              {isAdd ? '+' : isDelete ? '−' : ''}
            </span>
            <span className="pr-2 text-right text-ink-muted select-none">{number}</span>
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
