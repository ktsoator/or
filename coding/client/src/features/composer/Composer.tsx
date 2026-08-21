import { useEffect, useId, useRef, useState } from 'react'
import {
  ArrowUp,
  BookOpen,
  Info,
  Lightbulb,
  Square,
  X,
} from 'lucide-react'
import type {
  ApprovalChoice,
  ApprovalItem,
  QuestionAnswer,
  QuestionItem,
  ContextUsage,
  DeliveryMode,
  MessageImage,
  ModelOption,
  PermissionMode,
  PromptFile,
  QueuedMessage,
  ThinkingLevel,
  TodoSnapshot,
  WorkspaceSummary,
} from '@/types'
import { cn } from '@/lib/utils'
import {
  buildSkillInvocation,
  filterSkills,
  type SkillEntry,
} from '@/features/skills'
import { useComposerAttachments } from './useAttachments'
import { useComposerCatalogs } from './useCatalogs'
import { useComposerCompaction } from './useCompaction'
import { Approval } from './Approval'
import { ComposerAddMenu } from './ComposerAddMenu'
import { ComposerAttachments } from './ComposerAttachments'
import { ComposerSkillSuggestions } from './ComposerSkillSuggestions'
import {
  composerPreviewCommands,
  type ComposerPreviewCommand,
  moveSuggestionIndex,
  parseComposerCatalogQuery,
  parseExecutableComposerCommand,
  parsePlanComposerCommand,
  previewSkillCommandCount,
  skillSuggestionOptionID,
} from './panelStyles'
import { Question } from './Question'
import { PlanReview } from './PlanReview'
import { ModelSettingsMenu } from './ModelSettingsMenu'
import { ContextUsageMenu } from './ContextUsageMenu'
import { PermissionModeMenu } from './PermissionModeMenu'
import { ProjectPicker } from './ProjectPicker'
import { PendingQueue } from './PendingQueue'
import { RunDeliveryMenu } from './RunDeliveryMenu'
import { ComposerControlTooltip } from './ComposerControlTooltip'
import { TodoChecklist } from './TodoChecklist'
import { useI18n } from '@/i18n'
import { composerMenuTriggerClass } from '@/shared/ui/composerControlStyles'

