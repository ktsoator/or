import { useEffect, useMemo, useRef, useState } from 'react'
import {
  ArrowUp,
  BookOpen,
  Check,
  ChevronDown,
  FileCode2,
  Info,
  LoaderCircle,
  Square,
  X,
} from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import { isAPIError } from '@/api'
import type {
  ApprovalChoice,
  ApprovalItem,
  QuestionAnswer,
  QuestionItem,
  ContextUsage,
  DeliveryMode,
  MessageImage,
  ModelOption,
  PendingFile,
  PendingImage,
  PermissionMode,
  PromptFile,
  QueuedMessage,
  ThinkingLevel,
  WorkspaceSummary,
} from '@/types'
import { cn } from '@/lib/utils'
import {
  formatFileSize,
  maxTextFiles,
  readTextFile,
  validateTextFiles,
} from '@/attachments'
import {
  buildSkillInvocation,
  fetchSkills,
  filterSkills,
  parseSkillSlashQuery,
  skillArgumentsFromDraft,
  type SkillEntry,
  type SkillsResponse,
} from '@/skills'
import { Approval } from './Approval'
import { ComposerAddMenu } from './ComposerAddMenu'
import { ComposerSkillSuggestions } from './ComposerSkillSuggestions'
import {
  composerPreviewCommands,
  type ComposerPreviewCommand,
  parseExecutableComposerCommand,
  previewSkillCommandCount,
  skillSuggestionOptionID,
  skillSuggestionsID,
} from './composerPanelStyles'
import { Question } from './Question'
import { ModelSettingsMenu } from './ModelSettingsMenu'
import { PermissionModeMenu } from './PermissionModeMenu'
import { ProjectPicker } from './ProjectPicker'
import { useI18n } from '@/i18n'

