import { useLayoutEffect, useRef } from 'react'
import { LoaderCircle } from 'lucide-react'
import type { ModelOption, WorkspaceSummary } from '@/types'
import type { SessionThread } from '@/useSession'
import { useI18n } from '@/i18n'
import { groupItems } from '@/lib/steps'
import { cn } from '@/lib/utils'
import { Composer } from './Composer'
import { StepGroup } from './StepGroup'
import { AutoCompactionStatus, AwaitingResponse, ThreadItem } from './ConversationThread'

export function ConversationView({
  thread,
  models,
  workspaces,
  onConfigureModel,
}: {
  thread: SessionThread
  models: ModelOption[]
  workspaces: WorkspaceSummary[]
  onConfigureModel: () => void
}) {
  const { t } = useI18n()
  const logRef = useRef<HTMLDivElement>(null)
  const followLatestRef = useRef(true)
  const empty = !thread.loading && thread.items.length === 0 && !thread.approval
  const awaitingFirstOutput = thread.running && thread.items.at(-1)?.kind === 'user'

  useLayoutEffect(() => {
    const log = logRef.current
    if (log && followLatestRef.current) log.scrollTop = log.scrollHeight
  }, [thread.items])

  const composer = (centered = false) => (
    <Composer
      key={thread.session.id}
      connected={thread.status === 'ready'}
      running={thread.running}
      approval={thread.approval}
      queuedMessages={thread.queuedMessages}
      contextUsage={thread.contextUsage}
      centered={centered}
      projectPickerVisible={false}
      workspaces={workspaces}
      models={models}
      modelProvider={thread.session.modelProvider}
      modelID={thread.session.modelId}
      thinkingLevel={thread.session.thinkingLevel}
      permissionMode={thread.session.permissionMode}
      updatingSettings={thread.updatingSettings}
      compacting={thread.compacting}
      onSend={thread.send}
      onRemoveQueued={thread.removeQueuedMessage}
      onStop={thread.stop}
      onResolve={thread.resolveApproval}
      onSelectProject={() => {}}
      onBrowseProjects={() => {}}
      onConfigureModel={onConfigureModel}
      onSettingsChange={thread.updateSettings}
      onPermissionModeChange={thread.updatePermissionMode}
      onCompact={thread.compactContext}
    />
  )

  return (
    <section
      className="flex h-full min-h-0 flex-col bg-white"
      data-testid="workbench-conversation"
      aria-label={thread.session.title}
    >
      <div
        ref={logRef}
        className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto px-3 [scrollbar-gutter:stable_both-edges]"
        data-testid="workbench-conversation-transcript"
        onScroll={(event) => {
          const element = event.currentTarget
          followLatestRef.current =
            element.scrollHeight - element.scrollTop - element.clientHeight < 72
        }}
      >
        <div
          className={cn(
            'mx-auto min-h-full w-full max-w-[680px] pt-4 pb-7',
            (thread.loading || empty) && 'grid place-items-center',
          )}
        >
          {thread.loading ? (
            <div className="flex items-center gap-2 pb-[8vh] text-xs text-stone-400">
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
              {t('app.loadingSession')}
            </div>
          ) : empty ? (
            <div className="flex w-full -translate-y-[2vh] flex-col items-center gap-7">
              <div className="max-w-sm text-center">
                <h2 className="m-0 text-[1.25rem] leading-tight font-medium text-stone-900">
                  {t('app.emptyTitle')}
                </h2>
                <p className="mt-2 text-[0.875rem] leading-5 text-stone-500">
                  {t('app.emptyDescription')}
                </p>
              </div>
              {composer(true)}
            </div>
          ) : (
            <>
              {groupItems(thread.items).map((unit) =>
                unit.kind === 'steps' ? (
                  <StepGroup
                    key={unit.id}
                    items={unit.items}
                    cwd={thread.session.workspacePath}
                  />
                ) : (
                  <ThreadItem
                    key={unit.item.id}
                    item={unit.item}
                    cwd={thread.session.workspacePath}
                  />
                ),
              )}
              {thread.autoCompacting ? (
                <AutoCompactionStatus />
              ) : (
                awaitingFirstOutput && <AwaitingResponse />
              )}
            </>
          )}
        </div>
      </div>
      {!thread.loading && !empty && composer()}
    </section>
  )
}
