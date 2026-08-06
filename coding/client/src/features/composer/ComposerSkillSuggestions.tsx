import { useLayoutEffect, useRef } from 'react'
import {
  BookOpen,
  Bug,
  FileText,
  ListChecks,
  LoaderCircle,
  MessageSquarePlus,
  Minimize2,
  type LucideIcon,
} from 'lucide-react'
import { useI18n } from '@/i18n'
import type { SkillEntry } from '@/features/skills'
import type { PromptTemplateEntry } from '@/features/prompt-templates'
import { cn } from '@/lib/utils'
import {
  composerPreviewCommands,
  composerFloatingPanelClass,
  type ComposerPreviewCommand,
  skillSuggestionOptionID,
  skillSuggestionsID,
} from './panelStyles'

export function ComposerSkillSuggestions({
  visible,
  query,
  commandsEnabled,
  skillsEnabled,
  templates,
  skills,
  activeIndex,
  keyboardNavigating,
  loading,
  failed,
  templatesLoading,
  templatesFailed,
  onActiveIndexChange,
  onPointerNavigation,
  onCommandSelect,
  onTemplateSelect,
  onSelect,
}: {
  visible: boolean
  query: string
  commandsEnabled: boolean
  skillsEnabled: boolean
  templates: PromptTemplateEntry[]
  skills: SkillEntry[]
  activeIndex: number
  keyboardNavigating: boolean
  loading: boolean
  failed: boolean
  templatesLoading: boolean
  templatesFailed: boolean
  onActiveIndexChange: (index: number) => void
  onPointerNavigation: () => void
  onCommandSelect: (command: ComposerPreviewCommand) => void
  onTemplateSelect: (template: PromptTemplateEntry) => void
  onSelect: (skill: SkillEntry) => void
}) {
  const { t } = useI18n()
  const scrollAreaRef = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    if (!visible || !keyboardNavigating) return
    const scrollArea = scrollAreaRef.current
    const option = document.getElementById(skillSuggestionOptionID(activeIndex))
    if (!scrollArea || !option || !scrollArea.contains(option)) return

    const scrollAreaRect = scrollArea.getBoundingClientRect()
    const optionRect = option.getBoundingClientRect()
    if (optionRect.top < scrollAreaRect.top) {
      scrollArea.scrollTop -= scrollAreaRect.top - optionRect.top
    } else if (optionRect.bottom > scrollAreaRect.bottom) {
      scrollArea.scrollTop += optionRect.bottom - scrollAreaRect.bottom
    }
  }, [activeIndex, keyboardNavigating, visible])

  if (!visible) return null
  const normalizedQuery = query.trim().toLocaleLowerCase()
  const showSkills =
    skillsEnabled && (!normalizedQuery || loading || failed || skills.length > 0)
  const noMatchingSuggestions = Boolean(
    normalizedQuery &&
      !templatesLoading &&
      !templatesFailed &&
      templates.length === 0 &&
      !loading &&
      !failed &&
      skills.length === 0,
  )
  const previewCommands: Array<{
    command: ComposerPreviewCommand
    icon: LucideIcon
    label: string
    description: string
  }> = !commandsEnabled || normalizedQuery
    ? []
    : composerPreviewCommands.map((command) => {
        switch (command) {
          case 'review':
            return {
              command,
              icon: Bug,
              label: t('composer.codeReview'),
              description: t('composer.codeReviewDescription'),
            }
          case 'compact':
            return {
              command,
              icon: Minimize2,
              label: t('composer.compactCommand'),
              description: t('composer.compactCommandDescription'),
            }
          case 'continue':
            return {
              command,
              icon: MessageSquarePlus,
              label: t('composer.continueInNewChat'),
              description: t('composer.continueInNewChatDescription'),
            }
          case 'plan':
            return {
              command,
              icon: ListChecks,
              label: t('composer.planMode'),
              description: t('composer.planModeDescription'),
            }
        }
      })

  return (
    <div
      id={skillSuggestionsID}
      role="listbox"
      aria-label={t('composer.skillSuggestions')}
      className={cn(
        composerFloatingPanelClass,
        keyboardNavigating && 'cursor-none [&_button]:cursor-none',
      )}
      onMouseMove={onPointerNavigation}
    >
      <div
        ref={scrollAreaRef}
        className="flex max-h-[19rem] flex-col gap-0.5 overflow-y-auto"
      >
        {previewCommands.map((item, index) => {
          const Icon = item.icon
          return (
            <button
              key={item.command}
              id={skillSuggestionOptionID(index)}
              type="button"
              role="option"
              aria-selected={index === activeIndex}
              className={cn(
                suggestionRowClass,
                'cursor-pointer outline-none transition-colors',
                index === activeIndex
                  ? 'bg-surface-active'
                  : 'bg-transparent',
              )}
              onMouseEnter={() => onActiveIndexChange(index)}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => {
                onActiveIndexChange(index)
                onCommandSelect(item.command)
              }}
            >
              <Icon
                className={cn(
                  'size-4 shrink-0',
                  index === activeIndex ? 'text-ink-muted' : 'text-ink-faint',
                )}
                aria-hidden="true"
              />
              <span className="max-w-40 truncate font-medium text-ink-soft">
                {item.label}
              </span>
              <span className="min-w-0 truncate text-right text-ink-faint">
                {item.description}
              </span>
              {item.command !== 'compact' && (
                <span className="text-[0.6875rem] text-ink-faint">
                  {t('composer.comingSoon')}
                </span>
              )}
            </button>
          )
        })}
        {(templates.length > 0 || templatesLoading || templatesFailed) && (
          <div
            role="presentation"
            className="flex h-7 shrink-0 items-center px-2.5 text-[0.71875rem] font-medium text-ink-faint"
          >
            {t('promptTemplates.title')}
          </div>
        )}
        {templatesLoading ? (
          <div className="flex h-14 items-center justify-center gap-2 text-[0.75rem] text-ink-faint">
            <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
            {t('composer.loadingPromptTemplates')}
          </div>
        ) : templatesFailed ? (
          <p className="px-3 py-4 text-center text-[0.75rem] text-ink-faint">
            {t('composer.promptTemplatesLoadError')}
          </p>
        ) : templates.map((template, index) => {
          const optionIndex = previewCommands.length + index
          return (
            <button
              id={skillSuggestionOptionID(optionIndex)}
              key={`${template.source}-${template.name}`}
              type="button"
              role="option"
              aria-selected={optionIndex === activeIndex}
              className={cn(
                suggestionRowClass,
                'cursor-pointer outline-none transition-colors',
                optionIndex === activeIndex
                  ? 'bg-surface-active'
                  : 'bg-transparent',
              )}
              onMouseEnter={() => onActiveIndexChange(optionIndex)}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => onTemplateSelect(template)}
            >
              <FileText
                className={cn(
                  'size-4 shrink-0',
                  optionIndex === activeIndex ? 'text-ink-muted' : 'text-ink-faint',
                )}
                strokeWidth={1.8}
                aria-hidden="true"
              />
              <span
                className={cn(
                  'flex min-w-0 items-baseline gap-1.5 font-medium',
                  optionIndex === activeIndex ? 'text-ink' : 'text-ink-soft',
                )}
              >
                <span className="truncate">{template.name}</span>
                {template.argumentHint && (
                  <span className="shrink-0 text-[0.6875rem] font-normal text-ink-faint">
                    {template.argumentHint}
                  </span>
                )}
              </span>
              <span className="min-w-0 truncate text-right text-ink-faint">
                {template.description}
              </span>
              <span className="text-[0.6875rem] text-ink-faint">
                {template.source === 'project'
                  ? t('skills.systemSourceProject')
                  : t('skills.systemSourceUser')}
              </span>
            </button>
          )
        })}
        {showSkills && (
          <div
            role="presentation"
            className="flex h-7 shrink-0 items-center px-2.5 text-[0.71875rem] font-medium text-ink-faint"
          >
            {t('skills.title')}
          </div>
        )}
        {showSkills && (
          loading ? (
            <div className="flex h-20 items-center justify-center gap-2 text-[0.75rem] text-ink-faint">
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
              {t('composer.loadingSkills')}
            </div>
          ) : failed ? (
            <p className="px-3 py-6 text-center text-[0.75rem] text-ink-faint">
              {t('composer.skillsLoadError')}
            </p>
          ) : skills.length === 0 ? (
            <p className="px-3 py-6 text-center text-[0.75rem] text-ink-faint">
              {t('composer.noSkills')}
            </p>
          ) : skills.map((skill, index) => {
            const optionIndex = previewCommands.length + templates.length + index
            return (
              <button
                id={skillSuggestionOptionID(optionIndex)}
                key={`${skill.source}-${skill.name}`}
                type="button"
                role="option"
                aria-selected={optionIndex === activeIndex}
                className={cn(
                  suggestionRowClass,
                  'cursor-pointer outline-none transition-colors',
                  optionIndex === activeIndex
                    ? 'bg-surface-active'
                    : 'bg-transparent',
                )}
                onMouseEnter={() => onActiveIndexChange(optionIndex)}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => onSelect(skill)}
              >
                <BookOpen
                  className={cn(
                    'size-4',
                    optionIndex === activeIndex ? 'text-info' : 'text-ink-muted',
                  )}
                  strokeWidth={1.8}
                  aria-hidden="true"
                />
                <span
                  className={cn(
                    'max-w-40 truncate font-medium',
                    optionIndex === activeIndex ? 'text-ink' : 'text-ink-soft',
                  )}
                >
                  {skill.name}
                </span>
                <span className="min-w-0 truncate text-right text-ink-faint">
                  {skill.description}
                </span>
                <span className="flex items-center gap-1.5 pl-1">
                  <span className="text-[0.6875rem] text-ink-faint">
                    {skill.source === 'project'
                      ? t('skills.systemSourceProject')
                      : t('skills.systemSourceUser')}
                  </span>
                </span>
              </button>
            )
          })
        )}
        {noMatchingSuggestions && (
          <p className="px-3 py-5 text-center text-[0.75rem] text-ink-faint">
            {t('composer.noMatchingCommands')}
          </p>
        )}
      </div>
    </div>
  )
}

const suggestionRowClass =
  'grid h-[30px] w-full shrink-0 grid-cols-[1.25rem_minmax(0,auto)_minmax(5rem,1fr)_auto] items-center gap-2 rounded-[10px] px-2.5 text-left text-[0.8125rem] leading-4'
