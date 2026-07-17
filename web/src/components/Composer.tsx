import { useEffect, useRef } from 'react'
import { ArrowUp, Square } from 'lucide-react'
import type { ConfirmItem } from '@/types'
import { Approval } from './Approval'

export function Composer({
  connected,
  running,
  confirmation,
  onSend,
  onStop,
  onResolve,
}: {
  connected: boolean
  running: boolean
  confirmation?: ConfirmItem
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
    <footer className="z-30 shrink-0 bg-[#fcfcfb] px-6 pt-3 pb-3 max-md:px-3 max-md:pt-2">
      <div className="mx-auto flex w-full max-w-[896px] flex-col gap-2.5">
        {confirmation && <Approval key={confirmation.id} item={confirmation} onResolve={onResolve} />}

        <div>
          <div className="overflow-hidden rounded-xl border border-stone-300 bg-white transition-colors focus-within:border-stone-500">
            <textarea
              ref={ref}
              rows={1}
              disabled={disabled}
              className="block max-h-[168px] min-h-12 w-full resize-none border-0 bg-transparent px-3.5 pt-3 pb-1.5 text-[15px] leading-6 text-stone-900 outline-none placeholder:text-stone-400 disabled:cursor-not-allowed disabled:bg-stone-50/40"
              placeholder={
                awaitingApproval
                  ? 'Resolve the approval above to continue…'
                  : connected
                    ? 'Ask OR coding…'
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
            <div className="flex min-h-9 items-center justify-between px-2.5 pb-2">
              <div className="flex items-center gap-2 text-[10.5px] text-stone-400">
                <span className="rounded bg-[#f3f1e8] px-1.5 py-0.5 font-medium text-[#7d722c]">
                  Auto
                </span>
                <span className="max-md:hidden">
                  {awaitingApproval ? 'Waiting for your decision' : 'Writes require approval'}
                </span>
              </div>
              {running && !awaitingApproval ? (
                <button
                  className="grid size-7 place-items-center rounded-md bg-stone-700 text-white transition-colors hover:bg-stone-800 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-400"
                  type="button"
                  title="Stop"
                  onClick={onStop}
                >
                  <Square className="size-2.5 fill-current" aria-hidden="true" />
                  <span className="sr-only">Stop</span>
                </button>
              ) : (
                <button
                  className="grid size-7 place-items-center rounded-md bg-stone-800 text-white transition-colors hover:bg-stone-950 disabled:cursor-not-allowed disabled:opacity-25 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-400"
                  type="button"
                  title={
                    awaitingApproval
                      ? 'Resolve the approval first'
                      : connected
                        ? 'Send'
                        : 'Waiting for coding API'
                  }
                  disabled={!connected || awaitingApproval}
                  onClick={submit}
                >
                  <ArrowUp className="size-3.5" aria-hidden="true" />
                  <span className="sr-only">Send</span>
                </button>
              )}
            </div>
          </div>
          <div className="mt-1.5 text-center text-[10px] text-stone-400">
            {awaitingApproval
              ? 'Choose Allow once or Deny to continue'
              : 'Enter to send · Shift + Enter for a new line'}
          </div>
        </div>
      </div>
    </footer>
  )
}
