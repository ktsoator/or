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
  id?: string
  summary?: string
}

// Thread items are the declarative model the UI renders, derived from the wire
// event stream by the reducer.
export type MessageImage = {
  data: string
  mimeType: string
}

export type PendingImage = MessageImage & {
  id: string
  name: string
  size: number
}

export type UserItem = { kind: 'user'; id: string; text: string; images: MessageImage[] }
export type AssistantItem = { kind: 'assistant'; id: string; markdown: string; open: boolean }
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
  thinkingLevels: ThinkingLevel[]
  supportsImages: boolean
}

export type ModelCatalogResponse = {
  models: ModelOption[]
}

export type SessionSummary = {
  id: string
  title: string
  createdAt: string
  updatedAt: string
  running: boolean
  hasApproval: boolean
  modelProvider: string
  modelId: string
  modelName: string
  thinkingLevel: ThinkingLevel
}

export type HistoryResponse = {
  events: WireEvent[]
  running: boolean
}