export function Composer({
  connected,
  running,
  approval,
  question,
  queuedMessages,
  todos,
  planMode = false,
  contextUsage,
  centered = false,
  projectPickerVisible = false,
  workspaces,
  workspacePath,
  workspaceError,
  models,
  modelProvider,
  modelID,
  thinkingLevel,
  permissionMode,
  updatingSettings,
  compacting,
  onSend,
  onRemoveQueued,
  onStop,
  onResolve,
  onResolveQuestion,
  onSelectProject,
  onBrowseProjects,
  onConfigureModel,
  onSettingsChange,
  onPermissionModeChange,
  onPlanModeChange,
  onCompact,
}: {
  connected: boolean
  running: boolean
  approval?: ApprovalItem
  question?: QuestionItem
  queuedMessages: QueuedMessage[]
  todos?: TodoSnapshot | null
  planMode?: boolean
  contextUsage?: ContextUsage
  centered?: boolean
  projectPickerVisible?: boolean
  workspaces: WorkspaceSummary[]
  workspacePath?: string
  workspaceError?: string
  models: ModelOption[]
  modelProvider?: string
  modelID?: string
  thinkingLevel?: ThinkingLevel
  permissionMode: PermissionMode
  updatingSettings: boolean
  compacting: boolean
  onSend: (
    text: string,
    images: MessageImage[],
    files: PromptFile[],
    delivery?: DeliveryMode,
    planModeOverride?: boolean,
  ) => Promise<boolean>
  onRemoveQueued: (id: string) => Promise<void>
  onStop: () => void
  onResolve: (id: string, choice: ApprovalChoice) => Promise<void>
  onResolveQuestion: (id: string, answers: QuestionAnswer[]) => Promise<void>
  onSelectProject: (path?: string) => void
  onBrowseProjects: () => void
  onConfigureModel: () => void
  onSettingsChange: (
    provider: string,
    model: string,
    thinkingLevel: ThinkingLevel,
  ) => Promise<void>
  onPermissionModeChange: (mode: PermissionMode) => Promise<void>
  onPlanModeChange?: (active: boolean) => Promise<void>
  onCompact?: () => Promise<unknown>
}) {
  const { t } = useI18n()
  const ref = useRef<HTMLTextAreaElement>(null)
  const surfaceRef = useRef<HTMLDivElement>(null)
  const skillSuggestionsID = `composer-skill-suggestions-${useId()}`
  const composingRef = useRef(false)
  const submittingRef = useRef(false)
  const [settingsError, setSettingsError] = useState('')
  const [queueError, setQueueError] = useState('')
  const [sendError, setSendError] = useState('')
  const [planModeError, setPlanModeError] = useState('')
  const [planModeChanging, setPlanModeChanging] = useState(false)
  const [delivery, setDelivery] = useState<DeliveryMode>('steer')
  const [draftValue, setDraftValue] = useState('')
  const [selectedSkill, setSelectedSkill] = useState<SkillEntry>()
  const [skillSuggestionsDismissed, setSkillSuggestionsDismissed] = useState(false)
  const [activeSuggestionIndex, setActiveSuggestionIndex] = useState(0)
  const [skillKeyboardNavigating, setSkillKeyboardNavigating] = useState(false)
  const [addPanelOpen, setAddPanelOpen] = useState(false)
  const awaitingApproval = Boolean(approval)
  const awaitingQuestion = Boolean(question)
  // A question blocks the composer exactly as an approval does: the run is
  // parked inside a tool call until the user answers it.
  const awaitingUser = awaitingApproval || awaitingQuestion
  const modelConfigured = Boolean(modelProvider && modelID && thinkingLevel)
  const currentModel = models.find(
    (model) => model.provider === modelProvider && model.id === modelID,
  )
  const currentContextUsage =
    contextUsage && contextUsage.provider === modelProvider && contextUsage.model === modelID
      ? contextUsage
      : undefined
  const contextWindow = currentModel?.contextWindow ?? currentContextUsage?.contextWindow ?? 0
  const editorDisabled = awaitingUser || !connected || compacting || !modelConfigured
  const settingsLocked = running || editorDisabled || planModeChanging
  const settingsDisabled = settingsLocked || updatingSettings
  const sendDisabled = editorDisabled || updatingSettings || planModeChanging
  const supportsImages = Boolean(currentModel?.supportsImages)
  const {
    imageFileRef,
    textFileRef,
    images,
    files,
    error: attachmentError,
    imageLimitReached,
    fileLimitReached,
    addImages,
    addTextFiles,
    removeImage,
    removeFile,
    clear: clearAttachments,
    reportUnsupportedImages,
  } = useComposerAttachments(supportsImages)
  const catalogQueryEnabled =
    !running &&
    !selectedSkill &&
    !addPanelOpen &&
    !skillSuggestionsDismissed
  const slashQuery = catalogQueryEnabled
    ? parseComposerCatalogQuery(draftValue)
    : undefined
  const {
    skills: availableSkills,
    projectSkills,
    systemSkills,
    skillsLoaded,
    skillsLoading,
    skillsFailed,
  } = useComposerCatalogs({
    workspacePath,
    catalogOpen: Boolean(slashQuery),
  })
  const {
    feedback: compactFeedback,
    dismiss: dismissCompactFeedback,
    compact: compactContext,
  } = useComposerCompaction(onCompact)
  const suggestedProjectSkills = slashQuery
    ? filterSkills(projectSkills, slashQuery.query).slice(0, maxSkillSuggestions)
    : []
  const suggestedSystemSkills = slashQuery
    ? filterSkills(systemSkills, slashQuery.query).slice(
        0,
        maxSkillSuggestions - suggestedProjectSkills.length,
      )
    : []
  const suggestedSkills = [...suggestedProjectSkills, ...suggestedSystemSkills]
  const previewCommandCount = slashQuery
    ? previewSkillCommandCount(slashQuery.query)
    : 0
  const suggestionCount = previewCommandCount + suggestedSkills.length
  const skillSuggestionsVisible = Boolean(slashQuery && !editorDisabled)

  const autosize = () => {
    const el = ref.current
    if (!el) return
    el.style.height = '0px'
    const contentHeight = el.scrollHeight
    el.style.height = Math.min(contentHeight, 240) + 'px'
  }

  useEffect(() => {
    if (!editorDisabled) ref.current?.focus()
  }, [editorDisabled])

  useEffect(() => {
    if (!running) setDelivery('steer')
  }, [running])

  useEffect(() => setSettingsError(''), [modelProvider, modelID, thinkingLevel, permissionMode])

  useEffect(() => {
    setActiveSuggestionIndex(0)
    setSkillKeyboardNavigating(false)
  }, [slashQuery?.query])

  useEffect(() => {
    if (!addPanelOpen && !skillSuggestionsVisible) return
    const closeOutside = (event: PointerEvent) => {
      const target = event.target
      if (target instanceof Node && !surfaceRef.current?.contains(target)) {
        setAddPanelOpen(false)
        setSkillSuggestionsDismissed(true)
      }
    }
    document.addEventListener('pointerdown', closeOutside, true)
    return () => document.removeEventListener('pointerdown', closeOutside, true)
  }, [addPanelOpen, skillSuggestionsVisible])

  useEffect(() => {
    if (!selectedSkill || !skillsLoaded) return
    const stillAvailable = availableSkills.some(
      (skill) =>
        skill.name === selectedSkill.name && skill.source === selectedSkill.source,
    )
    if (!stillAvailable) setSelectedSkill(undefined)
  }, [availableSkills, selectedSkill, skillsLoaded])

  const clearSubmission = () => {
    setDraftValue('')
    setSelectedSkill(undefined)
    setSkillSuggestionsDismissed(false)
    clearAttachments()
    requestAnimationFrame(autosize)
  }

  const sendSubmission = async (text: string, planModeOverride?: boolean) => {
    if (!text && images.length === 0 && files.length === 0) return
    if (images.length > 0 && !supportsImages) {
      reportUnsupportedImages()
      return
    }
    submittingRef.current = true
    setSendError('')
    try {
      const accepted = await onSend(
        text,
        images.map(({ data, mimeType }) => ({ data, mimeType })),
        files.map(({ name, mimeType, size, file }) => ({
          name,
          mimeType,
          size,
          file,
        })),
        running ? delivery : undefined,
        planModeOverride,
      )
      if (!accepted) return
      clearSubmission()
    } catch (error) {
      setSendError(error instanceof Error ? error.message : t('composer.couldNotSend'))
    } finally {
      submittingRef.current = false
    }
  }

  const changePlanMode = async (active: boolean): Promise<boolean> => {
    if (!onPlanModeChange || planModeChanging) return false
    setPlanModeError('')
    setPlanModeChanging(true)
    try {
      await onPlanModeChange(active)
      return true
    } catch (error) {
      setPlanModeError(
        error instanceof Error ? error.message : t('composer.couldNotUpdatePlanMode'),
      )
      return false
    } finally {
      setPlanModeChanging(false)
    }
  }

  const runPreviewCommand = async (command: ComposerPreviewCommand) => {
    if (sendDisabled) return
    if (command === 'compact') {
      setDraftValue('')
      setAddPanelOpen(false)
      setSkillSuggestionsDismissed(true)
      requestAnimationFrame(autosize)
      await compactContext()
      return
    }
    if (command === 'plan' && onPlanModeChange) {
      if (await changePlanMode(true)) {
        setDraftValue('')
        setAddPanelOpen(false)
        setSkillSuggestionsDismissed(true)
        requestAnimationFrame(autosize)
      }
    }
  }

  const submit = async () => {
    const el = ref.current
    if (!el || submittingRef.current || sendDisabled) return

    const planCommand = onPlanModeChange
      ? parsePlanComposerCommand(draftValue)
      : undefined
    if (planCommand) {
      if (!(await changePlanMode(planCommand.active))) return
      if (!planCommand.message) {
        setDraftValue('')
        setSkillSuggestionsDismissed(false)
        requestAnimationFrame(autosize)
        return
      }
      await sendSubmission(planCommand.message, planCommand.active)
      return
    }

    const command = parseExecutableComposerCommand(draftValue)
    if (command) {
      await runPreviewCommand(command)
      return
    }
    const argumentsText = draftValue.trim()
    const text = selectedSkill
      ? buildSkillInvocation(selectedSkill, argumentsText)
      : argumentsText
    await sendSubmission(text)
  }

  const selectSkill = (skill: SkillEntry) => {
    const el = ref.current
    if (!el) return
    const argumentsText = selectedSkill
      ? draftValue
      : slashQuery?.argumentsText ?? draftValue
    setSelectedSkill(skill)
    setDraftValue(argumentsText)
    setAddPanelOpen(false)
    setSkillSuggestionsDismissed(true)
    requestAnimationFrame(() => {
      autosize()
      el.focus()
      el.setSelectionRange(argumentsText.length, argumentsText.length)
    })
  }

  const clearSelectedSkill = () => {
    setSelectedSkill(undefined)
    setSkillSuggestionsDismissed(false)
    requestAnimationFrame(() => ref.current?.focus())
  }

  const changeSettings = async (
    provider: string,
    model: string,
    nextThinking: ThinkingLevel,
  ) => {
    setSettingsError('')
    try {
      await onSettingsChange(provider, model, nextThinking)
    } catch (error) {
      setSettingsError(error instanceof Error ? error.message : t('composer.couldNotUpdateSettings'))
    }
  }

  const changePermissionMode = async (mode: PermissionMode) => {
    setSettingsError('')
    try {
      await onPermissionModeChange(mode)
    } catch (error) {
      const failure = error instanceof Error ? error : new Error(t('permission.couldNotUpdate'))
      setSettingsError(failure.message)
      throw failure
    }
  }

  const removeQueued = async (id: string) => {
    setQueueError('')
    try {
      await onRemoveQueued(id)
    } catch (error) {
      setQueueError(error instanceof Error ? error.message : t('composer.couldNotRemoveQueued'))
    }
  }

  return (
    <footer
      data-testid="composer"
      className={cn(
        'z-30 w-full',
        centered
          ? 'bg-transparent p-0'
          : 'shrink-0 bg-canvas px-3 pt-3 pb-4 md:px-8 max-md:pt-2',
      )}
    >
      <div className="relative mx-auto flex w-full max-w-[750px] flex-col gap-2">
        <TodoChecklist todos={todos?.todos ?? []} />
        {queuedMessages.length > 0 && (
          <PendingQueue messages={queuedMessages} onRemove={(id) => void removeQueued(id)} />
        )}
        {approval && <Approval key={approval.id} item={approval} onResolve={onResolve} />}
        {question && (
          question.questions[0]?.intent === 'plan_review' ? (
            <PlanReview key={question.id} item={question} onResolve={onResolveQuestion} />
          ) : (
            <Question key={question.id} item={question} onResolve={onResolveQuestion} />
          )
        )}

        <div
          ref={surfaceRef}
          data-testid="composer-surface"
          hidden={awaitingUser}
          className={cn(
            'relative min-h-[100px] rounded-[22px] border border-edge bg-canvas shadow-[0_10px_32px_-24px_rgba(28,25,23,0.32)] [container-type:inline-size]',
            !centered &&
              'transition-colors focus-within:border-edge-strong',
          )}
        >
          <ComposerSkillSuggestions
            id={skillSuggestionsID}
            visible={skillSuggestionsVisible}
            query={slashQuery?.query ?? ''}
            commandsEnabled={Boolean(slashQuery)}
            planEnabled={Boolean(onPlanModeChange)}
            skillsEnabled={Boolean(slashQuery)}
            projectSkills={suggestedProjectSkills}
            systemSkills={suggestedSystemSkills}
            activeIndex={activeSuggestionIndex}
            keyboardNavigating={skillKeyboardNavigating}
            loading={Boolean(slashQuery && skillsLoading)}
            failed={Boolean(slashQuery && skillsFailed)}
            onActiveIndexChange={setActiveSuggestionIndex}
            onPointerNavigation={() => setSkillKeyboardNavigating(false)}
            onCommandSelect={(command) => void runPreviewCommand(command)}
            onSelect={selectSkill}
          />
          <div
            className="grid min-h-[98px] grid-cols-[2.5rem_minmax(0,1fr)] grid-rows-[auto_2.5rem] items-center gap-x-3 gap-y-1 px-3 pt-2.5 pb-1.5 max-sm:gap-x-2"
          >
            <ComposerAddMenu
              disabled={editorDisabled}
              open={addPanelOpen}
              imageAttachmentAvailable={supportsImages}
              imageLimitReached={imageLimitReached}
              fileLimitReached={fileLimitReached}
              planMode={planMode}
              planModeDisabled={settingsDisabled}
              onOpenChange={(open) => {
                setAddPanelOpen(open)
                if (open) setSkillSuggestionsDismissed(true)
              }}
              onAttachImages={() => imageFileRef.current?.click()}
              onAttachFiles={() => textFileRef.current?.click()}
              onEnablePlanMode={
                onPlanModeChange
                  ? () => {
                      void changePlanMode(true)
                    }
                  : undefined
              }
            />
            <input
              ref={imageFileRef}
              className="sr-only"
              type="file"
              accept="image/png,image/jpeg,image/webp,image/gif"
              multiple
              tabIndex={-1}
              onChange={(event) => {
                void addImages(event.target.files)
                event.target.value = ''
              }}
            />
            <input
              ref={textFileRef}
              className="sr-only"
              type="file"
              multiple
              tabIndex={-1}
              onChange={(event) => {
                void addTextFiles(event.target.files)
                event.target.value = ''
              }}
            />
            <div className="col-span-2 col-start-1 row-start-1 flex min-w-0 flex-col gap-2">
              <ComposerAttachments
                files={files}
                images={images}
                onRemoveFile={removeFile}
                onRemoveImage={removeImage}
              />
              <div className="flex min-w-0 items-start px-1">
                {selectedSkill && (
                  <button
                    type="button"
                    className="mt-1.5 flex h-6 max-w-[45%] shrink-0 cursor-pointer items-center gap-1.5 rounded-md px-1.5 font-mono text-[13px] font-normal text-info outline-none transition-colors hover:bg-info-surface focus-visible:bg-info-surface"
                    aria-label={t('composer.removeSelectedSkill', {
                      name: selectedSkill.name,
                    })}
                    title={t('composer.removeSelectedSkill', {
                      name: selectedSkill.name,
                    })}
                    onClick={clearSelectedSkill}
                  >
                    <BookOpen
                      className="size-4 shrink-0"
                      strokeWidth={1.9}
                      aria-hidden="true"
                    />
                    <span className="truncate">{selectedSkill.name}</span>
                  </button>
                )}
                <textarea
                  ref={ref}
                  rows={1}
                  value={draftValue}
                  disabled={editorDisabled}
                  aria-autocomplete={!selectedSkill ? 'list' : undefined}
                  aria-controls={skillSuggestionsVisible ? skillSuggestionsID : undefined}
                  aria-expanded={!selectedSkill ? skillSuggestionsVisible : undefined}
                  aria-activedescendant={
                    skillSuggestionsVisible && suggestionCount > 0
                      ? skillSuggestionOptionID(
                          skillSuggestionsID,
                          Math.min(activeSuggestionIndex, suggestionCount - 1),
                        )
                      : undefined
                  }
                  className="block max-h-[15rem] min-h-8 min-w-0 flex-1 resize-none overflow-y-auto border-0 bg-transparent px-1 py-1.5 text-[13px] leading-5 font-normal text-ink outline-none placeholder:text-ink-faint disabled:cursor-not-allowed disabled:bg-transparent"
                  placeholder={
                    awaitingQuestion
                      ? t('composer.answerQuestionPlaceholder')
                      : awaitingApproval
                      ? t('composer.resolveApprovalPlaceholder')
                      : compacting
                        ? t('composer.compactingContext')
                      : !modelConfigured
                        ? t('composer.configureModelPlaceholder')
                      : connected
                        ? running
                          ? delivery === 'steer'
                            ? t('composer.guideRun')
                            : t('composer.queueFollowUpPlaceholder')
                          : t('composer.askAnything')
                        : t('composer.waitingForAPI')
                  }
                  onChange={(event) => {
                    setDraftValue(event.target.value)
                    setAddPanelOpen(false)
                    setSkillSuggestionsDismissed(false)
                    autosize()
                  }}
                  onFocus={() => {
                    if (!addPanelOpen) return
                    setAddPanelOpen(false)
                    setSkillSuggestionsDismissed(false)
                  }}
                  onCompositionStart={() => {
                    composingRef.current = true
                  }}
                  onCompositionEnd={() => {
                    composingRef.current = false
                  }}
                  onKeyDown={(event) => {
                    if (
                      composingRef.current ||
                      event.nativeEvent.isComposing ||
                      event.nativeEvent.keyCode === 229
                    ) {
                      return
                    }
                    if (
                      skillSuggestionsVisible &&
                      suggestionCount > 0 &&
                      event.key === 'ArrowDown'
                    ) {
                      event.preventDefault()
                      setSkillKeyboardNavigating(true)
                      setActiveSuggestionIndex(
                        (index) => moveSuggestionIndex(index, suggestionCount, 'next'),
                      )
                      return
                    }
                    if (
                      skillSuggestionsVisible &&
                      suggestionCount > 0 &&
                      event.key === 'ArrowUp'
                    ) {
                      event.preventDefault()
                      setSkillKeyboardNavigating(true)
                      setActiveSuggestionIndex(
                        (index) => moveSuggestionIndex(index, suggestionCount, 'previous'),
                      )
                      return
                    }
                    if (skillSuggestionsVisible && event.key === 'Escape') {
                      event.preventDefault()
                      setSkillSuggestionsDismissed(true)
                      return
                    }
                    const directCommand =
                      event.key === 'Enter' && !event.shiftKey
                        ? parseExecutableComposerCommand(draftValue)
                        : undefined
                    if (directCommand) {
                      event.preventDefault()
                      void runPreviewCommand(directCommand)
                      return
                    }
                    if (
                      skillSuggestionsVisible &&
                      suggestionCount > 0 &&
                      event.key === 'Enter' &&
                      !event.shiftKey
                    ) {
                      event.preventDefault()
                      if (activeSuggestionIndex < previewCommandCount) {
                        const command =
                          composerPreviewCommands[activeSuggestionIndex]
                        if (command) void runPreviewCommand(command)
                        return
                      }
                      const skill = suggestedSkills[
                        Math.min(
                          activeSuggestionIndex - previewCommandCount,
                          suggestedSkills.length - 1,
                        )
                      ]
                      if (skill) selectSkill(skill)
                      return
                    }
                    if (
                      selectedSkill &&
                      event.key === 'Backspace' &&
                      event.currentTarget.selectionStart === 0 &&
                      event.currentTarget.selectionEnd === 0
                    ) {
                      event.preventDefault()
                      clearSelectedSkill()
                      return
                    }
                    if (event.key === 'Enter' && !event.shiftKey) {
                      event.preventDefault()
                      void submit()
                    }
                  }}
                />
              </div>
            </div>
            <div className="col-start-2 row-start-2 -ml-[11px] flex min-w-0 items-center gap-1.5">
              <div
                data-testid="composer-permission-controls"
                className="flex min-w-0 shrink items-center gap-1"
              >
                <PermissionModeMenu
                  value={permissionMode}
                  disabled={settingsDisabled}
                  confirmationBlocked={settingsLocked}
                  onChange={changePermissionMode}
                />
                {onPlanModeChange && planMode && (
                  <button
                    type="button"
                    data-testid="composer-plan-mode"
                    aria-label={t('composer.disablePlanMode')}
                    aria-pressed={true}
                    disabled={settingsDisabled}
                    onClick={() => void changePlanMode(false)}
                    className={cn(
                      composerMenuTriggerClass,
                      'text-ink-muted hover:text-ink focus-visible:text-ink disabled:opacity-45',
                    )}
                  >
                    <span className="relative size-3.5 shrink-0" aria-hidden="true">
                      <Lightbulb
                        data-testid="plan-mode-lightbulb"
                        className="absolute inset-0 size-3.5 transition-opacity group-hover:opacity-0 group-focus-visible:opacity-0"
                      />
                      <span
                        data-testid="plan-mode-exit"
                        className="absolute inset-0 grid size-3.5 place-items-center rounded-full bg-ink-faint text-canvas opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100"
                      >
                        <X className="size-2.5" strokeWidth={2.4} />
                      </span>
                    </span>
                    <span className="max-w-24 truncate">{t('composer.planMode')}</span>
                    <ComposerControlTooltip align="start">
                      {t('composer.disablePlanMode')}
                    </ComposerControlTooltip>
                  </button>
                )}
                {projectPickerVisible && (
                  <ProjectPicker
                    workspaces={workspaces}
                    selectedPath={workspacePath}
                    disabled={settingsDisabled}
                    onSelect={onSelectProject}
                    onBrowse={onBrowseProjects}
                  />
                )}
              </div>
              <div
                data-testid="composer-model-controls"
                className="ml-auto flex min-w-0 items-center gap-1"
              >
                {modelConfigured && (
                  <ContextUsageMenu
                    usage={currentContextUsage}
                    contextWindow={contextWindow}
                    disabled={!connected}
                    compacting={compacting}
                    compactDisabled={settingsLocked || updatingSettings}
                    onCompact={
                      onCompact
                        ? () => {
                            void compactContext()
                          }
                        : undefined
                    }
                  />
                )}
                {modelConfigured ? (
                  <ModelSettingsMenu
                    models={models}
                    modelProvider={modelProvider}
                    modelID={modelID}
                    thinkingLevel={thinkingLevel}
                    disabled={settingsLocked}
                    updating={updatingSettings}
                    onChange={changeSettings}
                  />
                ) : (
                  <button
                    type="button"
                    onClick={onConfigureModel}
                    className={`${composerMenuTriggerClass} hover:text-ink`}
                  >
                    <span className="min-w-0 truncate">
                      {t('composer.configureModel')}
                    </span>
                    <ComposerControlTooltip align="end">
                      {t('composer.configureModel')}
                    </ComposerControlTooltip>
                  </button>
                )}
                {running && !awaitingApproval && (
                  <RunDeliveryMenu value={delivery} onValueChange={setDelivery} />
                )}
                {running && !awaitingApproval && (
                  <button
                    className="group relative grid size-[30px] shrink-0 cursor-pointer place-items-center rounded-full bg-canvas-strong text-ink-soft outline-none transition-colors hover:bg-ink-ghost focus-visible:bg-ink-ghost"
                    type="button"
                    aria-label={t('composer.stopGenerating')}
                    onClick={onStop}
                  >
                    <Square className="size-3 fill-current" aria-hidden="true" />
                    <ComposerControlTooltip align="end">
                      {t('composer.stopGenerating')}
                    </ComposerControlTooltip>
                  </button>
                )}
                <button
                  data-testid="composer-send"
                  className="group relative grid size-[30px] shrink-0 cursor-pointer place-items-center rounded-full bg-canvas-inverse text-ink-inverse outline-none transition-colors hover:bg-canvas-inverse focus-visible:bg-canvas-inverse disabled:cursor-not-allowed disabled:opacity-25"
                  type="button"
                  aria-label={
                    awaitingApproval
                      ? t('composer.resolveApprovalFirst')
                      : connected
                        ? running
                          ? delivery === 'steer'
                            ? t('composer.steerRun')
                            : t('composer.queueFollowUp')
                          : t('composer.sendPrompt')
                        : t('composer.waitingForCodingAPI')
                  }
                  disabled={sendDisabled}
                  onClick={() => void submit()}
                >
                  <ArrowUp className="size-4" aria-hidden="true" />
                  <ComposerControlTooltip align="end">
                    <span className="flex items-center gap-2">
                      <span>
                        {awaitingApproval
                          ? t('composer.resolveApprovalFirst')
                          : connected
                            ? running
                              ? delivery === 'steer'
                                ? t('composer.steerRun')
                                : t('composer.queueFollowUp')
                              : t('composer.sendPrompt')
                            : t('composer.waitingForAPIShort')}
                      </span>
                      {connected && !awaitingApproval && (
                        <kbd className="font-mono text-[0.6875rem] font-normal text-ink-faint">↵</kbd>
                      )}
                    </span>
                  </ComposerControlTooltip>
                </button>
              </div>
            </div>
          </div>
        </div>
        {compactFeedback && (
          <div
            className={cn(
              'absolute right-2 bottom-[calc(100%+0.625rem)] z-50 flex max-w-[calc(100vw-2rem)] animate-[fade-in_140ms_ease-out] items-center gap-2 border px-2.5 py-2 text-[0.8125rem] leading-5 shadow-[0_12px_32px_-18px_rgba(28,25,23,0.45)]',
              compactFeedback.kind === 'notice'
                ? 'rounded-lg border-edge bg-canvas text-ink-soft'
                : 'rounded-lg border-danger-edge bg-danger-surface text-danger',
            )}
            role={compactFeedback.kind === 'error' ? 'alert' : 'status'}
          >
            <Info
              className={cn(
                'size-4 shrink-0',
                compactFeedback.kind === 'notice' ? 'text-ink-muted' : 'text-danger-soft',
              )}
              aria-hidden="true"
            />
            <span>{compactFeedback.message}</span>
            <button
              type="button"
              className="grid size-6 shrink-0 cursor-pointer place-items-center rounded-md text-current opacity-55 outline-none transition-[background-color,opacity] hover:bg-scrim/5 hover:opacity-100 focus-visible:bg-scrim/5 focus-visible:opacity-100"
              aria-label={t('model.dismissCompactFeedback')}
              title={t('model.dismissCompactFeedback')}
              onClick={dismissCompactFeedback}
            >
              <X className="size-3.5" aria-hidden="true" />
            </button>
          </div>
        )}
        {(workspaceError || settingsError || planModeError || attachmentError || queueError || sendError) && (
          <p className="px-4 text-[0.75rem] leading-5 text-danger" role="alert">
            {workspaceError || settingsError || planModeError || attachmentError || queueError || sendError}
          </p>
        )}
      </div>
    </footer>
  )
}

const maxSkillSuggestions = 8
