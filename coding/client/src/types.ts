import type {
  BrowserResult,
  BrowserDisposition,
  Change,
  DeliveryMode,
  HistoryResponse,
  MessageFile,
  MessageImage,
  PreviewRequest,
  Question,
  TaskStatus,
  TitleGenerationStatus,
  ToolOutcome,
  Usage,
} from './generated/wire'

// The HTTP/SSE DTOs are generated from coding/internal/httpapi/wire_contract.go.
export type * from './generated/wire'

export type PreviewState = PreviewRequest & {
  revision: number
  commandID?: string
  disposition?: BrowserDisposition
}

export type BrowserCommandState = PreviewRequest & {
  revision: number
  commandID: string
  disposition: BrowserDisposition
}

export type BrowserInspectionCommandState = {
  commandID: string
  tabID?: string
}

export type BrowserTabsCommandState = {
  commandID: string
}

export type BrowserResultOutboxEntry = {
  commandID: string
  result: BrowserResult
}

// UI snapshots also cover a local degraded state when history cannot be read.
export type ThreadSnapshot = Pick<HistoryResponse, 'events' | 'running'> &
  Partial<
    Pick<
      HistoryResponse,
      | 'queue'
      | 'context'
      | 'tasks'
      | 'eventSeq'
      | 'title'
      | 'aiTitle'
      | 'customTitle'
      | 'titleGenerationStatus'
      | 'titleGenerationProvider'
      | 'titleGenerationModel'
      | 'titleGenerationErrorCode'
      | 'titleGenerationError'
      | 'titleGenerationAttemptedAt'
    >
  >

// Thread items are the declarative model the UI renders, derived from the wire
// event stream by the reducer.
export type PendingImage = MessageImage & {
  id: string
  name: string
  size: number
}

export type PromptFile = {
  name: string
  mimeType: string
  size: number
  file: File
}

export type PendingFile = MessageFile &
  PromptFile & {
    id: string
  }

export type QueuedMessage = {
  id: string
  text: string
  images: MessageImage[]
  files?: MessageFile[]
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
  files?: MessageFile[]
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
  outcome?: ToolOutcome
  change?: Change
}
export type ApprovalChoice = 'allow_once' | 'deny'
export type ApprovalItem = {
  kind: 'approval'
  id: string
  summary: string
  reason: string
  // The complete shell command, when the approval covers one. It is shown in
  // full rather than summarized: a decision must not rest on a truncated view.
  command: string
  // Conservative count of the commands the shell would run. Above one, the user
  // is told the command is compound.
  commandSegments: number
}
export type QuestionItem = {
  kind: 'question'
  id: string
  questions: Question[]
}
export type ErrorItem = { kind: 'error'; id: string; text: string }
export type TaskItem = {
  kind: 'task'
  id: string
  taskID: string
  status: Exclude<TaskStatus, 'running'>
  command: string
  description?: string
  outputPath: string
  exitCode: number
  completedAt?: string
}

export type Item =
  | UserItem
  | RunItem
  | AssistantItem
  | ThinkingItem
  | ToolItem
  | ApprovalItem
  | QuestionItem
  | TaskItem
  | ErrorItem

export type ConnectionStatus = 'connecting' | 'ready' | 'disconnected'

export type ThinkingLevel = 'off' | 'minimal' | 'low' | 'medium' | 'high' | 'xhigh' | 'max'
export type ModelThinkingVisibility = 'visible' | 'hidden'
export type PermissionMode = 'ask' | 'auto_edit' | 'read_only' | 'full_access'

export type ModelOption = {
  provider: string
  id: string
  name: string
  contextWindow: number
  thinkingLevels: ThinkingLevel[]
  thinkingVisibility?: ModelThinkingVisibility
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
  utilityModels: ProviderUtilityModelInfo[]
}

export type ProviderUtilityModelInfo = {
  id: string
  name: string
}

export type ActiveModelSelection = {
  provider: string
  model: string
  thinkingLevel: ThinkingLevel
}

export type ProviderListResponse = {
  providers: ProviderInfo[]
  activeModel?: ActiveModelSelection
  utilityModel?: UtilityModelSelection
  repairs?: ProviderSelectionRepair[]
}

export type ProviderSelectionRepair = {
  target: 'active_model' | 'utility_model'
  reason: 'unavailable' | 'unsupported_thinking_level'
  previous: ProviderModelReference
  replacement?: ProviderModelReference
}

export type ProviderModelReference = {
  provider: string
  model: string
  thinkingLevel?: ThinkingLevel
}

export type UtilityModelSelection = {
  provider: string
  model: string
  connectionId: string
  keyId: string
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
  titleGeneration: TitleGeneration
  workspacePath: string
  workspaceName: string
  scope: 'chat' | 'project'
  workspaceKind: 'scratch' | 'folder'
  createdAt: string
  updatedAt: string
  running: boolean
  hasApproval: boolean
  hasQuestion: boolean
  modelProvider: string
  modelId: string
  modelName: string
  thinkingLevel: ThinkingLevel
  permissionMode: PermissionMode
}

export type TitleGeneration = {
  status: TitleGenerationStatus
  provider?: string
  model?: string
  errorCode?: string
  error?: string
  attemptedAt?: string
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
