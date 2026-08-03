import type {
  ApprovalItem,
  BackgroundTask,
  BrowserCommandState,
  BrowserInspectionCommandState,
  BrowserResult,
  BrowserResultOutboxEntry,
  BrowserTabsCommandState,
  Change,
  ConnectionStatus,
  ContextUsage,
  DeliveryMode,
  Item,
  MessageFile,
  MessageImage,
  PreviewRequest,
  PreviewState,
  QueuedMessage,
  QuestionItem,
  TaskItem,
  ThreadSnapshot,
  ToolOutcome,
  Usage,
  WireEvent,
} from './types'

export type ThreadState = {
  items: Item[]
  tasks: Record<string, BackgroundTask>
  queue: QueuedMessage[]
  responseUsage: Usage
  contextUsage?: ContextUsage
  preview?: PreviewState
  browserCommands: BrowserCommandState[]
  browserResultOutbox: Record<string, BrowserResultOutboxEntry>
  browserTabsRequests: BrowserTabsCommandState[]
  browserInspections: BrowserInspectionCommandState[]
  previewOpen: boolean
  running: boolean
  autoCompacting: boolean
  status: ConnectionStatus
  serverEventSeq: number
  seq: number
  loaded: boolean
}

export type ThreadsState = Record<string, ThreadState>

function recordOf(value: unknown): Record<string, unknown> | undefined {
  return typeof value === 'object' && value !== null
    ? (value as Record<string, unknown>)
    : undefined
}

function outcomeChange(outcome: ToolOutcome | undefined): Change | undefined {
  const data = recordOf(outcome?.data)
  return data?.changeType === 'file' || data?.changeType === 'failure'
    ? (data as Change)
    : undefined
}

function outcomePreview(
  tool: string | undefined,
  outcome: ToolOutcome | undefined,
): PreviewRequest | undefined {
  if (tool !== 'open_preview') return undefined
  const data = recordOf(outcome?.data)
  if (!data || (typeof data.url !== 'string' && typeof data.path !== 'string')) return undefined
  return data as PreviewRequest
}

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

const emptyUsage = (): Usage => ({
  input: 0,
  output: 0,
  cacheRead: 0,
  cacheWrite: 0,
  totalTokens: 0,
  cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 },
})

export const createThreadState = (): ThreadState => ({
  items: [],
  tasks: {},
  queue: [],
  responseUsage: emptyUsage(),
  contextUsage: undefined,
  preview: undefined,
  browserCommands: [],
  browserResultOutbox: {},
  browserTabsRequests: [],
  browserInspections: [],
  previewOpen: false,
  running: false,
  autoCompacting: false,
  status: 'connecting',
  serverEventSeq: 0,
  seq: 0,
  loaded: false,
})

function lastIndex(items: Item[], pred: (it: Item) => boolean): number {
  for (let i = items.length - 1; i >= 0; i--) if (pred(items[i])) return i
  return -1
}

function replaceAt(items: Item[], index: number, next: Item): Item[] {
  const copy = items.slice()
  copy[index] = next
  return copy
}

function hasOnlyPreparingToolsAfter(items: Item[], index: number): boolean {
  return (
    index < items.length - 1 &&
    items
      .slice(index + 1)
      .every((item) => item.kind === 'tool' && item.status === 'preparing')
  )
}

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

