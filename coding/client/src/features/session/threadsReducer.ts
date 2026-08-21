import type {
  BrowserResult,
  ConnectionStatus,
  DeliveryMode,
  Item,
  MessageFile,
  MessageImage,
  PreviewState,
  ThreadSnapshot,
  WireEvent,
} from '@/types'
import {
  createThreadState,
  type ThreadsState,
} from './threadState'
import { reduceWire } from './wireReducer'

export type ThreadAction =
  | { t: 'reset'; sessionID: string; history: ThreadSnapshot }
  | { t: 'wire'; sessionID: string; ev: WireEvent; serverEventSeq?: number }
  | { t: 'status'; sessionID: string; status: ConnectionStatus }
  | { t: 'running'; sessionID: string; running: boolean }
  | {
      t: 'sendUser'
      sessionID: string
      id: string
      text: string
      images: MessageImage[]
      files?: MessageFile[]
      startedAt: string
      delivery?: DeliveryMode
    }
  | { t: 'queueFailed'; sessionID: string; id: string }
  | { t: 'queueStatus'; sessionID: string; id: string; status: 'queued' | 'removing' }
  | { t: 'queueRemove'; sessionID: string; id: string }
  | {
      t: 'contextInvalidate'
      sessionID: string
      provider: string
      model: string
      contextWindow: number
    }
  | { t: 'resolveApproval'; sessionID: string; id: string }
  | { t: 'resolveQuestion'; sessionID: string; id: string }
  | {
      t: 'browserResultQueued'
      sessionID: string
      id: string
      result: BrowserResult
    }
  | { t: 'browserResultAcknowledged'; sessionID: string; id: string }
  | { t: 'browserTabsHandled'; sessionID: string; id: string }
  | { t: 'browserInspectionHandled'; sessionID: string; id: string }
  | { t: 'forget'; sessionID: string }

