import { useEffect, useState } from 'react'
import { CircleAlert, LoaderCircle } from 'lucide-react'
import type { Item } from '@/types'
import { useI18n } from '@/i18n'
import { formatMessageTime } from '@/lib/time'
import { Markdown } from './Markdown'
import { ResponseActions } from './ResponseActions'
import { Thinking } from './Thinking'
import { ToolCard } from './ToolCard'

export function AwaitingResponse() {
  const { t } = useI18n()
  return (
    <div
      className="my-1 flex animate-[fade-in_160ms_ease-out] items-center gap-1.5 py-0.5 text-[0.8125rem] text-stone-400"
      role="status"
    >
      <span className="size-1 animate-pulse rounded-full bg-indigo-500" />
      <span className="streaming-sheen">{t('thinking.working')}</span>
    </div>
  )
}

export function AutoCompactionStatus() {
  const { t } = useI18n()
  const [visible, setVisible] = useState(false)

  useEffect(() => {
    const timer = window.setTimeout(() => setVisible(true), 350)
    return () => window.clearTimeout(timer)
  }, [])

  if (!visible) return null
  return (
    <div
      className="my-1 flex animate-[fade-in_160ms_ease-out] items-center gap-1.5 py-0.5 text-[0.8125rem] text-stone-400"
      role="status"
    >
      <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
      <span>{t('compaction.automatic')}</span>
    </div>
  )
}

export function ThreadItem({ item, cwd }: { item: Item; cwd?: string }) {
  const { locale, t } = useI18n()
  switch (item.kind) {
    case 'user':
      return (
        <section className="my-3.5 flex animate-[fade-in_160ms_ease-out] justify-end">
          <div className="flex max-w-[78%] flex-col items-end gap-2 max-md:max-w-[88%]">
            {item.images.length > 0 && (
              <div className="flex max-w-full flex-wrap justify-end gap-2">
                {item.images.map((image, index) => (
                  <img
                    key={`${image.mimeType}-${index}`}
                    className="size-[8.5rem] shrink-0 rounded-2xl border border-stone-200 bg-white object-cover shadow-[0_7px_18px_-15px_rgba(28,25,23,0.55)] max-sm:size-28"
                    src={`data:${image.mimeType};base64,${image.data}`}
                    alt={t('app.uploadedImage', { index: index + 1 })}
                  />
                ))}
              </div>
            )}
            {item.text && (
              <div className="rounded-[10px] bg-stone-100 px-3 py-2 text-[14px] leading-[22px] whitespace-pre-wrap">
                {item.text}
              </div>
            )}
            {(item.sentAt || item.deliveryStatus === 'failed') && (
              <div className="-mt-0.5 flex items-center justify-end gap-2 px-1 text-[0.75rem] leading-4 tabular-nums">
                {item.deliveryStatus === 'failed' && (
                  <span className="text-red-600">{t('app.notSent')}</span>
                )}
                {item.sentAt && (
                  <time className="text-stone-400" dateTime={item.sentAt}>
                    {formatMessageTime(item.sentAt, locale)}
                  </time>
                )}
              </div>
            )}
          </div>
        </section>
      )
    case 'assistant':
      return (
        <section className="my-3 animate-[fade-in_160ms_ease-out]">
          <Markdown source={item.markdown} />
          {item.complete && (
            <ResponseActions
              usage={item.usage}
              modelName={item.modelName || item.model}
              responseText={item.markdown}
              completedAt={item.completedAt}
            />
          )}
        </section>
      )
    case 'run':
      return <RunDuration item={item} />
    case 'thinking':
      return <Thinking item={item} />
    case 'tool':
      return <ToolCard item={item} cwd={cwd} />
    case 'error':
      return (
        <div
          className="my-3 flex animate-[fade-in_160ms_ease-out] gap-2.5 border-l-2 border-red-300 py-1 pl-3 text-red-700"
          role="alert"
        >
          <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
          <div className="flex flex-col gap-0.5">
            <strong className="text-[0.8125rem] font-semibold">{t('app.somethingWentWrong')}</strong>
            <span className="text-[0.8125rem]">{item.text}</span>
          </div>
        </div>
      )
  }
}

function RunDuration({ item }: { item: Extract<Item, { kind: 'run' }> }) {
  const { locale, t } = useI18n()
  const [now, setNow] = useState(() => Date.now())
  const running = item.durationMs === undefined

  useEffect(() => {
    if (!running) return
    setNow(Date.now())
    const interval = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(interval)
  }, [item.startedAt, running])

  const startedAt = new Date(item.startedAt).getTime()
  const durationMs =
    item.durationMs ?? (Number.isFinite(startedAt) ? Math.max(0, now - startedAt) : 0)
  const duration = formatRunDuration(durationMs, locale)

  return (
    <div className="mt-3.5 mb-2.5 animate-[fade-in_160ms_ease-out]">
      <div className="text-[0.8125rem] leading-5 text-stone-400 tabular-nums">
        {t(running ? 'run.working' : 'run.completed', { duration })}
      </div>
    </div>
  )
}

function formatRunDuration(durationMs: number, locale: 'en' | 'zh-CN'): string {
  const totalSeconds = Math.max(0, Math.floor(durationMs / 1000))
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60

  if (locale === 'zh-CN') {
    if (hours > 0) return `${hours} 小时 ${minutes} 分 ${seconds} 秒`
    if (minutes > 0) return `${minutes} 分 ${seconds} 秒`
    return `${seconds} 秒`
  }
  if (hours > 0) return `${hours}h ${minutes}m ${seconds}s`
  if (minutes > 0) return `${minutes}m ${seconds}s`
  return `${seconds}s`
}
