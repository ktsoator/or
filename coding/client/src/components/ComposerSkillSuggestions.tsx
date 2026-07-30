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
import type { SkillEntry } from '@/skills'
import { cn } from '@/lib/utils'
import {
  composerPreviewCommands,
  composerFloatingPanelClass,
  type ComposerPreviewCommand,
  skillSuggestionOptionID,
  skillSuggestionsID,
} from './composerPanelStyles'

export function ComposerSkillSuggestions({
  visible,
  query,
  skills,
  activeIndex,
  keyboardNavigating,
  loading,
  failed,
  onActiveIndexChange,
  onPointerNavigation,
  onCommandSelect,
  onSelect,
}: {
  visible: boolean
  query: string
  skills: SkillEntry[]
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
  if (!visible) return null
  const normalizedQuery = query.trim().toLocaleLowerCase()
  const previewCommands: Array<{
    command: ComposerPreviewCommand
    icon: LucideIcon
    label: string
    description: string
  }> = normalizedQuery
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
      <div className="flex max-h-[19rem] flex-col gap-0.5 overflow-y-auto">
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
                  ? 'bg-[rgb(241,241,241)]'
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
                  index === activeIndex ? 'text-stone-600' : 'text-stone-400',
                )}
                aria-hidden="true"
              />
              <span className="max-w-40 truncate font-medium text-stone-700">
                {item.label}
              </span>
              <span className="truncate text-stone-400">{item.description}</span>
              {item.command !== 'compact' && (
                <span className="text-[0.6875rem] text-stone-400">
                  {t('composer.comingSoon')}
                </span>
              )}
            </button>
          )
        })}
        <div
          role="presentation"
          className="flex h-7 shrink-0 items-center px-2.5 text-[0.71875rem] font-medium text-stone-400"
        >
          {t('skills.title')}
        </div>
        {loading ? (
          <div className="flex h-20 items-center justify-center gap-2 text-[0.75rem] text-stone-400">
            <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
            {t('composer.loadingSkills')}
          </div>
        ) : failed ? (
          <p className="px-3 py-6 text-center text-[0.75rem] text-stone-400">
            {t('composer.skillsLoadError')}
          </p>
        ) : skills.length === 0 ? (
          <p className="px-3 py-6 text-center text-[0.75rem] text-stone-400">
            {query ? t('composer.noMatchingSkills') : t('composer.noSkills')}
          </p>
        ) : (
          skills.map((skill, index) => {
            const optionIndex = previewCommands.length + index
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
                    ? 'bg-[rgb(241,241,241)]'
                    : 'bg-transparent',
                )}
                onMouseEnter={() => onActiveIndexChange(optionIndex)}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => onSelect(skill)}
              >
                <BookOpen
                  className={cn(
                    'size-4',
                    optionIndex === activeIndex ? 'text-blue-600' : 'text-stone-500',
                  )}
                  strokeWidth={1.8}
                  aria-hidden="true"
                />
                <span
                  className={cn(
                    'max-w-40 truncate font-medium',
                    optionIndex === activeIndex ? 'text-stone-950' : 'text-stone-800',
                  )}
                >
                  {skill.name}
                </span>
                <span className="truncate text-stone-400">{skill.description}</span>
                <span className="flex items-center gap-1.5 pl-1">
                  {skill.disableModelInvocation && (
                    <span className="rounded-md bg-white/80 px-1.5 py-0.5 text-[0.625rem] font-medium text-stone-500">
                      {t('skills.manual')}
                    </span>
                  )}
                  <span className="text-[0.6875rem] text-stone-400">
                    {skill.source === 'project'
                      ? t('skills.systemSourceProject')
                      : t('skills.systemSourceUser')}
                  </span>
                </span>
              </button>
            )
          })
        )}
      </div>
    </div>
  )
}

const suggestionRowClass =
  'grid h-[30px] w-full shrink-0 grid-cols-[1.25rem_minmax(0,auto)_minmax(5rem,1fr)_auto] items-center gap-2 rounded-[10px] px-2.5 text-left text-[0.8125rem] leading-4'
