import type {
  ApprovalItem,
  BrowserCommandState,
  Change,
  Item,
  MessageFile,
  MessageImage,
  PreviewRequest,
  QueuedMessage,
  QuestionItem,
  TaskItem,
  TodoSnapshot,
  ToolOutcome,
  Usage,
  WireEvent,
} from '@/types'
import type { ThreadState } from './threadState'
import { emptyUsage } from './threadState'

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

function outcomeTodos(
  tool: string | undefined,
  outcome: ToolOutcome | undefined,
): TodoSnapshot | undefined {
  if (tool !== 'todo_write' || outcome?.status !== 'success') return undefined
  const data = recordOf(outcome.data)
  if (!data || !Array.isArray(data.todos)) return undefined

  const todos: TodoSnapshot['todos'] = []
  for (const value of data.todos) {
    const item = recordOf(value)
    if (
      !item ||
      typeof item.content !== 'string' ||
      (item.status !== 'pending' &&
        item.status !== 'in_progress' &&
        item.status !== 'completed')
    ) {
      return undefined
    }
    todos.push({ content: item.content, status: item.status })
  }
  return { todos }
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

function hasOnlyPreparingToolsAfter(items: Item[], index: number): boolean {
  return (
    index < items.length - 1 &&
    items
      .slice(index + 1)
      .every((item) => item.kind === 'tool' && item.status === 'preparing')
  )
}

export function reduceWire(state: ThreadState, ev: WireEvent): ThreadState {
  let items = state.items
  let tasks = state.tasks
  let todos = state.todos
  let planMode = state.planMode
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
  const requestCorrelation = ev.providerRequestId
    ? { providerRequestId: ev.providerRequestId }
    : {}
  const matchesEventRequest = (item: { providerRequestId?: string }) =>
    !ev.providerRequestId || item.providerRequestId === ev.providerRequestId

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
    case 'plan_mode_changed':
      planMode = Boolean(ev.planMode)
      break

    case 'turn_start':
      todos = null
      break

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
          runId: ev.runId ?? ev.id ?? run.runId,
          startedAt: ev.startedAt ?? run.startedAt,
          durationMs: ev.durationMs ?? run.durationMs,
        })
      } else {
        const run = {
          kind: 'run' as const,
          id: ev.id ?? nextId(),
          runId: ev.runId ?? ev.id,
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
        const messageID = ev.messageID ?? existingUser?.messageID
        const user = {
          kind: 'user' as const,
          id: ev.id ?? (idx >= 0 ? items[idx].id : nextId()),
          ...(messageID ? { messageID } : {}),
          text,
          images,
          ...(files.length ? { files } : {}),
          sentAt:
            ev.sentAt ??
            existingUser?.sentAt ??
            (openRun?.kind === 'run' ? openRun.startedAt : undefined),
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
        const idx = lastIndex(
          items,
          (it) => it.kind === 'thinking' && it.streaming && matchesEventRequest(it),
        )
        if (idx >= 0) {
          const cur = items[idx] as Extract<Item, { kind: 'thinking' }>
          items = replaceAt(items, idx, {
            ...cur,
            ...requestCorrelation,
            text: cur.text + (ev.delta ?? ''),
          })
        } else {
          items = [
            ...items,
            {
              kind: 'thinking',
              id: nextId(),
              ...requestCorrelation,
              text: ev.delta ?? '',
              streaming: true,
            },
          ]
        }
      } else {
        completeThinking()
        const idx = lastIndex(
          items,
          (it) => it.kind === 'assistant' && it.open && matchesEventRequest(it),
        )
        if (idx >= 0) {
          const cur = items[idx] as Extract<Item, { kind: 'assistant' }>
          items = replaceAt(items, idx, {
            ...cur,
            ...requestCorrelation,
            markdown: cur.markdown + (ev.delta ?? ''),
          })
        } else {
          items = [
            ...items,
            {
              kind: 'assistant',
              id: nextId(),
              ...requestCorrelation,
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
            matchesEventRequest(it) &&
            it.toolContentIndex === ev.toolContentIndex,
        )
      }
      if (idx >= 0) {
        const cur = items[idx] as Extract<Item, { kind: 'tool' }>
        items = replaceAt(items, idx, { ...cur, ...requestCorrelation })
      } else {
        items = [
          ...items,
          {
            kind: 'tool',
            id: ev.id ?? nextId(),
            name: ev.tool ?? 'tool',
            args: undefined,
            status: 'preparing',
            ...requestCorrelation,
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
            matchesEventRequest(it) &&
            it.toolContentIndex === ev.toolContentIndex,
        )
      }
      if (idx >= 0) {
        const cur = items[idx] as Extract<Item, { kind: 'tool' }>
        items = replaceAt(items, idx, {
          ...cur,
          ...requestCorrelation,
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
            ...requestCorrelation,
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
            matchesEventRequest(it) &&
            it.toolContentIndex === ev.toolContentIndex,
        )
      }
      const patch = {
        name: ev.tool ?? 'tool',
        args: ev.args,
        status: 'preparing' as const,
        ...requestCorrelation,
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
            matchesEventRequest(it) &&
            (!ev.tool || it.name === ev.tool),
        )
      }
      if (idx >= 0) {
        const cur = items[idx] as Extract<Item, { kind: 'tool' }>
        items = replaceAt(items, idx, {
          ...cur,
          ...requestCorrelation,
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
            ...requestCorrelation,
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
            matchesEventRequest(it) &&
            (!ev.tool || it.name === ev.tool),
        )
      }
      const outcome: ToolOutcome = ev.outcome ?? { status: 'success' }
      const structuredChange = outcomeChange(outcome)
      const structuredPreview = outcomePreview(ev.tool, outcome)
      const todoSnapshot = outcomeTodos(ev.tool, outcome)
      const patch = {
        status: (outcome.status === 'success' ? 'complete' : 'error') as
          | 'error'
          | 'complete',
        result: ev.result,
        ...requestCorrelation,
        ...(ev.images ? { images: ev.images } : {}),
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
      if (todoSnapshot) todos = todoSnapshot
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
      if (ev.context) contextUsage = ev.context
      {
        let idx = lastIndex(
          items,
          (it) => it.kind === 'assistant' && it.open && matchesEventRequest(it),
        )
        if (idx < 0 && ev.text) {
          const runIndex = lastIndex(items, (item) => item.kind === 'run')
          const matchingAssistant = lastIndex(
            items,
            (item) =>
              item.kind === 'assistant' &&
              item.markdown === ev.text &&
              matchesEventRequest(item),
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
            items = replaceAt(items, idx, {
              ...cur,
              ...requestCorrelation,
              ...(ev.messageID ? { messageID: ev.messageID } : {}),
              markdown: ev.text,
              open: false,
            })
          } else {
            idx = items.length
            items = [
              ...items,
              {
                kind: 'assistant',
                id: nextId(),
                ...requestCorrelation,
                ...(ev.messageID ? { messageID: ev.messageID } : {}),
                markdown: ev.text,
                open: false,
                complete: false,
              },
            ]
          }
        } else if (idx >= 0) {
          const cur = items[idx] as Extract<Item, { kind: 'assistant' }>
          items = replaceAt(items, idx, {
            ...cur,
            ...requestCorrelation,
            ...(ev.messageID ? { messageID: ev.messageID } : {}),
            open: false,
          })
        }

        if (ev.finalResponse && idx >= 0) {
          const cur = items[idx] as Extract<Item, { kind: 'assistant' }>
          const runIndex = lastIndex(items, (item) => item.kind === 'run')
          const run = runIndex >= 0 ? items[runIndex] : undefined
          items = replaceAt(items, idx, {
            ...cur,
            runId: cur.runId ?? (run?.kind === 'run' ? run.runId : undefined),
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
        contextUsage = {
          ...contextUsage,
          usedTokens: 0,
          measured: false,
          breakdown: undefined,
        }
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
      {
        const runIndex = ev.startedAt
          ? lastIndex(items, (item) => item.kind === 'run' && item.startedAt === ev.startedAt)
          : lastIndex(items, (item) => item.kind === 'run')
        if (runIndex >= 0 && ev.userMessageIDs?.length) {
          const userIndexes: number[] = []
          for (let index = runIndex - 1; index >= 0 && items[index].kind === 'user'; index--) {
            userIndexes.unshift(index)
          }
          const offset = Math.max(0, userIndexes.length - ev.userMessageIDs.length)
          for (let index = 0; index < ev.userMessageIDs.length; index++) {
            const itemIndex = userIndexes[offset + index]
            const user = itemIndex === undefined ? undefined : items[itemIndex]
            if (user?.kind === 'user') {
              items = replaceAt(items, itemIndex, {
                ...user,
                messageID: ev.userMessageIDs[index],
              })
            }
          }
        }
        if (runIndex >= 0 && ev.assistantMessageID) {
          const assistantIndex = lastIndex(
            items,
            (item) => item.kind === 'assistant' && item.complete,
          )
          const assistant = assistantIndex > runIndex ? items[assistantIndex] : undefined
          if (assistant?.kind === 'assistant') {
            const run = items[runIndex]
            items = replaceAt(items, assistantIndex, {
              ...assistant,
              messageID: ev.assistantMessageID,
              runId: ev.runId ?? (run?.kind === 'run' ? run.runId : undefined),
            })
          }
        }
      }
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
    todos,
    planMode,
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

function elapsedSince(startedAt: string): number {
  const start = new Date(startedAt).getTime()
  return Number.isFinite(start) ? Math.max(0, Date.now() - start) : 0
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