export function Composer({
  connected,
  running,
  approval,
  question,
  queuedMessages,
  contextUsage,
  centered = false,
  projectPickerVisible = false,
  workspaces,
  workspacePath,
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
  onCompact,
}: {
  connected: boolean
  running: boolean
  approval?: ApprovalItem
  question?: QuestionItem
  queuedMessages: QueuedMessage[]
  contextUsage?: ContextUsage
  centered?: boolean
  projectPickerVisible?: boolean
  workspaces: WorkspaceSummary[]
  workspacePath?: string
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
  onCompact?: () => Promise<unknown>
}) {
  const { t } = useI18n()
  const ref = useRef<HTMLTextAreaElement>(null)
  const imageFileRef = useRef<HTMLInputElement>(null)
  const textFileRef = useRef<HTMLInputElement>(null)
  const surfaceRef = useRef<HTMLDivElement>(null)
  const composingRef = useRef(false)
  const submittingRef = useRef(false)
  const compactFeedbackTimerRef = useRef<number | undefined>(undefined)
  const [settingsError, setSettingsError] = useState('')
  const [attachmentError, setAttachmentError] = useState('')
  const [queueError, setQueueError] = useState('')
  const [sendError, setSendError] = useState('')
  const [compactFeedback, setCompactFeedback] = useState<CompactFeedback>()
  const [images, setImages] = useState<PendingImage[]>([])
  const [files, setFiles] = useState<PendingFile[]>([])
  const [delivery, setDelivery] = useState<DeliveryMode>('steer')
  const [draftValue, setDraftValue] = useState('')
  const [selectedSkill, setSelectedSkill] = useState<SkillEntry>()
  const [skillsData, setSkillsData] = useState<SkillsResponse>()
  const [skillsLoading, setSkillsLoading] = useState(true)
  const [skillsFailed, setSkillsFailed] = useState(false)
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
  const inputDisabled =
    awaitingUser || !connected || updatingSettings || compacting || !modelConfigured
  const settingsDisabled = running || inputDisabled
  const supportsImages = Boolean(
    models.find((model) => model.provider === modelProvider && model.id === modelID)
      ?.supportsImages,
  )
  const availableSkills = useMemo(
    () => [...(skillsData?.project ?? []), ...(skillsData?.user ?? [])],
    [skillsData],
  )
  const slashQuery =
    !running && !selectedSkill && !addPanelOpen && !skillSuggestionsDismissed
      ? parseSkillSlashQuery(draftValue)
      : undefined
  const suggestedSkills = slashQuery
    ? filterSkills(availableSkills, slashQuery.query).slice(0, maxSkillSuggestions)
    : []
  const previewCommandCount = slashQuery
    ? previewSkillCommandCount(slashQuery.query)
    : 0
  const suggestionCount = previewCommandCount + suggestedSkills.length
  const skillSuggestionsVisible = Boolean(
    slashQuery && !inputDisabled,
  )

  const autosize = () => {
    const el = ref.current
    if (!el) return
    el.style.height = '0px'
    const contentHeight = el.scrollHeight
    el.style.height = Math.min(contentHeight, 240) + 'px'
  }

  useEffect(() => {
    if (!inputDisabled) ref.current?.focus()
  }, [inputDisabled])

  useEffect(() => {
    if (!running) setDelivery('steer')
  }, [running])

  useEffect(() => setSettingsError(''), [modelProvider, modelID, thinkingLevel, permissionMode])

  useEffect(() => {
    if (supportsImages) setAttachmentError('')
  }, [supportsImages])

  useEffect(() => {
    const controller = new AbortController()
    setSkillsLoading(true)
    setSkillsFailed(false)
    setSkillsData(undefined)
    void fetchSkills(workspacePath, controller.signal)
      .then(setSkillsData)
      .catch((error) => {
        if (error instanceof DOMException && error.name === 'AbortError') return
        setSkillsFailed(true)
      })
      .finally(() => {
        if (!controller.signal.aborted) setSkillsLoading(false)
      })
    return () => controller.abort()
  }, [workspacePath])

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
    if (!selectedSkill || !skillsData) return
    const stillAvailable = availableSkills.some(
      (skill) =>
        skill.name === selectedSkill.name && skill.source === selectedSkill.source,
    )
    if (!stillAvailable) setSelectedSkill(undefined)
  }, [availableSkills, selectedSkill, skillsData])

  useEffect(
    () => () => {
      if (compactFeedbackTimerRef.current !== undefined) {
        window.clearTimeout(compactFeedbackTimerRef.current)
      }
    },
    [],
  )

  const dismissCompactFeedback = () => {
    if (compactFeedbackTimerRef.current !== undefined) {
      window.clearTimeout(compactFeedbackTimerRef.current)
      compactFeedbackTimerRef.current = undefined
    }
    setCompactFeedback(undefined)
  }

  const showCompactFeedback = (feedback: CompactFeedback) => {
    dismissCompactFeedback()
    setCompactFeedback(feedback)
    compactFeedbackTimerRef.current = window.setTimeout(() => {
      compactFeedbackTimerRef.current = undefined
      setCompactFeedback(undefined)
    }, 4000)
  }

  const compactContext = async () => {
    if (!onCompact) {
      showCompactFeedback({
        kind: 'notice',
        message: t('model.nothingToCompact'),
      })
      return
    }
    dismissCompactFeedback()
    try {
      await onCompact()
    } catch (error) {
      showCompactFeedback({
        kind: isAPIError(error, 'nothing_to_compact') ? 'notice' : 'error',
        message: isAPIError(error, 'nothing_to_compact')
          ? t('model.nothingToCompact')
          : t('model.compactFailed'),
      })
    }
  }

  const runPreviewCommand = async (command: ComposerPreviewCommand) => {
    if (command !== 'compact') return
    setDraftValue('')
    setAddPanelOpen(false)
    setSkillSuggestionsDismissed(true)
    requestAnimationFrame(autosize)
    await compactContext()
  }

  const submit = async () => {
    const el = ref.current
    if (!el || submittingRef.current) return
    const command = parseExecutableComposerCommand(draftValue)
    if (command) {
      await runPreviewCommand(command)
      return
    }
    const argumentsText = draftValue.trim()
    const text = selectedSkill
      ? buildSkillInvocation(selectedSkill.name, argumentsText)
      : argumentsText
    if ((!text && images.length === 0 && files.length === 0) || inputDisabled) return
    if (images.length > 0 && !supportsImages) {
      setAttachmentError(t('composer.modelNoImages'))
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
      )
      if (!accepted) return
      setDraftValue('')
      setSelectedSkill(undefined)
      setSkillSuggestionsDismissed(false)
      setImages([])
      setFiles([])
      setAttachmentError('')
      requestAnimationFrame(autosize)
    } catch (error) {
      setSendError(error instanceof Error ? error.message : t('composer.couldNotSend'))
    } finally {
      submittingRef.current = false
    }
  }

  const addImages = async (files: FileList | null) => {
    if (!files || files.length === 0 || !supportsImages) return
    setAttachmentError('')
    const selected = Array.from(files)
    if (images.length + selected.length > maxImages) {
      setAttachmentError(t('composer.maxImages', { count: maxImages }))
      return
    }
    const allowed = new Set(['image/gif', 'image/jpeg', 'image/png', 'image/webp'])
    if (selected.some((file) => !allowed.has(file.type))) {
      setAttachmentError(t('composer.imageTypes'))
      return
    }
    if (selected.some((file) => file.size > maxImageBytes)) {
      setAttachmentError(t('composer.imageTooLarge'))
      return
    }
    const totalBytes =
      images.reduce((total, image) => total + image.size, 0) +
      selected.reduce((total, file) => total + file.size, 0)
    if (totalBytes > maxImagesBytes) {
      setAttachmentError(t('composer.imagesTooLarge'))
      return
    }
    try {
      const added = await Promise.all(selected.map(readImage))
      setImages((current) => [...current, ...added])
    } catch {
      setAttachmentError(t('composer.couldNotReadImage'))
    }
  }

  const addTextFiles = async (selectedFiles: FileList | null) => {
    if (!selectedFiles || selectedFiles.length === 0) return
    setAttachmentError('')
    const selected = Array.from(selectedFiles)
    const validation = validateTextFiles(files, selected)
    if (validation) {
      switch (validation) {
        case 'count':
          setAttachmentError(t('composer.maxFiles', { count: maxTextFiles }))
          break
        case 'type':
          setAttachmentError(t('composer.fileTypes'))
          break
        case 'file_size':
          setAttachmentError(t('composer.fileTooLarge'))
          break
        case 'total_size':
          setAttachmentError(t('composer.filesTooLarge'))
          break
      }
      return
    }
    try {
      const added = await Promise.all(selected.map(readTextFile))
      setFiles((current) => [...current, ...added])
    } catch {
      setAttachmentError(t('composer.fileNotText'))
    }
  }

  const selectSkill = (skill: SkillEntry) => {
    const el = ref.current
    if (!el) return
    const argumentsText = selectedSkill
      ? draftValue
      : skillArgumentsFromDraft(draftValue)
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
      setSettingsError(error instanceof Error ? error.message : t('permission.couldNotUpdate'))
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
        {queuedMessages.length > 0 && (
          <PendingQueue messages={queuedMessages} onRemove={(id) => void removeQueued(id)} />
        )}
        {approval && <Approval key={approval.id} item={approval} onResolve={onResolve} />}
        {question && (
          <Question key={question.id} item={question} onResolve={onResolveQuestion} />
        )}

        <div
          ref={surfaceRef}
          hidden={awaitingUser}
          className={cn(
            'relative rounded-[28px] border border-edge bg-canvas [container-type:inline-size]',
            !centered &&
              'transition-colors focus-within:border-edge-strong',
          )}
        >
          <ComposerSkillSuggestions
            visible={skillSuggestionsVisible}
            query={slashQuery?.query ?? ''}
            skills={suggestedSkills}
            activeIndex={activeSuggestionIndex}
            keyboardNavigating={skillKeyboardNavigating}
            loading={skillsLoading}
            failed={skillsFailed}
            onActiveIndexChange={setActiveSuggestionIndex}
            onPointerNavigation={() => setSkillKeyboardNavigating(false)}
            onCommandSelect={(command) => void runPreviewCommand(command)}
            onSelect={selectSkill}
          />
          <div
            className="grid min-h-24 grid-cols-[2.5rem_minmax(0,1fr)] grid-rows-[auto_2.5rem] items-center gap-x-3 gap-y-1 px-3 py-2.5 max-sm:gap-x-2"
          >
            <ComposerAddMenu
              disabled={inputDisabled}
              open={addPanelOpen}
              imageAttachmentAvailable={supportsImages}
              imageLimitReached={images.length >= maxImages}
              fileLimitReached={files.length >= maxTextFiles}
              onOpenChange={(open) => {
                setAddPanelOpen(open)
                if (open) setSkillSuggestionsDismissed(true)
              }}
              onAttachImages={() => imageFileRef.current?.click()}
              onAttachFiles={() => textFileRef.current?.click()}
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
              {files.length > 0 && (
                <div className="flex flex-wrap gap-1.5 px-1 pt-1">
                  {files.map((file) => (
                    <div
                      key={file.id}
                      className="group/file flex h-8 max-w-[15rem] items-center gap-1.5 rounded-lg border border-edge bg-canvas-raised pr-1 pl-2 text-[0.75rem] text-ink-muted"
                    >
                      <FileCode2
                        className="size-3.5 shrink-0 text-ink-muted"
                        aria-hidden="true"
                      />
                      <span className="min-w-0 truncate font-medium text-ink-soft">
                        {file.name}
                      </span>
                      <span className="shrink-0 text-[0.6875rem] text-ink-faint">
                        {formatFileSize(file.size)}
                      </span>
                      <button
                        className="grid size-6 shrink-0 cursor-pointer place-items-center rounded-md text-ink-faint outline-none transition-colors hover:bg-canvas-strong/70 hover:text-ink-soft focus-visible:bg-canvas-strong/70 focus-visible:text-ink-soft"
                        type="button"
                        aria-label={t('composer.removeFile', { name: file.name })}
                        title={t('composer.removeFile', { name: file.name })}
                        onClick={() => {
                          setFiles((current) =>
                            current.filter((item) => item.id !== file.id),
                          )
                          setAttachmentError('')
                        }}
                      >
                        <X className="size-3.5" aria-hidden="true" />
                      </button>
                    </div>
                  ))}
                </div>
              )}
              {images.length > 0 && (
                <div className="flex flex-wrap gap-2 px-1 pt-1">
                  {images.map((image) => (
                    <div
                      key={image.id}
                      className="group/image relative size-16 overflow-hidden rounded-xl border border-edge bg-canvas-raised shadow-sm"
                    >
                      <img
                        className="size-full object-cover"
                        src={`data:${image.mimeType};base64,${image.data}`}
                        alt={image.name}
                      />
                      <button
                        className="absolute top-1 right-1 grid size-5 cursor-pointer place-items-center rounded-full bg-scrim/85 text-ink-inverse opacity-100 shadow-sm transition-opacity focus-visible:opacity-100 md:opacity-0 md:group-hover/image:opacity-100"
                        type="button"
                        aria-label={t('composer.removeImage', { name: image.name })}
                        onClick={() => {
                          setImages((current) =>
                            current.filter((item) => item.id !== image.id),
                          )
                          setAttachmentError('')
                        }}
                      >
                        <X className="size-3" aria-hidden="true" />
                      </button>
                    </div>
                  ))}
                </div>
              )}
              <div className="flex min-w-0 items-start px-1">
                {selectedSkill && (
                  <button
                    type="button"
                    className="mt-1.5 flex h-6 max-w-[45%] shrink-0 cursor-pointer items-center gap-1.5 rounded-md px-1.5 font-mono text-[14px] font-medium text-info outline-none transition-colors hover:bg-info-surface focus-visible:bg-info-surface"
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
                  disabled={inputDisabled}
                  aria-autocomplete={!selectedSkill ? 'list' : undefined}
                  aria-controls={skillSuggestionsVisible ? skillSuggestionsID : undefined}
                  aria-expanded={!selectedSkill ? skillSuggestionsVisible : undefined}
                  aria-activedescendant={
                    skillSuggestionsVisible && suggestionCount > 0
                      ? skillSuggestionOptionID(
                          Math.min(activeSuggestionIndex, suggestionCount - 1),
                        )
                      : undefined
                  }
                  className="block max-h-[15rem] min-h-8 min-w-0 flex-1 resize-none overflow-y-auto border-0 bg-transparent px-1 py-1.5 text-[14px] leading-6 text-ink outline-none placeholder:text-ink-faint disabled:cursor-not-allowed disabled:bg-transparent"
                  placeholder={
                    awaitingQuestion
                      ? t('composer.answerQuestionPlaceholder')
                      : awaitingApproval
                      ? t('composer.resolveApprovalPlaceholder')
                      : compacting
                        ? t('composer.compactingContext')
                        : updatingSettings
                          ? t('composer.updatingSettings')
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
                        (activeSuggestionIndex + 1) % suggestionCount,
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
                        (activeSuggestionIndex - 1 + suggestionCount) %
                          suggestionCount,
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
            <div className="col-start-2 row-start-2 flex min-w-0 items-center gap-2.5 max-sm:gap-1.5">
              <div
                data-testid="composer-permission-controls"
                className="flex min-w-0 shrink items-center gap-1"
              >
                <PermissionModeMenu
                  value={permissionMode}
                  disabled={settingsDisabled}
                  onChange={changePermissionMode}
                />
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
                className="ml-auto flex min-w-0 items-center gap-2.5 max-sm:gap-1.5"
              >
                {modelConfigured ? (
                  <ModelSettingsMenu
                    models={models}
                    modelProvider={modelProvider}
                    modelID={modelID}
                    thinkingLevel={thinkingLevel}
                    contextUsage={contextUsage}
                    disabled={settingsDisabled}
                    onChange={changeSettings}
                    compacting={compacting}
                    onCompact={onCompact ? compactContext : undefined}
                  />
                ) : (
                  <button
                    type="button"
                    onClick={onConfigureModel}
                    className="inline-flex h-[30px] min-w-0 cursor-pointer items-center truncate rounded-[10px] px-3 text-[0.8125rem] font-medium text-ink-muted outline-none transition-colors hover:bg-surface-active hover:text-ink focus-visible:bg-surface-active"
                  >
                    {t('composer.configureModel')}
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
                    <span
                      className="pointer-events-none absolute right-0 bottom-[calc(100%+0.5625rem)] z-50 translate-y-1 whitespace-nowrap rounded-md bg-canvas-inverse px-2.5 py-1.5 text-[0.75rem] leading-4 font-medium text-ink-inverse opacity-0 shadow-lg transition-[opacity,transform] duration-150 group-hover:translate-y-0 group-hover:opacity-100 group-focus-visible:translate-y-0 group-focus-visible:opacity-100"
                      aria-hidden="true"
                    >
                      {t('composer.stopGenerating')}
                    </span>
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
                  disabled={inputDisabled}
                  onClick={() => void submit()}
                >
                  <ArrowUp className="size-4" aria-hidden="true" />
                  <span
                    className="pointer-events-none absolute right-0 bottom-[calc(100%+0.5625rem)] z-50 flex translate-y-1 items-center gap-2 whitespace-nowrap rounded-md bg-canvas-inverse px-2.5 py-1.5 text-[0.75rem] leading-4 font-medium text-ink-inverse opacity-0 shadow-lg transition-[opacity,transform] duration-150 group-hover:translate-y-0 group-hover:opacity-100 group-focus-visible:translate-y-0 group-focus-visible:opacity-100"
                    aria-hidden="true"
                  >
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
        {(settingsError || attachmentError || queueError || sendError) && (
          <p className="px-4 text-[0.75rem] leading-5 text-danger" role="alert">
            {settingsError || attachmentError || queueError || sendError}
          </p>
        )}
      </div>
    </footer>
  )
}

type CompactFeedback = {
  kind: 'notice' | 'error'
  message: string
}

function PendingQueue({
  messages,
  onRemove,
}: {
  messages: QueuedMessage[]
  onRemove: (id: string) => void
}) {
  const { t } = useI18n()
  return (
    <section
      className="overflow-hidden rounded-[18px] border border-edge/90 bg-canvas-raised text-ink-soft shadow-[0_8px_24px_-22px_rgba(28,25,23,0.5)]"
      aria-label={t('queue.pendingMessages')}
      aria-live="polite"
    >
      <div className="flex h-8 items-center justify-between px-3.5 text-[0.71875rem] leading-none text-ink-muted">
        <span className="font-medium text-ink-muted">{t('queue.upNext')}</span>
        <span>{messages.length}</span>
      </div>
      <div className="max-h-[8.25rem] overflow-y-auto border-t border-edge/80">
        {messages.map((message, index) => (
          <div
            key={message.id}
            className={cn(
              'group/queue flex min-h-11 items-center gap-2.5 py-2 pr-2 pl-3.5 text-[0.8125rem]',
              index > 0 && 'border-t border-edge/70',
            )}
          >
            <span
              className={cn(
                'size-1.5 shrink-0 rounded-full',
                message.status === 'failed' ? 'bg-danger-soft' : 'bg-ink-faint',
              )}
              aria-hidden="true"
            />
            <span className="shrink-0 font-medium text-ink-soft">
              {message.delivery === 'steer' ? t('queue.steer') : t('queue.followUp')}
            </span>
            <span className="min-w-0 flex-1 truncate text-ink-muted">
              {message.text ||
                ((message.files?.length ?? 0) > 0
                  ? `${message.files?.length ?? 0} ${
                      message.files?.length === 1 ? t('queue.file') : t('queue.files')
                    }`
                  : `${message.images.length} ${
                      message.images.length === 1 ? t('queue.image') : t('queue.images')
                    }`)}
            </span>
            {message.text && message.images.length > 0 && (
              <span className="shrink-0 text-[0.71875rem] text-ink-faint">
                +{message.images.length}{' '}
                {message.images.length === 1 ? t('queue.image') : t('queue.images')}
              </span>
            )}
            {message.text && (message.files?.length ?? 0) > 0 && (
              <span className="shrink-0 text-[0.71875rem] text-ink-faint">
                +{message.files?.length ?? 0}{' '}
                {message.files?.length === 1 ? t('queue.file') : t('queue.files')}
              </span>
            )}
            <span
              className={cn(
                'shrink-0 text-[0.71875rem]',
                message.status === 'failed' ? 'text-danger-soft' : 'text-ink-faint',
              )}
            >
              {message.status === 'failed'
                ? t('app.notSent')
                : message.status === 'removing'
                  ? t('queue.removing')
                  : t('queue.waiting')}
            </span>
            <button
              className="grid size-7 shrink-0 cursor-pointer place-items-center rounded-lg text-ink-faint outline-none transition-colors hover:bg-canvas-strong/80 hover:text-ink-soft focus-visible:bg-canvas-strong/80 focus-visible:text-ink-soft disabled:cursor-wait disabled:opacity-55"
              type="button"
              aria-label={
                message.delivery === 'steer'
                  ? t('queue.removeSteer')
                  : t('queue.removeFollowUp')
              }
              title={t('queue.remove')}
              disabled={message.status === 'removing'}
              onClick={() => onRemove(message.id)}
            >
              {message.status === 'removing' ? (
                <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
              ) : (
                <X className="size-3.5" aria-hidden="true" />
              )}
            </button>
          </div>
        ))}
      </div>
    </section>
  )
}

function RunDeliveryMenu({
  value,
  onValueChange,
}: {
  value: DeliveryMode
  onValueChange: (value: DeliveryMode) => void
}) {
  const { t } = useI18n()
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          className="group inline-flex h-[30px] cursor-pointer items-center gap-1 rounded-[10px] px-2.5 text-[0.8125rem] font-medium text-ink-muted outline-none transition-colors hover:bg-surface-active focus-visible:bg-surface-active data-[state=open]:bg-surface-selected"
          type="button"
          aria-label={t('delivery.choose')}
        >
          <span>{value === 'steer' ? t('queue.steer') : t('queue.followUp')}</span>
          <ChevronDown
            className="size-3.5 text-ink-faint transition-transform group-data-[state=open]:rotate-180"
            aria-hidden="true"
          />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          side="top"
          align="end"
          sideOffset={7}
          collisionPadding={10}
          className="z-[110] min-w-[14.75rem] animate-[fade-in_110ms_ease-out] rounded-2xl border border-edge bg-canvas p-1 text-[0.8125rem] text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
        >
          <DropdownMenu.RadioGroup
            className="flex flex-col gap-0.5"
            value={value}
            onValueChange={(next) => onValueChange(next as DeliveryMode)}
          >
            <DeliveryOption
              value="steer"
              label={t('composer.steerRun')}
              hint={t('delivery.steerHint')}
            />
            <DeliveryOption
              value="followup"
              label={t('composer.queueFollowUp')}
              hint={t('delivery.followUpHint')}
            />
          </DropdownMenu.RadioGroup>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

function DeliveryOption({ value, label, hint }: { value: DeliveryMode; label: string; hint: string }) {
  return (
    <DropdownMenu.RadioItem
      value={value}
      className="relative flex h-[35px] cursor-default select-none items-center gap-2 rounded-[10px] px-2.5 pr-8 outline-none data-[highlighted]:bg-surface-active data-[state=checked]:bg-surface-selected"
    >
      <span className="font-medium">{label}</span>
      <span className="ml-auto text-[0.71875rem] text-ink-faint">{hint}</span>
      <DropdownMenu.ItemIndicator className="absolute right-2 grid size-4 place-items-center text-ink-soft">
        <Check className="size-3.5" aria-hidden="true" />
      </DropdownMenu.ItemIndicator>
    </DropdownMenu.RadioItem>
  )
}

const maxImages = 4
const maxImageBytes = 10 * 1024 * 1024
const maxImagesBytes = 20 * 1024 * 1024
const maxSkillSuggestions = 8

function readImage(file: File): Promise<PendingImage> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(reader.error)
    reader.onload = () => {
      const result = typeof reader.result === 'string' ? reader.result : ''
      const comma = result.indexOf(',')
      if (comma < 0) {
        reject(new Error('invalid image data'))
        return
      }
      resolve({
        id: `${file.name}-${file.lastModified}-${crypto.randomUUID()}`,
        name: file.name,
        size: file.size,
        mimeType: file.type,
        data: result.slice(comma + 1),
      })
    }
    reader.readAsDataURL(file)
  })
}
