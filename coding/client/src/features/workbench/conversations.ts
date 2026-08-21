import type { SessionDraft, SessionThread } from '@/features/session'
import type { MessageImage, PromptFile } from '@/types'

export type WorkbenchSessionConversation = {
  id: string
  kind: 'session'
  thread: SessionThread
}

export type WorkbenchDraftConversation = {
  id: string
  kind: 'draft'
  draft: SessionDraft
  connected: boolean
  creating: boolean
  onChange: (draft: SessionDraft) => void
  onSend: (
    text: string,
    images: MessageImage[],
    files: PromptFile[],
    planModeOverride?: boolean,
  ) => Promise<boolean>
}

export type WorkbenchConversation =
  | WorkbenchSessionConversation
  | WorkbenchDraftConversation
