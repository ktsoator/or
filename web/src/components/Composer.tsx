import { useEffect, useRef } from 'react'
import { ArrowUp, Plus, Square } from 'lucide-react'
import type { ConfirmItem } from '@/types'
import { cn } from '@/lib/utils'
import { Approval } from './Approval'

export function Composer({
  connected,
  running,
  confirmation,
  centered = false,
  onSend,
  onStop,
  onResolve,
}: {
  connected: boolean
  running: boolean
  confirmation?: ConfirmItem
  centered?: boolean
  onSend: (text: string) => void
  onStop: () => void
  onResolve: (id: string, allow: boolean) => Promise<void>
}) {
  const ref = useRef<HTMLTextAreaElement>(null)
  const awaitingApproval = Boolean(confirmation)
  const disabled = running || awaitingApproval || !connected

  const autosize = () => {
    const el = ref.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 168) + 'px'
  }

  useEffect(() => {
    if (!disabled) ref.current?.focus()
  }, [disabled])

  const submit = () => {
    const el = ref.current
    if (!el) return
    const text = el.value.trim()
    if (!text || disabled) return
    onSend(text)
    el.value = ''
    autosize()
  }

  return (
    <footer
      className={cn(
        'z-30 w-full',
        centered
          ? 'bg-transparent p-0'
          : 'shrink-0 bg-[#fcfcfb] px-6 pt-3 pb-4 max-md:px-3 max-md:pt-2',
      )}
    >
      <div className="mx-auto flex w-full max-w-[896px] flex-col gap-2">
        {confirmation && <Approval key={confirmation.id} item={confirmation} onResolve={onResolve} />}

        <div className="rounded-[28px] border border-stone-200 bg-white shadow-[0_10px_30px_-20px_rgba(28,25,23,0.38)] transition-[border-color,box-shadow] focus-within:border-stone-400 focus-within:shadow-[0_12px_32px_-20px_rgba(28,25,23,0.48)]">
          <div className="flex min-h-15 items-center gap-3 px-3 py-2.5 max-sm:gap-2">
            <span
              className="grid size-10 shrink-0 place-items-center rounded-full text-stone-700"
              aria-hidden="true"
            >
              <Plus className="size-5" />
            </span>
            <textarea
              ref={ref}
              rows={1}
              disabled={disabled}
              className="block max-h-[168px] min-h-8 min-w-0 flex-1 resize-none border-0 bg-transparent py-1.5 text-[16.5px] leading-6 text-stone-900 outline-none placeholder:text-stone-400 disabled:cursor-not-allowed disabled:bg-transparent"
              placeholder={
                awaitingApproval
                  ? 'Resolve the approval above to continue…'
                  : connected
                    ? 'Ask anything'
                    : 'Waiting for coding API…'
              }
              onInput={autosize}
              onKeyDown={(event) => {
                if (event.key === 'Enter' && !event.shiftKey) {
                  event.preventDefault()
                  submit()
                }
              }}
            />
            <div className="flex shrink-0 items-center gap-2.5 max-sm:gap-1.5">
              <span
                className="px-1 text-[15px] font-medium text-stone-400 max-sm:hidden"
                title={awaitingApproval ? 'Waiting for your decision' : 'Writes require approval'}
              >
                Auto
              </span>
              {running && !awaitingApproval ? (
                <button
                  className="group relative grid size-10 cursor-pointer place-items-center rounded-full bg-stone-700 text-white transition-colors hover:bg-stone-800 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-400"
                  type="button"
                  aria-label="Stop generating"
                  onClick={onStop}
                >
                  <Square className="size-3 fill-current" aria-hidden="true" />
                  <span
                    className="pointer-events-none absolute right-0 bottom-[calc(100%+9px)] z-50 translate-y-1 whitespace-nowrap rounded-md bg-stone-900 px-2.5 py-1.5 text-[12px] leading-4 font-medium text-white opacity-0 shadow-lg transition-[opacity,transform] duration-150 group-hover:translate-y-0 group-hover:opacity-100 group-focus-visible:translate-y-0 group-focus-visible:opacity-100"
                    aria-hidden="true"
                  >
                    Stop generating
                  </span>
                </button>
              ) : (
                <button
                  className="group relative grid size-10 cursor-pointer place-items-center rounded-full bg-black text-white transition-colors hover:bg-stone-800 disabled:cursor-not-allowed disabled:opacity-25 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-400"
                  type="button"
                  aria-label={
                    awaitingApproval
                      ? 'Resolve the approval first'
                      : connected
                        ? 'Send prompt'
                        : 'Waiting for coding API'
                  }
                  disabled={!connected || awaitingApproval}
                  onClick={submit}
                >
                  <ArrowUp className="size-4" aria-hidden="true" />
                  <span
                    className="pointer-events-none absolute right-0 bottom-[calc(100%+9px)] z-50 flex translate-y-1 items-center gap-2 whitespace-nowrap rounded-md bg-stone-900 px-2.5 py-1.5 text-[12px] leading-4 font-medium text-white opacity-0 shadow-lg transition-[opacity,transform] duration-150 group-hover:translate-y-0 group-hover:opacity-100 group-focus-visible:translate-y-0 group-focus-visible:opacity-100"
                    aria-hidden="true"
                  >
                    <span>
                      {awaitingApproval
                        ? 'Resolve approval first'
                        : connected
                          ? 'Send prompt'
                          : 'Waiting for API'}
                    </span>
                    {connected && !awaitingApproval && (
                      <kbd className="font-mono text-[11px] font-normal text-stone-400">↵</kbd>
                    )}
                  </span>
                </button>
              )}
            </div>
          </div>
        </div>
      </div>
    </footer>
  )
}
