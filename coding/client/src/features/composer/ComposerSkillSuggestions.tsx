import { useLayoutEffect, useRef } from 'react'
import {
  BookOpen,
  Bug,
  ListChecks,
  LoaderCircle,
  MessageSquarePlus,
  Minimize2,
  type LucideIcon,
} from 'lucide-react'
import { useI18n } from '@/i18n'
import type { SkillEntry } from '@/features/skills'
import { cn } from '@/lib/utils'
import {
  composerPreviewCommands,
  composerFloatingPanelClass,
  type ComposerPreviewCommand,
  skillSuggestionOptionID,
} from './panelStyles'

export function ComposerSkillSuggestions({
  id,
  visible,
  query,
  commandsEnabled,
  planEnabled,
  skillsEnabled,
  projectSkills,
  systemSkills,
  activeIndex,
  keyboardNavigating,
  loading,
  failed,
  onActiveIndexChange,
  onPointerNavigation,
  onCommandSelect,
  onSelect,
}: {
  id: string
  visible: boolean
  query: string
  commandsEnabled: boolean
  planEnabled: boolean
  skillsEnabled: boolean
  projectSkills: SkillEntry[]
  systemSkills: SkillEntry[]
  activeIndex: number
  keyboardNavigating: boolean
  loading: boolean
  failed: boolean
  onActiveIndexChange: (index: number) => void
  onPointerNavigation: () => void
  onCommandSelect: (command: ComposerPreviewCommand) => void
  onSelect: (skill: SkillEntry) => void
}) {
  const { t } = useI18n()
  const scrollAreaRef = useRef<HTMLDivElement>(null)
  const previousActiveIndexRef = useRef(activeIndex)
  const normalizedQuery = query.trim().toLocaleLowerCase()
  const skills = [...projectSkills, ...systemSkills]
  const optionCount =
    (commandsEnabled && !normalizedQuery ? composerPreviewCommands.length : 0) +
    skills.length

  useLayoutEffect(() => {
    const previousActiveIndex = previousActiveIndexRef.current
    previousActiveIndexRef.current = activeIndex
    if (!visible || !keyboardNavigating) return
    const scrollArea = scrollAreaRef.current
    const option = document.getElementById(skillSuggestionOptionID(id, activeIndex))
    if (!scrollArea || !option || !scrollArea.contains(option)) return

    if (optionCount > 1 && previousActiveIndex === optionCount - 1 && activeIndex === 0) {
      scrollArea.scrollTop = 0
      return
    }
    if (optionCount > 1 && previousActiveIndex === 0 && activeIndex === optionCount - 1) {
      scrollArea.scrollTop = scrollArea.scrollHeight - scrollArea.clientHeight
      return
    }

    const scrollAreaRect = scrollArea.getBoundingClientRect()
    const optionRect = option.getBoundingClientRect()
    if (optionRect.top < scrollAreaRect.top) {
      scrollArea.scrollTop -= scrollAreaRect.top - optionRect.top
    } else if (optionRect.bottom > scrollAreaRect.bottom) {
      scrollArea.scrollTop += optionRect.bottom - scrollAreaRect.bottom
    }
  }, [activeIndex, id, keyboardNavigating, optionCount, visible])

  if (!visible) return null
  const showSkills =
    skillsEnabled && (!normalizedQuery || loading || failed || skills.length > 0)
  const noMatchingSuggestions = Boolean(
    normalizedQuery &&
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
      id={id}
      role="listbox"
      aria-label={t('composer.skillSuggestions')}
      className={cn(composerFloatingPanelClass, 'cursor-default')}
    >
      <div
        ref={scrollAreaRef}
        className="flex max-h-[19rem] min-w-0 flex-col overflow-x-hidden overflow-y-auto overscroll-contain"
      >
        {previewCommands.map((item, index) => {
          const Icon = item.icon
          return (
            <button
              key={item.command}
              id={skillSuggestionOptionID(id, index)}
              type="button"
              role="option"
              aria-selected={index === activeIndex}
              className={cn(
                commandSuggestionRowClass,
                'cursor-pointer outline-none',
                keyboardNavigating ? 'transition-none' : 'transition-colors',
                index === activeIndex
                  ? 'bg-surface-active'
                  : 'bg-transparent',
              )}
              onMouseEnter={() => {
                if (!keyboardNavigating) onActiveIndexChange(index)
              }}
              onMouseMove={(event) => {
                if (event.movementX === 0 && event.movementY === 0) return
                onPointerNavigation()
                onActiveIndexChange(index)
              }}
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
              <span className="min-w-0 truncate font-medium text-ink-soft">
                {item.label}
              </span>
              <span className="min-w-0 truncate text-right text-ink-faint">
                {item.description}
              </span>
              {item.command !== 'compact' && !(item.command === 'plan' && planEnabled) && (
                <span className="shrink-0 whitespace-nowrap text-[0.6875rem] text-ink-faint">
                  {t('composer.comingSoon')}
                </span>
              )}
            </button>
          )
        })}
        {showSkills && (loading || failed || skills.length === 0) && (
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
          ) : (
            <>
              <SkillSuggestionGroup
                title={t('skills.projectSection')}
                skills={projectSkills}
                suggestionsID={id}
                indexOffset={previewCommands.length}
                activeIndex={activeIndex}
                keyboardNavigating={keyboardNavigating}
                onActiveIndexChange={onActiveIndexChange}
                onPointerNavigation={onPointerNavigation}
                onSelect={onSelect}
              />
              <SkillSuggestionGroup
                title={t('skills.systemSection')}
                skills={systemSkills}
                suggestionsID={id}
                indexOffset={previewCommands.length + projectSkills.length}
                activeIndex={activeIndex}
                keyboardNavigating={keyboardNavigating}
                onActiveIndexChange={onActiveIndexChange}
                onPointerNavigation={onPointerNavigation}
                onSelect={onSelect}
              />
            </>
          )
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

function SkillSuggestionGroup({
  title,
  skills,
  suggestionsID,
  indexOffset,
  activeIndex,
  keyboardNavigating,
  onActiveIndexChange,
  onPointerNavigation,
  onSelect,
}: {
  title: string
  skills: SkillEntry[]
  suggestionsID: string
  indexOffset: number
  activeIndex: number
  keyboardNavigating: boolean
  onActiveIndexChange: (index: number) => void
  onPointerNavigation: () => void
  onSelect: (skill: SkillEntry) => void
}) {
  if (skills.length === 0) return null
  return (
    <div
      role="group"
      aria-label={title}
      className="mt-1 min-w-0 border-t border-edge/60 pt-1"
    >
      <div
        role="presentation"
        className="flex h-7 min-w-0 items-center px-2.5 text-[0.71875rem] font-medium text-ink-faint"
      >
        <span className="truncate">{title}</span>
      </div>
      {skills.map((skill, index) => {
        const optionIndex = indexOffset + index
        return (
          <button
            id={skillSuggestionOptionID(suggestionsID, optionIndex)}
            key={`${skill.source}-${skill.name}`}
            type="button"
            role="option"
            aria-selected={optionIndex === activeIndex}
            className={cn(
              skillSuggestionRowClass,
              'cursor-pointer outline-none',
              keyboardNavigating ? 'transition-none' : 'transition-colors',
              optionIndex === activeIndex ? 'bg-surface-active' : 'bg-transparent',
            )}
            onMouseEnter={() => {
              if (!keyboardNavigating) onActiveIndexChange(optionIndex)
            }}
            onMouseMove={(event) => {
              if (event.movementX === 0 && event.movementY === 0) return
              onPointerNavigation()
              onActiveIndexChange(optionIndex)
            }}
            onMouseDown={(event) => event.preventDefault()}
            onClick={() => onSelect(skill)}
          >
            <BookOpen
              className={cn(
                'size-4 shrink-0',
                optionIndex === activeIndex ? 'text-info' : 'text-ink-muted',
              )}
              strokeWidth={1.8}
              aria-hidden="true"
            />
            <span
              className={cn(
                'min-w-0 truncate font-medium',
                optionIndex === activeIndex ? 'text-ink' : 'text-ink-soft',
              )}
            >
              {skill.name}
            </span>
            <span className="min-w-0 truncate text-right text-ink-faint">
              {skill.description}
            </span>
          </button>
        )
      })}
    </div>
  )
}

const suggestionRowBaseClass =
  'grid h-[30px] w-full min-w-0 shrink-0 items-center gap-2 overflow-hidden rounded-[10px] px-2.5 text-left text-[0.8125rem] leading-4'

const commandSuggestionRowClass = `${suggestionRowBaseClass} grid-cols-[1.25rem_minmax(0,10rem)_minmax(5rem,1fr)_auto]`
const skillSuggestionRowClass = `${suggestionRowBaseClass} grid-cols-[1.25rem_minmax(0,10rem)_minmax(5rem,1fr)]`
