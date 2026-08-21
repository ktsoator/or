import type { SessionDraft } from './store'
import type {
  ApprovalChoice,
  ApprovalItem,
  BackgroundTask,
  BrowserCommandState,
  BrowserInspectionCommandState,
  BrowserResult,
  BrowserTabsCommandState,
  CompactionResult,
  ConnectionStatus,
  ContextUsage,
  DeliveryMode,
  Item,
  MessageImage,
  ModelOption,
  PermissionMode,
  PreviewState,
  PromptFile,
  QueuedMessage,
  QuestionAnswer,
  QuestionItem,
  SessionSummary,
  TaskOutputResponse,
  ThinkingLevel,
  TodoSnapshot,
  WorkspaceSummary,
} from '@/types'

type ThreadView = {
  items: Item[]
  tasks: BackgroundTask[]
  todos: TodoSnapshot | null
  queuedMessages: QueuedMessage[]
  contextUsage?: ContextUsage
  preview?: PreviewState
  browserCommands: BrowserCommandState[]
  browserTabsRequests: BrowserTabsCommandState[]
  browserInspections: BrowserInspectionCommandState[]
  previewOpen: boolean
  approval?: ApprovalItem
  question?: QuestionItem
  running: boolean
  autoCompacting: boolean
  loading: boolean
  updatingSettings: boolean
  compacting: boolean
  status: ConnectionStatus
}

type ThreadActions = {
  send: (
    text: string,
    images: MessageImage[],
    files: PromptFile[],
    delivery?: DeliveryMode,
  ) => Promise<boolean>
  removeQueuedMessage: (id: string) => Promise<void>
  stop: () => void
  stopTask: (id: string) => Promise<void>
  readTaskOutput: (id: string) => Promise<TaskOutputResponse>
  resolveApproval: (id: string, choice: ApprovalChoice) => Promise<void>
  resolveQuestion: (id: string, answers: QuestionAnswer[]) => Promise<void>
  updateSettings: (
    provider: string,
    model: string,
    thinkingLevel: ThinkingLevel,
  ) => Promise<void>
  updatePermissionMode: (mode: PermissionMode) => Promise<void>
  compactContext: () => Promise<CompactionResult>
}

export type SessionDraftSubmission = {
  text: string
  images: MessageImage[]
  files: PromptFile[]
}

export type SessionThread = ThreadView &
  ThreadActions & {
    session: SessionSummary
  }

export type Session = ThreadView &
  ThreadActions & {
    sessions: SessionSummary[]
    workspaces: WorkspaceSummary[]
    draft?: SessionDraft
    activeSession?: SessionSummary
    activeSessionID?: string
    creating: boolean
    forking: boolean
    models: ModelOption[]
    refreshModels: () => Promise<void>
    registerWorkspace: (path: string) => Promise<WorkspaceSummary>
    removeWorkspace: (path: string) => Promise<void>
    startDraft: (workspacePath?: string, projectScoped?: boolean) => void
    createChatDraft: () => SessionDraft
    createChatSession: (
      draft: SessionDraft,
      submission: SessionDraftSubmission,
    ) => Promise<SessionSummary>
    draftReady: boolean
    updateDraftWorkspace: (workspacePath?: string, projectScoped?: boolean) => void
    deleteSession: (id: string) => Promise<void>
    renameSession: (id: string, customTitle: string) => Promise<SessionSummary>
    forkMessage: (
      messageID: string,
      mode: 'before_user' | 'after_assistant',
      text?: string,
    ) => Promise<SessionSummary>
    editMessage: (messageID: string, text: string) => Promise<SessionSummary>
    selectSession: (id: string) => void
    queueBrowserResult: (sessionID: string, id: string, result: BrowserResult) => void
    handleBrowserTabs: (sessionID: string, id: string) => void
    handleBrowserInspection: (sessionID: string, id: string) => void
    secondaryThreads: SessionThread[]
  }
