import type {
  BackgroundTask,
  BrowserCommandState,
  BrowserInspectionCommandState,
  BrowserResultOutboxEntry,
  BrowserTabsCommandState,
  ConnectionStatus,
  ContextUsage,
  Item,
  PreviewState,
  QueuedMessage,
  Usage,
} from '@/types'

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

export const emptyUsage = (): Usage => ({
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
