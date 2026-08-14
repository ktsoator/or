import {
  BookOpenText,
  LogOut,
  Megaphone,
  Settings,
  type LucideIcon,
} from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import avatarImage from '@/assets/avatar.jpg'
import { cn } from '@/lib/utils'
import { useI18n } from '@/i18n'

export function ProfileMenu({
  collapsed,
  onOpenSettings,
}: {
  collapsed: boolean
  onOpenSettings: () => void
}) {
  const { t } = useI18n()

  return (
    <div className="w-full shrink-0 border-t border-edge/70 px-3 py-2 max-md:w-[17.5rem]">
      <div className="flex items-center gap-2">
        <DropdownMenu.Root>
          <DropdownMenu.Trigger asChild>
            <button
              className={cn(
                'flex h-8 cursor-pointer items-center overflow-hidden outline-none transition-colors hover:bg-surface-hover focus-visible:bg-surface-hover data-[state=open]:bg-surface-selected',
                collapsed
                  ? 'w-8 flex-none justify-center rounded-full p-0.5'
                  : 'min-w-0 flex-1 gap-2.5 rounded-[10px] px-2.5 text-left',
              )}
              type="button"
              aria-label={t('profile.openMenu')}
            >
              <Avatar />
              <span
                className={cn(
                  'min-w-0 truncate whitespace-nowrap text-[0.875rem] font-medium text-ink transition-opacity duration-100 ease-out motion-reduce:transition-none',
                  collapsed ? 'w-0 opacity-0' : 'opacity-100',
                )}
                aria-hidden={collapsed}
              >
                Ktsoator
              </span>
            </button>
          </DropdownMenu.Trigger>

          <DropdownMenu.Portal>
            <DropdownMenu.Content
              side="top"
              align="start"
              sideOffset={7}
              collisionPadding={10}
              className="z-[120] animate-[fade-in_110ms_ease-out] rounded-2xl border border-edge bg-canvas p-1 text-[0.875rem] text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
              style={{
                width: collapsed ? '14.5rem' : 'var(--radix-dropdown-menu-trigger-width)',
              }}
            >
              <DropdownMenu.Label className="flex h-9 items-center gap-2.5 px-2.5">
                <Avatar />
                <span className="truncate font-medium">Ktsoator</span>
              </DropdownMenu.Label>
              <DropdownMenu.Separator className="mx-1.5 my-0.5 h-px bg-canvas-sunken" />

              <ProfileItem icon={BookOpenText} label={t('profile.documentation')} />
              <ProfileItem icon={Megaphone} label={t('profile.changelog')} />
              <ProfileItem
                icon={Settings}
                label={t('profile.settings')}
                shortcut="⌘,"
                onSelect={onOpenSettings}
              />

              <ProfileItem icon={LogOut} label={t('profile.logOut')} />
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>

      </div>
    </div>
  )
}

function Avatar() {
  return (
    <img
      className="size-7 shrink-0 rounded-full border border-edge object-cover shadow-sm"
      src={avatarImage}
      alt=""
      aria-hidden="true"
    />
  )
}

function ProfileItem({
  icon: Icon,
  label,
  shortcut,
  onSelect,
}: {
  icon: LucideIcon
  label: string
  shortcut?: string
  onSelect?: () => void
}) {
  return (
    <DropdownMenu.Item className={profileItemClass} onSelect={onSelect}>
      <Icon className="size-[1.0625rem] shrink-0 text-ink-muted" aria-hidden="true" />
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {shortcut && <span className="text-[0.75rem] text-ink-faint">{shortcut}</span>}
    </DropdownMenu.Item>
  )
}

const profileItemClass =
  'relative mb-0.5 flex h-9 cursor-default select-none items-center gap-2.5 rounded-[10px] px-2.5 outline-none last:mb-0 data-[highlighted]:bg-surface-active data-[state=open]:bg-surface-selected'
