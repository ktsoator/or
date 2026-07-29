import { useEffect, useState } from 'react'
import {
  BookPlus,
  FolderOpen,
  ImagePlus,
  ListChecks,
  Plus,
  ScanLine,
  Target,
  type LucideIcon,
} from 'lucide-react'
import { useI18n } from '@/i18n'
import { cn } from '@/lib/utils'
import { composerFloatingPanelClass } from './composerPanelStyles'

export const composerAddPanelID = 'composer-add-panel'
const composerAddOptionID = (index: number) => `${composerAddPanelID}-${index}`

export function ComposerAddMenu({
  disabled,
  open,
  imageAttachmentAvailable,
  imageLimitReached,
  onOpenChange,
  onAttachImages,
}: {
  disabled: boolean
  open: boolean
  imageAttachmentAvailable: boolean
  imageLimitReached: boolean
  onOpenChange: (open: boolean) => void
  onAttachImages: () => void
}) {
  const { t } = useI18n()
  const [activeIndex, setActiveIndex] = useState(0)
  const [keyboardNavigating, setKeyboardNavigating] = useState(false)
  const attachDisabled =
    disabled || !imageAttachmentAvailable || imageLimitReached
  const previewItems: Array<{
    icon: LucideIcon
    label: string
    description: string
  }> = [
    {
      icon: FolderOpen,
      label: t('composer.addFiles'),
      description: t('composer.addFilesDescription'),
    },
    {
      icon: ScanLine,
      label: t('composer.captureApp'),
      description: t('composer.captureAppDescription'),
    },
    {
      icon: Target,
      label: t('composer.addGoal'),
      description: t('composer.addGoalDescription'),
    },
    {
      icon: ListChecks,
      label: t('composer.planMode'),
      description: t('composer.planModeDescription'),
    },
    {
      icon: BookPlus,
      label: t('composer.createSkill'),
      description: t('composer.createSkillDescription'),
    },
  ]
  const optionCount = previewItems.length + 1

  useEffect(() => {
    if (!open) return
    setActiveIndex(0)
    setKeyboardNavigating(false)
  }, [open])

  const attachImages = () => {
    if (attachDisabled) return
    onOpenChange(false)
    onAttachImages()
  }

  return (
    <>
      <button
        className={cn(
          'group relative col-start-1 row-start-2 grid size-[30px] cursor-pointer place-items-center rounded-full text-stone-700 outline-none transition-colors hover:bg-stone-100 focus-visible:bg-stone-100 disabled:cursor-not-allowed disabled:opacity-30',
          open && 'bg-stone-100',
        )}
        type="button"
        aria-label={t('composer.addContent')}
        title={t('composer.addContent')}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? composerAddPanelID : undefined}
        aria-activedescendant={open ? composerAddOptionID(activeIndex) : undefined}
        disabled={disabled}
        onClick={() => onOpenChange(!open)}
        onKeyDown={(event) => {
          if (!open) return
          if (event.key === 'Escape') {
            event.preventDefault()
            onOpenChange(false)
            return
          }
          if (event.key === 'ArrowDown') {
            event.preventDefault()
            setKeyboardNavigating(true)
            setActiveIndex((activeIndex + 1) % optionCount)
            return
          }
          if (event.key === 'ArrowUp') {
            event.preventDefault()
            setKeyboardNavigating(true)
            setActiveIndex((activeIndex - 1 + optionCount) % optionCount)
            return
          }
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault()
            if (activeIndex === 0) attachImages()
          }
        }}
      >
        <Plus
          className={cn(
            'size-[1.125rem] transition-transform duration-150',
            open && 'rotate-45',
          )}
          aria-hidden="true"
        />
      </button>
      {open && (
        <div
          id={composerAddPanelID}
          role="listbox"
          aria-label={t('composer.addContent')}
          className={cn(
            composerFloatingPanelClass,
            keyboardNavigating && 'cursor-none [&_button]:cursor-none',
          )}
          onMouseMove={() => setKeyboardNavigating(false)}
        >
          <div className="flex max-h-[19rem] flex-col gap-0.5 overflow-y-auto">
            <button
              type="button"
              id={composerAddOptionID(0)}
              role="option"
              aria-selected={activeIndex === 0}
              className={cn(
                addPanelRowClass,
                'cursor-pointer outline-none transition-colors disabled:cursor-not-allowed disabled:opacity-40',
                activeIndex === 0 ? 'bg-[rgb(241,241,241)]' : 'bg-transparent',
              )}
              disabled={attachDisabled}
              onMouseEnter={() => setActiveIndex(0)}
              onMouseDown={(event) => event.preventDefault()}
              onClick={attachImages}
            >
              <ImagePlus className="size-4 shrink-0 text-stone-500" aria-hidden="true" />
              <span className="max-w-40 truncate font-medium text-stone-800">
                {t('composer.attachImages')}
              </span>
              <span className="truncate text-stone-400">
                {t('composer.attachImagesDescription')}
              </span>
              {!imageAttachmentAvailable && (
                <span className="text-[0.6875rem] text-stone-400">
                  {t('composer.modelNoImagesShort')}
                </span>
              )}
            </button>
            {previewItems.map((item, index) => {
              const Icon = item.icon
              const optionIndex = index + 1
              return (
                <button
                  key={item.label}
                  id={composerAddOptionID(optionIndex)}
                  type="button"
                  role="option"
                  aria-selected={activeIndex === optionIndex}
                  className={cn(
                    addPanelRowClass,
                    'cursor-pointer outline-none transition-colors',
                    activeIndex === optionIndex
                      ? 'bg-[rgb(241,241,241)]'
                      : 'bg-transparent',
                  )}
                  onMouseEnter={() => setActiveIndex(optionIndex)}
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => setActiveIndex(optionIndex)}
                >
                  <Icon
                    className={cn(
                      'size-4 shrink-0',
                      activeIndex === optionIndex ? 'text-stone-600' : 'text-stone-400',
                    )}
                    aria-hidden="true"
                  />
                  <span className="max-w-40 truncate font-medium text-stone-700">
                    {item.label}
                  </span>
                  <span className="truncate text-stone-400">{item.description}</span>
                  <span className="text-[0.6875rem] text-stone-400">
                    {t('composer.comingSoon')}
                  </span>
                </button>
              )
            })}
          </div>
        </div>
      )}
    </>
  )
}

const addPanelRowClass =
  'grid h-[30px] w-full grid-cols-[1.25rem_minmax(0,auto)_minmax(5rem,1fr)_auto] items-center gap-2 rounded-[10px] px-2.5 text-left text-[0.8125rem] leading-4'