export function reduceWire(state: ThreadState, ev: WireEvent): ThreadState {
  let items = state.items
  let tasks = state.tasks
  let queue = state.queue
  let responseUsage = state.responseUsage
  let contextUsage = state.contextUsage
  let preview = state.preview
  let browserCommands = state.browserCommands
  const browserResultOutbox = state.browserResultOutbox
  let browserTabsRequests = state.browserTabsRequests
  let browserInspections = state.browserInspections
  let previewOpen = state.previewOpen
  let running = state.running
  let autoCompacting = state.autoCompacting
  let seq = state.seq
  const nextId = () => `i-${seq++}`

  const closeAssistant = () => {
    items = items.map((it) => (it.kind === 'assistant' && it.open ? { ...it, open: false } : it))
  }
  const completeThinking = () => {
    items = items.map((it) =>
      it.kind === 'thinking' && it.streaming ? { ...it, streaming: false } : it,
    )
  }
  const removePreparingTools = () => {
    items = items.filter((it) => it.kind !== 'tool' || it.status !== 'preparing')
  }
  const completeRun = (durationMs?: number, startedAt?: string) => {
    let idx = startedAt
      ? lastIndex(items, (item) => item.kind === 'run' && item.startedAt === startedAt)
      : -1
    if (idx < 0) {
      idx = lastIndex(items, (item) => item.kind === 'run' && item.durationMs === undefined)
    }
    if (idx < 0) return
    const run = items[idx] as Extract<Item, { kind: 'run' }>
    if (durationMs === undefined && run.durationMs !== undefined) return
    items = replaceAt(items, idx, {
      ...run,
      durationMs:
        durationMs === undefined
          ? elapsedSince(run.startedAt)
          : Math.max(durationMs, elapsedSince(run.startedAt)),
    })
  }

  switch (ev.type) {
    case 'run_start': {
      const exactIndex = ev.startedAt
        ? lastIndex(items, (it) => it.kind === 'run' && it.startedAt === ev.startedAt)
        : -1
      const idx =
        exactIndex >= 0
          ? exactIndex
          : lastIndex(items, (it) => it.kind === 'run' && it.durationMs === undefined)
      if (idx >= 0) {
        const run = items[idx] as Extract<Item, { kind: 'run' }>
        items = replaceAt(items, idx, {
          ...run,
          startedAt: ev.startedAt ?? run.startedAt,
          durationMs: ev.durationMs ?? run.durationMs,
        })
      } else {
        const run = {
          kind: 'run' as const,
          id: ev.id ?? nextId(),
          startedAt: ev.startedAt ?? new Date().toISOString(),
          durationMs: ev.durationMs,
        }
        items = [...items, run]
      }
      const projectedRunIndex = idx >= 0 ? idx : items.length - 1
      const projectedRun = items[projectedRunIndex]
      const precedingItem = items[projectedRunIndex - 1]
      if (projectedRun.kind === 'run' && precedingItem?.kind === 'user' && !precedingItem.sentAt) {
        items = replaceAt(items, projectedRunIndex - 1, {
          ...precedingItem,
          sentAt: projectedRun.startedAt,
        })
      }
      if (projectedRun.kind === 'run' && projectedRun.durationMs === undefined) running = true
      break
    }

    case 'user_message':
      {
        const text = ev.text ?? ''
        const images = ev.images ?? []
        const files = ev.files ?? []
        if (ev.queued && ev.delivery) {
          let queueIndex = ev.id ? queue.findIndex((message) => message.id === ev.id) : -1
          if (queueIndex < 0) {
            queueIndex = queue.findIndex((message) =>
              sameUserMessage(
                message.text,
                message.images,
                message.files,
                text,
                images,
                files,
              ),
            )
          }
          const message: QueuedMessage = {
            id: ev.id ?? `queued-${nextId()}`,
            text,
            images,
            ...(files.length ? { files } : {}),
            delivery: ev.delivery,
            status: 'queued',
          }
          queue =
            queueIndex >= 0
              ? replaceQueueAt(queue, queueIndex, message)
              : [...queue, message]
          break
        }

        let queueIndex = ev.id ? queue.findIndex((message) => message.id === ev.id) : -1
        if (queueIndex < 0) {
          queueIndex = queue.findIndex((message) =>
            sameUserMessage(
              message.text,
              message.images,
              message.files,
              text,
              images,
              files,
            ),
          )
        }
        if (queueIndex >= 0) queue = queue.filter((_, index) => index !== queueIndex)

        let idx = ev.id
          ? items.findIndex((item) => item.kind === 'user' && item.id === ev.id)
          : -1
        if (idx < 0) {
          idx = items.findIndex(
            (item) =>
              item.kind === 'user' &&
              item.deliveryStatus === 'sending' &&
              sameUserMessage(item.text, item.images, item.files, text, images, files),
          )
        }
        if (idx < 0) {
          const runIndex = lastIndex(items, (item) => item.kind === 'run')
          const candidate = items[runIndex - 1]
          if (
            candidate?.kind === 'user' &&
              sameUserMessage(
                candidate.text,
                candidate.images,
                candidate.files,
                text,
                images,
                files,
              )
          ) {
            idx = runIndex - 1
          }
        }
        const openRunIndex = lastIndex(
          items,
          (item) => item.kind === 'run' && item.durationMs === undefined,
        )
        const existingItem = idx >= 0 ? items[idx] : undefined
        const existingUser = existingItem?.kind === 'user' ? existingItem : undefined
        const openRun = openRunIndex >= 0 ? items[openRunIndex] : undefined
        const user = {
          kind: 'user' as const,
          id: ev.id ?? (idx >= 0 ? items[idx].id : nextId()),
          text,
          images,
          ...(files.length ? { files } : {}),
          sentAt:
            existingUser?.sentAt ?? (openRun?.kind === 'run' ? openRun.startedAt : undefined),
        }
        if (idx >= 0) {
          items = replaceAt(items, idx, user)
        } else {
          items =
            openRunIndex >= 0 && !ev.delivery
              ? [...items.slice(0, openRunIndex), user, ...items.slice(openRunIndex)]
              : [...items, user]
        }
      }
      break

    case 'queue_cancelled':
      if (ev.id) {
        queue = queue.map((message) =>
          message.id === ev.id ? { ...message, status: 'failed' } : message,
        )
      }
      break

    case 'queue_removed':
      if (ev.id) queue = queue.filter((message) => message.id !== ev.id)
      break

    case 'delta':
      if (ev.kind === 'thinking') {
        const idx = lastIndex(items, (it) => it.kind === 'thinking' && it.streaming)
        if (idx >= 0) {
          const cur = items[idx] as Extract<Item, { kind: 'thinking' }>
          items = replaceAt(items, idx, { ...cur, text: cur.text + (ev.delta ?? '') })
        } else {
          items = [
            ...items,
            { kind: 'thinking', id: nextId(), text: ev.delta ?? '', streaming: true },
          ]
        }
      } else {
        completeThinking()
        const idx = lastIndex(items, (it) => it.kind === 'assistant' && it.open)
        if (idx >= 0) {
          const cur = items[idx] as Extract<Item, { kind: 'assistant' }>
          items = replaceAt(items, idx, {
            ...cur,
            markdown: cur.markdown + (ev.delta ?? ''),
          })
        } else {
          items = [
            ...items,
            {
              kind: 'assistant',
              id: nextId(),
              markdown: ev.delta ?? '',
              open: true,
              complete: false,
            },
          ]
        }
      }
      break

    case 'tool_input_start': {
      closeAssistant()
      completeThinking()
      let idx = ev.id ? lastIndex(items, (it) => it.kind === 'tool' && it.id === ev.id) : -1
      if (idx < 0 && ev.toolContentIndex !== undefined) {
        idx = lastIndex(
          items,
          (it) =>
            it.kind === 'tool' &&
            it.status === 'preparing' &&
            it.toolContentIndex === ev.toolContentIndex,
        )
      }
      if (idx < 0) {
        items = [
          ...items,
          {
            kind: 'tool',
            id: ev.id ?? nextId(),
            name: ev.tool ?? 'tool',
            args: undefined,
            status: 'preparing',
            toolContentIndex: ev.toolContentIndex,
            generatedBytes: 0,
          },
        ]
      }
      break
    }

    case 'tool_input_delta': {
      let idx = ev.id ? lastIndex(items, (it) => it.kind === 'tool' && it.id === ev.id) : -1
      if (idx < 0 && ev.toolContentIndex !== undefined) {
        idx = lastIndex(
          items,
          (it) =>
            it.kind === 'tool' &&
            it.status === 'preparing' &&
            it.toolContentIndex === ev.toolContentIndex,
        )
      }
      if (idx >= 0) {
        const cur = items[idx] as Extract<Item, { kind: 'tool' }>
        items = replaceAt(items, idx, {
          ...cur,
          id: ev.id ?? cur.id,
          name: ev.tool || cur.name,
          args: `${typeof cur.args === 'string' ? cur.args : ''}${ev.delta ?? ''}`,
          generatedBytes: (cur.generatedBytes ?? 0) + (ev.bytes ?? 0),
        })
      } else {
        items = [
          ...items,
          {
            kind: 'tool',
            id: ev.id ?? nextId(),
            name: ev.tool ?? 'tool',
            args: ev.delta ?? '',
            status: 'preparing',
            toolContentIndex: ev.toolContentIndex,
            generatedBytes: ev.bytes ?? 0,
          },
        ]
      }
      break
    }

    case 'tool_input_end': {
      let idx = ev.id ? lastIndex(items, (it) => it.kind === 'tool' && it.id === ev.id) : -1
      if (idx < 0 && ev.toolContentIndex !== undefined) {
        idx = lastIndex(
          items,
          (it) =>
            it.kind === 'tool' &&
            it.status === 'preparing' &&
            it.toolContentIndex === ev.toolContentIndex,
        )
      }
      const patch = {
        name: ev.tool ?? 'tool',
        args: ev.args,
        status: 'preparing' as const,
        toolContentIndex: ev.toolContentIndex,
      }
      if (idx >= 0) {
        const cur = items[idx] as Extract<Item, { kind: 'tool' }>
        items = replaceAt(items, idx, {
          ...cur,
          ...patch,
          id: ev.id ?? cur.id,
          name: ev.tool || cur.name,
        })
      } else {
        items = [
          ...items,
          { kind: 'tool', id: ev.id ?? nextId(), generatedBytes: 0, ...patch },
        ]
      }
      break
    }

    case 'tool_start': {
      closeAssistant()
      completeThinking()
      let idx = ev.id ? lastIndex(items, (it) => it.kind === 'tool' && it.id === ev.id) : -1
      if (idx < 0) {
        idx = lastIndex(
          items,
          (it) =>
            it.kind === 'tool' &&
            it.status === 'preparing' &&
            (!ev.tool || it.name === ev.tool),
        )
      }
      if (idx >= 0) {
        const cur = items[idx] as Extract<Item, { kind: 'tool' }>
        items = replaceAt(items, idx, {
          ...cur,
          id: ev.id ?? cur.id,
          name: ev.tool || cur.name,
          args: ev.args ?? cur.args,
          status: 'running',
        })
      } else {
        items = [
          ...items,
          {
            kind: 'tool',
            id: ev.id ?? nextId(),
            name: ev.tool ?? 'tool',
            args: ev.args,
            status: 'running',
          },
        ]
      }
      break
    }

    case 'tool_end': {
      let idx = ev.id ? lastIndex(items, (it) => it.kind === 'tool' && it.id === ev.id) : -1
      if (idx < 0) {
        idx = lastIndex(
          items,
          (it) =>
            it.kind === 'tool' &&
            (it.status === 'running' || it.status === 'preparing') &&
            (!ev.tool || it.name === ev.tool),
        )
      }
      const outcome: ToolOutcome = ev.outcome ?? {
        status: ev.isError ? 'failed' : 'success',
        ...(ev.isError ? { errorCode: 'legacy_tool_error' } : {}),
      }
      const structuredChange = outcomeChange(outcome)
      const structuredPreview = outcomePreview(ev.tool, outcome)
      const patch = {
        status: (outcome.status === 'success' ? 'complete' : 'error') as
          | 'error'
          | 'complete',
        result: ev.result,
        outcome,
        change: structuredChange,
      }
      if (idx >= 0) {
        const cur = items[idx] as Extract<Item, { kind: 'tool' }>
        items = replaceAt(items, idx, { ...cur, ...patch })
      } else {
        items = [
          ...items,
          { kind: 'tool', id: ev.id ?? nextId(), name: ev.tool ?? 'tool', args: undefined, ...patch },
        ]
      }
      if (structuredPreview?.url || structuredPreview?.path) {
        const sameTarget =
          preview?.url === structuredPreview.url &&
          preview?.path === structuredPreview.path &&
          preview?.relativePath === structuredPreview.relativePath
        const pendingCommand = preview?.commandID
          ? browserCommands.some((command) => command.commandID === preview?.commandID)
          : false
        preview = sameTarget && (preview?.commandID || preview?.disposition)
          ? pendingCommand
            ? { ...structuredPreview, ...preview }
            : {
                ...structuredPreview,
                disposition: preview.disposition,
                revision: preview.revision,
              }
          : { ...structuredPreview, revision: (preview?.revision ?? 0) + 1 }
        previewOpen = preview.disposition !== 'new_background_tab'
      } else if (structuredChange?.changeType === 'file' && preview?.path) {
        preview = {
          ...preview,
          revision: preview.revision + 1,
        }
      }
      break
    }

    case 'browser_request': {
      if (
        ev.id &&
        !browserResultOutbox[ev.id] &&
        (ev.preview?.url || ev.preview?.path)
      ) {
        const existing = browserCommands.find((command) => command.commandID === ev.id)
        const revision = existing?.revision ?? (preview?.revision ?? 0) + 1
        const command: BrowserCommandState = {
          ...ev.preview,
          commandID: ev.id,
          disposition: ev.disposition ?? 'reuse_agent_tab',
          revision,
        }
        browserCommands = existing
          ? browserCommands.map((current) =>
              current.commandID === command.commandID ? command : current,
            )
          : [...browserCommands, command]
        preview = command
        previewOpen = command.disposition !== 'new_background_tab'
      }
      break
    }

    case 'browser_tabs_request': {
      if (
        ev.id &&
        !browserTabsRequests.some((command) => command.commandID === ev.id)
      ) {
        browserTabsRequests = [...browserTabsRequests, { commandID: ev.id }]
      }
      break
    }

    case 'browser_inspect_request': {
      if (
        ev.id &&
        !browserInspections.some((command) => command.commandID === ev.id)
      ) {
        browserInspections = [
          ...browserInspections,
          { commandID: ev.id, tabID: ev.tabID || undefined },
        ]
      }
      break
    }

    case 'task_started':
    case 'task_notification': {
      if (!ev.task) break
      const backgroundTask = ev.task
      tasks = { ...tasks, [backgroundTask.id]: backgroundTask }
      if (ev.type === 'task_started' || backgroundTask.status === 'running') break
      const status: TaskItem['status'] =
        backgroundTask.status === 'stopped'
          ? 'stopped'
          : backgroundTask.status === 'succeeded'
            ? 'succeeded'
            : 'failed'
      const task: TaskItem = {
        kind: 'task',
        id: `task-${backgroundTask.id}`,
        taskID: backgroundTask.id,
        status,
        command: backgroundTask.command,
        description: backgroundTask.description,
        outputPath: backgroundTask.outputPath,
        exitCode: backgroundTask.exitCode ?? 0,
        completedAt: backgroundTask.completedAt,
      }
      const index = items.findIndex(
        (item) => item.kind === 'task' && item.taskID === backgroundTask.id,
      )
      items = index >= 0 ? replaceAt(items, index, task) : [...items, task]
      break
    }

    case 'message_end':
      completeThinking()
      responseUsage = mergeUsage(responseUsage, ev.usage)
      if (ev.usage) {
        const usedTokens = usageTokens(ev.usage)
        if (usedTokens > 0) {
          contextUsage = {
            provider: contextUsage?.provider ?? '',
            model: contextUsage?.model ?? '',
            usedTokens,
            contextWindow: contextUsage?.contextWindow ?? 0,
            measured: true,
          }
        }
      }
      {
        let idx = lastIndex(items, (it) => it.kind === 'assistant' && it.open)
        if (idx < 0 && ev.text) {
          const runIndex = lastIndex(items, (item) => item.kind === 'run')
          const matchingAssistant = lastIndex(
            items,
            (item) => item.kind === 'assistant' && item.markdown === ev.text,
          )
          if (
            matchingAssistant > runIndex &&
            (matchingAssistant === items.length - 1 ||
              hasOnlyPreparingToolsAfter(items, matchingAssistant))
          ) {
            idx = matchingAssistant
          }
        }
        if (ev.text) {
          if (idx >= 0) {
            const cur = items[idx] as Extract<Item, { kind: 'assistant' }>
            items = replaceAt(items, idx, { ...cur, markdown: ev.text, open: false })
          } else {
            idx = items.length
            items = [
              ...items,
              {
                kind: 'assistant',
                id: nextId(),
                markdown: ev.text,
                open: false,
                complete: false,
              },
            ]
          }
        } else if (idx >= 0) {
          const cur = items[idx] as Extract<Item, { kind: 'assistant' }>
          items = replaceAt(items, idx, { ...cur, open: false })
        }

        if (ev.finalResponse && idx >= 0) {
          const cur = items[idx] as Extract<Item, { kind: 'assistant' }>
          items = replaceAt(items, idx, {
            ...cur,
            open: false,
            complete: true,
            usage: hasUsage(responseUsage) ? responseUsage : undefined,
            provider: ev.provider,
            model: ev.model,
            modelName: ev.modelName,
            completedAt: ev.completedAt,
          })
          responseUsage = emptyUsage()
        }
      }
      break

    case 'approval_request': {
      completeThinking()
      running = true
      const id = ev.id ?? nextId()
      const idx = lastIndex(items, (it) => it.kind === 'approval' && it.id === id)
      const approval: ApprovalItem = {
        kind: 'approval',
        id,
        summary: ev.summary ?? '',
        reason: ev.reason ?? '',
        command: ev.command ?? '',
        commandSegments: ev.commandSegments ?? 0,
      }
      items = idx >= 0 ? replaceAt(items, idx, approval) : [...items, approval]
      break
    }

    case 'approval_resolved':
    case 'approval_cancelled':
      if (ev.id) items = items.filter((item) => !(item.kind === 'approval' && item.id === ev.id))
      break

    case 'question_request': {
      completeThinking()
      running = true
      const id = ev.id ?? nextId()
      const idx = lastIndex(items, (it) => it.kind === 'question' && it.id === id)
      const question: QuestionItem = { kind: 'question', id, questions: ev.questions ?? [] }
      items = idx >= 0 ? replaceAt(items, idx, question) : [...items, question]
      break
    }

    case 'question_resolved':
    case 'question_cancelled':
      if (ev.id) items = items.filter((item) => !(item.kind === 'question' && item.id === ev.id))
      break

    case 'turn_discard': {
      removePreparingTools()
      const assistantIndex = lastIndex(items, (item) => item.kind === 'assistant')
      const boundaryIndex = lastIndex(
        items,
        (item) => item.kind === 'user' || item.kind === 'run' || item.kind === 'tool',
      )
      if (assistantIndex > boundaryIndex) {
        const assistant = items[assistantIndex]
        if (assistant.kind === 'assistant' && assistant.usage) {
          responseUsage = mergeUsage(responseUsage, assistant.usage)
        }
        let start = assistantIndex
        while (start > 0 && items[start - 1].kind === 'thinking') start--
        items = [...items.slice(0, start), ...items.slice(assistantIndex + 1)]
      } else {
        let end = items.length
        while (end > boundaryIndex + 1 && items[end - 1].kind === 'thinking') end--
        if (end < items.length) items = items.slice(0, end)
      }
      break
    }

    case 'compaction_start':
      autoCompacting = true
      break

    case 'compaction_end':
      autoCompacting = false
      if (!ev.isError && contextUsage) {
        contextUsage = { ...contextUsage, usedTokens: 0, measured: false }
      }
      break

    case 'error':
      removePreparingTools()
      completeRun(ev.durationMs, ev.startedAt)
      items = [...items, { kind: 'error', id: nextId(), text: ev.text ?? '' }]
      running = false
      autoCompacting = false
      closeAssistant()
      completeThinking()
      responseUsage = emptyUsage()
      break

    case 'done':
      removePreparingTools()
      completeRun(ev.durationMs, ev.startedAt)
      running = false
      autoCompacting = false
      closeAssistant()
      completeThinking()
      responseUsage = emptyUsage()
      break
  }

  return {
    ...state,
    items,
    tasks,
    queue,
    responseUsage,
    contextUsage,
    preview,
    browserCommands,
    browserTabsRequests,
    browserInspections,
    previewOpen,
    running,
    autoCompacting,
    seq,
  }
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

function completeOpenRun(items: Item[]): Item[] {
  const index = lastIndex(items, (item) => item.kind === 'run' && item.durationMs === undefined)
  if (index < 0) return items
  const run = items[index] as Extract<Item, { kind: 'run' }>
  return replaceAt(items, index, { ...run, durationMs: elapsedSince(run.startedAt) })
}

function replaceQueueAt(
  queue: QueuedMessage[],
  index: number,
  next: QueuedMessage,
): QueuedMessage[] {
  const copy = queue.slice()
  copy[index] = next
  return copy
}

function sameUserMessage(
  leftText: string,
  leftImages: MessageImage[],
  leftFiles: MessageFile[] | undefined,
  rightText: string,
  rightImages: MessageImage[],
  rightFiles: MessageFile[] | undefined,
): boolean {
  const normalizedLeftFiles = leftFiles ?? []
  const normalizedRightFiles = rightFiles ?? []
  if (
    leftText !== rightText ||
    leftImages.length !== rightImages.length ||
    normalizedLeftFiles.length !== normalizedRightFiles.length
  ) {
    return false
  }
  return (
    leftImages.every(
      (image, index) =>
        image.mimeType === rightImages[index]?.mimeType && image.data === rightImages[index]?.data,
    ) &&
    normalizedLeftFiles.every(
      (file, index) =>
        file.name === normalizedRightFiles[index]?.name &&
        file.size === normalizedRightFiles[index]?.size,
    )
  )
}

function mergeUsage(current: Usage, next?: Usage): Usage {
  if (!next) return current
  return {
    input: current.input + next.input,
    inputUnknown: Boolean(current.inputUnknown || next.inputUnknown),
    output: current.output + next.output,
    cacheRead: current.cacheRead + next.cacheRead,
    cacheWrite: current.cacheWrite + next.cacheWrite,
    totalTokens:
      current.totalTokens +
      (next.totalTokens || next.input + next.output + next.cacheRead + next.cacheWrite),
    cost: {
      input: current.cost.input + next.cost.input,
      output: current.cost.output + next.cost.output,
      cacheRead: current.cost.cacheRead + next.cost.cacheRead,
      cacheWrite: current.cost.cacheWrite + next.cost.cacheWrite,
      total: current.cost.total + next.cost.total,
    },
  }
}

function usageTokens(usage: Usage): number {
  return (
    usage.totalTokens ||
    usage.input + usage.output + usage.cacheRead + usage.cacheWrite
  )
}

function hasUsage(usage: Usage): boolean {
  return (
    Boolean(usage.inputUnknown) ||
    usage.input !== 0 ||
    usage.output !== 0 ||
    usage.cacheRead !== 0 ||
    usage.cacheWrite !== 0 ||
    usage.totalTokens !== 0 ||
    usage.cost.total !== 0
  )
}
