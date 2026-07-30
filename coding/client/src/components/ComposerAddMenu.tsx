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
  fileLimitReached,
  onOpenChange,
  onAttachImages,
  onAttachFiles,
}: {
  disabled: boolean
  open: boolean
  imageAttachmentAvailable: boolean
  imageLimitReached: boolean
  fileLimitReached: boolean
  onOpenChange: (open: boolean) => void
  onAttachImages: () => void
  onAttachFiles: () => void
}) {
  const { t } = useI18n()
  const [activeIndex, setActiveIndex] = useState(0)
  const [keyboardNavigating, setKeyboardNavigating] = useState(false)
  const attachDisabled =
    disabled || !imageAttachmentAvailable || imageLimitReached
  const attachFilesDisabled = disabled || fileLimitReached
  const items: Array<{
    action: 'files' | 'images' | 'preview'
    icon: LucideIcon
    label: string
    description: string
    disabled?: boolean
    unavailable?: string
  }> = [
    {
      action: 'files',
      icon: FolderOpen,
      label: t('composer.addFiles'),
      description: t('composer.addFilesDescription'),
      disabled: attachFilesDisabled,
    },
    {
      action: 'images',
      icon: ImagePlus,
      label: t('composer.attachImages'),
      description: t('composer.attachImagesDescription'),
      disabled: attachDisabled,
      unavailable: !imageAttachmentAvailable
        ? t('composer.modelNoImagesShort')
        : undefined,
    },
    {
      action: 'preview',
      icon: ScanLine,
      label: t('composer.captureApp'),
      description: t('composer.captureAppDescription'),
    },
    {
      action: 'preview',
      icon: Target,
      label: t('composer.addGoal'),
      description: t('composer.addGoalDescription'),
    },
    {
      action: 'preview',
      icon: ListChecks,
      label: t('composer.planMode'),
      description: t('composer.planModeDescription'),
    },
    {
      action: 'preview',
      icon: BookPlus,
      label: t('composer.createSkill'),
      description: t('composer.createSkillDescription'),
    },
  ]
  const optionCount = items.length

  useEffect(() => {
    if (!open) return
    setActiveIndex(!attachFilesDisabled ? 0 : !attachDisabled ? 1 : 2)
    setKeyboardNavigating(false)
  }, [attachDisabled, attachFilesDisabled, open])

  const moveActive = (offset: -1 | 1) => {
    let next = activeIndex
    for (let visited = 0; visited < optionCount; visited++) {
      next = (next + offset + optionCount) % optionCount
      if (!items[next]?.disabled) {
        setActiveIndex(next)
        return
      }
    }
  }

  const selectItem = (index: number) => {
    const item = items[index]
    if (!item || item.disabled || item.action === 'preview') return
    onOpenChange(false)
    if (item.action === 'files') onAttachFiles()
    if (item.action === 'images') onAttachImages()
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
            moveActive(1)
            return
          }
          if (event.key === 'ArrowUp') {
            event.preventDefault()
            setKeyboardNavigating(true)
            moveActive(-1)
            return
          }
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault()
            selectItem(activeIndex)
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
            {items.map((item, index) => {
              const Icon = item.icon
              return (
                <button
                  key={item.label}
                  id={composerAddOptionID(index)}
                  type="button"
                  role="option"
                  aria-selected={activeIndex === index}
                  className={cn(
                    addPanelRowClass,
                    'cursor-pointer outline-none transition-colors disabled:cursor-not-allowed disabled:opacity-40',
                    activeIndex === index
                      ? 'bg-[rgb(241,241,241)]'
                      : 'bg-transparent',
                  )}
                  disabled={item.disabled}
                  onMouseEnter={() => setActiveIndex(index)}
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => {
                    setActiveIndex(index)
                    selectItem(index)
                  }}
                >
                  <Icon
                    className={cn(
                      'size-4 shrink-0',
                      activeIndex === index ? 'text-stone-600' : 'text-stone-400',
                    )}
                    aria-hidden="true"
                  />
                  <span className="max-w-40 truncate font-medium text-stone-700">
                    {item.label}
                  </span>
                  <span className="truncate text-stone-400">{item.description}</span>
                  {item.unavailable ? (
                    <span className="text-[0.6875rem] text-stone-400">
                      {item.unavailable}
                    </span>
                  ) : item.action === 'preview' ? (
                    <span className="text-[0.6875rem] text-stone-400">
                      {t('composer.comingSoon')}
                    </span>
                  ) : null}
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
