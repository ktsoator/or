import { LoaderCircle, X } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useI18n } from '@/i18n'
import type { QueuedMessage } from '@/types'

export function PendingQueue({
  messages,
  onRemove,
}: {
  messages: QueuedMessage[]
  onRemove: (id: string) => void
}) {
  const { t } = useI18n()

  return (
    <section
      className="overflow-hidden rounded-[18px] border border-edge/90 bg-canvas-raised text-ink-soft shadow-[0_8px_24px_-22px_rgba(28,25,23,0.5)]"
      aria-label={t('queue.pendingMessages')}
      aria-live="polite"
    >
      <div className="flex h-8 items-center justify-between px-3.5 text-[0.71875rem] leading-none text-ink-muted">
        <span className="font-medium text-ink-muted">{t('queue.upNext')}</span>
        <span>{messages.length}</span>
      </div>
      <div className="max-h-[8.25rem] overflow-y-auto border-t border-edge/80">
        {messages.map((message, index) => (
          <div
            key={message.id}
            className={cn(
              'group/queue flex min-h-11 items-center gap-2.5 py-2 pr-2 pl-3.5 text-[0.8125rem]',
              index > 0 && 'border-t border-edge/70',
            )}
          >
            <span
              className={cn(
                'size-1.5 shrink-0 rounded-full',
                message.status === 'failed' ? 'bg-danger-soft' : 'bg-ink-faint',
              )}
              aria-hidden="true"
            />
            <span className="shrink-0 font-medium text-ink-soft">
              {message.delivery === 'steer' ? t('queue.steer') : t('queue.followUp')}
            </span>
            <span className="min-w-0 flex-1 truncate text-ink-muted">
              {message.text ||
                ((message.files?.length ?? 0) > 0
                  ? `${message.files?.length ?? 0} ${
                      message.files?.length === 1 ? t('queue.file') : t('queue.files')
                    }`
                  : `${message.images.length} ${
                      message.images.length === 1 ? t('queue.image') : t('queue.images')
                    }`)}
            </span>
            {message.text && message.images.length > 0 && (
              <span className="shrink-0 text-[0.71875rem] text-ink-faint">
                +{message.images.length}{' '}
                {message.images.length === 1 ? t('queue.image') : t('queue.images')}
              </span>
            )}
            {message.text && (message.files?.length ?? 0) > 0 && (
              <span className="shrink-0 text-[0.71875rem] text-ink-faint">
                +{message.files?.length ?? 0}{' '}
                {message.files?.length === 1 ? t('queue.file') : t('queue.files')}
              </span>
            )}
            <span
              className={cn(
                'shrink-0 text-[0.71875rem]',
                message.status === 'failed' ? 'text-danger-soft' : 'text-ink-faint',
              )}
            >
              {message.status === 'failed'
                ? t('app.notSent')
                : message.status === 'removing'
                  ? t('queue.removing')
                  : t('queue.waiting')}
            </span>
            <button
              className="grid size-7 shrink-0 cursor-pointer place-items-center rounded-lg text-ink-faint outline-none transition-colors hover:bg-canvas-strong/80 hover:text-ink-soft focus-visible:bg-canvas-strong/80 focus-visible:text-ink-soft disabled:cursor-wait disabled:opacity-55"
              type="button"
              aria-label={
                message.delivery === 'steer'
                  ? t('queue.removeSteer')
                  : t('queue.removeFollowUp')
              }
              title={t('queue.remove')}
              disabled={message.status === 'removing'}
              onClick={() => onRemove(message.id)}
            >
              {message.status === 'removing' ? (
                <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
              ) : (
                <X className="size-3.5" aria-hidden="true" />
              )}
            </button>
          </div>
        ))}
      </div>
    </section>
  )
}
