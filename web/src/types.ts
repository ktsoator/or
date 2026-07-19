// Wire types mirror the JSON the Go API emits over SSE (/api/events) and the
// history snapshot (/api/history). Keep these in sync with coding/internal/app/web.

export type Hunk = {
  oldStart: number
  oldLines: number
  newStart: number
  newLines: number
  lines: string[]
}

export type FileChangePayload = {
  changeType: 'file'
  path: string
  op: 'create' | 'update'
  additions: number
  deletions: number
  bytes: number
  hunks: Hunk[]
}

export type FailureChangePayload = {
  changeType: 'failure'
  path: string
  reason: string
  detail: string
}

export type Change = FileChangePayload | FailureChangePayload

export type WireEvent = {
  type:
    | 'user_message'
    | 'delta'
    | 'tool_start'
    | 'tool_end'
    | 'message_end'
    | 'confirm_request'
    | 'queue_cancelled'
    | 'queue_removed'
    | 'error'
    | 'done'
  kind?: 'text' | 'thinking'
  delta?: string
  tool?: string
  args?: unknown
  result?: string
  change?: Change
  isError?: boolean
  text?: string
  images?: MessageImage[]
  delivery?: DeliveryMode
  queued?: boolean
  finalResponse?: boolean
  usage?: Usage
  provider?: string
  model?: string
  modelName?: string
  id?: string
  summary?: string
}

// Thread items are the declarative model the UI renders, derived from the wire
// event stream by the reducer.
export type MessageImage = {
  data: string
  mimeType: string
}

export type DeliveryMode = 'steer' | 'followup'

export type PendingImage = MessageImage & {
  id: string
  name: string
  size: number
}

export type QueuedMessage = {
  id: string
  text: string
  images: MessageImage[]
  delivery: DeliveryMode
  status: 'queued' | 'removing' | 'failed'
}

export type UsageCost = {
  input: number
  output: number
  cacheRead: number
  cacheWrite: number
  total: number
}

export type Usage = {
  input: number
  output: number
  cacheRead: number
  cacheWrite: number
  totalTokens: number
  cost: UsageCost
}

export type UsageTotals = Usage & {
  requests: number
}

export type ModelUsageSummary = UsageTotals & {
  provider: string
  model: string
  name: string
  responseModel?: string
  lastUsedAt: string
}

export type UsageReport = {
  total: UsageTotals
  models: ModelUsageSummary[]
  generatedAt: string
}

export type UsageEvent = {
  id: string
  sessionId: string
  provider: string
  model: string
  responseModel?: string
  responseId?: string
  timestamp: string
  usage: Usage
}

export type UsageEventPage = {
  events: UsageEvent[]
  total: number
  limit: number
  offset: number
}

export type UserItem = {
  kind: 'user'
  id: string
  text: string
  images: MessageImage[]
  deliveryStatus?: 'sending' | 'failed'
}
export type AssistantItem = {
  kind: 'assistant'
  id: string
  markdown: string
  open: boolean
  complete: boolean
  usage?: Usage
  provider?: string
  model?: string
  modelName?: string
}
export type ThinkingItem = { kind: 'thinking'; id: string; text: string; streaming: boolean }
export type ToolItem = {
  kind: 'tool'
  id: string
  name: string
  args: unknown
  status: 'running' | 'complete' | 'error'
  result?: string
  change?: Change
}
export type ConfirmItem = {
  kind: 'confirm'
  id: string
  summary: string
}
export type ErrorItem = { kind: 'error'; id: string; text: string }

export type Item =
  | UserItem
  | AssistantItem
  | ThinkingItem
  | ToolItem
  | ConfirmItem
  | ErrorItem

export type ConnectionStatus = 'connecting' | 'ready' | 'disconnected'

export type ThinkingLevel = 'off' | 'minimal' | 'low' | 'medium' | 'high' | 'xhigh'

export type ModelOption = {
  provider: string
  id: string
  name: string
  contextWindow: number
  thinkingLevels: ThinkingLevel[]
  supportsImages: boolean
}

export type ModelCatalogResponse = {
  models: ModelOption[]
  defaultProvider: string
  defaultModel: string
  defaultThinkingLevel: ThinkingLevel
}

export type SessionSummary = {
  id: string
  title: string
  workspacePath: string
  workspaceName: string
  scope: 'chat' | 'project'
  workspaceKind: 'scratch' | 'folder'
  createdAt: string
  updatedAt: string
  running: boolean
  hasApproval: boolean
  modelProvider: string
  modelId: string
  modelName: string
  thinkingLevel: ThinkingLevel
}

export type WorkspaceSummary = {
  path: string
  name: string
  addedAt: string
}

export type DirectoryEntry = {
  name: string
  path: string
}

export type DirectoryListing = {
  path: string
  parent: string
  directories: DirectoryEntry[]
}

export type ContextUsage = {
  provider: string
  model: string
  usedTokens: number
  contextWindow: number
  measured: boolean
}

export type HistoryResponse = {
  events: WireEvent[]
  queue?: WireEvent[]
  context?: ContextUsage
  running: boolean
}
