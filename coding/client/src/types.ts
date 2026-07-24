import type {
  Change,
  DeliveryMode,
  HistoryResponse,
  MessageImage,
  PreviewRequest,
  Usage,
} from './generated/wire'

// The HTTP/SSE DTOs are generated from coding/internal/httpapi/wire_contract.go.
export type * from './generated/wire'

export type PreviewState = PreviewRequest & {
  revision: number
}

// UI snapshots also cover a local degraded state when history cannot be read.
export type ThreadSnapshot = Pick<HistoryResponse, 'events' | 'running'> &
  Partial<Pick<HistoryResponse, 'queue' | 'context' | 'eventSeq'>>

// Thread items are the declarative model the UI renders, derived from the wire
// event stream by the reducer.
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
  sentAt?: string
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
  completedAt?: string
}
export type RunItem = {
  kind: 'run'
  id: string
  startedAt: string
  durationMs?: number
}
export type ThinkingItem = { kind: 'thinking'; id: string; text: string; streaming: boolean }
export type ToolItem = {
  kind: 'tool'
  id: string
  name: string
  args: unknown
  status: 'preparing' | 'running' | 'complete' | 'error'
  toolContentIndex?: number
  generatedBytes?: number
  result?: string
  change?: Change
}
export type ApprovalChoice = 'allow_once' | 'deny'
export type ApprovalItem = {
  kind: 'approval'
  id: string
  summary: string
  reason: string
}
export type ErrorItem = { kind: 'error'; id: string; text: string }

export type Item =
  | UserItem
  | RunItem
  | AssistantItem
  | ThinkingItem
  | ToolItem
  | ApprovalItem
  | ErrorItem

export type ConnectionStatus = 'connecting' | 'ready' | 'disconnected'

export type ThinkingLevel = 'off' | 'minimal' | 'low' | 'medium' | 'high' | 'xhigh'
export type PermissionMode = 'ask' | 'auto_edit' | 'read_only'

export type ModelOption = {
  provider: string
  id: string
  name: string
  contextWindow: number
  thinkingLevels: ThinkingLevel[]
  supportsImages: boolean
}

export type ProviderInfo = {
  id: string
  name: string
  configured: boolean
  models: number
  officialBaseURL?: string
  effectiveBaseURL?: string
  activeConnectionId: string
  connections: ProviderConnectionInfo[]
}

export type ActiveModelSelection = {
  provider: string
  model: string
  thinkingLevel: ThinkingLevel
}

export type ProviderListResponse = {
  providers: ProviderInfo[]
  activeModel?: ActiveModelSelection
}

export type ProviderConnectionInfo = {
  id: string
  name: string
  baseURL: string
  official: boolean
  activeKeyId?: string
  keys: ProviderKeyInfo[]
}

export type ProviderKeyInfo = {
  id: string
  name: string
  preview: string
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
  aiTitle?: string
  customTitle?: string
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
  permissionMode: PermissionMode
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

export type CompactionResult = {
  summary: string
  firstKeptEntryId: string
  tokensBefore: number
  tokensAfter: number
}