export function threadsReducer(state: ThreadsState, action: ThreadAction): ThreadsState {
  if (action.t === 'forget') {
    const next = { ...state }
    delete next[action.sessionID]
    return next
  }
  const current = state[action.sessionID] ?? createThreadState()
  let next = current

  switch (action.t) {
    case 'reset': {
      const browserResultOutbox = current.browserResultOutbox
      next = {
        ...createThreadState(),
        status: current.status,
        running: action.history.running,
        serverEventSeq: action.history.eventSeq ?? 0,
        loaded: true,
        todos: action.history.todos ?? null,
        tasks: Object.fromEntries(
          (action.history.tasks ?? []).map((task) => [task.id, task]),
        ),
      }
      for (const ev of action.history.events) next = reduceWire(next, ev)
      for (const ev of action.history.queue ?? []) next = reduceWire(next, ev)
      const restoredPreview = next.preview
      const preview = browserResultOutbox[restoredPreview?.commandID ?? '']
        ? withoutBrowserCommand(restoredPreview)
        : restoredPreview ?? current.preview
      next = {
        ...next,
        browserResultOutbox,
        browserCommands: next.browserCommands.filter(
          (command) => !browserResultOutbox[command.commandID],
        ),
        contextUsage: action.history.context,
        preview,
        // History makes the last preview available as a tab, but only a live
        // open_preview event should bring the workbench forward.
        previewOpen:
          restoredPreview?.commandID && !browserResultOutbox[restoredPreview.commandID]
            ? restoredPreview.disposition !== 'new_background_tab'
            : Boolean(current.previewOpen && preview),
        running: action.history.running,
        items: action.history.running ? next.items : completeOpenRun(next.items),
      }
      break
    }
    case 'status':
      if (current.status === action.status) return state
      next = { ...current, status: action.status }
      break
    case 'running':
      if (current.running === action.running) return state
      next = {
        ...current,
        running: action.running,
        autoCompacting: action.running ? current.autoCompacting : false,
        items: action.running ? current.items : completeOpenRun(current.items),
      }
      break
    case 'sendUser':
      next = action.delivery
        ? {
            ...current,
            running: true,
            queue: [
              ...current.queue,
              {
                id: action.id,
                text: action.text,
                images: action.images,
                ...(action.files?.length ? { files: action.files } : {}),
                delivery: action.delivery,
                status: 'queued',
              },
            ],
          }
        : {
            ...current,
            seq: current.seq + 1,
            running: true,
            items: [
              ...current.items,
              {
                kind: 'user',
                id: action.id,
                text: action.text,
                images: action.images,
                ...(action.files?.length ? { files: action.files } : {}),
                sentAt: action.startedAt,
                deliveryStatus: 'sending',
              },
              {
                kind: 'run',
                id: `run-${action.id}`,
                startedAt: action.startedAt,
              },
            ],
          }
      break
    case 'queueFailed':
      next = {
        ...current,
        queue: current.queue.map((message) =>
          message.id === action.id ? { ...message, status: 'failed' } : message,
        ),
        items: current.items.map((item) =>
          item.kind === 'user' && item.id === action.id
            ? { ...item, deliveryStatus: 'failed' }
            : item,
        ),
      }
      break
    case 'queueStatus':
      next = {
        ...current,
        queue: current.queue.map((message) =>
          message.id === action.id ? { ...message, status: action.status } : message,
        ),
      }
      break
    case 'queueRemove':
      next = {
        ...current,
        queue: current.queue.filter((message) => message.id !== action.id),
      }
      break
    case 'contextInvalidate':
      next = {
        ...current,
        contextUsage: {
          provider: action.provider,
          model: action.model,
          usedTokens: 0,
          contextWindow: action.contextWindow,
          measured: false,
        },
      }
      break
    case 'resolveApproval':
      next = {
        ...current,
        items: current.items.filter(
          (item) => !(item.kind === 'approval' && item.id === action.id),
        ),
      }
      break
    case 'resolveQuestion':
      next = {
        ...current,
        items: current.items.filter(
          (item) => !(item.kind === 'question' && item.id === action.id),
        ),
      }
      break
    case 'browserResultQueued':
      if (current.browserResultOutbox[action.id]) return state
      next = {
        ...current,
        browserCommands: current.browserCommands.filter(
          (command) => command.commandID !== action.id,
        ),
        browserResultOutbox: {
          ...current.browserResultOutbox,
          [action.id]: { commandID: action.id, result: action.result },
        },
        preview: withoutBrowserCommand(current.preview, action.id),
      }
      break
    case 'browserResultAcknowledged': {
      if (!current.browserResultOutbox[action.id]) return state
      const browserResultOutbox = { ...current.browserResultOutbox }
      delete browserResultOutbox[action.id]
      next = { ...current, browserResultOutbox }
      break
    }
    case 'browserInspectionHandled':
      next = {
        ...current,
        browserInspections: current.browserInspections.filter(
          (command) => command.commandID !== action.id,
        ),
      }
      break
    case 'browserTabsHandled':
      next = {
        ...current,
        browserTabsRequests: current.browserTabsRequests.filter(
          (command) => command.commandID !== action.id,
        ),
      }
      break
    case 'wire': {
      if (
        action.serverEventSeq !== undefined &&
        action.serverEventSeq <= current.serverEventSeq
      ) {
        return state
      }
      next = reduceWire(current, action.ev)
      if (
        action.serverEventSeq !== undefined &&
        action.serverEventSeq > current.serverEventSeq
      ) {
        next = { ...next, serverEventSeq: action.serverEventSeq }
      }
      break
    }
  }

  return { ...state, [action.sessionID]: next }
}

function withoutBrowserCommand(
  preview: PreviewState | undefined,
  commandID?: string,
): PreviewState | undefined {
  if (!preview?.commandID || (commandID && preview.commandID !== commandID)) return preview
  const completed = { ...preview }
  delete completed.commandID
  return completed
}

function elapsedSince(startedAt: string): number {
  const start = new Date(startedAt).getTime()
  return Number.isFinite(start) ? Math.max(0, Date.now() - start) : 0
}

function lastIndex(items: Item[], pred: (it: Item) => boolean): number {
  for (let i = items.length - 1; i >= 0; i--) if (pred(items[i])) return i
  return -1
}

function replaceAt(items: Item[], index: number, next: Item): Item[] {
  const copy = items.slice()
  copy[index] = next
  return copy
}

function completeOpenRun(items: Item[]): Item[] {
  const index = lastIndex(items, (item) => item.kind === 'run' && item.durationMs === undefined)
  if (index < 0) return items
  const run = items[index] as Extract<Item, { kind: 'run' }>
  return replaceAt(items, index, { ...run, durationMs: elapsedSince(run.startedAt) })
}
